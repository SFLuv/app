package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// VolunteerEventRow is the raw events row plus derived counts. Handlers map it
// into structs.VolunteerEvent; keeping the DB shape separate means the wire
// contract can change (RFC3339, computed status, summaries) without reshaping
// queries.
type VolunteerEventRow struct {
	Id             string
	Title          string
	Description    string
	Slug           string
	Amount         uint64
	StartAt        int64
	Expiration     int64
	Timezone       string
	Owner          string
	OrganizationId *int64

	MaxParticipants int
	SignupMode      string
	SignupURL       string
	ReviewStatus    string
	CancelledAt     *int64
	QRLiveAt        *int64
	CodesGenerated  bool
	FundingStatus   string

	// LocationId is a soft reference into the app database's locations table —
	// events live in the bot database, so this cannot be a foreign key and is
	// resolved by the handler.
	LocationId *int64

	RecurrenceFrequency   string
	RecurrenceInterval    int
	RecurrenceMonthlyMode string
	RecurrenceDayOfMonth  *int
	RecurrenceWeekOfMonth *int
	RecurrenceWeekday     *int
	RecurrenceUntil       *int64
	SeriesId              *string

	SignupCount int
	CreatedAt   int64
	UpdatedAt   int64
}

// volunteerEventColumns is shared by the list and detail queries so the two can
// never drift out of sync on column order.
const volunteerEventColumns = `
	e.id,
	COALESCE(e.title, ''),
	COALESCE(e.description, ''),
	e.slug,
	e.amount,
	e.start_at,
	e.expiration,
	e.timezone,
	COALESCE(e.owner, ''),
	NULLIF(e.organization_id, 0),
	e.max_participants,
	e.signup_mode,
	e.signup_url,
	e.review_status,
	e.cancelled_at,
	e.qr_live_at,
	e.codes_generated,
	e.funding_status,
	e.location_id,
	e.recurrence_frequency,
	e.recurrence_interval,
	e.recurrence_monthly_mode,
	e.recurrence_day_of_month,
	e.recurrence_week_of_month,
	e.recurrence_weekday,
	e.recurrence_until,
	e.series_id,
	(
		SELECT COUNT(*) FROM event_signups s
		WHERE s.event_id = e.id AND s.cancelled_at IS NULL
	),
	e.created_at,
	e.updated_at
`

func scanVolunteerEventRow(row pgx.Row, out *VolunteerEventRow, total *int) error {
	targets := []any{
		&out.Id,
		&out.Title,
		&out.Description,
		&out.Slug,
		&out.Amount,
		&out.StartAt,
		&out.Expiration,
		&out.Timezone,
		&out.Owner,
		&out.OrganizationId,
		&out.MaxParticipants,
		&out.SignupMode,
		&out.SignupURL,
		&out.ReviewStatus,
		&out.CancelledAt,
		&out.QRLiveAt,
		&out.CodesGenerated,
		&out.FundingStatus,
		&out.LocationId,
		&out.RecurrenceFrequency,
		&out.RecurrenceInterval,
		&out.RecurrenceMonthlyMode,
		&out.RecurrenceDayOfMonth,
		&out.RecurrenceWeekOfMonth,
		&out.RecurrenceWeekday,
		&out.RecurrenceUntil,
		&out.SeriesId,
		&out.SignupCount,
		&out.CreatedAt,
		&out.UpdatedAt,
	}
	if total != nil {
		targets = append(targets, total)
	}
	return row.Scan(targets...)
}

// GetPublicVolunteerEvents lists approved (and cancelled-but-upcoming)
// volunteer events. Cancelled events stay visible so someone who already signed
// up finds out rather than watching the event silently vanish; they are
// excluded from the open-signups filter and can never be signed up for.
func (s *BotDB) GetPublicVolunteerEvents(ctx context.Context, f *structs.VolunteerEventsFilter) ([]*VolunteerEventRow, int, error) {
	if f.Count <= 0 {
		f.Count = 20
	}
	if f.Count > 50 {
		f.Count = 50
	}
	if f.Page < 0 {
		f.Page = 0
	}

	args := []any{}
	where := []string{
		"e.is_volunteer = TRUE",
		"e.review_status IN ('approved', 'cancelled')",
	}

	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}

	if search := strings.TrimSpace(f.Search); search != "" {
		add("(COALESCE(e.title,'') ILIKE $%[1]d OR COALESCE(e.description,'') ILIKE $%[1]d)", "%"+search+"%")
	}

	switch f.OrganizerType {
	case structs.OrganizerTypeSFLuv:
		where = append(where, "e.organization_id IS NULL")
	case structs.OrganizerTypeAffiliate:
		where = append(where, "e.organization_id IS NOT NULL")
	}
	if f.OrganizationId != nil {
		add("e.organization_id = $%d", *f.OrganizationId)
	}

	switch f.When {
	case "past":
		where = append(where, "e.expiration < EXTRACT(EPOCH FROM NOW())")
	case "all":
		// no time bound
	default: // upcoming
		where = append(where, "e.expiration >= EXTRACT(EPOCH FROM NOW())")
	}

	if f.From != nil {
		add("e.start_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("e.start_at <= $%d", *f.To)
	}

	// "Has open spots" only means something for internal signups, where we hold
	// the records. Cancelled events are never open.
	if f.OpenSignups {
		where = append(where,
			"e.signup_mode = 'internal'",
			"e.review_status <> 'cancelled'",
			`e.max_participants > (
				SELECT COUNT(*) FROM event_signups s
				WHERE s.event_id = e.id AND s.cancelled_at IS NULL
			)`,
		)
	}

	order := "e.start_at ASC, e.id ASC"
	if f.When == "past" {
		order = "e.start_at DESC, e.id ASC"
	}
	if f.Sort == "newest" {
		order = "e.created_at DESC, e.id ASC"
	}

	args = append(args, f.Count, f.Page*f.Count)
	query := fmt.Sprintf(`
		SELECT %s, COUNT(*) OVER()
		FROM events e
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d;
	`, volunteerEventColumns, strings.Join(where, " AND "), order, len(args)-1, len(args))

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("error querying volunteer events: %s", err)
	}
	defer rows.Close()

	events := []*VolunteerEventRow{}
	total := 0
	for rows.Next() {
		event := &VolunteerEventRow{}
		rowTotal := 0
		if err := scanVolunteerEventRow(rows, event, &rowTotal); err != nil {
			return nil, 0, fmt.Errorf("error scanning volunteer event row: %s", err)
		}
		total = rowTotal
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

// GetPublicVolunteerEvent returns a single approved/cancelled volunteer event.
// Unapproved events 404 for every caller, admin included — management reads go
// through the admin endpoints instead.
func (s *BotDB) GetPublicVolunteerEvent(ctx context.Context, id string) (*VolunteerEventRow, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM events e
		WHERE e.id = $1
			AND e.is_volunteer = TRUE
			AND e.review_status IN ('approved', 'cancelled');
	`, volunteerEventColumns)

	event := &VolunteerEventRow{}
	if err := scanVolunteerEventRow(s.db.QueryRow(ctx, query, id), event, nil); err != nil {
		return nil, err
	}
	return event, nil
}

type VolunteerEventPhotoRow struct {
	Id       string
	EventId  string
	Width    int
	Height   int
	Position int
}

// GetVolunteerEventPhotos loads photo metadata (never bytes) for a set of
// events in one round trip, so a list page does not fan out per event.
func (s *BotDB) GetVolunteerEventPhotos(ctx context.Context, eventIds []string) (map[string][]VolunteerEventPhotoRow, error) {
	result := map[string][]VolunteerEventPhotoRow{}
	if len(eventIds) == 0 {
		return result, nil
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, event_id, width, height, position
		FROM event_photos
		WHERE event_id = ANY($1)
		ORDER BY event_id, position ASC, created_at ASC;
	`, eventIds)
	if err != nil {
		return nil, fmt.Errorf("error querying event photos: %s", err)
	}
	defer rows.Close()

	for rows.Next() {
		photo := VolunteerEventPhotoRow{}
		if err := rows.Scan(&photo.Id, &photo.EventId, &photo.Width, &photo.Height, &photo.Position); err != nil {
			return nil, err
		}
		result[photo.EventId] = append(result[photo.EventId], photo)
	}

	return result, rows.Err()
}

type StoredPhoto struct {
	Data        []byte
	ContentType string
}

func (s *BotDB) GetVolunteerEventPhotoData(ctx context.Context, photoId string) (*StoredPhoto, error) {
	photo := &StoredPhoto{}
	err := s.db.QueryRow(ctx, `
		SELECT photo_data, content_type
		FROM event_photos
		WHERE id = $1;
	`, photoId).Scan(&photo.Data, &photo.ContentType)
	if err != nil {
		return nil, err
	}
	return photo, nil
}

type VolunteerViewerSignup struct {
	SignupId string
	EventId  string
}

// GetViewerSignups returns the caller's live signups for the given events, so
// the viewer block can be filled without an N+1.
func (s *BotDB) GetViewerSignups(ctx context.Context, userId string, eventIds []string) (map[string]VolunteerViewerSignup, error) {
	result := map[string]VolunteerViewerSignup{}
	if userId == "" || len(eventIds) == 0 {
		return result, nil
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, event_id
		FROM event_signups
		WHERE user_id = $1 AND event_id = ANY($2) AND cancelled_at IS NULL;
	`, userId, eventIds)
	if err != nil {
		return nil, fmt.Errorf("error querying viewer signups: %s", err)
	}
	defer rows.Close()

	for rows.Next() {
		signup := VolunteerViewerSignup{}
		if err := rows.Scan(&signup.SignupId, &signup.EventId); err != nil {
			return nil, err
		}
		result[signup.EventId] = signup
	}

	return result, rows.Err()
}

// GetViewerRedeemedEvents reports which of the given events the caller's
// wallets have already redeemed a code for.
func (s *BotDB) GetViewerRedeemedEvents(ctx context.Context, addresses []string, eventIds []string) (map[string]bool, error) {
	result := map[string]bool{}
	if len(addresses) == 0 || len(eventIds) == 0 {
		return result, nil
	}

	lowered := make([]string, 0, len(addresses))
	for _, address := range addresses {
		lowered = append(lowered, strings.ToLower(strings.TrimSpace(address)))
	}

	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT event
		FROM redemptions
		WHERE address = ANY($1) AND event = ANY($2);
	`, lowered, eventIds)
	if err != nil {
		return nil, fmt.Errorf("error querying viewer redemptions: %s", err)
	}
	defer rows.Close()

	for rows.Next() {
		var eventId string
		if err := rows.Scan(&eventId); err != nil {
			return nil, err
		}
		result[eventId] = true
	}

	return result, rows.Err()
}

type VolunteerOrganizerFacetRow struct {
	OrganizationId *int64
	EventCount     int
}

// GetVolunteerOrganizerFacets counts currently-listable events per organizer.
// Organization names and logos live in the app database, so they are attached
// in the handler rather than joined here.
func (s *BotDB) GetVolunteerOrganizerFacets(ctx context.Context) ([]VolunteerOrganizerFacetRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT NULLIF(organization_id, 0), COUNT(*)
		FROM events
		WHERE is_volunteer = TRUE
			AND review_status IN ('approved', 'cancelled')
			AND expiration >= EXTRACT(EPOCH FROM NOW())
		GROUP BY NULLIF(organization_id, 0)
		ORDER BY COUNT(*) DESC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying organizer facets: %s", err)
	}
	defer rows.Close()

	facets := []VolunteerOrganizerFacetRow{}
	for rows.Next() {
		facet := VolunteerOrganizerFacetRow{}
		if err := rows.Scan(&facet.OrganizationId, &facet.EventCount); err != nil {
			return nil, err
		}
		facets = append(facets, facet)
	}

	return facets, rows.Err()
}
