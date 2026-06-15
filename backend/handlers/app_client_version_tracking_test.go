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

func TestIsLikelyLegacyMobileClientUserAgents(t *testing.T) {
	cases := []struct {
		name      string
		userAgent string
		legacy    bool
	}{
		{
			name:      "legacy ios native client",
			userAgent: "SFLuv/1 CFNetwork/1496.0.7 Darwin/23.5.0",
			legacy:    true,
		},
		{
			name:      "legacy android native client",
			userAgent: "okhttp/4.9.2",
			legacy:    true,
		},
		{
			name:      "ios safari without origin or sec-fetch headers",
			userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 15_8 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.6 Mobile/15E148 Safari/604.1",
			legacy:    false,
		},
		{
			name:      "android chrome without origin or sec-fetch headers",
			userAgent: "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Mobile Safari/537.36",
			legacy:    false,
		},
		{
			name:      "desktop safari without origin or sec-fetch headers",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.1 Safari/605.1.15",
			legacy:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/users", nil)
			if err != nil {
				t.Fatalf("error creating request: %s", err)
			}
			req.Header.Set("User-Agent", tc.userAgent)

			if got := isLikelyLegacyMobileClient(req); got != tc.legacy {
				t.Fatalf("isLikelyLegacyMobileClient = %t, expected %t for %q", got, tc.legacy, tc.userAgent)
			}
		})
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
