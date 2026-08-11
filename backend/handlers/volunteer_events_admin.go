package handlers

import (
	"context"
	"encoding/json"
	"errors"
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

const (
	maxEventPhotoBytes = 8 << 20 // 8 MiB
	maxEventPhotos     = 6
)

// localWallClockLayouts are the accepted wall-clock forms. All are
// offset-free by design: an offset would mean the client had already done the
// conversion, which is exactly what this endpoint takes responsibility for.
var localWallClockLayouts = []string{
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
}

// parseLocalWallClock converts "2026-08-06T13:00:00" in the given IANA zone to
// a UTC instant.
//
// This is the single place wall-clock becomes an instant. Doing it server-side
// means one tested implementation instead of every client re-deriving it, and
// it is what keeps a recurring series anchored to the same local time across a
// DST boundary.
func parseLocalWallClock(value string, timezone string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("time is required")
	}
	if strings.HasSuffix(value, "Z") || strings.Contains(value[10:], "+") {
		return 0, fmt.Errorf("time must be wall clock without a timezone offset")
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		return 0, fmt.Errorf("unknown timezone %q", timezone)
	}

	for _, layout := range localWallClockLayouts {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed.UTC().Unix(), nil
		}
	}

	return 0, fmt.Errorf("time must look like 2026-08-06T13:00:00")
}

// validateVolunteerEventRequest normalizes and checks a create request,
// returning a message safe to show the admin.
// defaultQRGracePeriod is how long after an event ends its codes stay
// redeemable when no explicit cutoff is given. Someone still in the queue when
// an event wraps up should not lose their reward to the clock.
const defaultQRGracePeriod = 24 * time.Hour

func validateVolunteerEventRequest(req *structs.VolunteerEventCreateRequest) (startAt int64, endAt int64, until *int64, qrCutoff int64, errMsg string) {
	req.Title = strings.TrimSpace(req.Title)
	req.Description = strings.TrimSpace(req.Description)
	req.Timezone = strings.TrimSpace(req.Timezone)
	req.SignupMode = strings.TrimSpace(req.SignupMode)
	req.SignupURL = strings.TrimSpace(req.SignupURL)

	if req.Title == "" {
		return 0, 0, nil, 0, "title is required"
	}
	if req.Timezone == "" {
		req.Timezone = "America/Los_Angeles"
	}

	startAt, err := parseLocalWallClock(req.StartAtLocal, req.Timezone)
	if err != nil {
		return 0, 0, nil, 0, "start: " + err.Error()
	}
	endAt, err = parseLocalWallClock(req.EndAtLocal, req.Timezone)
	if err != nil {
		return 0, 0, nil, 0, "end: " + err.Error()
	}
	if endAt <= startAt {
		return 0, 0, nil, 0, "end must be after start"
	}

	// An event in the past cannot be attended, and its codes would already be
	// live — a small grace window absorbs clock skew and the seconds between
	// filling the form and submitting it.
	if startAt < time.Now().Add(-5*time.Minute).Unix() {
		return 0, 0, nil, 0, "start must be in the future"
	}

	// Redemption stays open for a grace period after the end unless an explicit
	// cutoff is given.
	qrCutoff = endAt + int64(defaultQRGracePeriod.Seconds())
	if strings.TrimSpace(req.QRCutoffLocal) != "" {
		explicit, err := parseLocalWallClock(req.QRCutoffLocal, req.Timezone)
		if err != nil {
			return 0, 0, nil, 0, "QR cutoff: " + err.Error()
		}
		if explicit < endAt {
			return 0, 0, nil, 0, "the QR cutoff must not be before the event ends"
		}
		qrCutoff = explicit
	}

	switch req.SignupMode {
	case structs.SignupModeNone, structs.SignupModeInternal:
		req.SignupURL = ""
	case structs.SignupModeExternal:
		if req.SignupURL == "" {
			return 0, 0, nil, 0, "an external signup needs a signup link"
		}
		if !isSafePartnerLink(req.SignupURL) {
			return 0, 0, nil, 0, "signup link must be a full http(s) URL"
		}
	default:
		return 0, 0, nil, 0, "signup mode must be none, external, or internal"
	}

	// max_participants is the number of QR codes minted, so it bounds the
	// faucet exposure of the event and cannot be open-ended.
	if req.MaxParticipants <= 0 {
		return 0, 0, nil, 0, "max participants must be at least 1"
	}
	if req.MaxParticipants > 10000 {
		return 0, 0, nil, 0, "max participants must be 10000 or fewer"
	}

	if req.Recurrence != nil {
		switch req.Recurrence.Frequency {
		case "", structs.RecurrenceNone:
			req.Recurrence = nil
		case structs.RecurrenceDaily, structs.RecurrenceWeekly:
		case structs.RecurrenceMonthly:
			switch req.Recurrence.MonthlyMode {
			case structs.MonthlyModeDayOfMonth, structs.MonthlyModeDayOfWeek:
			case "":
				req.Recurrence.MonthlyMode = structs.MonthlyModeDayOfMonth
			default:
				return 0, 0, nil, 0, "monthly mode must be day_of_month or day_of_week"
			}
		default:
			return 0, 0, nil, 0, "recurrence must be none, daily, weekly, or monthly"
		}
	}

	if req.Recurrence != nil && req.Recurrence.UntilLocal != nil && strings.TrimSpace(*req.Recurrence.UntilLocal) != "" {
		untilUnix, err := parseLocalWallClock(*req.Recurrence.UntilLocal, req.Timezone)
		if err != nil {
			return 0, 0, nil, 0, "repeat-until: " + err.Error()
		}
		if untilUnix < startAt {
			return 0, 0, nil, 0, "repeat-until must be after the first event"
		}
		until = &untilUnix
	}

	return startAt, endAt, until, qrCutoff, ""
}

// validateVolunteerLocation rejects a location_id that does not refer to a real
// volunteer location. The read path already filters to volunteer-kind rows, so
// a bad id could not leak a merchant's address — but storing an unvalidated
// foreign reference relies on that downstream filter staying correct forever.
func (a *AppService) validateVolunteerLocation(ctx context.Context, locationId *int64) string {
	if locationId == nil {
		return ""
	}
	exists, err := a.db.VolunteerLocationExists(ctx, *locationId)
	if err != nil {
		a.logger.Logf("error validating volunteer location %d: %s", *locationId, err)
		return "could not verify the selected location"
	}
	if !exists {
		return "the selected location does not exist"
	}
	return ""
}

// AdminCreateVolunteerEvent creates an event that is approved on creation:
// admins can generate any event they want, so there is nobody to approve it.
// Codes are minted and the faucet allocation reserved in the same transaction.
func (a *AppService) AdminCreateVolunteerEvent(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
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
	if errMsg == "" {
		errMsg = a.validateVolunteerLocation(r.Context(), req.LocationId)
	}
	if errMsg != "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(errMsg))
		return
	}

	photoIds, errMsg := validateStagedPhotoIds(req.PhotoIds)
	if errMsg != "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(errMsg))
		return
	}

	// Approval is gated on the faucet being able to cover one occurrence:
	// reward x participants. Without this an event could mint codes the faucet
	// cannot honour, which surfaces as a failed redemption at the event itself.
	required := int64(req.RewardAmountSfluv) * int64(req.MaxParticipants)
	if required > 0 {
		unallocated, err := a.bot.unallocatedBalanceTokens(r.Context())
		if err != nil {
			a.logger.Logf("error checking unallocated faucet balance for volunteer event: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if unallocated.Int64() < required {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(fmt.Sprintf(
				"not enough unallocated faucet balance: this event needs %d SFLUV (%d x %d) and %s is available",
				required, req.RewardAmountSfluv, req.MaxParticipants, unallocated.String(),
			)))
			return
		}
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
		ReviewStatus:        structs.EventReviewApproved,
		MintCodes:           true,
		RecurrenceFrequency: structs.RecurrenceNone,
		StagedPhotoIds:      photoIds,
	}

	if req.Recurrence != nil {
		params.RecurrenceFrequency = req.Recurrence.Frequency
		params.RecurrenceMonthlyMode = req.Recurrence.MonthlyMode
		params.RecurrenceDayOfMonth = req.Recurrence.DayOfMonth
		params.RecurrenceWeekOfMonth = req.Recurrence.WeekOfMonth
		params.RecurrenceUntil = until

		// Anchor weekday/day-of-month to the start date when unspecified, so the
		// series repeats on the day the admin actually picked.
		location, err := time.LoadLocation(req.Timezone)
		if err != nil {
			location = time.UTC
		}
		localStart := time.Unix(startAt, 0).In(location)
		weekday := int(localStart.Weekday())
		params.RecurrenceWeekday = &weekday
		if req.Recurrence.Frequency == structs.RecurrenceMonthly &&
			req.Recurrence.MonthlyMode == structs.MonthlyModeDayOfMonth &&
			req.Recurrence.DayOfMonth == nil {
			day := localStart.Day()
			params.RecurrenceDayOfMonth = &day
		}
	}

	id, err := a.bot.db.CreateVolunteerEvent(r.Context(), params)
	if err != nil {
		a.logger.Logf("error creating volunteer event: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	row, err := a.bot.db.GetPublicVolunteerEvent(r.Context(), id)
	if err != nil {
		a.logger.Logf("error reloading volunteer event %s after create: %s", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	eventCtx := a.bot.buildVolunteerEventContext(r, []*db.VolunteerEventRow{row})
	event := a.bot.mapVolunteerEvent(row, eventCtx)
	decorateManagementFields(event, row)
	event.Creator = a.resolveEventCreators(r.Context(), []*db.VolunteerEventRow{row})[row.Id]

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(event)
}

// decorateManagementFields adds the admin/affiliate-only fields that are
// deliberately withheld from public responses.
// decoratePendingEdits marks the events in a management list that have an edit
// awaiting approval. Non-fatal: a lookup failure costs the badge, not the list.
func (a *AppService) decoratePendingEdits(ctx context.Context, events []*structs.VolunteerEvent, rows []*db.VolunteerEventRow) {
	if len(events) == 0 {
		return
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Id)
	}

	pending, err := a.bot.db.GetPendingEventEditRequests(ctx, ids)
	if err != nil {
		a.logger.Logf("error loading pending edit requests: %s", err)
		return
	}
	for _, event := range events {
		if request, ok := pending[event.Id]; ok {
			summary := &structs.VolunteerEventPendingEdit{RequestedAt: rfc3339(request.CreatedAt)}
			// The proposed title is the one field worth showing without opening
			// the event — a rename is the edit most likely to matter at a glance.
			var proposed structs.VolunteerEventCreateRequest
			if json.Unmarshal([]byte(request.Payload), &proposed) == nil {
				summary.Title = proposed.Title
			}
			event.PendingEdit = summary
		}
	}
}

func decorateManagementFields(event *structs.VolunteerEvent, row *db.VolunteerEventRow) {
	event.ReviewStatus = row.ReviewStatus
	event.FundingStatus = row.FundingStatus

	qr := &structs.VolunteerEventQR{CodesGenerated: row.CodesGenerated}
	if row.QRLiveAt != nil {
		liveAt := rfc3339(*row.QRLiveAt)
		qr.LiveAt = &liveAt
		qr.Live = time.Now().Unix() >= *row.QRLiveAt
	}
	event.QR = qr
}

func (a *AppService) AdminListVolunteerEvents(w http.ResponseWriter, r *http.Request) {
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	filter := parseVolunteerEventsFilter(r)
	reviewStatus := strings.TrimSpace(r.URL.Query().Get("review_status"))

	rows, total, err := a.bot.db.GetAdminVolunteerEvents(r.Context(), filter, reviewStatus)
	if err != nil {
		a.logger.Logf("error listing admin volunteer events: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	eventCtx := a.bot.buildVolunteerEventContext(r, rows)
	creators := a.resolveEventCreators(r.Context(), rows)
	events := make([]*structs.VolunteerEvent, 0, len(rows))
	for _, row := range rows {
		event := a.bot.mapVolunteerEvent(row, eventCtx)
		decorateManagementFields(event, row)
		event.Creator = creators[row.Id]
		events = append(events, event)
	}
	a.decoratePendingEdits(r.Context(), events, rows)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(structs.VolunteerEventsResponse{
		Events:  events,
		Page:    filter.Page,
		Count:   filter.Count,
		Total:   total,
		HasMore: (filter.Page+1)*filter.Count < total,
	})
}

// AdminUploadVolunteerEventPhoto attaches a cover photo. Bytes are sniffed
// rather than trusted, since these are served to every visitor of the public
// portal.
func (a *AppService) AdminUploadVolunteerEventPhoto(w http.ResponseWriter, r *http.Request) {
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

	file, header, err := r.FormFile("photo")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("photo file is required"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxEventPhotoBytes+1))
	if err != nil || len(data) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("photo file is empty"))
		return
	}
	if len(data) > maxEventPhotoBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte(fmt.Sprintf("photo must be %d MB or smaller", maxEventPhotoBytes>>20)))
		return
	}

	contentType := resolvePartnerLogoContentType(data, header.Filename)
	if contentType == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("cover photo must be a PNG, JPEG, GIF, WebP, or SVG image"))
		return
	}

	existing, err := a.bot.db.GetVolunteerEventPhotos(r.Context(), []string{eventId})
	if err == nil && len(existing[eventId]) >= maxEventPhotos {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("an event can have at most %d cover photos", maxEventPhotos)))
		return
	}

	width, height := partnerLogoDimensions(data, contentType)

	photoId, err := a.bot.db.AddVolunteerEventPhoto(r.Context(), eventId, data, contentType, header.Filename, width, height)
	if err != nil {
		a.logger.Logf("error storing cover photo for event %s: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(structs.VolunteerEventPhoto{
		Id:     photoId,
		URL:    publicURL("/volunteer-events/photos/" + photoId),
		Width:  width,
		Height: height,
	})
}

func (a *AppService) AdminDeleteVolunteerEventPhoto(w http.ResponseWriter, r *http.Request) {
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	photoId := strings.TrimSpace(r.PathValue("photo_id"))
	if photoId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.bot.db.DeleteVolunteerEventPhoto(r.Context(), photoId); err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error deleting event photo %s: %s", photoId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminCancelVolunteerEvent cancels an event and tells everyone holding a spot.
//
// The event stays visible in the public list with status "cancelled" rather
// than disappearing — someone who signed up needs to find out — but relying on
// them revisiting the page is weak, so the email is the actual notification.
// The faucet allocation is released, since a cancelled event will not pay out.
func (a *AppService) AdminCancelVolunteerEvent(w http.ResponseWriter, r *http.Request) {
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	eventId := strings.TrimSpace(r.PathValue("id"))
	if eventId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	row, err := a.bot.db.GetPublicVolunteerEvent(r.Context(), eventId)
	if err != nil && err != pgx.ErrNoRows {
		a.logger.Logf("error loading event %s before cancel: %s", eventId, err)
	}

	emails, err := a.bot.db.CancelVolunteerEvent(r.Context(), eventId)
	if err != nil {
		if err == db.ErrEventNotFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error cancelling volunteer event %s: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if row != nil && len(emails) > 0 {
		go a.sendVolunteerEventCancellationEmails(row.Title, row.StartAt, emails)
	}

	w.WriteHeader(http.StatusNoContent)
}

// sendVolunteerEventCancellationEmails notifies signed-up volunteers. Detached
// from the request: the cancellation itself has already committed, and a slow
// mail provider must not hold the admin's response open.
func (a *AppService) sendVolunteerEventCancellationEmails(title string, startAt int64, emails []string) {
	sender := utils.NewEmailSender()
	if sender == nil {
		return
	}

	when := time.Unix(startAt, 0).UTC().Format("Mon 2 Jan 2006")
	details := fmt.Sprintf(`
<p style="font-size:14px; color:#111827;">
  Unfortunately <strong>%s</strong> on %s has been cancelled. You do not need to do anything —
  your spot has been released.
</p>`, utils.EscapeEmailHTML(title), utils.EscapeEmailHTML(when))

	html := utils.BuildStyledEmail("Volunteer event cancelled", "An event you signed up for has been cancelled.", details)

	for _, email := range emails {
		if strings.TrimSpace(email) == "" {
			continue
		}
		if err := sender.SendEmail(email, "Volunteer", "Volunteer event cancelled", html, utils.NotificationFromEmail(), "SFLuv Volunteering"); err != nil {
			a.logger.Logf("error sending cancellation email to %s: %s", email, err)
		}
	}
}

// nextVolunteerOccurrence computes the next start/end for a recurring event.
//
// The arithmetic runs in the event's own timezone, so a series stays anchored
// to the same LOCAL wall-clock time across a DST boundary — "first Thursday at
// 9am" must remain 9am, not drift to 8am or 10am when the offset changes. Doing
// this in UTC is the bug that makes every occurrence after a DST change wrong.
func nextVolunteerOccurrence(row *db.VolunteerEventRow) (int64, int64, bool) {
	location, err := time.LoadLocation(row.Timezone)
	if err != nil || location == nil {
		location = time.UTC
	}

	start := time.Unix(row.StartAt, 0).In(location)
	duration := time.Duration(row.Expiration-row.StartAt) * time.Second

	var next time.Time
	switch row.RecurrenceFrequency {
	case structs.RecurrenceDaily:
		next = start.AddDate(0, 0, 1)
	case structs.RecurrenceWeekly:
		next = start.AddDate(0, 0, 7)
	case structs.RecurrenceMonthly:
		if row.RecurrenceMonthlyMode == structs.MonthlyModeDayOfWeek {
			next = nextMonthlyWeekday(start, row)
		} else {
			next = nextMonthlyDate(start, row)
		}
	default:
		return 0, 0, false
	}

	if next.IsZero() {
		return 0, 0, false
	}
	if row.RecurrenceUntil != nil && next.Unix() > *row.RecurrenceUntil {
		return 0, 0, false
	}

	return next.Unix(), next.Add(duration).Unix(), true
}

// nextMonthlyDate advances one month, keeping the day-of-month. A day that does
// not exist in the target month (the 31st of a 30-day month) clamps to the last
// day rather than silently rolling into the next month, which is what naive
// AddDate(0,1,0) does.
func nextMonthlyDate(start time.Time, row *db.VolunteerEventRow) time.Time {
	day := start.Day()
	if row.RecurrenceDayOfMonth != nil && *row.RecurrenceDayOfMonth > 0 {
		day = *row.RecurrenceDayOfMonth
	}

	firstOfNextMonth := time.Date(start.Year(), start.Month(), 1, start.Hour(), start.Minute(), start.Second(), 0, start.Location()).AddDate(0, 1, 0)
	daysInMonth := time.Date(firstOfNextMonth.Year(), firstOfNextMonth.Month()+1, 0, 0, 0, 0, 0, firstOfNextMonth.Location()).Day()
	if day > daysInMonth {
		day = daysInMonth
	}

	return time.Date(firstOfNextMonth.Year(), firstOfNextMonth.Month(), day, start.Hour(), start.Minute(), start.Second(), 0, start.Location())
}

// nextMonthlyWeekday advances to the same ordinal weekday of the next month —
// "first Thursday", "last Monday". Ordinal -1 means last.
func nextMonthlyWeekday(start time.Time, row *db.VolunteerEventRow) time.Time {
	weekday := start.Weekday()
	if row.RecurrenceWeekday != nil {
		weekday = time.Weekday(*row.RecurrenceWeekday % 7)
	}
	ordinal := 1
	if row.RecurrenceWeekOfMonth != nil {
		ordinal = *row.RecurrenceWeekOfMonth
	}

	firstOfNextMonth := time.Date(start.Year(), start.Month(), 1, start.Hour(), start.Minute(), start.Second(), 0, start.Location()).AddDate(0, 1, 0)

	if ordinal == -1 {
		lastDay := time.Date(firstOfNextMonth.Year(), firstOfNextMonth.Month()+1, 0, start.Hour(), start.Minute(), start.Second(), 0, start.Location())
		offset := (int(lastDay.Weekday()) - int(weekday) + 7) % 7
		return lastDay.AddDate(0, 0, -offset)
	}

	offset := (int(weekday) - int(firstOfNextMonth.Weekday()) + 7) % 7
	candidate := firstOfNextMonth.AddDate(0, 0, offset+(ordinal-1)*7)
	if candidate.Month() != firstOfNextMonth.Month() {
		// e.g. a "fifth Tuesday" in a month that has only four: fall back to
		// the last one rather than skipping the month entirely.
		candidate = candidate.AddDate(0, 0, -7)
	}
	return candidate
}

// GenerateRecurringVolunteerEvents advances every recurring series whose latest
// occurrence has ended, and retries events that were created underfunded.
//
// Called from the maintenance scheduler rather than from a request: a series
// must advance whether or not anyone happens to open a page.
func (a *AppService) GenerateRecurringVolunteerEvents(ctx context.Context) {
	if a.bot == nil || a.bot.db == nil {
		return
	}

	elapsed, err := a.bot.db.GetElapsedRecurringEventsNeedingSuccessor(ctx, 50)
	if err != nil {
		a.logger.Logf("error loading recurring events needing a successor: %s", err)
		return
	}

	for _, row := range elapsed {
		if ctx.Err() != nil {
			return
		}

		startAt, endAt, ok := nextVolunteerOccurrence(row)
		if !ok {
			continue
		}

		required := int64(row.Amount) * int64(row.MaxParticipants)
		funded := a.volunteerFaucetCanCover(ctx, required)

		successorId, err := a.bot.db.CreateRecurringSuccessor(ctx, row, startAt, endAt, funded)
		if err != nil {
			a.logger.Logf("error generating successor for series %v: %s", row.SeriesId, err)
			continue
		}

		if !funded {
			// The occurrence still exists so it stays on the public calendar and
			// the admin can see what needs funding; only the codes are withheld.
			a.logger.Logf("recurring volunteer event %s created awaiting funding (needs %d SFLUV)", successorId, required)
			a.sendVolunteerFundingShortfallEmail(row.Title, required)
		}
	}

	a.retryUnfundedVolunteerEvents(ctx)
}

// retryUnfundedVolunteerEvents mints codes for events that were generated
// underfunded, once the faucet has been topped up. This is what clears the
// admin's funding alert without anyone re-triggering anything.
func (a *AppService) retryUnfundedVolunteerEvents(ctx context.Context) {
	unfunded, err := a.bot.db.GetUnfundedVolunteerEvents(ctx, 50)
	if err != nil {
		a.logger.Logf("error loading unfunded volunteer events: %s", err)
		return
	}

	for _, row := range unfunded {
		if ctx.Err() != nil {
			return
		}

		required := int64(row.Amount) * int64(row.MaxParticipants)
		if !a.volunteerFaucetCanCover(ctx, required) {
			continue
		}

		if err := a.bot.db.MintCodesForEvent(ctx, row.Id, row.MaxParticipants); err != nil {
			a.logger.Logf("error minting codes for previously unfunded event %s: %s", row.Id, err)
			continue
		}
		a.logger.Logf("minted codes for volunteer event %s after faucet top-up", row.Id)
	}
}

func (a *AppService) volunteerFaucetCanCover(ctx context.Context, required int64) bool {
	if required <= 0 {
		return true
	}
	if a.bot == nil {
		return false
	}

	unallocated, err := a.bot.unallocatedBalanceTokens(ctx)
	if err != nil {
		a.logger.Logf("error checking faucet balance for volunteer event funding: %s", err)
		return false
	}
	return unallocated.Int64() >= required
}

func (a *AppService) sendVolunteerFundingShortfallEmail(title string, required int64) {
	adminEmail := strings.TrimSpace(os.Getenv("AFFILIATE_ADMIN_EMAIL"))
	sender := utils.NewEmailSender()
	if adminEmail == "" || sender == nil {
		return
	}

	details := fmt.Sprintf(`
<p style="font-size:14px; color:#111827;">
  The next occurrence of <strong>%s</strong> was created, but its QR codes could not be generated:
  it needs <strong>%d SFLUV</strong> and the faucet does not have that much unallocated.
</p>
<p style="font-size:14px; color:#111827;">
  Top up the faucet and the codes will be generated automatically — no further action needed.
</p>`, utils.EscapeEmailHTML(title), required)

	html := utils.BuildStyledEmail("Volunteer event needs funding", "A recurring volunteer event could not mint its QR codes.", details)
	if err := sender.SendEmail(adminEmail, "Admin", "Volunteer event needs funding", html, utils.NotificationFromEmail(), "SFLuv Volunteering"); err != nil {
		a.logger.Logf("error sending volunteer funding shortfall email: %s", err)
	}
}

// applyVolunteerEventEdit validates a proposed edit and applies it.
//
// The faucet is checked on the DELTA rather than the whole cost: the event's
// existing reservation is already committed, so an edit only needs to cover
// what it adds. Charging the full amount again would refuse edits that free
// funds up.
func (a *AppService) applyVolunteerEventEdit(
	ctx context.Context, eventId string, req *structs.VolunteerEventCreateRequest,
) (int, string) {
	startAt, endAt, until, qrCutoff, errMsg := validateVolunteerEventRequest(req)
	if errMsg == "" {
		errMsg = a.validateVolunteerLocation(ctx, req.LocationId)
	}
	if errMsg != "" {
		return http.StatusBadRequest, errMsg
	}

	current, err := a.bot.db.GetVolunteerEventForManagement(ctx, eventId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return http.StatusNotFound, "event not found"
		}
		a.logger.Logf("error loading event %s for edit: %s", eventId, err)
		return http.StatusInternalServerError, "could not load the event"
	}

	previousCost := int64(current.Amount) * int64(current.MaxParticipants)
	newCost := int64(req.RewardAmountSfluv) * int64(req.MaxParticipants)
	if delta := newCost - previousCost; delta > 0 && !a.volunteerFaucetCanCover(ctx, delta) {
		return http.StatusBadRequest, fmt.Sprintf(
			"not enough unallocated faucet balance: this edit needs %d more SFLUV", delta,
		)
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

	if err := a.bot.db.UpdateVolunteerEvent(ctx, eventId, params); err != nil {
		switch {
		case errors.Is(err, db.ErrEventElapsed):
			return http.StatusConflict, "this occurrence has already ended; past events keep the details they ran with"
		case errors.Is(err, db.ErrEventNotFound):
			return http.StatusNotFound, "event not found"
		}
		a.logger.Logf("error applying edit to event %s: %s", eventId, err)
		return http.StatusInternalServerError, "could not apply the edit"
	}

	return http.StatusOK, ""
}

// AdminUpdateVolunteerEvent applies an edit immediately — admins do not queue
// for their own approval — subject to the same faucet rule as creation.
func (a *AppService) AdminUpdateVolunteerEvent(w http.ResponseWriter, r *http.Request) {
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	eventId := strings.TrimSpace(r.PathValue("id"))
	if eventId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	req, ok := decodeVolunteerEventRequest(w, r)
	if !ok {
		return
	}

	if status, message := a.applyVolunteerEventEdit(r.Context(), eventId, req); status != http.StatusOK {
		w.WriteHeader(status)
		w.Write([]byte(message))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminApproveVolunteerEventEdit applies an affiliate's parked edit. Approval is
// where the faucet is checked, exactly as it is for a new event.
func (a *AppService) AdminApproveVolunteerEventEdit(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil || a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	eventId := strings.TrimSpace(r.PathValue("id"))
	pending, err := a.bot.db.GetPendingEventEditRequest(r.Context(), eventId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("no edit is awaiting approval for this event"))
			return
		}
		a.logger.Logf("error loading edit request for %s: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.VolunteerEventCreateRequest
	if err := json.Unmarshal([]byte(pending.Payload), &req); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("the stored edit could not be read"))
		return
	}

	if status, message := a.applyVolunteerEventEdit(r.Context(), eventId, &req); status != http.StatusOK {
		w.WriteHeader(status)
		w.Write([]byte(message))
		return
	}

	if err := a.bot.db.ResolveEventEditRequest(r.Context(), eventId, *userDid, true, ""); err != nil {
		a.logger.Logf("error marking edit request approved for %s: %s", eventId, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) AdminRejectVolunteerEventEdit(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil || a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	eventId := strings.TrimSpace(r.PathValue("id"))

	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	var decision volunteerEventReviewRequest
	if len(body) > 0 {
		_ = json.Unmarshal(body, &decision)
	}

	if err := a.bot.db.ResolveEventEditRequest(r.Context(), eventId, *userDid, false, decision.Reason); err != nil {
		a.logger.Logf("error rejecting edit request for %s: %s", eventId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// decodeVolunteerEventRequest reads a create/edit payload.
func decodeVolunteerEventRequest(w http.ResponseWriter, r *http.Request) (*structs.VolunteerEventCreateRequest, bool) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}

	var req structs.VolunteerEventCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("could not read event"))
		return nil, false
	}
	return &req, true
}

// shortCreatorName renders "Ada L." from a stored contact name.
func shortCreatorName(fullName string) string {
	parts := strings.Fields(strings.TrimSpace(fullName))
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		last := parts[len(parts)-1]
		return fmt.Sprintf("%s %s.", parts[0], strings.ToUpper(last[:1]))
	}
}

// resolveEventCreators maps each event to its creator's display form.
//
// The email is attached only when the short name is ambiguous within that
// event's organization — two "Ada L."s on the same roster. Ambiguity is
// computed against the whole membership rather than just the people who have
// created events, because a second Ada L. makes the short form ambiguous
// whether or not she has ever made one.
func (a *AppService) resolveEventCreators(ctx context.Context, rows []*db.VolunteerEventRow) map[string]*structs.VolunteerEventCreator {
	creators := map[string]*structs.VolunteerEventCreator{}
	if len(rows) == 0 {
		return creators
	}

	orgIds := []int64{}
	orglessOwners := []string{}
	seenOrg := map[int64]struct{}{}
	for _, row := range rows {
		if row.OrganizationId != nil {
			if _, ok := seenOrg[*row.OrganizationId]; !ok {
				seenOrg[*row.OrganizationId] = struct{}{}
				orgIds = append(orgIds, *row.OrganizationId)
			}
			continue
		}
		if strings.TrimSpace(row.Owner) != "" {
			orglessOwners = append(orglessOwners, row.Owner)
		}
	}

	// Per-org rosters, plus which short names collide inside each org.
	byOrgUser := map[int64]map[string]db.OrganizationMemberIdentity{}
	ambiguous := map[int64]map[string]bool{}
	if members, err := a.db.GetOrganizationMemberIdentities(ctx, orgIds); err != nil {
		a.logger.Logf("error loading organization members for event creators: %s", err)
	} else {
		counts := map[int64]map[string]int{}
		for _, member := range members {
			if byOrgUser[member.OrganizationId] == nil {
				byOrgUser[member.OrganizationId] = map[string]db.OrganizationMemberIdentity{}
				counts[member.OrganizationId] = map[string]int{}
				ambiguous[member.OrganizationId] = map[string]bool{}
			}
			byOrgUser[member.OrganizationId][member.UserId] = member
			if short := shortCreatorName(member.Name); short != "" {
				counts[member.OrganizationId][short]++
			}
		}
		for orgId, byName := range counts {
			for name, count := range byName {
				if count > 1 {
					ambiguous[orgId][name] = true
				}
			}
		}
	}

	orgless, err := a.db.GetUserIdentities(ctx, orglessOwners)
	if err != nil {
		a.logger.Logf("error loading user identities for event creators: %s", err)
	}

	for _, row := range rows {
		var identity db.OrganizationMemberIdentity
		var found bool

		if row.OrganizationId != nil {
			identity, found = byOrgUser[*row.OrganizationId][row.Owner]
		} else {
			identity, found = orgless[row.Owner]
		}
		if !found {
			continue
		}

		name := shortCreatorName(identity.Name)
		if name == "" {
			// No stored name to shorten; the address is all we have.
			name = identity.Email
		}
		if name == "" {
			continue
		}

		creator := &structs.VolunteerEventCreator{Name: name}
		if row.OrganizationId != nil && ambiguous[*row.OrganizationId][name] {
			creator.Email = identity.Email
		}
		creators[row.Id] = creator
	}

	return creators
}
