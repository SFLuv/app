package main

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"testing"
)

func TestSafeLimit(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "default", in: 0, want: defaultLimit},
		{name: "negative", in: -1, want: defaultLimit},
		{name: "keeps small", in: 25, want: 25},
		{name: "caps large", in: maxLimit + 1, want: maxLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeLimit(tt.in); got != tt.want {
				t.Fatalf("safeLimit(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestEnvInt64(t *testing.T) {
	t.Setenv("SFLUV_TEST_CHAIN_ID", "123")
	if got := envInt64(1, "MISSING_CHAIN_ID", "SFLUV_TEST_CHAIN_ID"); got != 123 {
		t.Fatalf("envInt64 got %d, want 123", got)
	}

	t.Setenv("SFLUV_TEST_BAD_CHAIN_ID", "nope")
	if got := envInt64(7, "SFLUV_TEST_BAD_CHAIN_ID"); got != 7 {
		t.Fatalf("envInt64 fallback got %d, want 7", got)
	}

	_ = os.Unsetenv("SFLUV_TEST_CHAIN_ID")
}

func TestRolesFromBools(t *testing.T) {
	roles := rolesFromBools([10]bool{true, false, false, true, true})
	want := []string{"admin", "improver", "proposer"}
	if len(roles) != len(want) {
		t.Fatalf("roles length = %d, want %d (%v)", len(roles), len(want), roles)
	}
	for i := range want {
		if roles[i] != want[i] {
			t.Fatalf("roles[%d] = %q, want %q", i, roles[i], want[i])
		}
	}
}

func TestVerifyPKCE(t *testing.T) {
	verifier := "plain-test-verifier"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if !verifyPKCE(verifier, challenge) {
		t.Fatal("expected PKCE verifier to match challenge")
	}
	if verifyPKCE("wrong", challenge) {
		t.Fatal("expected wrong PKCE verifier to fail")
	}
}

func TestBearerToken(t *testing.T) {
	if got := bearerToken("Bearer abc123"); got != "abc123" {
		t.Fatalf("bearerToken got %q", got)
	}
	if got := bearerToken("Basic abc123"); got != "" {
		t.Fatalf("bearerToken accepted wrong scheme: %q", got)
	}
}

func TestScopeAllowed(t *testing.T) {
	if !scopeAllowed("") {
		t.Fatal("empty scope should default to the MCP scope")
	}
	if !scopeAllowed("openid sfluv.admin.read") {
		t.Fatal("scope list containing the MCP scope should be accepted")
	}
	if scopeAllowed("openid email") {
		t.Fatal("scope list without the MCP scope should be rejected")
	}
}

func TestValidRedirectURI(t *testing.T) {
	for _, uri := range []string{
		"https://client.example.com/callback",
		"http://localhost:1234/callback",
		"http://127.0.0.1:1234/callback",
	} {
		if !validRedirectURI(uri) {
			t.Fatalf("validRedirectURI rejected %s", uri)
		}
	}

	for _, uri := range []string{
		"http://client.example.com/callback",
		"javascript:alert(1)",
		"not-a-url",
	} {
		if validRedirectURI(uri) {
			t.Fatalf("validRedirectURI accepted %s", uri)
		}
	}
}
