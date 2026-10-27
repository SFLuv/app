package handlers

import (
	"context"
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
	"github.com/SFLuv/app/backend/v4s"
	"github.com/jackc/pgx/v5"
)

// publicURL turns a root-relative API path into an absolute URL. Image fields
// are emitted as absolute URLs (both clients asked for URLs rather than inline
// base64), falling back to root-relative paths when the origin is not
// configured — a relative URL still resolves for same-origin callers.
func publicURL(path string) string {
	return utils.PublicBackendURL(path)
}

// sfluvOrganizerLogoURL is the mark shown for SFLuv-run events, which have no
// organization of their own. It is the same asset the app serves as its site
// icon and every transactional email already embeds, so the brand is identical
// across surfaces. Derived from APP_BASE_URL so a dev boot points at itself.
// isHouseOrganization reports whether an organization is SFLuv's own, rather
// than a partner.
//
// Staff belong to an "SFLuv" organization so they can use the affiliate tools,
// which means an event they file through the affiliate route carries an
// organization_id like any other — and the paired branding then credits the
// event to "SFLuv and SFLuv". The organization is real and the membership is
// deliberate; what is wrong is treating it as a third party.
//
// This used to match the normalized name, with a note that a column would be
// the durable fix. Migration 1.65 added `organizations.is_house` and backfilled
// it from that same name match, so the answer is unchanged and a rename can no
// longer move an event's attribution.
func isHouseOrganization(org *structs.Organization) bool {
	if org == nil {
		return false
	}
	return org.IsHouse
}

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
	// v4sDeadline bounds how long one request may spend waiting on partner
	// sites in total. A cold fetch is roughly a second; a listing with several
	// unrelated partners could otherwise stack those up one per event. Past the
	// deadline the remaining events use whatever is cached and the refresh
	// happens behind the response, so the page stays fast and the counts catch
	// up on the next view.
	v4sDeadline time.Time
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
		Visibility:        row.Visibility,
		CreatedAt:         rfc3339(row.CreatedAt),
		UpdatedAt:         rfc3339(row.UpdatedAt),
	}

	// Availability is knowable for internal signups, and for external ones only
	// where the partner publishes it — applyV4SAvailability fills those in
	// below from a Volunteers for Salesforce listing. For every other external
	// event the organizer owns the list and we must not imply a spot count.
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
	// The house organization keeps the plain SFLuv organizer seeded above: an
	// event staff filed through the affiliate tools is still ours, and pairing
	// the mark with itself reads as a mistake on a printed card.
	if row.OrganizationId != nil && !isHouseOrganization(ctx.organizations[*row.OrganizationId]) {
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

	// Live availability from a partner's V4S site, when their signup link is one.
	s.applyV4SAvailability(event, row, ctx)

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
	ctx.v4sDeadline = time.Now().Add(v4sRequestBudget)
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
	// Where the SFLuv facet sits in the slice, so house-organization rows fold
	// into it instead of adding a second "SFLuv" entry to the filter list. The
	// rows arrive grouped by organization_id, and events with no organization
	// and events filed by staff through the affiliate tools are the same
	// organizer as far as anybody reading the list is concerned.
	sfluvFacet := -1
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
			// Same rule as the payload, so the filter list and the events it
			// filters agree about who the organizer is.
			if !isHouseOrganization(org) {
				facet.Type = structs.OrganizerTypeAffiliate
				facet.OrganizationId = row.OrganizationId
				facet.Name = org.Name
				if org.Logo != nil && strings.TrimSpace(*org.Logo) != "" {
					logoURL := publicURL(fmt.Sprintf("/organizers/%d/logo", org.Id))
					facet.LogoURL = &logoURL
				}
			}
		}
		if facet.Type == structs.OrganizerTypeSFLuv {
			if sfluvFacet >= 0 {
				facets[sfluvFacet].EventCount += facet.EventCount
				continue
			}
			sfluvFacet = len(facets)
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

// ── Partner availability (Volunteers for Salesforce) ─────────────────────────

// v4sRequestBudget is how long one request may spend in total waiting on
// partner sites before it stops waiting and serves what it has.
const v4sRequestBudget = 2500 * time.Millisecond

// applyV4SAvailability fills in live spot counts for an event whose external
// signup link points at a partner's Volunteers for Salesforce job.
//
// Until now an external event could say nothing about availability — the
// comment on buildSignupInfo called it unknowable, because the partner owns the
// list. For V4S partners it is knowable: their public job listing states, per
// shift, how many places are confirmed and how many remain. Reading it lets a
// volunteer see "3 spots left" on sfluv.org instead of finding out only after
// clicking through.
//
// The read is pull-based, not polled. Nothing fetches on a timer; a request for
// an event triggers at most one fetch, and only when the cached snapshot has
// aged past a minute. A stale snapshot is served while the refresh runs behind
// the request, so a partner's slow site never slows our page.
//
// Anything that is not a V4S link, or any shift we cannot match to this
// occurrence, leaves the event exactly as it was.
func (s *BotService) applyV4SAvailability(
	event *structs.VolunteerEvent,
	row *db.VolunteerEventRow,
	reqCtx *volunteerEventContext,
) {
	// Off unless the integrations file turned it on. A nil client is the
	// disabled state, so nothing below runs — no parsing, no requests.
	if s.v4s == nil || event == nil || row == nil || reqCtx == nil {
		return
	}
	if row.SignupMode != structs.SignupModeExternal || row.SignupURL == "" {
		return
	}

	// Whose event this is decides whether we may read it at all. A V4S link on
	// an organization with no integration is left alone: being able to parse a
	// link is not a reason to go and fetch somebody's website.
	if row.OrganizationId == nil {
		return
	}
	org := reqCtx.organizations[*row.OrganizationId]
	if org == nil {
		return
	}
	integration, ok := s.v4sConfig.For(org.Name, org.IsHouse)
	if !ok {
		return
	}

	source, parsed := v4s.ParseSource(row.SignupURL)
	if !parsed {
		return
	}
	// The link must point at the site this organization was configured with.
	// Pairing the two means a mistyped or hostile link on an otherwise-allowed
	// organization's event resolves to nothing rather than to another host.
	if !integration.AllowsSite(source.Site) {
		return
	}

	// Within the request's budget we may wait for a cold fetch; past it we take
	// only what is already cached, so one slow partner cannot hold up a page.
	var snapshot v4s.Snapshot
	var err error
	if reqCtx != nil && !reqCtx.v4sDeadline.IsZero() && time.Now().After(reqCtx.v4sDeadline) {
		snapshot, _ = s.v4s.Cached(source)
	} else {
		fetchCtx, cancel := context.WithTimeout(context.Background(), v4sRequestBudget)
		snapshot, err = s.v4s.GetWithTTL(fetchCtx, source, integration.TTL())
		cancel()
	}
	if err != nil && len(snapshot.Shifts) == 0 {
		// No numbers is the correct outcome here, not zero: a partner site that
		// is down must not render as "no spots left".
		return
	}

	// Match on the organization's local date. V4S anchors a recurring shift to
	// local time, so the same morning shift is an hour apart in UTC either side
	// of a daylight-saving change and a UTC-date match would miss half the year.
	loc := time.UTC
	if row.Timezone != "" {
		if parsed, lerr := time.LoadLocation(row.Timezone); lerr == nil {
			loc = parsed
		}
	}
	start := time.Unix(row.StartAt, 0)

	// Matches on local date, and on time of day only when the day holds more
	// than one shift — a job with a morning and an afternoon sitting. An
	// occurrence that cannot be told apart from its neighbour is left without
	// availability rather than given someone else's numbers.
	shift, found := v4s.MatchOccurrence(snapshot.Shifts, start, loc)
	if !found {
		return
	}

	confirmed := shift.Confirmed
	remaining := shift.Available
	event.SignupCount = &confirmed
	event.SpotsRemaining = &remaining
	// The partner's capacity, not ours. For an external event our own
	// max_participants describes nothing — we do not hold the list — so the
	// honest total to show beside "spots left" is the one their page states.
	if capacity := shift.Capacity(); capacity > 0 {
		event.MaxParticipants = capacity
	}

	// Deep link this occurrence to its own shift rather than the job, so a
	// volunteer lands on the day they were looking at. This is also what makes
	// a recurring event work: our series maps to their job, our occurrence to
	// the shift on that date, with no need for the partner to expose their
	// recurrence schedule.
	if shift.Id != "" {
		// In the organization's own time zone: dtMonthFilter names a local day.
		deep := v4s.ShiftSignupURL(source.Site, source.JobId, shift.Id, shift.StartUTC.In(loc))
		event.Signup.URL = &deep
	}
	if shift.Full() {
		event.Signup.Open = false
		reason := structs.SignupClosedFull
		event.Signup.ClosedReason = &reason
	}
}
