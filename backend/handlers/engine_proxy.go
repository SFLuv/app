package handlers

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

// Engine proxy — a temporary bridge around an expired upstream certificate.
//
// On 2026-09-10 the Let's Encrypt certificate on 42220.engine.citizenwallet.xyz
// lapsed at 05:24 UTC. The engine itself stayed healthy — eth_chainId answered
// 0xa4ec in under a second — so nothing was down except the ability of a client
// to validate the certificate. That distinction is what makes this workable:
// there is a working service behind a certificate no browser will accept.
//
// TLS verification is performed by the client, and this proxy inserts a client
// we control. The browser and the mobile app terminate TLS against the SFLuv
// backend's own valid certificate; only this process ever sees the expired one.
// A web page cannot do the same thing itself — fetch() exposes no TLS policy,
// and a site cannot grant an exception for its own subresource requests.
//
// WHAT IS GIVEN UP, PRECISELY. An expired certificate still proves possession
// of the matching private key, and the CA's signature over it still verifies.
// notAfter guards against a certificate that was legitimately issued but should
// no longer be trusted: a key compromise handled by letting it lapse rather
// than revoking, or a domain changing hands since issuance. Waiving it costs us
// exactly that, over a window that began hours ago.
//
// Everything else is still enforced: the chain must build to a trusted root and
// the leaf must cover this hostname, so a self-signed certificate or one from
// an untrusted CA is refused exactly as it would be normally.
//
// This is a bridge, not a fix. DELETE IT once the upstream renews — but it is
// deliberately built so that not deleting it promptly is safe. A renewed
// certificate passes this verifier for the ordinary reason, so renewal does not
// interrupt traffic and removal is never itself an incident.

const (
	engineProxyPathPrefix = "/bundler"
	engineProxyTimeout    = 30 * time.Second
)

// EngineProxy reverse-proxies the Citizen Wallet engine over a TLS transport
// that waives only certificate expiry. The zero value is unusable; build one
// with NewEngineProxy.
type EngineProxy struct {
	upstream  *url.URL
	publicURL string
	proxy     *httputil.ReverseProxy
}

// EngineProxyEnabled reports whether the operator has switched the bridge on.
// Disabled is the correct default: with the upstream healthy this whole file
// should be inert.
func EngineProxyEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENGINE_PROXY_ENABLED")), "true")
}

// EngineProxyPublicURL is the base clients should call instead of the engine,
// e.g. "https://api.sfluv.org/bundler". Empty disables the config rewrite even
// when the proxy itself is mounted, so the two can be staged independently.
func EngineProxyPublicURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("ENGINE_PROXY_PUBLIC_URL")), "/")
}

// NewEngineProxy builds the bridge. Misconfiguration is refused at startup
// rather than degrading into something that quietly trusts anything.
func NewEngineProxy() (*EngineProxy, error) {
	rawUpstream := strings.TrimSpace(os.Getenv("ENGINE_PROXY_UPSTREAM"))
	if rawUpstream == "" {
		return nil, errors.New("ENGINE_PROXY_UPSTREAM is required when ENGINE_PROXY_ENABLED=true")
	}
	upstream, err := url.Parse(rawUpstream)
	if err != nil {
		return nil, fmt.Errorf("invalid ENGINE_PROXY_UPSTREAM %q: %w", rawUpstream, err)
	}
	if upstream.Scheme != "https" || upstream.Host == "" {
		return nil, fmt.Errorf("ENGINE_PROXY_UPSTREAM must be an https URL with a host, got %q", rawUpstream)
	}

	// Optional, and informational only: the key we expect while the upstream is
	// still on its expired certificate. A change is the renewal signal, logged
	// once. It is NOT a gate — see expiryTolerantVerifier for why.
	observePin := enginePinObserver(strings.TrimSpace(os.Getenv("ENGINE_PROXY_PIN_SPKI")))
	verify := expiryTolerantVerifier(upstream.Hostname())

	transport := &http.Transport{
		// Go's own verification is replaced, not removed — see VerifyPeerCertificate.
		// Scoped to this transport and this upstream; never the process default.
		TLSClientConfig: &tls.Config{
			ServerName:         upstream.Hostname(),
			InsecureSkipVerify: true, //nolint:gosec // replaced by expiryTolerantVerifier
			VerifyPeerCertificate: func(rawCerts [][]byte, chains [][]*x509.Certificate) error {
				if err := verify(rawCerts, chains); err != nil {
					return err
				}
				if leaf, err := x509.ParseCertificate(rawCerts[0]); err == nil {
					observePin(leaf)
				}
				return nil
			},
		},
		ResponseHeaderTimeout: engineProxyTimeout,
		MaxIdleConnsPerHost:   16,
	}

	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Director: func(r *http.Request) {
			r.URL.Scheme = upstream.Scheme
			r.URL.Host = upstream.Host
			r.Host = upstream.Host
			r.URL.Path = strings.TrimPrefix(r.URL.Path, engineProxyPathPrefix)
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
			// The engine authenticates nothing; strip client headers that would
			// only leak our callers' details upstream.
			r.Header.Del("Cookie")
			r.Header.Del("Authorization")
			r.Header.Del("Access-Token")
		},
		// The engine sets its own CORS headers, and this backend's middleware
		// sets ours. Passing both through produces a response carrying two
		// Access-Control-Allow-Origin values, which every browser rejects —
		// "contains multiple values, but only one is allowed" — while curl
		// reports a perfectly healthy 200. Strip the upstream's and let ours be
		// the only answer.
		ModifyResponse: func(res *http.Response) error {
			for _, header := range []string{
				"Access-Control-Allow-Origin",
				"Access-Control-Allow-Methods",
				"Access-Control-Allow-Headers",
				"Access-Control-Allow-Credentials",
				"Access-Control-Expose-Headers",
				"Access-Control-Max-Age",
			} {
				res.Header.Del(header)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			// Certificate validation failures land here — which now means a
			// genuinely untrusted chain or wrong hostname, not a routine renewal.
			http.Error(w, fmt.Sprintf("engine proxy upstream error: %v", err), http.StatusBadGateway)
		},
	}

	return &EngineProxy{upstream: upstream, publicURL: EngineProxyPublicURL(), proxy: proxy}, nil
}

// expiryTolerantVerifier performs ordinary certificate validation — chain built
// to a system root, hostname checked — and waives exactly one thing: notAfter.
//
// A key pin was the obvious first answer here and it was the wrong one. Pinning
// fails closed the moment the upstream renews with a fresh key, which is the
// normal outcome of renewal, so the safety mechanism would itself have caused a
// second payments outage that stayed down until somebody noticed and edited an
// env var by hand. A bridge whose removal is an incident is a bad bridge.
//
// Waiving only expiry has no such cliff. A renewed certificate is simply a
// valid certificate: it passes this verifier for the ordinary reason, traffic
// keeps flowing, and the bridge becomes redundant rather than dangerous. It can
// then be removed on a weekday afternoon instead of at 3am.
//
// What is still enforced: the chain must build to a trusted root, and the leaf
// must cover this hostname. A self-signed certificate, or one from an untrusted
// CA, or one for a different host, is refused exactly as it would be normally.
//
// What is given up: an attacker holding a previously-valid, now-expired
// certificate for this exact hostname — plus network position on the hop
// between this backend and the engine — would not be caught. In practice that
// means the upstream operator or someone who has compromised their key, which
// is the same trust we extend by using their service at all.
func expiryTolerantVerifier(host string) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("engine proxy: upstream presented no certificate")
		}

		certs := make([]*x509.Certificate, 0, len(rawCerts))
		for _, raw := range rawCerts {
			cert, err := x509.ParseCertificate(raw)
			if err != nil {
				return fmt.Errorf("engine proxy: unparseable certificate: %w", err)
			}
			certs = append(certs, cert)
		}

		leaf := certs[0]
		intermediates := x509.NewCertPool()
		for _, cert := range certs[1:] {
			intermediates.AddCert(cert)
		}

		// Normally "now". Only when the leaf has already expired do we evaluate
		// the chain as of a moment inside its validity window — which is the
		// single concession this bridge makes. Once the upstream renews, this
		// is ordinary present-time validation and the branch never fires.
		at := time.Now()
		if at.After(leaf.NotAfter) {
			at = leaf.NotAfter.Add(-time.Minute)
		}

		if _, err := leaf.Verify(x509.VerifyOptions{
			DNSName:       host,
			Intermediates: intermediates,
			CurrentTime:   at,
		}); err != nil {
			return fmt.Errorf("engine proxy: certificate for %s failed validation: %w", host, err)
		}
		return nil
	}
}

// enginePinObserver logs when the upstream's public key changes. This is the
// renewal signal, kept deliberately as telemetry rather than as a gate: it tells
// operations the bridge can come out, without being able to take traffic down
// when it fires.
func enginePinObserver(expected string) func(*x509.Certificate) {
	if expected == "" {
		return func(*x509.Certificate) {}
	}
	var reported bool
	return func(leaf *x509.Certificate) {
		if reported {
			return
		}
		sum := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
		if got := base64.StdEncoding.EncodeToString(sum[:]); got != expected {
			reported = true
			stdlog.Printf(
				"engine proxy: upstream key changed (was %s, now %s, notAfter %s) — "+
					"the certificate has been renewed and this bridge can be removed",
				expected, got, leaf.NotAfter.Format(time.RFC3339),
			)
		}
	}
}

// ServeHTTP forwards anything under /bundler to the engine. The path is
// preserved because the SDK addresses several endpoints beneath it —
// /v1/rpc/<paymaster>, /v1/accounts/<sender>/exists and /v1/logs. A proxy that
// handled only the RPC path would half-work: the accounts check gates initCode,
// so failing it produces a user operation for the wrong account rather than an
// obvious error.
func (p *EngineProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p == nil || p.proxy == nil {
		http.Error(w, "engine proxy is not configured", http.StatusServiceUnavailable)
		return
	}
	p.proxy.ServeHTTP(w, r)
}

// withEngineProxyURL rewrites chains[*].node.url so clients address the proxy
// instead of the engine directly.
//
// This is the whole reason the bridge is deployable without shipping anything
// to a client: both the web app and the mobile app derive the bundler from this
// single string (frontend lib/community-config.ts, mobile mapClientConfig),
// and neither hardcodes the engine host. Reads are unaffected — they come from
// extras.rpc_url, which still points at a full node.
//
// On any decode problem it returns the raw config untouched. A malformed
// overlay must never take down /config, which every client polls at startup.
func withEngineProxyURL(raw []byte) []byte {
	base := EngineProxyPublicURL()
	if !EngineProxyEnabled() || base == "" {
		return raw
	}

	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		return raw
	}

	chains, ok := config["chains"].(map[string]any)
	if !ok || len(chains) == 0 {
		return raw
	}

	rewrote := false
	for _, entry := range chains {
		chain, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		node, ok := chain["node"].(map[string]any)
		if !ok {
			continue
		}
		if _, ok := node["url"].(string); !ok {
			continue
		}
		node["url"] = base
		// ws_url is left pointing upstream on purpose: no client reads it
		// today, and proxying websockets is work this bridge does not need.
		rewrote = true
	}
	if !rewrote {
		return raw
	}

	merged, err := json.Marshal(config)
	if err != nil {
		return raw
	}
	return merged
}
