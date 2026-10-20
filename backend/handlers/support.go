package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
)

// Website "get in touch" requests.
//
// This replaces a mailto: link, which had no record of anything: whether a
// message was answered depended on whose inbox it landed in, and a person who
// wrote in had nothing to quote if they followed up. Now a request is a row
// with a reference, the sender is told that reference, and the same reference
// appears in the alert — so a reply can be matched to a request by anyone, not
// only by whoever happened to receive the mail.

const (
	supportRateWindow   = time.Minute
	supportRatePerIP    = 3
	supportMaxMessage   = 8000
	supportMaxSubject   = 200
	supportMaxName      = 200
	supportSenderHeader = "X-SFLUV-Proxy-Key"
)

// supportLimiter is a per-process speed bump, the same shape as the one on
// volunteer signups: the website proxies these server-side, so without the
// proxy key every submission would share one bucket.
var supportLimiter = struct {
	mu       sync.Mutex
	attempts map[string]*supportAttempt
}{attempts: map[string]*supportAttempt{}}

type supportAttempt struct {
	count   int
	resetAt time.Time
}

func supportRateLimited(key string) bool {
	supportLimiter.mu.Lock()
	defer supportLimiter.mu.Unlock()

	now := time.Now()
	entry, seen := supportLimiter.attempts[key]
	if !seen || now.After(entry.resetAt) {
		supportLimiter.attempts[key] = &supportAttempt{count: 1, resetAt: now.Add(supportRateWindow)}
		return false
	}
	entry.count++
	return entry.count > supportRatePerIP
}

// supportProxyKey is the shared secret the website presents. It falls back to
// the volunteer key so a deployment does not have to configure two secrets to
// get one behaviour.
func supportProxyKey() string {
	if key := strings.TrimSpace(os.Getenv("SUPPORT_PROXY_KEY")); key != "" {
		return key
	}
	return strings.TrimSpace(os.Getenv("VOLUNTEER_PROXY_KEY"))
}

// CreateSupportRequest receives a submission from the website.
func (a *AppService) CreateSupportRequest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Could not read that."})
		return
	}

	var sub structs.SupportRequestSubmission
	if err := json.Unmarshal(body, &sub); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Could not read that."})
		return
	}

	// Honeypot. Accepted rather than refused, so a bot cannot tell it failed.
	if strings.TrimSpace(sub.Company) != "" {
		writeJSON(w, http.StatusCreated, map[string]string{"reference": "SUP-000000"})
		return
	}

	sub.Email = strings.TrimSpace(sub.Email)
	sub.Name = truncate(strings.TrimSpace(sub.Name), supportMaxName)
	sub.Subject = truncate(strings.TrimSpace(sub.Subject), supportMaxSubject)
	sub.Message = truncate(strings.TrimSpace(sub.Message), supportMaxMessage)

	// net/mail, matching how the rest of the backend validates an address, so
	// the form accepts exactly what the mailer will accept.
	if parsed, perr := mail.ParseAddress(sub.Email); perr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "That email address does not look right. We need it to reply to you.",
		})
		return
	} else {
		sub.Email = parsed.Address
	}
	if sub.Message == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "Please tell us what you need.",
		})
		return
	}

	if supportRateLimited(clientIPForRateLimit(r)) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "That is a lot of messages at once. Try again in a minute.",
		})
		return
	}

	request, err := a.db.CreateSupportRequest(r.Context(), sub)
	if err != nil {
		a.logger.Logf("error storing a support request from %s: %s", sub.Email, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Something went wrong on our side. Please try again.",
		})
		return
	}

	// The reference goes out before anything else can fail: the person has
	// written to us and is owed an acknowledgement regardless of whether the
	// alert lands.
	a.sendSupportReceivedEmail(request)
	a.postSupportRequestAlert(r.Context(), request)

	a.logger.Logf("support request %s from %s", request.Reference, request.Email)
	writeJSON(w, http.StatusCreated, map[string]string{"reference": request.Reference})
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut]
}

// sendSupportReceivedEmail confirms receipt and hands over the reference.
func (a *AppService) sendSupportReceivedEmail(request *structs.SupportRequest) {
	sender := utils.NewEmailSender()
	if sender == nil {
		a.logger.Logf("support request %s: no mailer configured, nobody confirmed receipt", request.Reference)
		return
	}

	subject := request.Subject
	if subject == "" {
		subject = "your message"
	}

	content := fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:140px;">Reference</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; font-weight:600;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Subject</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Your message</td>
    <td style="padding:12px 0; font-size:13px; color:#111827; white-space:pre-wrap;">%s</td>
  </tr>
</table>`,
		utils.EscapeEmailHTML(request.Reference),
		utils.EscapeEmailHTML(subject),
		utils.EscapeEmailHTML(request.Message),
	)

	html := utils.BuildStyledEmailWithFooter(
		"We have your message",
		"Someone will read this and get back to you.",
		content,
		"Quote "+request.Reference+" if you need to follow up.",
	)

	toName := request.Name
	if toName == "" {
		toName = "there"
	}
	if err := sender.SendEmail(request.Email, toName,
		"We have your message ("+request.Reference+")",
		html, utils.NotificationFromEmail(), "SFLuv"); err != nil {
		a.logger.Logf("support request %s: could not confirm receipt to %s: %s", request.Reference, request.Email, err)
	}
}

// postSupportRequestAlert puts the request in front of the support set.
//
// The message itself goes in the alert's body rather than only in a link,
// because a support request is usually answerable from reading it, and making
// somebody open the admin panel to find out what was asked is how a queue stops
// being read.
//
// The reply button carries a mailto the device opens itself. The Telegram
// version of this needed a server-side redirect and a minted token for the same
// button, because a chat client will not accept a mailto in a button at all —
// an endpoint, a token table row and a 302 to do what a URL now does.
func (a *AppService) postSupportRequestAlert(ctx context.Context, request *structs.SupportRequest) {
	if a.notifications == nil {
		return
	}

	subject := request.Subject
	if subject == "" {
		subject = "(no subject)"
	}

	from := strings.TrimSpace(request.Name)
	if from == "" {
		from = request.Email
	} else {
		from = from + " <" + request.Email + ">"
	}

	if err := a.notifications.Post(ctx, Alert{
		Set:   structs.NotificationSetSupport,
		Key:   "support:" + request.Reference,
		Kind:  structs.InternalKindSupportRequest,
		Title: "Support request: " + subject,
		Body:  request.Message,
		Fields: []structs.NotificationField{
			{Label: "From", Value: from},
			{Label: "Reference", Value: request.Reference},
		},
		Payload: map[string]string{
			"id":        strconv.FormatInt(request.Id, 10),
			"reference": request.Reference,
			"email":     request.Email,
			"subject":   subject,
			"message":   request.Message,
		},
	}); err != nil {
		a.logger.Logf("support request %s: could not post the alert: %s", request.Reference, err)
	}
}

// ── Admin ────────────────────────────────────────────────────────────────────

// ListSupportRequests answers the admin panel's Support tab.
func (a *AppService) ListSupportRequests(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	switch status {
	case "", structs.SupportRequestNew, structs.SupportRequestArchived:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown status"})
		return
	}

	requests, err := a.db.ListSupportRequests(r.Context(), status)
	if err != nil {
		a.logger.Logf("error listing support requests: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, requests)
}

// SetSupportRequestArchived archives or reopens a request from the panel.
func (a *AppService) SetSupportRequestArchived(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}

	defer r.Body.Close()
	var body struct {
		Archived bool `json:"archived"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body"})
		return
	}

	who := "an admin"
	if did := utils.GetDid(r); did != nil {
		who = *did
	}

	request, err := a.db.SetSupportRequestArchived(r.Context(), id, body.Archived, who)
	if err != nil {
		a.logger.Logf("error archiving support request %d: %s", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if request == nil {
		// Already in that state. Report the current row rather than an error:
		// two admins clicking the same button is not a failure.
		existing, lookupErr := a.db.GetSupportRequest(r.Context(), id)
		if lookupErr != nil || existing == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, existing)
		return
	}

	// Archiving in the panel retires the alert too, so the browser and the
	// phones do not disagree about what is outstanding.
	if body.Archived {
		if err := a.notifications.Resolve(r.Context(), "support:"+request.Reference,
			"Archived in the admin panel."); err != nil {
			a.logger.Logf("error retracting the alert for %s: %s", request.Reference, err)
		}
	}

	writeJSON(w, http.StatusOK, request)
}
