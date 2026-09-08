// Command w9probe makes the one round trip the TaxBandits adapter was written
// without.
//
// Every response shape in w9provider/taxbandits.go came from documentation. The
// Track1099 integration was written the same way twice and was wrong both
// times, so this tool exists to settle the difference before any of it is
// trusted: it authenticates, creates a form request, reads the status back, and
// prints what actually arrived.
//
// It is deliberately a separate binary rather than a test. It talks to a real
// vendor over the network, so it must never run in CI or as part of `go test`.
//
// Usage, with credentials exported or in backend/.env:
//
//	go run ./cmd/w9probe -step auth
//	go run ./cmd/w9probe -step business
//	go run ./cmd/w9probe -step request -email you@example.com
//	go run ./cmd/w9probe -step status -submission <id>
//	go run ./cmd/w9probe -step idempotency -email you@example.com
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/w9provider"
)

// probeReturnURL mirrors the backend's default so the probe exercises what
// production will actually send.
func probeReturnURL() string {
	if raw := strings.TrimSpace(os.Getenv("W9_RETURN_URL")); raw != "" {
		return raw
	}
	base := strings.TrimSpace(os.Getenv("PUBLIC_BACKEND_URL"))
	if base == "" {
		base = "http://localhost:8080"
	}
	return strings.TrimSuffix(base, "/") + "/w9/complete"
}

func main() {
	step := flag.String("step", "auth", "auth | servertime | business | business-create | request | status | adapter | idempotency")
	email := flag.String("email", "", "recipient email for a form request")
	submission := flag.String("submission", "", "submission id for a status read")
	payeeRef := flag.String("payee-ref", "sfluv:probe:2026", "payee reference to use")
	flag.Parse()

	loadDotEnv("../backend/.env", ".env")

	cfg := probeConfig{
		clientID:     strings.TrimSpace(os.Getenv("W9_PROVIDER_CLIENT_ID")),
		clientSecret: strings.TrimSpace(os.Getenv("W9_PROVIDER_CLIENT_SECRET")),
		userToken:    strings.TrimSpace(os.Getenv("W9_PROVIDER_USER_TOKEN")),
		businessID:   strings.TrimSpace(os.Getenv("W9_PROVIDER_BUSINESS_ID")),
		apiVersion:   envOr("W9_PROVIDER_API_VERSION", "v1.7.3"),
		apiBase:      envOr("W9_PROVIDER_API_BASE", "https://testapi.taxbandits.com"),
		authURL:      envOr("W9_PROVIDER_AUTH_URL", "https://testoauth.expressauth.net/v2/tbsauth"),
	}

	if cfg.clientID == "" || cfg.clientSecret == "" || cfg.userToken == "" {
		fmt.Println("missing credentials. Set these three, then re-run:")
		fmt.Println("  W9_PROVIDER_CLIENT_ID")
		fmt.Println("  W9_PROVIDER_CLIENT_SECRET")
		fmt.Println("  W9_PROVIDER_USER_TOKEN")
		os.Exit(1)
	}
	fmt.Printf("api  %s/%s\nauth %s\n\n", cfg.apiBase, cfg.apiVersion, cfg.authURL)

	switch *step {
	case "servertime":
		cfg.serverTime()
	case "auth":
		if _, err := cfg.token(); err != nil {
			fail(err)
		}
	case "business":
		cfg.call("GET", "/Business/List?Page=1&PageSize=25", nil)
	case "business-create":
		// Deliberately fake. A sandbox tenant needs a payer to exist before any
		// form can name one, but it does not need the real charity's EIN — and
		// putting a real one into a test tenant buys nothing. Production must
		// be created separately with the actual registered details.
		cfg.call("POST", "/Business/Create", map[string]any{
			"BusinessNm":        "SFLUV Sandbox Test Payer",
			"PayerRef":          "sfluv-sandbox",
			"IsEIN":             true,
			"EINorSSN":          "20-1652598",
			"IsDefaultBusiness": true,
			"IsForeign":         false,
			"Email":             envOr("W9_PROBE_EMAIL", "ops@sfluv.org"),
			"USAddress": map[string]any{
				"Address1": "1 Test Street",
				"City":     "San Francisco",
				"State":    "CA",
				"ZipCd":    "94103",
			},
		})
	case "request":
		if *email == "" {
			fail(fmt.Errorf("-email is required for a form request"))
		}
		cfg.requestByURL(*payeeRef, *email)
	case "status":
		if *submission != "" {
			cfg.call("GET", "/FormW9/Status?SubmissionId="+*submission, nil)
			return
		}
		cfg.call("GET", "/FormW9/Status?PayeeRef="+*payeeRef, nil)
	case "adapter":
		// Runs the real w9provider adapter rather than this file's hand-rolled
		// HTTP, so what is printed is exactly what the payout code will act on.
		// Proving the JSON is not the same as proving the mapping.
		if *submission == "" {
			fail(fmt.Errorf("-submission is required"))
		}
		tb := w9provider.NewTaxBandits(w9provider.Config{
			ClientID: cfg.clientID, ClientSecret: cfg.clientSecret, UserToken: cfg.userToken,
			BusinessID: cfg.businessID, APIVersion: cfg.apiVersion, Environment: "sandbox",
		})
		tb.SetWarningLogger(func(m string) { fmt.Println("WARN:", m) })
		status, err := tb.GetW9Status(context.Background(), *submission)
		if err != nil {
			fail(err)
		}
		completedAt := "<nil>"
		if status.CompletedAt != nil {
			completedAt = status.CompletedAt.Format(time.RFC3339)
		}
		fmt.Printf("adapter GetW9Status(%s)\n", *submission)
		fmt.Printf("  Status      %q\n", status.Status)
		fmt.Printf("  CompletedAt %s\n", completedAt)
		fmt.Printf("  TINMatch    %q\n", status.TINMatch)
		fmt.Printf("  TINType     %q\n", status.TINType)
		fmt.Printf("\n  releases money: %v\n", status.Status == w9provider.StatusCompleted)
	case "idempotency":
		if *email == "" {
			fail(fmt.Errorf("-email is required"))
		}
		fmt.Println("--- Q1: does one PayeeRef yield one submission? ---")
		cfg.requestByURL(*payeeRef, *email)
		cfg.requestByURL(*payeeRef, *email)
		fmt.Println("--- read TotalRecords below: 1 = idempotent, 2 = not ---")
		cfg.call("GET", "/FormW9/Status?PayeeRef="+*payeeRef, nil)
	default:
		fail(fmt.Errorf("unknown step %q", *step))
	}
}

type probeConfig struct {
	clientID, clientSecret, userToken string
	businessID                        string
	apiVersion, apiBase, authURL      string
}

// serverTime is the first thing to check when auth fails: their clock and ours
// must agree or no token is ever issued, and the failure looks like bad
// credentials rather than a clock problem.
func (c probeConfig) serverTime() {
	host := c.authURL
	if i := strings.Index(host, "/v2/"); i > 0 {
		host = host[:i]
	}
	req, _ := http.NewRequest(http.MethodGet, host+"/v2/getservertime", nil)
	// Verified against the sandbox: this endpoint is NOT open. Without the
	// assertion it returns AUTH-100002 "Authentication header cannot be empty",
	// which makes it a diagnostic for a mildly skewed clock rather than a way
	// to bootstrap one that is badly skewed.
	req.Header.Set("Authentication", c.assertion())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	fmt.Printf("HTTP %d\n%s\n\nlocal now: %s\n", resp.StatusCode, redact(string(body)), time.Now().Format(time.RFC3339))
}

func (c probeConfig) assertion() string {
	header := base64url(`{"alg":"HS256","typ":"JWT"}`)
	payload := base64url(fmt.Sprintf(`{"iss":%q,"sub":%q,"aud":%q,"iat":%d}`,
		c.clientID, c.clientID, c.userToken, time.Now().Unix()))
	mac := hmac.New(sha256.New, []byte(c.clientSecret))
	mac.Write([]byte(header + "." + payload))
	return header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (c probeConfig) token() (string, error) {
	assertion := c.assertion()

	req, _ := http.NewRequest(http.MethodGet, c.authURL, nil)
	// Not "Authorization", and a GET. Both are easy to get wrong and both
	// present identically as a credential failure.
	req.Header.Set("Authentication", assertion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	fmt.Printf("AUTH HTTP %d\n%s\n\n", resp.StatusCode, redact(string(body)))

	var envelope struct {
		AccessToken string `json:"AccessToken"`
	}
	_ = json.Unmarshal(body, &envelope)
	if envelope.AccessToken == "" {
		return "", fmt.Errorf("no access token issued — check the four traps: header name, GET, HS256, and clock skew (-step servertime)")
	}
	fmt.Println("✓ authenticated — Go-Live checklist item 1 demonstrated")
	return envelope.AccessToken, nil
}

func (c probeConfig) call(method, suffix string, body any) {
	token, err := c.token()
	if err != nil {
		fail(err)
	}

	var reader io.Reader
	if body != nil {
		encoded, _ := json.MarshalIndent(body, "", "  ")
		fmt.Printf("REQUEST %s %s\n%s\n\n", method, suffix, redact(string(encoded)))
		reader = bytes.NewReader(encoded)
	}

	req, _ := http.NewRequest(method, fmt.Sprintf("%s/%s%s", c.apiBase, c.apiVersion, suffix), reader)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var pretty bytes.Buffer
	if json.Indent(&pretty, raw, "", "  ") == nil {
		raw = pretty.Bytes()
	}
	fmt.Printf("RESPONSE %s %s → HTTP %d\n%s\n", method, suffix, resp.StatusCode, redact(string(raw)))
}

func (c probeConfig) requestByURL(payeeRef, email string) {
	if c.businessID == "" {
		fail(fmt.Errorf("W9_PROVIDER_BUSINESS_ID is not set — run -step business first and copy the GUID"))
	}
	c.call("POST", "/FormW9/RequestByUrl", map[string]any{
		"Requester": map[string]any{"BusinessId": c.businessID},
		"Recipient": map[string]any{
			"PayeeRef":      payeeRef,
			"Email":         email,
			"IsTINMatching": true,
		},
		// A page, not the app's URL scheme — see w9ReturnURL. A scheme here
		// makes iOS switch to the app mid-flow, which dismisses the browser as
		// a side effect and looks like success while breaking the wait.
		"RedirectUrls": map[string]any{"ReturnUrl": probeReturnURL(), "RedirectTime": 5},
		"PrefLang":     "en-US",
	})
}

// redact blanks anything that looks like a taxpayer identification number, and
// the bearer token alongside it.
//
// The Requester block echoes our own payer EIN, and a payee TIN must never
// reach a terminal or a scrollback buffer. Printing is the entire point of this
// tool, so the redaction is not optional.
// Keys are matched whole rather than by substring. An earlier substring
// version redacted "BusinessNm" — because "BusineSSNm" contains "SSN" — which
// hid useful output while proving the danger of clever patterns here.
var tinPattern = regexp.MustCompile(`(?i)"(TIN|SSN|EIN|EINorSSN|TINType|MaskedTIN|AccessToken)"\s*:\s*"[^"]*"`)

func redact(s string) string {
	s = tinPattern.ReplaceAllString(s, `"$1":"[redacted]"`)
	return regexp.MustCompile(`\b\d{3}-?\d{2}-?\d{4}\b`).ReplaceAllString(s, "[redacted]")
}

func base64url(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv fills in anything not already exported. Values already in the
// environment win, so a shell export always beats a file on disk.
//
// The file is collected into a map before anything is set, so a key repeated
// in the file resolves to its LAST occurrence — matching godotenv, which is
// what the server itself uses. Reading first-wins here while the server reads
// last-wins would make the probe and the server disagree about a duplicated
// key, which is the kind of difference that costs an afternoon.
func loadDotEnv(paths ...string) {
	found := map[string]string{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			if !strings.HasPrefix(key, "W9_PROVIDER_") {
				continue
			}
			found[key] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	for key, value := range found {
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "\nFAILED: %v\n", err)
	os.Exit(1)
}
