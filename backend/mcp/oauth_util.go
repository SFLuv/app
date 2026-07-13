package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// oauthScope is the single read-only scope this resource server supports.
const oauthScope = "sfluv.admin.read"

// randomToken returns a URL-safe, unguessable token with nBytes of entropy.
func randomToken(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is catastrophic; never fall back to a weak token.
		panic("mcp oauth: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// hashToken returns a stable hash of a secret token. Only hashes are stored, so
// a database leak never exposes usable tokens.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// verifyPKCE checks an S256 PKCE code_verifier against the stored code_challenge
// using a constant-time comparison.
func verifyPKCE(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// validRedirectURI allows https redirect URIs and http only for loopback (native
// MCP clients on localhost). Everything else is rejected to prevent open
// redirects / token exfiltration to attacker-controlled hosts.
func validRedirectURI(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	default:
		// Custom-scheme native redirects (e.g. some desktop clients) are not
		// accepted; keep the surface to https + loopback only.
		return false
	}
}

func scopeAllowed(scope string) bool {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return true
	}
	for _, s := range strings.Fields(scope) {
		if s != oauthScope {
			return false
		}
	}
	return true
}

func normalizeScope(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return oauthScope
	}
	return strings.TrimSpace(scope)
}

// redirectOAuthError sends the user's browser back to the client's redirect_uri
// with a standard OAuth error, but only if the redirect_uri is safe; otherwise
// it returns a plain error so we never redirect to an unvalidated destination.
func redirectOAuthError(w http.ResponseWriter, r *http.Request, redirectURI, state, code string) {
	if !validRedirectURI(redirectURI) {
		http.Error(w, "invalid_request: "+code, http.StatusBadRequest)
		return
	}
	u, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, "invalid_request: "+code, http.StatusBadRequest)
		return
	}
	q := u.Query()
	q.Set("error", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
