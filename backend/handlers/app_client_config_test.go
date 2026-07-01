package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientVersionRequiresUpdate(t *testing.T) {
	cases := []struct {
		name    string
		version string
		min     string
		want    bool
	}{
		{name: "same", version: "1.0.2", min: "1.0.2", want: false},
		{name: "patch below", version: "1.0.0", min: "1.0.1", want: true},
		{name: "minor below", version: "1.0.1", min: "1.1.0", want: true},
		{name: "major below", version: "1.3.0", min: "2.0.0", want: true},
		{name: "numeric tuple compare", version: "1.10.0", min: "1.2.0", want: false},
		{name: "malformed client", version: "1.0", min: "1.0.0", want: true},
		{name: "empty client", version: "", min: "1.0.0", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clientVersionRequiresUpdate(tc.version, tc.min); got != tc.want {
				t.Fatalf("clientVersionRequiresUpdate(%q, %q) = %t; want %t", tc.version, tc.min, got, tc.want)
			}
		})
	}
}

func TestGetClientVersionUsesSemverAndOmitsRecommended(t *testing.T) {
	t.Setenv("CLIENT_MIN_VERSION", "1.0.2")
	t.Setenv("CLIENT_UPDATE_REQUIRED_MESSAGE", "Please update.")

	service := &AppService{}
	req := httptest.NewRequest(http.MethodGet, "/client-version?platform=ios&version=1.0.1&build=999", nil)
	rec := httptest.NewRecorder()

	service.GetClientVersion(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d", rec.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error decoding response: %s", err)
	}

	if body["status"] != "update_required" {
		t.Fatalf("status = %v; want update_required", body["status"])
	}
	if body["force_update"] != true {
		t.Fatalf("force_update = %v; want true", body["force_update"])
	}
	if body["message"] != "Please update." {
		t.Fatalf("message = %v; want Please update.", body["message"])
	}
	if _, ok := body["recommended"]; ok {
		t.Fatal("recommended should not be present")
	}
}

func TestGetClientVersionIgnoresBuildForEnforcement(t *testing.T) {
	t.Setenv("CLIENT_MIN_VERSION", "1.0.2")

	service := &AppService{}
	req := httptest.NewRequest(http.MethodGet, "/client-version?platform=android&version=1.0.2&build=0", nil)
	rec := httptest.NewRecorder()

	service.GetClientVersion(rec, req)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error decoding response: %s", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %v; want ok", body["status"])
	}
}

func TestGetClientVersionUsesPerPlatformUpdateURL(t *testing.T) {
	t.Setenv("CLIENT_UPDATE_URL", "https://app.sfluv.org/update")
	t.Setenv("CLIENT_UPDATE_URL_IOS", "https://apps.apple.com/us/app/sfluv/id6762672190")
	t.Setenv("CLIENT_UPDATE_URL_ANDROID", "https://play.google.com/store/apps/details?id=org.sfluv.wallet")

	cases := []struct {
		name     string
		platform string
		want     string
	}{
		{name: "ios uses ios link", platform: "ios", want: "https://apps.apple.com/us/app/sfluv/id6762672190"},
		{name: "android uses android link", platform: "android", want: "https://play.google.com/store/apps/details?id=org.sfluv.wallet"},
		{name: "web falls back to generic", platform: "web", want: "https://app.sfluv.org/update"},
		{name: "missing platform falls back to generic", platform: "", want: "https://app.sfluv.org/update"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &AppService{}
			req := httptest.NewRequest(http.MethodGet, "/client-version?version=1.0.1&platform="+tc.platform, nil)
			rec := httptest.NewRecorder()

			service.GetClientVersion(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; want %d", rec.Code, http.StatusOK)
			}

			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("error decoding response: %s", err)
			}
			if body["update_url"] != tc.want {
				t.Fatalf("update_url = %v; want %v", body["update_url"], tc.want)
			}
		})
	}
}
