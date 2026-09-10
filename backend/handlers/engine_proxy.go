package handlers

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
// In exchange we pin the upstream's public key, which is STRICTER than ordinary
// CA validation in one respect: normal validation accepts any certificate any
// trusted CA vouches for, so a mis-issued but currently-valid certificate would
// pass. The pin does not. We trade a weak general guarantee for a narrow strong
// one — "the exact key that served this host while its certificate was valid".
//
// This is a bridge, not a fix. DELETE IT once the upstream renews. The pin will
// almost certainly fail at that moment, because Let's Encrypt issues a fresh
// key pair on renewal — treat a pin mismatch as the signal to remove the proxy,
// never as a prompt to quietly update the pin.

const (
	engineProxyPathPrefix = "/bundler"
	engineProxyTimeout    = 30 * time.Second
)

// EngineProxy reverse-proxies the Citizen Wallet engine over a pinned TLS
// transport. The zero value is unusable; build one with NewEngineProxy.
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

// NewEngineProxy builds the bridge. It fails loudly rather than falling back to
// unpinned TLS: a silent downgrade here would be worse than staying broken.
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

	pin := strings.TrimSpace(os.Getenv("ENGINE_PROXY_PIN_SPKI"))
	if pin == "" {
		return nil, errors.New("ENGINE_PROXY_PIN_SPKI is required: refusing to proxy over unverified TLS")
	}

	transport := &http.Transport{
		// Go's own verification is replaced, not removed — see VerifyPeerCertificate.
		// Scoped to this transport and this upstream; never the process default.
		TLSClientConfig: &tls.Config{
			ServerName:            upstream.Hostname(),
			InsecureSkipVerify:    true, //nolint:gosec // replaced by the SPKI pin below
			VerifyPeerCertificate: pinnedVerifier(pin, upstream.Hostname()),
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
			// A pin mismatch reaches here. Say so plainly: it almost certainly
			// means the upstream renewed and this bridge should be removed.
			http.Error(w, fmt.Sprintf("engine proxy upstream error: %v", err), http.StatusBadGateway)
		},
	}

	return &EngineProxy{upstream: upstream, publicURL: EngineProxyPublicURL(), proxy: proxy}, nil
}

// pinnedVerifier accepts exactly one public key, ignoring notAfter and doing no
// chain building. Everything this waives and everything it adds is set out in
// the file comment above.
func pinnedVerifier(pin string, host string) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("engine proxy: upstream presented no certificate")
		}
		leaf, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("engine proxy: unparseable leaf certificate: %w", err)
		}

		sum := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
		got := base64.StdEncoding.EncodeToString(sum[:])
		if got != pin {
			return fmt.Errorf(
				"engine proxy: SPKI pin mismatch for %s (got %s, expected %s) — "+
					"the upstream has almost certainly renewed; remove the proxy "+
					"rather than updating the pin",
				host, got, pin,
			)
		}

		// The hostname is still checked. Only expiry is deliberately skipped.
		if err := leaf.VerifyHostname(host); err != nil {
			return fmt.Errorf("engine proxy: certificate does not cover %s: %w", host, err)
		}
		return nil
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
