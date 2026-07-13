package mcp

import (
	"context"
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
func (s stubAuthority) IsAdmin(context.Context, string) bool       { return s.admin }

func gateWith(authority adminAuthority, validate func(string) (string, error)) *authGate {
	return &authGate{authority: authority, validate: validate}
}

func okValidate(did string) func(string) (string, error) {
	return func(string) (string, error) { return did, nil }
}

func TestBearerToken(t *testing.T) {
	cases := []struct {
		name       string
		authHeader string
		accessTok  string
		want       string
	}{
		{"authorization bearer", "Bearer abc.def.ghi", "", "abc.def.ghi"},
		{"case-insensitive scheme", "bearer tok", "", "tok"},
		{"access-token fallback", "", "legacy-tok", "legacy-tok"},
		{"authorization wins", "Bearer primary", "secondary", "primary"},
		{"no token", "", "", ""},
		{"non-bearer scheme ignored", "Basic Zm9v", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tc.authHeader != "" {
				r.Header.Set("Authorization", tc.authHeader)
			}
			if tc.accessTok != "" {
				r.Header.Set("Access-Token", tc.accessTok)
			}
			if got := bearerToken(r); got != tc.want {
				t.Fatalf("bearerToken = %q; want %q", got, tc.want)
			}
		})
	}
}

// TestMiddlewareRejectsUnlessActiveAdmin is the core security check: the wrapped
// handler must only be reached by a validated, active SFLUV admin.
func TestMiddlewareRejectsUnlessActiveAdmin(t *testing.T) {
	badToken := func(string) (string, error) { return "", errors.New("invalid token") }

	cases := []struct {
		name       string
		token      string
		validate   func(string) (string, error)
		authority  adminAuthority
		wantStatus int
		wantNext   bool
	}{
		{"missing token", "", okValidate("did:1"), stubAuthority{true, true}, http.StatusUnauthorized, false},
		{"invalid token", "x", badToken, stubAuthority{true, true}, http.StatusUnauthorized, false},
		{"valid but not active", "x", okValidate("did:1"), stubAuthority{active: false, admin: true}, http.StatusUnauthorized, false},
		{"valid but not admin", "x", okValidate("did:1"), stubAuthority{active: true, admin: false}, http.StatusUnauthorized, false},
		{"active admin passes", "x", okValidate("did:1"), stubAuthority{active: true, admin: true}, http.StatusOK, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})

			gate := gateWith(tc.authority, tc.validate)
			r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()

			gate.middleware(next).ServeHTTP(rec, r)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d; want %d", rec.Code, tc.wantStatus)
			}
			if nextCalled != tc.wantNext {
				t.Fatalf("next called = %v; want %v", nextCalled, tc.wantNext)
			}
			if !tc.wantNext {
				if got := rec.Header().Get("WWW-Authenticate"); got == "" {
					t.Fatal("expected WWW-Authenticate header on rejection")
				}
			}
		})
	}
}
