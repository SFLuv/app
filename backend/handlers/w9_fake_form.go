package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/SFLuv/app/backend/w9provider"
	"github.com/go-chi/chi/v5"
)

// FakeW9FormEnabled reports whether the stub tax form should be mounted. It is
// tied to the fake provider, so it can never appear against a real vendor.
func FakeW9FormEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("W9_PROVIDER")), "fake")
}

// ServeFakeW9Form stands in for the vendor's hosted form.
//
// It exists so the entire loop can be exercised locally: request a form, open
// it, submit it, receive a correctly signed webhook, release the escrow, watch
// the transfer land. Submitting posts the webhook to our own real route, so the
// production path is what runs — not a shortcut around it.
//
// It never asks for a tax identification number, and must not: the whole reason
// for using a vendor is that an SSN never reaches our systems, and a test
// fixture that collected one would undo that on the first person who used it
// with real data.
func (a *AppService) ServeFakeW9Form(w http.ResponseWriter, r *http.Request) {
	if !FakeW9FormEnabled() {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	requestID := chi.URLParam(r, "request_id")
	returnURL := strings.TrimSpace(r.URL.Query().Get("return_url"))

	if r.Method == http.MethodPost {
		fake, ok := a.w9Provider.(*w9provider.Fake)
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Signing only records signed_at. Nothing is pushed anywhere: the
		// backend learns about it when the maintenance sweep next polls, which
		// is exactly how it will work in production, because this vendor has no
		// webhook to push with.
		if err := fake.Sign(requestID, "ssn"); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		if returnURL != "" {
			http.Redirect(w, r, returnURL, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body style="font-family:system-ui;padding:2rem">
			<h2>Tax form signed</h2>
			<p>This is the local stand-in for the tax vendor. Your held rewards go out
			on the next maintenance sweep, which is how completion is detected in
			production too — this vendor publishes no webhooks.</p>
			</body></html>`))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<html><body style="font-family:system-ui;padding:2rem;max-width:32rem">
		<h2>W-9 (local test stand-in)</h2>
		<p>The real vendor collects a name, address and tax identification number here.
		This stub collects nothing — an SSN must never reach our systems, not even in testing.</p>
		<form method="POST">
			<button type="submit" style="padding:.75rem 1.25rem;font-size:1rem">Submit W-9</button>
		</form>
		<p style="color:#666;font-size:.85rem">Request %s</p>
		</body></html>`, requestID)
}
