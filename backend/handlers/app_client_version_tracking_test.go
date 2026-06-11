package handlers

import (
	"net/http"
	"testing"
)

func TestIsLikelyLegacyMobileClient(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/users", nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	req.Header.Set("User-Agent", "SFLuv/1 CFNetwork/1496.0.7 Darwin/23.5.0")

	if !isLikelyLegacyMobileClient(req) {
		t.Fatal("expected native mobile request without client headers to be treated as legacy")
	}

	req.Header.Set(clientVersionHeader, "1.0.1")
	if isLikelyLegacyMobileClient(req) {
		t.Fatal("expected request with client version header not to be treated as legacy")
	}

	req.Header.Del(clientVersionHeader)
	req.Header.Set("Origin", "https://app.sfluv.org")
	if isLikelyLegacyMobileClient(req) {
		t.Fatal("expected browser-like request not to be treated as legacy mobile")
	}
}

func TestLegacyMobileClientBlockEnabled(t *testing.T) {
	t.Setenv(legacyMobileClientBlockEnvKey, "")
	if !legacyMobileClientBlockEnabled() {
		t.Fatal("expected legacy mobile block to default on")
	}

	t.Setenv(legacyMobileClientBlockEnvKey, "false")
	if legacyMobileClientBlockEnabled() {
		t.Fatal("expected legacy mobile block to be disabled by false env value")
	}

	t.Setenv(legacyMobileClientBlockEnvKey, "1")
	if !legacyMobileClientBlockEnabled() {
		t.Fatal("expected legacy mobile block to accept truthy env value")
	}
}
