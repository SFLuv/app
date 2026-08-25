package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/w9provider"
	"github.com/go-chi/chi/v5"
)

// fakeW9ReturnURL is where the stub sends somebody once they have signed.
//
// Must match the redirect the app hands to WebBrowser.openAuthSessionAsync, or
// the sheet will sit there after submitting: that call watches for exactly this
// URL and closes on seeing it.
const fakeW9ReturnURL = "sfluv://w9/complete"

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
// ForgetFakeW9 clears the stand-in's memory of one person's filing.
//
// Needed because the fake keeps its state in this process while reset-w9.sh
// clears the database, so a reset used to leave a signed submission behind for
// the same payee reference. The next crossing then released immediately and the
// tiers cleared themselves — correct behaviour for a filed W-9, and completely
// baffling when you did not file one.
//
// Mounted only alongside the stub form, so it cannot exist against a real
// vendor. Admin-guarded even so: it is a write, and the fake form route beside
// it is deliberately open because a person has to reach it from a browser.
func (a *AppService) ForgetFakeW9(w http.ResponseWriter, r *http.Request) {
	if !FakeW9FormEnabled() {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	fake, ok := a.w9Provider.(*w9provider.Fake)
	if !ok {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user_id is required"))
		return
	}

	taxYear, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("tax_year")))
	if err != nil || taxYear <= 0 {
		taxYear = time.Now().UTC().Year()
	}

	removed := fake.Forget(userID, taxYear)
	if a.logger != nil {
		a.logger.Logf("w9 fake: forgot %d submission(s) for %s / %d", removed, userID, taxYear)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"forgotten": removed})
}

// postFakeW9Webhook delivers a signed Form W-9 Status Change callback to our
// own receiver, in the background so the form's own response is not held up.
func (a *AppService) postFakeW9Webhook(fake *w9provider.Fake, submissionID string) {
	if !fake.WebhookCredentialsConfigured() {
		if a.logger != nil {
			a.logger.Logf("w9 fake: no client credentials, so no callback was sent for %s; the sweep will pick it up", submissionID)
		}
		return
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body, err := json.Marshal(map[string]any{"SubmissionId": submissionID})
	if err != nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Straight at our own listener. The vendor would reach a public HTTPS
		// URL; locally there is no tunnel to route through and no certificate
		// to present.
		url := strings.TrimSuffix(fakeWebhookBaseURL(), "/") + "/w9/webhook/taxbandits"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("TimeStamp", timestamp)
		req.Header.Set("Signature", fake.SignWebhookAs(timestamp))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if a.logger != nil {
				a.logger.Logf("w9 fake: callback for %s could not be delivered: %s", submissionID, err)
			}
			return
		}
		defer resp.Body.Close()
		if a.logger != nil {
			a.logger.Logf("w9 fake: callback for %s delivered, receiver answered %d", submissionID, resp.StatusCode)
		}
	}()
}

// fakeWebhookBaseURL is where our own receiver is listening.
func fakeWebhookBaseURL() string {
	if base := strings.TrimSpace(os.Getenv("W9_PROVIDER_BASE_URL")); base != "" {
		return base
	}
	return "http://localhost:8080"
}

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

		if err := fake.Sign(requestID, "ssn"); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		// Then post ourselves a callback, signed the way the vendor signs one.
		//
		// The point is to exercise the receiver rather than to save five
		// minutes. A stand-in that quietly marked the filing complete would
		// leave the verification, the lookup by SubmissionId and the detached
		// sync untested until the first real delivery — and the first real
		// delivery releases money. Unsigned unless credentials are configured,
		// in which case the receiver refuses it, which is also worth seeing.
		a.postFakeW9Webhook(fake, requestID)

		// Redirect back into the app, which is what closes the sheet.
		//
		// The form opens in an ASWebAuthenticationSession, and that dismisses
		// itself the moment the page navigates to the scheme it was given —
		// nothing else will close it. A "thanks, you may go now" page leaves
		// somebody reading a dead end and tapping Done, which is a worse
		// version of what the vendor does: theirs redirects to a return URL
		// too.
		//
		// return_url=none opts out, for opening the form in a desktop browser
		// where a custom scheme has nothing to hand off to.
		if returnURL == "none" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<html><body style="font-family:system-ui;padding:2rem">
				<h2>Tax form signed</h2>
				<p>Opened outside the app, so there is nowhere to return to. In the
				app this redirects back and the sheet closes on its own.</p>
				</body></html>`))
			return
		}
		if returnURL == "" {
			returnURL = fakeW9ReturnURL
		}
		http.Redirect(w, r, returnURL, http.StatusFound)
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
