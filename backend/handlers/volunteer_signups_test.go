package handlers

import (
	"net/http/httptest"
	"testing"
)

// X-Forwarded-For must be trusted only when the caller proves it is our proxy.
//
// sfluv.org proxies signups server-side, so without honouring the header every
// web signup shares one rate-limit bucket. But honouring it unconditionally is
// worse than not having it: anyone could forge an address per request and evade
// the per-IP limit entirely.
func TestClientIPForRateLimitTrustsForwardedOnlyWithProxyKey(t *testing.T) {
	t.Run("valid proxy key: forwarded address is used", func(t *testing.T) {
		t.Setenv("VOLUNTEER_PROXY_KEY", "s3cret")
		r := httptest.NewRequest("POST", "/volunteer-events/abc/signup", nil)
		r.RemoteAddr = "10.0.0.1:5000"
		r.Header.Set("X-SFLUV-Proxy-Key", "s3cret")
		r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")

		if got := clientIPForRateLimit(r); got != "203.0.113.9" {
			t.Fatalf("got %q, want the original client IP 203.0.113.9", got)
		}
	})

	t.Run("wrong proxy key: forwarded address is ignored", func(t *testing.T) {
		t.Setenv("VOLUNTEER_PROXY_KEY", "s3cret")
		r := httptest.NewRequest("POST", "/volunteer-events/abc/signup", nil)
		r.RemoteAddr = "10.0.0.1:5000"
		r.Header.Set("X-SFLUV-Proxy-Key", "guess")
		r.Header.Set("X-Forwarded-For", "203.0.113.9")

		if got := clientIPForRateLimit(r); got != "10.0.0.1" {
			t.Fatalf("got %q, want the socket IP 10.0.0.1", got)
		}
	})

	t.Run("no proxy key configured: forwarded address is never trusted", func(t *testing.T) {
		t.Setenv("VOLUNTEER_PROXY_KEY", "")
		r := httptest.NewRequest("POST", "/volunteer-events/abc/signup", nil)
		r.RemoteAddr = "10.0.0.1:5000"
		// An attacker can always set these headers themselves.
		r.Header.Set("X-SFLUV-Proxy-Key", "")
		r.Header.Set("X-Forwarded-For", "203.0.113.9")

		if got := clientIPForRateLimit(r); got != "10.0.0.1" {
			t.Fatalf("got %q, want the socket IP when no key is configured", got)
		}
	})

	t.Run("no forwarded header falls back to the socket IP", func(t *testing.T) {
		t.Setenv("VOLUNTEER_PROXY_KEY", "s3cret")
		r := httptest.NewRequest("POST", "/volunteer-events/abc/signup", nil)
		r.RemoteAddr = "198.51.100.4:41000"
		r.Header.Set("X-SFLUV-Proxy-Key", "s3cret")

		if got := clientIPForRateLimit(r); got != "198.51.100.4" {
			t.Fatalf("got %q, want 198.51.100.4", got)
		}
	})
}

// Both opt-in spellings are accepted so a client using either wording cannot
// silently fail to subscribe someone who ticked the box.
func TestSignupOptInAcceptsBothFieldNames(t *testing.T) {
	yes, no := true, false

	if !(&volunteerSignupRequest{VolunteerListOptIn: &yes}).optedIn() {
		t.Error("volunteer_list_opt_in=true should opt in")
	}
	if !(&volunteerSignupRequest{MarketingOptIn: &yes}).optedIn() {
		t.Error("marketing_opt_in=true should opt in")
	}
	if (&volunteerSignupRequest{MarketingOptIn: &no}).optedIn() {
		t.Error("marketing_opt_in=false should not opt in")
	}
	// Absent means absent: never subscribe someone who did not ask.
	if (&volunteerSignupRequest{}).optedIn() {
		t.Error("an omitted opt-in must default to false")
	}
	// An explicit volunteer_list_opt_in wins over a stale marketing_opt_in.
	if (&volunteerSignupRequest{VolunteerListOptIn: &no, MarketingOptIn: &yes}).optedIn() {
		t.Error("explicit volunteer_list_opt_in=false should win")
	}
}

func TestSplitContactName(t *testing.T) {
	cases := []struct{ in, first, last string }{
		{"Ada Lovelace", "Ada", "Lovelace"},
		{"Ada", "Ada", ""},
		{"Ada King Lovelace", "Ada", "King Lovelace"},
		{"   ", "", ""},
		{"", "", ""},
	}
	for _, testCase := range cases {
		first, last := splitContactName(testCase.in)
		if first != testCase.first || last != testCase.last {
			t.Errorf("splitContactName(%q) = (%q, %q), want (%q, %q)", testCase.in, first, last, testCase.first, testCase.last)
		}
	}
}
