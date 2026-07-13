package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubAuthority lets tests control the live active/admin decision without a DB.
type stubAuthority struct {
	active bool
	admin  bool
}

func (s stubAuthority) UserIsActive(context.Context, string) bool { return s.active }
func (s stubAuthority) IsAdmin(context.Context, string) bool      { return s.admin }

func TestBearerToken(t *testing.T) {
	cases := []struct {
		name       string
		authHeader string
		want       string
	}{
		{"authorization bearer", "Bearer abc.def.ghi", "abc.def.ghi"},
		{"case-insensitive scheme", "bearer tok", "tok"},
		{"no header", "", ""},
		{"non-bearer scheme ignored", "Basic Zm9v", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tc.authHeader != "" {
				r.Header.Set("Authorization", tc.authHeader)
			}
			if got := bearerToken(r); got != tc.want {
				t.Fatalf("bearerToken = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestVerifyPKCE(t *testing.T) {
	verifier := "the-quick-brown-fox-jumps-over-the-lazy-dog-1234567890"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	if !verifyPKCE(verifier, challenge) {
		t.Fatal("valid S256 verifier should match its challenge")
	}
	if verifyPKCE("wrong-verifier", challenge) {
		t.Fatal("mismatched verifier must not verify")
	}
	if verifyPKCE("", challenge) || verifyPKCE(verifier, "") {
		t.Fatal("empty verifier/challenge must not verify")
	}
}

func TestValidRedirectURI(t *testing.T) {
	cases := []struct {
		uri  string
		want bool
	}{
		{"https://claude.ai/api/mcp/auth_callback", true},
		{"http://localhost:6274/callback", true},
		{"http://127.0.0.1:8080/cb", true},
		{"http://evil.example.com/cb", false}, // plain http non-loopback
		{"ftp://host/cb", false},
		{"javascript:alert(1)", false},
		{"", false},
		{"not a url", false},
	}
	for _, tc := range cases {
		if got := validRedirectURI(tc.uri); got != tc.want {
			t.Fatalf("validRedirectURI(%q) = %v; want %v", tc.uri, got, tc.want)
		}
	}
}

func TestScopeAllowed(t *testing.T) {
	if !scopeAllowed("") || !scopeAllowed(oauthScope) {
		t.Fatal("empty scope and the read scope must be allowed")
	}
	if scopeAllowed("sfluv.admin.write") || scopeAllowed(oauthScope+" something.else") {
		t.Fatal("unknown scopes must be rejected")
	}
}

// TestRequireBearer is the core resource-server security check: the MCP tool
// handler must only be reached with a valid AS token whose owner is STILL a live
// admin.
func TestRequireBearer(t *testing.T) {
	badLookup := func(context.Context, string) (string, error) { return "", errors.New("no such token") }
	okLookup := func(context.Context, string) (string, error) { return "did:privy:1", nil }

	cases := []struct {
		name       string
		token      string
		lookup     func(context.Context, string) (string, error)
		authority  adminAuthority
		wantStatus int
		wantNext   bool
	}{
		{"missing token", "", okLookup, stubAuthority{true, true}, http.StatusUnauthorized, false},
		{"unknown/expired token", "x", badLookup, stubAuthority{true, true}, http.StatusUnauthorized, false},
		{"valid token but not active", "x", okLookup, stubAuthority{active: false, admin: true}, http.StatusUnauthorized, false},
		{"valid token but not admin", "x", okLookup, stubAuthority{active: true, admin: false}, http.StatusUnauthorized, false},
		{"active admin passes", "x", okLookup, stubAuthority{active: true, admin: true}, http.StatusOK, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})

			o := &oauthServer{authority: tc.authority, lookupToken: tc.lookup, baseURL: "https://api.example.com"}
			r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()

			o.requireBearer(next).ServeHTTP(rec, r)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d; want %d", rec.Code, tc.wantStatus)
			}
			if nextCalled != tc.wantNext {
				t.Fatalf("next called = %v; want %v", nextCalled, tc.wantNext)
			}
			if !tc.wantNext && rec.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("expected WWW-Authenticate header on rejection")
			}
		})
	}
}
