package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

const (
	maxBlastSubjectLength = 120
	maxBlastMessageLength = 4000
	// A portal signup holds its spot this long without confirmation before the
	// spot is released, so an abandoned or bot-submitted form cannot occupy a
	// place indefinitely.
	unconfirmedSignupTTL = 24 * time.Hour
)

type eventBlastRequest struct {
	Subject  string   `json:"subject"`
	Message  string   `json:"message"`
	ImageIds []string `json:"image_ids,omitempty"`
}

type eventBlastResponse struct {
	Recipients int `json:"recipients"`
	Pushed     int `json:"pushed"`
	Emailed    int `json:"emailed"`
}

// AdminSendEventBlast messages everyone holding a confirmed spot.
func (a *AppService) AdminSendEventBlast(w http.ResponseWriter, r *http.Request) {
	a.sendEventBlast(w, r, nil)
}

// AffiliateSendEventBlast is the same, scoped to the caller's organization so an
// organizer can only message their own event's volunteers.
func (a *AppService) AffiliateSendEventBlast(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	org, _, err := a.db.GetOrganizationByUser(r.Context(), *userDid)
	if err != nil || org == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	a.sendEventBlast(w, r, &org.Id)
}

// sendEventBlast delivers an organizer's message to the event's volunteers:
// a push for people whose account has a registered device, an email for
// everyone else. One delivery per person — a push suppresses the email rather
// than doubling up.
func (a *AppService) sendEventBlast(w http.ResponseWriter, r *http.Request, requiredOrgId *int64) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	eventId := strings.TrimSpace(r.PathValue("id"))
	if eventId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req eventBlastRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	req.Subject = strings.TrimSpace(req.Subject)
	req.Message = strings.TrimSpace(req.Message)
	if req.Subject == "" || req.Message == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("a subject and a message are required"))
		return
	}
	if len(req.Subject) > maxBlastSubjectLength || len(req.Message) > maxBlastMessageLength {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("subject must be under %d characters and the message under %d", maxBlastSubjectLength, maxBlastMessageLength)))
		return
	}

	row, err := a.bot.db.GetVolunteerEventForManagement(r.Context(), eventId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading event %s for blast: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if requiredOrgId != nil {
		// 404 rather than 403 so a wrong-org id cannot confirm the event exists.
		if row.OrganizationId == nil || *row.OrganizationId != *requiredOrgId {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	recipients, err := a.bot.db.GetEventBlastRecipients(r.Context(), eventId)
	if err != nil {
		a.logger.Logf("error loading blast recipients for %s: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	pushed, emailed := a.deliverEventBlast(r.Context(), eventId, row.Title, req.Subject, req.Message, blastImageURLs(req.ImageIds), recipients)

	if err := a.bot.db.RecordEventBlast(r.Context(), eventId, *userDid, req.Subject, req.Message, pushed, emailed); err != nil {
		a.logger.Logf("error recording event blast for %s: %s", eventId, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(eventBlastResponse{
		Recipients: len(recipients), Pushed: pushed, Emailed: emailed,
	})
}

func (a *AppService) deliverEventBlast(
	ctx context.Context, eventId string, eventTitle string, subject string, message string,
	imageURLs []string, recipients []db.EventBlastRecipient,
) (int, int) {
	sender := utils.NewEmailSender()
	pushed, emailed := 0, 0

	// Collapse duplicates: the same person can hold both an in-app and a portal
	// signup for one event, and should still hear from us exactly once.
	seen := map[string]struct{}{}

	for _, recipient := range recipients {
		key := strings.ToLower(strings.TrimSpace(recipient.Email))
		if recipient.UserId != nil && strings.TrimSpace(*recipient.UserId) != "" {
			key = "user:" + *recipient.UserId
		}
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		// Push when the account has a registered device; otherwise email. Never
		// both — a duplicate notification is worse than a single channel.
		if recipient.UserId != nil && a.pushEventBlast(ctx, *recipient.UserId, eventId, subject, message) {
			pushed++
			continue
		}

		if sender == nil || strings.TrimSpace(recipient.Email) == "" {
			continue
		}
		if err := sender.SendEmail(
			recipient.Email,
			firstNameOrFallback(recipient.FirstName),
			subject,
			buildEventBlastEmail(eventTitle, subject, message, imageURLs),
			utils.NotificationFromEmail(),
			"SFLuv Volunteering",
		); err != nil {
			a.logger.Logf("error sending event blast email to %s: %s", recipient.Email, err)
			continue
		}
		emailed++
	}

	return pushed, emailed
}

func firstNameOrFallback(name string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return "Volunteer"
}

func (a *AppService) pushEventBlast(ctx context.Context, userId string, eventId string, subject string, message string) bool {
	subscriptions, err := a.db.GetMobilePushSubscriptionsByUser(ctx, userId)
	if err != nil {
		return false
	}

	delivered := false
	tokensSeen := map[string]struct{}{}
	for _, subscription := range subscriptions {
		if subscription == nil || !subscription.Active || !subscription.DeviceRegistered {
			continue
		}
		token := strings.TrimSpace(subscription.Token)
		if token == "" {
			continue
		}
		if _, ok := tokensSeen[token]; ok {
			continue
		}
		tokensSeen[token] = struct{}{}

		// event_id so the app can deep-link to the event, reusing the plumbing
		// already built for reminders.
		if _, err := sendExpoPushNotification(ctx, token, subject, message, map[string]string{
			"type":     "volunteer_event_blast",
			"event_id": eventId,
		}); err != nil {
			a.logger.Logf("error pushing event blast to %s: %s", userId, err)
			continue
		}
		delivered = true
	}

	return delivered
}

var (
	blastBoldPattern   = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	blastItalicPattern = regexp.MustCompile(`_([^_\n]+)_`)
	blastLinkPattern   = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)
)

// renderBlastBody turns an organizer's message into safe email HTML.
//
// The client never sends HTML. Everything is escaped FIRST, then a small,
// closed set of markers is turned into tags — so an organizer typing "<b>" gets
// the literal text, while "**bold**" gets emphasis. Link targets are re-checked
// against the same http(s) allowlist used for partner links, because an
// organizer-supplied "javascript:" href would otherwise ship inside an email
// under our brand.
func renderBlastBody(message string) string {
	escaped := utils.EscapeEmailHTML(message)

	escaped = blastLinkPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		parts := blastLinkPattern.FindStringSubmatch(match)
		label, target := parts[1], parts[2]
		// The URL was escaped along with everything else; undo the entity for
		// "&" so query strings survive, then validate the result.
		target = strings.ReplaceAll(target, "&amp;", "&")
		if !isSafePartnerLink(target) {
			return label
		}
		return fmt.Sprintf(
			`<a href="%s" style="color:#eb6c6c; text-decoration:underline;">%s</a>`,
			utils.EscapeEmailHTML(target), label,
		)
	})

	escaped = blastBoldPattern.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = blastItalicPattern.ReplaceAllString(escaped, "<em>$1</em>")

	// Paragraph breaks on blank lines, single newlines as line breaks.
	paragraphs := strings.Split(escaped, "\n\n")
	rendered := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		rendered = append(rendered, fmt.Sprintf(
			`<p style="font-size:14px; color:#111827; line-height:1.6; margin:0 0 16px;">%s</p>`,
			strings.ReplaceAll(paragraph, "\n", "<br />"),
		))
	}

	return strings.Join(rendered, "")
}

// buildEventBlastEmail renders the organizer's message inside the standard
// SFLuv email shell. Used for both the preview and the send, so what an
// organizer approves is byte-for-byte what goes out.
func buildEventBlastEmail(eventTitle string, subject string, message string, imageURLs []string) string {
	details := fmt.Sprintf(`
<p style="font-size:13px; color:#6b7280; margin:0 0 4px;">About your volunteer event</p>
<p style="font-size:16px; font-weight:600; color:#111827; margin:0 0 16px;">%s</p>
%s`, utils.EscapeEmailHTML(eventTitle), renderBlastBody(message))

	for _, url := range imageURLs {
		details += fmt.Sprintf(
			`<img src="%s" alt="" style="display:block; width:100%%; max-width:504px; height:auto; border-radius:12px; margin:16px 0;" />`,
			utils.EscapeEmailHTML(url),
		)
	}

	// BuildStyledEmail escapes the title and subtitle itself; escaping here too
	// would double-encode and render a literal "&lt;" in the header.
	return utils.BuildStyledEmail(subject, "A message from your event organizer", details)
}

// sendVolunteerSignupConfirmationEmail asks a portal signup to confirm their
// address. The spot is held meanwhile, so the copy must not imply it is at risk
// — only that confirmation is still needed.
func (a *AppService) sendVolunteerSignupConfirmationEmail(email string, firstName string, eventTitle string, startAt int64, token string) {
	sender := utils.NewEmailSender()
	if sender == nil || strings.TrimSpace(email) == "" || strings.TrimSpace(token) == "" {
		return
	}

	base := strings.TrimRight(strings.TrimSpace(os.Getenv("VOLUNTEER_PORTAL_BASE_URL")), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")
	}
	// Deliberately NOT /volunteers/confirm: the portal serves event pages from
	// the dynamic route /volunteers/{slug}-{id}, so that path would sit inside
	// its namespace and resolve only because static segments beat dynamic ones
	// in the router. This sibling path cannot collide, and matches the
	// /volunteer-email/* pages the list emails already link to.
	confirmURL := fmt.Sprintf("%s/volunteer-signup/confirm?token=%s", base, token)

	when := time.Unix(startAt, 0).UTC().Format("Mon 2 Jan 2006")

	details := fmt.Sprintf(`
<p style="font-size:14px; color:#111827; line-height:1.6;">
  Thanks for signing up for <strong>%s</strong> on %s. Please confirm your email address so we can send you
  event details and reminders.
</p>
<p style="margin:24px 0;">
  <a href="%s" style="background:#eb6c6c; color:#ffffff; text-decoration:none; padding:12px 22px; border-radius:999px; font-size:14px; font-weight:600; display:inline-block;">
    Confirm my sign-up
  </a>
</p>
<p style="font-size:13px; color:#6b7280; line-height:1.6;">
  Your spot is held while you confirm. If you did not sign up for this event, you can ignore this email.
</p>`,
		utils.EscapeEmailHTML(eventTitle), utils.EscapeEmailHTML(when), utils.EscapeEmailHTML(confirmURL))

	html := utils.BuildStyledEmail("Confirm your sign-up", "One step left to secure your volunteer spot.", details)
	if err := sender.SendEmail(email, firstNameOrFallback(firstName), "Confirm your volunteer sign-up", html, utils.NotificationFromEmail(), "SFLuv Volunteering"); err != nil {
		a.logger.Logf("error sending volunteer signup confirmation email: %s", err)
	}
}

// GetVolunteerSignupConfirmState is the read half of the signup confirmation:
// it renders the landing page without confirming, so a mail scanner prefetching
// the link cannot confirm on the user's behalf.
func (a *AppService) GetVolunteerSignupConfirmState(w http.ResponseWriter, r *http.Request) {
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	email, title, err := a.bot.db.PeekVolunteerSignupByToken(r.Context(), strings.TrimSpace(r.URL.Query().Get("token")))
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error reading signup confirm token: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"email": email, "event_title": title, "status": "pending_confirmation"})
}

// PostVolunteerSignupConfirm completes the signup on an explicit POST.
func (a *AppService) PostVolunteerSignupConfirm(w http.ResponseWriter, r *http.Request) {
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	var req volunteerListTokenRequest
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}
	if req.Token == "" {
		req.Token = strings.TrimSpace(r.URL.Query().Get("token"))
	}

	eventId, email, err := a.bot.db.ConfirmVolunteerSignup(r.Context(), req.Token)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error confirming volunteer signup: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"event_id": eventId, "email": email, "status": "confirmed"})
}

// ExpireUnconfirmedSignups releases spots held by portal signups that were never
// confirmed. Run from the maintenance sweep so an abandoned form frees its place
// without anyone intervening.
func (a *AppService) ExpireUnconfirmedSignups(ctx context.Context) {
	if a.bot == nil || a.bot.db == nil {
		return
	}

	cutoff := time.Now().Add(-unconfirmedSignupTTL).Unix()
	expired, err := a.bot.db.ExpireUnconfirmedVolunteerSignups(ctx, cutoff)
	if err != nil {
		a.logger.Logf("error expiring unconfirmed volunteer signups: %s", err)
		return
	}
	if expired > 0 {
		a.logger.Logf("released %d unconfirmed volunteer signup spot(s)", expired)
	}
}

// blastImageURLs maps uploaded image ids to their public URLs. Email clients
// cannot present credentials, so these must be reachable unauthenticated.
func blastImageURLs(ids []string) []string {
	urls := make([]string, 0, len(ids))
	for _, id := range ids {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			urls = append(urls, publicURL("/volunteer-events/blast-images/"+trimmed))
		}
	}
	return urls
}

// PreviewEventBlast renders exactly what would be sent, without sending it.
//
// Rendered on the server rather than re-implemented in the client on purpose: a
// client-side preview can drift from the real template, and the whole point of
// a preview is that it is truthful.
func (a *AppService) PreviewEventBlast(w http.ResponseWriter, r *http.Request) {
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	eventId := strings.TrimSpace(r.PathValue("id"))
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req eventBlastRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	title := "your volunteer event"
	if row, err := a.bot.db.GetVolunteerEventForManagement(r.Context(), eventId); err == nil && row != nil {
		title = row.Title
	}

	recipients, err := a.bot.db.GetEventBlastRecipients(r.Context(), eventId)
	if err != nil {
		a.logger.Logf("error counting blast recipients for preview %s: %s", eventId, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"html":       buildEventBlastEmail(title, strings.TrimSpace(req.Subject), strings.TrimSpace(req.Message), blastImageURLs(req.ImageIds)),
		"subject":    strings.TrimSpace(req.Subject),
		"recipients": len(recipients),
	})
}

// UploadEventBlastImage accepts an inline image for a message. Bytes are
// sniffed rather than trusted, since the result is served publicly so email
// clients can fetch it.
func (a *AppService) UploadEventBlastImage(w http.ResponseWriter, r *http.Request) {
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	eventId := strings.TrimSpace(r.PathValue("id"))
	if eventId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := r.ParseMultipartForm(maxEventPhotoBytes); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("could not read upload"))
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("image")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("image file is required"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxEventPhotoBytes+1))
	if err != nil || len(data) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("image file is empty"))
		return
	}
	if len(data) > maxEventPhotoBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte(fmt.Sprintf("image must be %d MB or smaller", maxEventPhotoBytes>>20)))
		return
	}

	contentType := resolvePartnerLogoContentType(data, header.Filename)
	if contentType == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("attachments must be a PNG, JPEG, GIF, WebP, or SVG image"))
		return
	}

	imageId, err := a.bot.db.AddEventBlastImage(r.Context(), eventId, data, contentType)
	if err != nil {
		a.logger.Logf("error storing blast image for %s: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":  imageId,
		"url": publicURL("/volunteer-events/blast-images/" + imageId),
	})
}

// GetEventBlastImage serves an inline image. Public by necessity: an email
// client fetching it cannot present credentials. Ids are unguessable UUIDs.
func (a *AppService) GetEventBlastImage(w http.ResponseWriter, r *http.Request) {
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	image, err := a.bot.db.GetEventBlastImage(r.Context(), strings.TrimSpace(r.PathValue("image_id")))
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading blast image: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", image.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image.Data)
}
