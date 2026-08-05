package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

// AffiliateRequestVolunteerEvent submits an event for admin approval.
//
// Affiliates no longer create events directly (PJ's Q2 ruling): the request is
// stored as 'pending' with NO codes minted and NO faucet allocation reserved,
// because approval is what commits faucet funds. Until then the event is
// invisible to the public portal.
func (a *AppService) AffiliateRequestVolunteerEvent(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	org, _, err := a.db.GetOrganizationByUser(r.Context(), *userDid)
	if err != nil || org == nil {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("you must belong to an organization to request an event"))
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.VolunteerEventCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("could not read event"))
		return
	}

	startAt, endAt, until, qrCutoff, errMsg := validateVolunteerEventRequest(&req)
	if errMsg != "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(errMsg))
		return
	}

	params := &db.CreateVolunteerEventParams{
		Title:               req.Title,
		Description:         req.Description,
		Slug:                slugifyTitle(req.Title),
		Timezone:            req.Timezone,
		StartAt:             startAt,
		EndAt:               endAt,
		QRExpiresAt:         qrCutoff,
		MaxParticipants:     req.MaxParticipants,
		RewardAmount:        req.RewardAmountSfluv,
		SignupMode:          req.SignupMode,
		SignupURL:           req.SignupURL,
		LocationId:          req.LocationId,
		Owner:               *userDid,
		OrganizationId:      &org.Id,
		ReviewStatus:        structs.EventReviewPending,
		MintCodes:           false,
		RecurrenceFrequency: structs.RecurrenceNone,
	}

	if req.Recurrence != nil {
		params.RecurrenceFrequency = req.Recurrence.Frequency
		params.RecurrenceMonthlyMode = req.Recurrence.MonthlyMode
		params.RecurrenceDayOfMonth = req.Recurrence.DayOfMonth
		params.RecurrenceWeekOfMonth = req.Recurrence.WeekOfMonth
		params.RecurrenceUntil = until

		location, err := time.LoadLocation(req.Timezone)
		if err != nil {
			location = time.UTC
		}
		weekday := int(time.Unix(startAt, 0).In(location).Weekday())
		params.RecurrenceWeekday = &weekday
	}

	id, err := a.bot.db.CreateVolunteerEvent(r.Context(), params)
	if err != nil {
		a.logger.Logf("error creating affiliate volunteer event request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	go a.sendVolunteerEventRequestEmail(org.Name, req.Title, int64(req.RewardAmountSfluv)*int64(req.MaxParticipants))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "review_status": structs.EventReviewPending})
}

// AffiliateListVolunteerEvents lists the caller's organization's events across
// every review state, so an affiliate can see their own pending requests —
// which the public list deliberately never shows.
func (a *AppService) AffiliateListVolunteerEvents(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	org, _, err := a.db.GetOrganizationByUser(r.Context(), *userDid)
	if err != nil || org == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	filter := parseVolunteerEventsFilter(r)
	rows, total, err := a.bot.db.GetOrganizationVolunteerEvents(r.Context(), org.Id, filter)
	if err != nil {
		a.logger.Logf("error listing organization volunteer events: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	eventCtx := a.bot.buildVolunteerEventContext(r, rows)
	events := make([]*structs.VolunteerEvent, 0, len(rows))
	for _, row := range rows {
		event := a.bot.mapVolunteerEvent(row, eventCtx)
		decorateManagementFields(event, row)
		events = append(events, event)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(structs.VolunteerEventsResponse{
		Events: events, Page: filter.Page, Count: filter.Count, Total: total,
		HasMore: (filter.Page+1)*filter.Count < total,
	})
}

type volunteerEventReviewRequest struct {
	Reason string `json:"reason"`
}

// AdminApproveVolunteerEvent approves a pending affiliate request.
//
// This is the point faucet funds are committed, so it is also where the balance
// is checked: approving an event the faucet cannot cover would mint codes that
// fail at redemption, in front of volunteers standing at the event.
func (a *AppService) AdminApproveVolunteerEvent(w http.ResponseWriter, r *http.Request) {
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

	row, err := a.bot.db.GetVolunteerEventForManagement(r.Context(), eventId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading event %s for approval: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	required := int64(row.Amount) * int64(row.MaxParticipants)
	funded := a.volunteerFaucetCanCover(r.Context(), required)
	if !funded {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf(
			"not enough unallocated faucet balance to approve: this event needs %d SFLUV", required,
		)))
		return
	}

	if err := a.bot.db.ApproveVolunteerEvent(r.Context(), eventId, *userDid, funded); err != nil {
		if err == db.ErrEventNotFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "not pending") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("event is not pending approval"))
			return
		}
		a.logger.Logf("error approving volunteer event %s: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	go a.sendVolunteerEventDecisionEmail(row.Owner, row.Title, true, "")

	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) AdminRejectVolunteerEvent(w http.ResponseWriter, r *http.Request) {
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
	body, _ := io.ReadAll(r.Body)
	var req volunteerEventReviewRequest
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}

	row, err := a.bot.db.GetVolunteerEventForManagement(r.Context(), eventId)
	if err != nil && err != pgx.ErrNoRows {
		a.logger.Logf("error loading event %s for rejection: %s", eventId, err)
	}

	if err := a.bot.db.RejectVolunteerEvent(r.Context(), eventId, *userDid, req.Reason); err != nil {
		if err == db.ErrEventNotFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error rejecting volunteer event %s: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if row != nil {
		go a.sendVolunteerEventDecisionEmail(row.Owner, row.Title, false, req.Reason)
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminDownloadVolunteerEventCodes serves the event's QR codes as CSV.
//
// Downloadable as soon as the codes exist, which is deliberately earlier than
// they are spendable — organizers need to print them ahead of the event, and
// the 24h redemption gate is what stops early use.
// AdminDownloadVolunteerEventCodes serves any event's QR codes as CSV.
func (a *AppService) AdminDownloadVolunteerEventCodes(w http.ResponseWriter, r *http.Request) {
	a.writeVolunteerEventCodesCSV(w, r, nil)
}

// AffiliateDownloadVolunteerEventCodes serves an affiliate's own event codes.
//
// Organizers need to print their codes ahead of the event just as admins do, so
// losing self-serve event CREATION does not mean losing access to the codes for
// an event that was approved. Scoped to the caller's organization: these codes
// are bearer tokens for faucet funds, so an affiliate must never be able to
// download another organization's — or an SFLuv-run event's — codes by id.
func (a *AppService) AffiliateDownloadVolunteerEventCodes(w http.ResponseWriter, r *http.Request) {
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

	a.writeVolunteerEventCodesCSV(w, r, &org.Id)
}

// writeVolunteerEventCodesCSV emits the QR codes for an event.
//
// Downloadable as soon as the codes exist, which is deliberately earlier than
// they are spendable — organizers print ahead of the event, and the 24h
// redemption gate is what prevents early use. When requiredOrgId is non-nil the
// event must belong to that organization.
func (a *AppService) writeVolunteerEventCodesCSV(w http.ResponseWriter, r *http.Request, requiredOrgId *int64) {
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	eventId := strings.TrimSpace(r.PathValue("id"))
	if eventId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	row, err := a.bot.db.GetVolunteerEventForManagement(r.Context(), eventId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading event %s for code download: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if requiredOrgId != nil {
		// 404 rather than 403: a wrong-org id should not confirm the event exists.
		if row.OrganizationId == nil || *row.OrganizationId != *requiredOrgId {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	codes, err := a.bot.db.GetVolunteerEventCodes(r.Context(), eventId)
	if err != nil {
		a.logger.Logf("error loading codes for event %s: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	appBase := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-qr-codes.csv"`, slugifyTitle(row.Title)))
	w.WriteHeader(http.StatusOK)

	writer := csv.NewWriter(w)
	defer writer.Flush()

	_ = writer.Write([]string{"code", "redeem_url", "event", "amount_sfluv", "live_at"})

	liveAt := ""
	if row.QRLiveAt != nil {
		liveAt = rfc3339(*row.QRLiveAt)
	}
	for _, code := range codes {
		// Same redeem link format the mobile scanner and deep links already
		// parse — no second redemption path.
		redeemURL := fmt.Sprintf("%s/faucet/redeem?code=%s", appBase, code)
		_ = writer.Write([]string{code, redeemURL, row.Title, fmt.Sprintf("%d", row.Amount), liveAt})
	}
}

func (a *AppService) sendVolunteerEventRequestEmail(orgName string, title string, required int64) {
	adminEmail := strings.TrimSpace(os.Getenv("AFFILIATE_ADMIN_EMAIL"))
	sender := utils.NewEmailSender()
	if adminEmail == "" || sender == nil {
		return
	}

	details := fmt.Sprintf(`
<p style="font-size:14px; color:#111827;">
  <strong>%s</strong> has requested a volunteer event: <strong>%s</strong>.
</p>
<p style="font-size:14px; color:#111827;">
  Approving it will reserve <strong>%d SFLUV</strong> from the faucet and generate its QR codes.
</p>`, utils.EscapeEmailHTML(orgName), utils.EscapeEmailHTML(title), required)

	html := utils.BuildStyledEmail("New volunteer event request", "An affiliate has requested a volunteer event.", details)
	if err := sender.SendEmail(adminEmail, "Admin", "New volunteer event request", html, utils.NotificationFromEmail(), "SFLuv Volunteering"); err != nil {
		a.logger.Logf("error sending volunteer event request email: %s", err)
	}
}

func (a *AppService) sendVolunteerEventDecisionEmail(ownerId string, title string, approved bool, reason string) {
	sender := utils.NewEmailSender()
	if sender == nil {
		return
	}

	ctx, cancel := workflowMaintenanceContext()
	defer cancel()

	user, err := a.db.GetUserById(ctx, ownerId)
	if err != nil || user == nil || user.Email == nil || strings.TrimSpace(*user.Email) == "" {
		return
	}

	subject := "Volunteer event approved"
	heading := "Your volunteer event was approved"
	body := fmt.Sprintf(`
<p style="font-size:14px; color:#111827;">
  <strong>%s</strong> has been approved and is now live on the volunteer portal. Its QR codes are ready to
  download and become redeemable 24 hours before the event starts.
</p>`, utils.EscapeEmailHTML(title))

	if !approved {
		subject = "Volunteer event not approved"
		heading = "Your volunteer event was not approved"
		body = fmt.Sprintf(`
<p style="font-size:14px; color:#111827;">
  <strong>%s</strong> was not approved.
</p>`, utils.EscapeEmailHTML(title))
		if strings.TrimSpace(reason) != "" {
			body += fmt.Sprintf(`<p style="font-size:14px; color:#111827;">Reason: %s</p>`, utils.EscapeEmailHTML(reason))
		}
	}

	html := utils.BuildStyledEmail(heading, subject, body)
	if err := sender.SendEmail(*user.Email, "Organizer", subject, html, utils.NotificationFromEmail(), "SFLuv Volunteering"); err != nil {
		a.logger.Logf("error sending volunteer event decision email: %s", err)
	}
}

// AffiliateUpdateVolunteerEvent edits one of the caller's organization's events.
//
// A request that has not been approved yet is still theirs to change, so it
// applies directly. Once an event is live the edit is parked for admin review
// instead: the change may raise the reward cost, and committing faucet funds is
// an admin decision — the same rule that governs creation.
func (a *AppService) AffiliateUpdateVolunteerEvent(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil || a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	org, _, err := a.db.GetOrganizationByUser(r.Context(), *userDid)
	if err != nil || org == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	eventId := strings.TrimSpace(r.PathValue("id"))
	if eventId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	row, err := a.bot.db.GetVolunteerEventForManagement(r.Context(), eventId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading event %s for affiliate edit: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// 404 rather than 403 so an id cannot be used to probe other organizations.
	if row.OrganizationId == nil || *row.OrganizationId != org.Id {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.VolunteerEventCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("could not read event"))
		return
	}

	if row.ReviewStatus == structs.EventReviewPending {
		if status, message := a.applyVolunteerEventEdit(r.Context(), eventId, &req); status != http.StatusOK {
			w.WriteHeader(status)
			w.Write([]byte(message))
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Validate before parking it, so an admin is never asked to approve
	// something that cannot be applied.
	if _, _, _, _, errMsg := validateVolunteerEventRequest(&req); errMsg != "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(errMsg))
		return
	}

	normalized, err := json.Marshal(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, err := a.bot.db.CreateEventEditRequest(r.Context(), eventId, *userDid, string(normalized)); err != nil {
		a.logger.Logf("error recording edit request for %s: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	go a.sendVolunteerEventRequestEmail(org.Name, "Edit to "+row.Title, int64(req.RewardAmountSfluv)*int64(req.MaxParticipants))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "pending_approval",
		"message": "Your changes were submitted for admin approval. The published event is unchanged until then.",
	})
}
