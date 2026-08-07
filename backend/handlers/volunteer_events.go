package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

// publicBackendBase is the externally reachable origin of this backend. Image
// fields are emitted as absolute URLs (both clients asked for URLs rather than
// inline base64), falling back to root-relative paths when the origin is not
// configured — a relative URL still resolves for same-origin callers.
func publicBackendBase() string {
	for _, key := range []string{"PUBLIC_BACKEND_URL", "NEXT_PUBLIC_BACKEND_URL", "MCP_PUBLIC_BASE_URL"} {
		if value := strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/"); value != "" {
			return value
		}
	}
	return ""
}

func publicURL(path string) string {
	return publicBackendBase() + path
}

// sfluvOrganizerLogoURL is the mark shown for SFLuv-run events, which have no
// organization of their own. It is the same asset the app serves as its site
// icon and every transactional email already embeds, so the brand is identical
// across surfaces. Derived from APP_BASE_URL so a dev boot points at itself.
func sfluvOrganizerLogoURL() string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")
	if base == "" {
		base = "https://app.sfluv.org"
	}
	return base + "/icon.png"
}

// icalWeekdays maps Go's Sunday=0 weekday numbering to the two-letter codes the
// contract uses.
var icalWeekdays = [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

var weekdayNames = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

var weekOrdinalNames = map[int]string{
	1: "First", 2: "Second", 3: "Third", 4: "Fourth", 5: "Fifth", -1: "Last",
}

func ordinalSuffix(day int) string {
	if day%100 >= 11 && day%100 <= 13 {
		return "th"
	}
	switch day % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// slugifyTitle builds the SEO slug the webpage uses in /volunteers/{slug}-{id}.
// Deliberately non-unique and unconstrained: the id in the URL is authoritative,
// so an admin editing a title can never orphan a link.
func slugifyTitle(title string) string {
	var builder strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-")
	}
	return slug
}

func rfc3339(unix int64) string {
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

func rfc3339Ptr(unix *int64) *string {
	if unix == nil || *unix == 0 {
		return nil
	}
	value := rfc3339(*unix)
	return &value
}

// buildRecurrenceSummary renders the human-readable recurrence string
// server-side so the web, mobile, and admin clients cannot each invent a
// different phrasing of the same rule.
func buildRecurrenceSummary(row *db.VolunteerEventRow, start time.Time) string {
	switch row.RecurrenceFrequency {
	case structs.RecurrenceDaily:
		return "Every day"
	case structs.RecurrenceWeekly:
		return fmt.Sprintf("Every %s", weekdayNames[int(start.Weekday())])
	case structs.RecurrenceMonthly:
		if row.RecurrenceMonthlyMode == structs.MonthlyModeDayOfWeek {
			weekday := int(start.Weekday())
			if row.RecurrenceWeekday != nil {
				weekday = *row.RecurrenceWeekday
			}
			ordinal := 1
			if row.RecurrenceWeekOfMonth != nil {
				ordinal = *row.RecurrenceWeekOfMonth
			}
			name, ok := weekOrdinalNames[ordinal]
			if !ok {
				name = "First"
			}
			return fmt.Sprintf("%s %s of every month", name, weekdayNames[weekday%7])
		}
		day := start.Day()
		if row.RecurrenceDayOfMonth != nil {
			day = *row.RecurrenceDayOfMonth
		}
		return fmt.Sprintf("The %d%s of every month", day, ordinalSuffix(day))
	}
	return ""
}

func buildRecurrence(row *db.VolunteerEventRow) *structs.VolunteerEventRecurrence {
	if row.RecurrenceFrequency == "" || row.RecurrenceFrequency == structs.RecurrenceNone {
		return nil
	}

	// Recurrence is expressed in the event's own timezone so that "first
	// Thursday, 9am" stays 9am local across daylight-saving changes.
	location, err := time.LoadLocation(row.Timezone)
	if err != nil || location == nil {
		location = time.UTC
	}
	start := time.Unix(row.StartAt, 0).In(location)

	interval := row.RecurrenceInterval
	if interval <= 0 {
		interval = 1
	}

	recurrence := &structs.VolunteerEventRecurrence{
		Frequency: row.RecurrenceFrequency,
		Interval:  interval,
		Until:     rfc3339Ptr(row.RecurrenceUntil),
		Summary:   buildRecurrenceSummary(row, start),
	}

	switch row.RecurrenceFrequency {
	case structs.RecurrenceWeekly:
		recurrence.Weekdays = []string{icalWeekdays[int(start.Weekday())]}
	case structs.RecurrenceMonthly:
		recurrence.MonthlyMode = row.RecurrenceMonthlyMode
		if row.RecurrenceMonthlyMode == structs.MonthlyModeDayOfWeek {
			weekday := int(start.Weekday())
			if row.RecurrenceWeekday != nil {
				weekday = *row.RecurrenceWeekday
			}
			code := icalWeekdays[weekday%7]
			recurrence.Weekday = &code
			recurrence.WeekOfMonth = row.RecurrenceWeekOfMonth
		} else {
			day := start.Day()
			if row.RecurrenceDayOfMonth != nil {
				day = *row.RecurrenceDayOfMonth
			}
			recurrence.DayOfMonth = &day
		}
	}

	return recurrence
}

// occurrenceStatus is the public lifecycle: where the event is in real time.
// Distinct from review_status, which is the moderation lifecycle and never
// reaches public callers.
func occurrenceStatus(row *db.VolunteerEventRow, now int64) string {
	if row.ReviewStatus == structs.EventReviewCancelled || row.CancelledAt != nil {
		return structs.EventStatusCancelled
	}
	if row.Expiration != 0 && now > row.Expiration {
		return structs.EventStatusEnded
	}
	if now >= row.StartAt {
		return structs.EventStatusLive
	}
	return structs.EventStatusScheduled
}

func buildSignupInfo(row *db.VolunteerEventRow, status string) structs.VolunteerEventSignupInfo {
	info := structs.VolunteerEventSignupInfo{Mode: row.SignupMode}
	if row.SignupMode == structs.SignupModeExternal && row.SignupURL != "" {
		url := row.SignupURL
		info.URL = &url
	}

	if row.SignupMode == structs.SignupModeNone {
		return info
	}

	closed := func(reason string) structs.VolunteerEventSignupInfo {
		info.Open = false
		info.ClosedReason = &reason
		return info
	}

	switch status {
	case structs.EventStatusCancelled:
		return closed(structs.SignupClosedCancelled)
	case structs.EventStatusEnded:
		return closed(structs.SignupClosedEnded)
	}

	if row.SignupMode == structs.SignupModeInternal &&
		row.MaxParticipants > 0 && row.SignupCount >= row.MaxParticipants {
		return closed(structs.SignupClosedFull)
	}

	info.Open = true
	return info
}

type volunteerEventContext struct {
	organizations map[int64]*structs.Organization
	locations     map[int64]*structs.VolunteerEventLocation
	photos        map[string][]db.VolunteerEventPhotoRow
	signups       map[string]db.VolunteerViewerSignup
	redeemed      map[string]bool
	authenticated bool
}

func (s *BotService) mapVolunteerEvent(row *db.VolunteerEventRow, ctx *volunteerEventContext) *structs.VolunteerEvent {
	now := time.Now().Unix()
	status := occurrenceStatus(row, now)

	slug := row.Slug
	if slug == "" {
		slug = slugifyTitle(row.Title)
	}

	event := &structs.VolunteerEvent{
		Id:                row.Id,
		SeriesId:          row.SeriesId,
		Slug:              slug,
		Title:             row.Title,
		Description:       row.Description,
		CoverPhotos:       []structs.VolunteerEventPhoto{},
		StartAt:           rfc3339(row.StartAt),
		EndAt:             rfc3339(row.Expiration),
		Timezone:          row.Timezone,
		Recurrence:        buildRecurrence(row),
		MaxParticipants:   row.MaxParticipants,
		RewardAmountSfluv: row.Amount,
		Signup:            buildSignupInfo(row, status),
		Status:            status,
		CreatedAt:         rfc3339(row.CreatedAt),
		UpdatedAt:         rfc3339(row.UpdatedAt),
	}

	// Availability is only knowable for internal signups; for external events
	// the organizer owns the list and we must not imply a spot count.
	if row.SignupMode == structs.SignupModeInternal {
		count := row.SignupCount
		remaining := row.MaxParticipants - count
		if remaining < 0 {
			remaining = 0
		}
		event.SignupCount = &count
		event.SpotsRemaining = &remaining
	}

	sfluvLogo := sfluvOrganizerLogoURL()
	event.Organizer = structs.VolunteerEventOrganizer{
		Type:    structs.OrganizerTypeSFLuv,
		Name:    "SFLuv",
		LogoURL: &sfluvLogo,
	}
	if row.OrganizationId != nil {
		event.Organizer.Type = structs.OrganizerTypeAffiliate
		event.Organizer.OrganizationId = row.OrganizationId
		// The SFLuv logo is the intended fallback for a partner that has not
		// uploaded one, but the SFLuv *name* is not: this event is theirs, not
		// ours. The organization can be missing here when its row is gone or
		// when the batch load failed, and leaving the seeded name in place
		// credited every one of a partner's events to SFLuv.
		event.Organizer.Name = "Partner organization"
		if org, ok := ctx.organizations[*row.OrganizationId]; ok && org != nil {
			event.Organizer.Name = org.Name
			if org.Logo != nil && strings.TrimSpace(*org.Logo) != "" {
				logoURL := publicURL(fmt.Sprintf("/organizers/%d/logo", org.Id))
				event.Organizer.LogoURL = &logoURL
			}
		}
	}

	for _, photo := range ctx.photos[row.Id] {
		event.CoverPhotos = append(event.CoverPhotos, structs.VolunteerEventPhoto{
			Id:       photo.Id,
			URL:      publicURL("/volunteer-events/photos/" + photo.Id),
			Width:    photo.Width,
			Height:   photo.Height,
			Position: photo.Position,
		})
	}

	if row.LocationId != nil {
		if location, ok := ctx.locations[*row.LocationId]; ok {
			event.Location = location
		}
	}

	// The viewer block is present whenever the caller is authenticated,
	// regardless of signup mode, so both clients apply one rule.
	if ctx.authenticated {
		viewer := &structs.VolunteerEventViewer{Redeemed: ctx.redeemed[row.Id]}
		if signup, ok := ctx.signups[row.Id]; ok {
			viewer.SignedUp = true
			signupId := signup.SignupId
			viewer.SignupId = &signupId
		}
		event.Viewer = viewer
	}

	return event
}

// buildVolunteerEventContext resolves everything the rows reference in batch:
// organizations and locations from the app database, photos and viewer state
// from the bot database. Events and organizations live in different databases,
// so this stitching cannot be a SQL join.
func (s *BotService) buildVolunteerEventContext(r *http.Request, rows []*db.VolunteerEventRow) *volunteerEventContext {
	ctx := &volunteerEventContext{
		organizations: map[int64]*structs.Organization{},
		locations:     map[int64]*structs.VolunteerEventLocation{},
		photos:        map[string][]db.VolunteerEventPhotoRow{},
		signups:       map[string]db.VolunteerViewerSignup{},
		redeemed:      map[string]bool{},
	}
	if len(rows) == 0 {
		return ctx
	}

	eventIds := make([]string, 0, len(rows))
	orgIds := []int64{}
	locationIds := []int64{}
	seenOrg := map[int64]struct{}{}
	seenLocation := map[int64]struct{}{}

	for _, row := range rows {
		eventIds = append(eventIds, row.Id)
		if row.OrganizationId != nil {
			if _, ok := seenOrg[*row.OrganizationId]; !ok {
				seenOrg[*row.OrganizationId] = struct{}{}
				orgIds = append(orgIds, *row.OrganizationId)
			}
		}
		if row.LocationId != nil {
			if _, ok := seenLocation[*row.LocationId]; !ok {
				seenLocation[*row.LocationId] = struct{}{}
				locationIds = append(locationIds, *row.LocationId)
			}
		}
	}

	if photos, err := s.db.GetVolunteerEventPhotos(r.Context(), eventIds); err == nil {
		ctx.photos = photos
	} else {
		fmt.Printf("error loading volunteer event photos: %s\n", err)
	}

	if s.appDb != nil {
		if orgs, err := s.appDb.GetOrganizationsByIds(r.Context(), orgIds); err == nil {
			ctx.organizations = orgs
		} else {
			fmt.Printf("error loading volunteer event organizations: %s\n", err)
		}

		if locations, err := s.appDb.GetVolunteerLocationsByIds(r.Context(), locationIds); err == nil {
			ctx.locations = locations
		} else {
			fmt.Printf("error loading volunteer event locations: %s\n", err)
		}
	}

	if userDid := utils.GetDid(r); userDid != nil {
		ctx.authenticated = true
		if signups, err := s.db.GetViewerSignups(r.Context(), *userDid, eventIds); err == nil {
			ctx.signups = signups
		} else {
			fmt.Printf("error loading viewer signups: %s\n", err)
		}

		if s.appDb != nil {
			if owned, err := s.appDb.GetOwnedWalletAddressSet(r.Context(), *userDid); err == nil && len(owned) > 0 {
				addresses := make([]string, 0, len(owned))
				for address := range owned {
					addresses = append(addresses, address)
				}
				if redeemed, err := s.db.GetViewerRedeemedEvents(r.Context(), addresses, eventIds); err == nil {
					ctx.redeemed = redeemed
				}
			}
		}
	}

	return ctx
}

func parseVolunteerEventsFilter(r *http.Request) *structs.VolunteerEventsFilter {
	params := r.URL.Query()
	page, count := parsePageAndCount(params, 20, 50)

	filter := &structs.VolunteerEventsFilter{
		Search:      strings.TrimSpace(params.Get("search")),
		When:        strings.ToLower(strings.TrimSpace(params.Get("when"))),
		OpenSignups: params.Get("open_signups") == "true",
		Sort:        strings.ToLower(strings.TrimSpace(params.Get("sort"))),
		Page:        page,
		Count:       count,
	}

	// organizer accepts "sfluv", "affiliate", or "org:<id>".
	organizer := strings.TrimSpace(params.Get("organizer"))
	switch {
	case organizer == structs.OrganizerTypeSFLuv:
		filter.OrganizerType = structs.OrganizerTypeSFLuv
	case organizer == structs.OrganizerTypeAffiliate:
		filter.OrganizerType = structs.OrganizerTypeAffiliate
	case strings.HasPrefix(organizer, "org:"):
		if id, err := strconv.ParseInt(strings.TrimPrefix(organizer, "org:"), 10, 64); err == nil && id > 0 {
			filter.OrganizationId = &id
		}
	}
	if raw := strings.TrimSpace(params.Get("organizer_id")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			filter.OrganizationId = &id
		}
	}

	if from, ok := parseTimeParam(params.Get("from")); ok {
		filter.From = &from
	}
	if to, ok := parseTimeParam(params.Get("to")); ok {
		filter.To = &to
	}

	return filter
}

// parseTimeParam accepts RFC3339 (the contract's wire format) and bare unix
// seconds, so a client that sends either is not silently ignored.
func parseTimeParam(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.Unix(), true
	}
	if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return parsed, true
	}
	return 0, false
}

// setVolunteerCacheHeaders makes anonymous reads cacheable by the webpage's ISR
// layer and any CDN, while personalized (authenticated) responses are never
// stored — the viewer block would otherwise leak between users.
func setVolunteerCacheHeaders(w http.ResponseWriter, authenticated bool) {
	if authenticated {
		w.Header().Set("Cache-Control", "private, no-store")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
}

func (s *BotService) GetVolunteerEvents(w http.ResponseWriter, r *http.Request) {
	filter := parseVolunteerEventsFilter(r)

	rows, total, err := s.db.GetPublicVolunteerEvents(r.Context(), filter)
	if err != nil {
		fmt.Printf("error listing volunteer events: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	eventCtx := s.buildVolunteerEventContext(r, rows)
	events := make([]*structs.VolunteerEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, s.mapVolunteerEvent(row, eventCtx))
	}

	response := structs.VolunteerEventsResponse{
		Events:  events,
		Page:    filter.Page,
		Count:   filter.Count,
		Total:   total,
		HasMore: (filter.Page+1)*filter.Count < total,
	}

	setVolunteerCacheHeaders(w, eventCtx.authenticated)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (s *BotService) GetVolunteerEvent(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	row, err := s.db.GetPublicVolunteerEvent(r.Context(), id)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Printf("error getting volunteer event %s: %s\n", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	eventCtx := s.buildVolunteerEventContext(r, []*db.VolunteerEventRow{row})

	setVolunteerCacheHeaders(w, eventCtx.authenticated)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(s.mapVolunteerEvent(row, eventCtx))
}

func (s *BotService) GetVolunteerEventOrganizers(w http.ResponseWriter, r *http.Request) {
	facetRows, err := s.db.GetVolunteerOrganizerFacets(r.Context())
	if err != nil {
		fmt.Printf("error getting volunteer organizer facets: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	orgIds := []int64{}
	for _, facet := range facetRows {
		if facet.OrganizationId != nil {
			orgIds = append(orgIds, *facet.OrganizationId)
		}
	}

	organizations := map[int64]*structs.Organization{}
	if s.appDb != nil && len(orgIds) > 0 {
		if loaded, err := s.appDb.GetOrganizationsByIds(r.Context(), orgIds); err == nil {
			organizations = loaded
		}
	}

	facets := make([]structs.VolunteerEventOrganizerFacet, 0, len(facetRows))
	for _, row := range facetRows {
		sfluvLogo := sfluvOrganizerLogoURL()
		facet := structs.VolunteerEventOrganizerFacet{
			Type:       structs.OrganizerTypeSFLuv,
			Name:       "SFLuv",
			LogoURL:    &sfluvLogo,
			EventCount: row.EventCount,
		}
		if row.OrganizationId != nil {
			org, ok := organizations[*row.OrganizationId]
			if !ok || org == nil {
				continue
			}
			facet.Type = structs.OrganizerTypeAffiliate
			facet.OrganizationId = row.OrganizationId
			facet.Name = org.Name
			if org.Logo != nil && strings.TrimSpace(*org.Logo) != "" {
				logoURL := publicURL(fmt.Sprintf("/organizers/%d/logo", org.Id))
				facet.LogoURL = &logoURL
			}
		}
		facets = append(facets, facet)
	}

	setVolunteerCacheHeaders(w, false)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(facets)
}

func (s *BotService) GetVolunteerEventPhoto(w http.ResponseWriter, r *http.Request) {
	photoId := strings.TrimSpace(r.PathValue("photo_id"))
	if photoId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	photo, err := s.db.GetVolunteerEventPhotoData(r.Context(), photoId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Printf("error getting volunteer event photo %s: %s\n", photoId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	contentType := photo.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Photo ids are content-addressed and immutable, so this is safe to cache
	// aggressively.
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(photo.Data)
}

// GetOrganizerLogo serves an organization's logo as real image bytes. Logos are
// stored as base64 data URLs in a TEXT column; decoding them here is what lets
// both clients consume a plain URL instead of inflating every list payload with
// inline base64.
func (s *BotService) GetOrganizerLogo(w http.ResponseWriter, r *http.Request) {
	orgId, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || orgId <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if s.appDb == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Only organizations with a published event are part of the public portal.
	// Serving any org's logo by id leaked the existence and branding of orgs
	// that have never had an event approved.
	if published, err := s.db.OrganizationHasPublishedEvents(r.Context(), orgId); err != nil || !published {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	logo, err := s.appDb.GetOrganizationLogo(r.Context(), orgId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Printf("error getting organization logo %d: %s\n", orgId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data, contentType, ok := decodeDataURL(logo)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// decodeDataURL turns a stored "data:image/png;base64,..." value into bytes and
// a content type. Returns false for anything that is not a base64 data URL.
func decodeDataURL(value string) ([]byte, string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "data:") {
		return nil, "", false
	}

	comma := strings.Index(value, ",")
	if comma < 0 {
		return nil, "", false
	}

	meta := value[5:comma]
	if !strings.Contains(meta, "base64") {
		return nil, "", false
	}

	contentType, _, _ := strings.Cut(meta, ";")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	data, err := base64.StdEncoding.DecodeString(value[comma+1:])
	if err != nil {
		return nil, "", false
	}

	return data, contentType, true
}
