package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/utils"
)

// Asking somebody for a W-9 by hand.
//
// The thresholds ask on their own as money is earned, but an admin needs to be
// able to ask anyone, any time — while enforcement is off, ahead of a big
// payout, or for someone the automatic path missed. One button sends the same
// form three ways, because any one of them can be missed: an email with a link
// that does not expire, a push, and the in-app prompt with its bell entry.

const (
	w9RequestPushTitle = "SFLuv needs a W-9 from you"
	w9RequestPushBody  = "Tap to fill it in. It takes a couple of minutes."
)

// W9RequestResult is what the admin is shown after sending.
type W9RequestResult struct {
	UserID        string `json:"user_id"`
	TaxYear       int    `json:"tax_year"`
	FilingStatus  string `json:"filing_status"`
	Tier          string `json:"tier"`
	PushDelivered bool   `json:"push_delivered"`
	EmailSent     bool   `json:"email_sent"`
	Email         string `json:"email,omitempty"`
	EmailError    string `json:"email_error,omitempty"`
}

// GetW9StatusForUser is one person's W-9 position, for the request card.
func (a *AppService) GetW9StatusForUser(w http.ResponseWriter, r *http.Request) {
	userID := chiParam(r, "user_id")
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	status, err := a.buildW9Status(r, userID)
	if err != nil {
		a.logger.Logf("error building w9 status for %s: %s", userID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// RequestW9FromUser prepares the form and tells the person about it.
//
// Refused once a W-9 is on file — there is nothing left to ask for. Otherwise
// safe to press again: the form link is reused, not re-created, and the prompt
// and bell entry come back even if they were dismissed.
func (a *AppService) RequestW9FromUser(w http.ResponseWriter, r *http.Request) {
	if a.payouts == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	userID := chiParam(r, "user_id")
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	taxYear := time.Now().UTC().Year()

	user, err := a.db.GetUserById(ctx, userID)
	if err != nil || user == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	existing, err := a.db.GetW9Filing(ctx, userID, taxYear)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if existing != nil && db.W9StatusClears(existing.Status) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "This person already has a W-9 on file for " + fmt.Sprint(taxYear) + "."})
		return
	}

	filing, err := a.payouts.EnsureW9Request(ctx, userID, taxYear)
	if err != nil {
		a.logger.Logf("w9 admin request: could not prepare a form for %s: %s", userID, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "The tax form could not be prepared: " + err.Error()})
		return
	}

	tier, err := a.armAdminW9Request(ctx, userID, taxYear)
	if err != nil {
		a.logger.Logf("w9 admin request: could not arm the in-app prompt for %s: %s", userID, err)
	}

	result := W9RequestResult{UserID: userID, TaxYear: taxYear, Tier: tier}
	if filing != nil {
		result.FilingStatus = filing.Status
	}
	result.PushDelivered = a.pushW9Notice(ctx, userID, "w9_requested", w9RequestPushTitle, w9RequestPushBody)

	name := ""
	if user.Name != nil {
		name = strings.TrimSpace(*user.Name)
	}
	formURL := ""
	if filing != nil {
		formURL = filing.FormURL
	}
	result.Email, result.EmailSent, result.EmailError = a.sendW9RequestEmail(ctx, userID, name, formURL, taxYear)

	requestedBy := "admin key"
	if did := utils.GetDid(r); did != nil {
		requestedBy = *did
	}
	a.logger.Logf("w9 admin request: %s asked %s for a %d W-9 (tier %s, push %t, email %t)",
		requestedBy, userID, taxYear, tier, result.PushDelivered, result.EmailSent)

	writeJSON(w, http.StatusOK, result)
}

// armAdminW9Request re-arms the in-app prompt that best fits what the person
// has earned.
//
// Installed apps can only draw the four threshold prompts, so a direct request
// borrows one. Only the first two are ever borrowed: the escrow and blocked
// prompts tell somebody their money is being held or refused, and a request is
// not that. Below the warning line it is the notice; at or past it, the warning
// — "close to the limit" reads a little short for someone already over it, but
// what it asks for is true, which the other two would not be.
func (a *AppService) armAdminW9Request(ctx context.Context, userID string, taxYear int) (string, error) {
	tier := db.W9TierNotice
	earned, err := a.db.SumUserEarnedForYear(ctx, userID, taxYear)
	if err != nil {
		return "", err
	}
	if warning := w9Thresholds().Warning; warning != nil && earned.Cmp(warning) >= 0 {
		tier = db.W9TierWarning
	}
	if err := a.db.RearmW9Tier(ctx, userID, taxYear, tier); err != nil {
		return "", err
	}
	return tier, a.db.RearmW9Tier(ctx, userID, taxYear, db.W9TierAdminRequest)
}

// sendW9RequestEmail mails the form link. Returns the address used, whether it
// went, and why not.
func (a *AppService) sendW9RequestEmail(ctx context.Context, userID string, name string, formURL string, taxYear int) (string, bool, string) {
	email, err := a.db.GetUserContactEmail(ctx, userID)
	if err != nil || email == nil || strings.TrimSpace(*email) == "" {
		return "", false, "no email address on file"
	}
	to := strings.TrimSpace(*email)
	if formURL == "" {
		return to, false, "no form link to send"
	}
	sender := utils.NewEmailSender()
	if sender == nil {
		return to, false, "email is not configured"
	}

	greeting := "Hi,"
	if name != "" {
		greeting = "Hi " + utils.EscapeEmailHTML(name) + ","
	}
	details := fmt.Sprintf(`
<p style="margin:0 0 12px; font-size:14px; color:#111827;">%s</p>
<p style="margin:0 0 12px; font-size:14px; color:#111827;">
  SFLuv needs a W-9 on file for you for %d. It takes a couple of minutes, and the form is hosted securely by our tax provider, TaxBandits.
</p>
<p style="margin:18px 0;">
  <a href="%s" style="display:inline-block; padding:12px 20px; background:#eb6c6c; color:#ffffff; text-decoration:none; border-radius:8px; font-size:14px; font-weight:600;">Fill out your W-9</a>
</p>
<p style="margin:0 0 8px; font-size:13px; color:#6b7280;">
  You can also fill it in from the SFLuv app. This link doesn't expire.
</p>
<p style="margin:0; font-size:12px; color:#6b7280;">
  Questions? Email techsupport@sfluv.org.
</p>`, greeting, taxYear, utils.EscapeEmailHTML(formURL))

	html := utils.BuildStyledEmail(w9RequestPushTitle, "Your tax form is ready to fill in.", details)
	if err := sender.SendEmail(to, name, w9RequestPushTitle, html, utils.NotificationFromEmail(), "SFLuv"); err != nil {
		a.logger.Logf("w9 admin request: email to %s failed: %s", userID, err)
		return to, false, "the email could not be sent"
	}
	return to, true, ""
}
