package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
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

// GetVolunteerEventPhotoData serves a cover photo, but ONLY for an event the
// public list would show.
//
// Selecting on the photo id alone made a photo attached to a pending or
// rejected event publicly fetchable by anyone holding the id — an
// unauthenticated read of content that has not been published. The id is an
// unguessable UUID, so this was low severity, but the join costs nothing and
// removes the class entirely.
func (s *BotDB) GetVolunteerEventPhotoData(ctx context.Context, photoId string) (*StoredPhoto, error) {
	photo := &StoredPhoto{}
	err := s.db.QueryRow(ctx, `
		SELECT p.photo_data, p.content_type
		FROM event_photos p
		JOIN events e ON e.id = p.event_id
		WHERE p.id = $1
			AND e.is_volunteer = TRUE
			AND e.review_status IN ('approved', 'cancelled');
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

// CreateVolunteerEventParams carries fully-resolved values: the handler has
// already converted wall-clock times to UTC instants and validated the rule.
type CreateVolunteerEventParams struct {
	Title       string
	Description string
	Slug        string
	Timezone    string

	StartAt int64
	EndAt   int64
	// QRExpiresAt closes the redemption window; defaults to 24h after EndAt.
	QRExpiresAt int64

	MaxParticipants int
	RewardAmount    uint64

	SignupMode string
	SignupURL  string
	LocationId *int64

	RecurrenceFrequency   string
	RecurrenceMonthlyMode string
	RecurrenceDayOfMonth  *int
	RecurrenceWeekOfMonth *int
	RecurrenceWeekday     *int
	RecurrenceUntil       *int64

	Owner          string
	OrganizationId *int64
	ReviewStatus   string

	// MintCodes generates the QR codes and reserves the faucet allocation.
	// Admin-created events are approved on creation so this is true; an
	// affiliate request stays pending and mints nothing until approval.
	MintCodes bool

	// StagedPhotoIds are cover photos already uploaded by Owner and waiting for
	// an event. They are attached inside the creation transaction, so an event
	// is never published missing the artwork it was created with.
	StagedPhotoIds []string
}

// CreateVolunteerEvent inserts the event and, when approved, mints one QR code
// per participant and records the faucet allocation — all in one transaction,
// so an event can never exist with a partial set of codes or an allocation that
// does not match the codes actually minted.
func (s *BotDB) CreateVolunteerEvent(ctx context.Context, p *CreateVolunteerEventParams) (string, error) {
	id := uuid.NewString()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())

	seriesId := any(nil)
	if p.RecurrenceFrequency != "" && p.RecurrenceFrequency != structs.RecurrenceNone {
		// A recurring event is the first instance of its own series.
		seriesId = id
	}

	// QR codes are downloadable immediately but only spendable from 24h before
	// the event starts; the redemption gate reads this column.
	qrLiveAt := p.StartAt - 86400

	fundingStatus := structs.FundingStatusFunded
	if !p.MintCodes {
		fundingStatus = structs.FundingStatusAwaiting
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO events (
			id, title, description, amount, start_at, expiration, owner, organization_id,
			is_volunteer, slug, timezone, max_participants, signup_mode, signup_url,
			review_status, qr_live_at, qr_expires_at, codes_generated, funding_status, location_id,
			recurrence_frequency, recurrence_interval, recurrence_monthly_mode,
			recurrence_day_of_month, recurrence_week_of_month, recurrence_weekday,
			recurrence_until, series_id, series_index, requested_by, approved_by, approved_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			TRUE, $9, $10, $11, $12, $13,
			$14, $15, $28, $16, $17, $18,
			$19, 1, $20,
			$21, $22, $23,
			$24, $25, 0, $7, $26, $27
		);
	`,
		id, p.Title, p.Description, p.RewardAmount, p.StartAt, p.EndAt, p.Owner, p.OrganizationId,
		p.Slug, p.Timezone, p.MaxParticipants, p.SignupMode, p.SignupURL,
		p.ReviewStatus, qrLiveAt, p.MintCodes, fundingStatus, p.LocationId,
		p.RecurrenceFrequency, p.RecurrenceMonthlyMode,
		p.RecurrenceDayOfMonth, p.RecurrenceWeekOfMonth, p.RecurrenceWeekday,
		p.RecurrenceUntil, seriesId,
		approvedBy(p),
		approvedAtOrNil(p),
		nullableUnix(p.QRExpiresAt),
	)
	if err != nil {
		return "", fmt.Errorf("error inserting volunteer event: %s", err)
	}

	// Inside the transaction: a photo that cannot be claimed rolls the event
	// back with it, so creation is all-or-nothing rather than leaving a
	// published event with artwork missing.
	if err := attachStagedPhotos(ctx, tx, id, p.Owner, p.StagedPhotoIds); err != nil {
		return "", err
	}

	if p.MintCodes {
		for range p.MaxParticipants {
			if _, err := tx.Exec(ctx, `
				INSERT INTO codes (id, event, code_number)
				VALUES ($1, $2, (SELECT COALESCE(MAX(code_number), 0) + 1 FROM codes WHERE event = $2));
			`, uuid.NewString(), id); err != nil {
				return "", fmt.Errorf("error minting volunteer event codes: %s", err)
			}
		}

		cycle := allocationCycleForRecurrence(p.RecurrenceFrequency)
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_allocations (id, event_id, series_id, organization_id, cycle, amount)
			VALUES ($1, $2, $3, $4, $5, $6);
		`, uuid.NewString(), id, seriesId, p.OrganizationId, cycle,
			int64(p.RewardAmount)*int64(p.MaxParticipants)); err != nil {
			return "", fmt.Errorf("error recording volunteer event allocation: %s", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("error committing volunteer event: %s", err)
	}

	return id, nil
}

// nullableUnix keeps a zero timestamp as SQL NULL, so "unset" stays
// distinguishable from "the epoch".
func nullableUnix(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

// approvedBy is the approver to record at insert time.
//
// Empty rather than NULL for anything not already approved. events.approved_by
// is TEXT NOT NULL DEFAULT ” — the same "unset means empty string" convention
// as requested_by beside it — and a column default only applies when the column
// is left out of the INSERT. This one is always listed, so passing NULL here
// violated the constraint outright and every affiliate request (which is
// created pending, by definition unapproved) failed with SQLSTATE 23502.
// Admin-created events set a real approver, which is why the bug only ever
// showed on the affiliate path.
func approvedBy(p *CreateVolunteerEventParams) string {
	if p.ReviewStatus == structs.EventReviewApproved {
		return p.Owner
	}
	return ""
}

// approvedAtOrNil, unlike approvedBy, really does write NULL: approved_at is a
// nullable BIGINT, and "not approved yet" has no timestamp to invent.
func approvedAtOrNil(p *CreateVolunteerEventParams) any {
	if p.ReviewStatus == structs.EventReviewApproved {
		return time.Now().UTC().Unix()
	}
	return nil
}

// allocationCycleForRecurrence maps an event's recurrence onto the faucet
// allocation cycle it reserves against: a one-off reserves once, a recurring
// event reserves one cycle's worth for as long as it is active.
func allocationCycleForRecurrence(frequency string) string {
	switch frequency {
	case structs.RecurrenceDaily:
		return structs.AllocationCycleDaily
	case structs.RecurrenceWeekly:
		return structs.AllocationCycleWeekly
	case structs.RecurrenceMonthly:
		return structs.AllocationCycleMonthly
	}
	return structs.AllocationCycleOneTime
}

// managementReviewOrder sorts a management list by how much attention a row
// needs: anything awaiting approval first, then live events, with rejected and
// cancelled events last. Applied in SQL rather than in the client because these
// lists are paginated — sorting a page after it arrives would only order the
// rows that happened to land on it.
const managementReviewOrder = `
		CASE e.review_status
			WHEN 'pending' THEN 0
			WHEN 'rejected' THEN 2
			WHEN 'cancelled' THEN 2
			ELSE 1
		END ASC,
		e.start_at DESC,
		e.id ASC
`

// GetAdminVolunteerEvents lists volunteer events for the admin panel across all
// review states, unlike the public list which is approved-only.
func (s *BotDB) GetAdminVolunteerEvents(ctx context.Context, f *structs.VolunteerEventsFilter, reviewStatus string) ([]*VolunteerEventRow, int, error) {
	if f.Count <= 0 {
		f.Count = 20
	}
	if f.Count > 100 {
		f.Count = 100
	}
	if f.Page < 0 {
		f.Page = 0
	}

	args := []any{}
	where := []string{"e.is_volunteer = TRUE"}

	if search := strings.TrimSpace(f.Search); search != "" {
		args = append(args, "%"+search+"%")
		where = append(where, fmt.Sprintf("(COALESCE(e.title,'') ILIKE $%[1]d OR COALESCE(e.description,'') ILIKE $%[1]d)", len(args)))
	}
	if reviewStatus != "" && reviewStatus != "all" {
		args = append(args, reviewStatus)
		where = append(where, fmt.Sprintf("e.review_status = $%d", len(args)))
	}
	if f.OrganizationId != nil {
		args = append(args, *f.OrganizationId)
		where = append(where, fmt.Sprintf("e.organization_id = $%d", len(args)))
	}

	args = append(args, f.Count, f.Page*f.Count)
	query := fmt.Sprintf(`
		SELECT %s, COUNT(*) OVER()
		FROM events e
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d;
	`, volunteerEventColumns, strings.Join(where, " AND "), managementReviewOrder, len(args)-1, len(args))

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("error querying admin volunteer events: %s", err)
	}
	defer rows.Close()

	events := []*VolunteerEventRow{}
	total := 0
	for rows.Next() {
		event := &VolunteerEventRow{}
		rowTotal := 0
		if err := scanVolunteerEventRow(rows, event, &rowTotal); err != nil {
			return nil, 0, fmt.Errorf("error scanning admin volunteer event: %s", err)
		}
		total = rowTotal
		events = append(events, event)
	}

	return events, total, rows.Err()
}

// AddVolunteerEventPhoto stores a cover photo and returns its id.
func (s *BotDB) AddVolunteerEventPhoto(ctx context.Context, eventId string, data []byte, contentType string, fileName string, width int, height int) (string, error) {
	id := uuid.NewString()

	var position int
	if err := s.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(position), -1) + 1 FROM event_photos WHERE event_id = $1;
	`, eventId).Scan(&position); err != nil {
		return "", fmt.Errorf("error computing photo position: %s", err)
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO event_photos (id, event_id, position, file_name, content_type, photo_data, size_bytes, width, height)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
	`, id, eventId, position, fileName, contentType, data, len(data), width, height); err != nil {
		return "", fmt.Errorf("error storing event photo: %s", err)
	}

	return id, nil
}

// StageVolunteerEventPhoto stores a cover photo that has no event yet.
//
// Uploading only becomes possible once an event exists is what forced the old
// create-then-attach dance; a staged photo can be uploaded the moment it is
// chosen and claimed later. `staged_by` is who may claim it.
func (s *BotDB) StageVolunteerEventPhoto(ctx context.Context, stagedBy string, data []byte, contentType string, fileName string, width int, height int) (string, error) {
	id := uuid.NewString()

	if _, err := s.db.Exec(ctx, `
		INSERT INTO event_photos (id, event_id, staged_by, position, file_name, content_type, photo_data, size_bytes, width, height)
		VALUES ($1, NULL, $2, 0, $3, $4, $5, $6, $7, $8);
	`, id, stagedBy, fileName, contentType, data, len(data), width, height); err != nil {
		return "", fmt.Errorf("error staging event photo: %s", err)
	}

	return id, nil
}

// attachStagedPhotos claims staged photos for a freshly created event.
//
// Runs inside the creation transaction and is strict on purpose: an id that is
// missing, already attached, or staged by somebody else fails the whole
// creation. Quietly dropping one would publish an event missing a photo its
// author believed they had added, which is precisely the outcome the staging
// flow exists to prevent. Order is preserved so the first photo chosen stays
// the cover.
func attachStagedPhotos(ctx context.Context, tx pgx.Tx, eventId string, owner string, photoIds []string) error {
	for position, photoId := range photoIds {
		trimmed := strings.TrimSpace(photoId)
		if trimmed == "" {
			return fmt.Errorf("error attaching event photo: empty photo id")
		}

		tag, err := tx.Exec(ctx, `
			UPDATE event_photos
			SET event_id = $1, position = $2, staged_by = ''
			WHERE id = $3 AND event_id IS NULL AND staged_by = $4;
		`, eventId, position, trimmed, owner)
		if err != nil {
			return fmt.Errorf("error attaching event photo %s: %s", trimmed, err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("error attaching event photo %s: it is no longer available", trimmed)
		}
	}

	return nil
}

// DeleteStagedVolunteerEventPhoto discards a staged photo, for a client that
// removes one before submitting. Scoped to the uploader so an id cannot be used
// to delete somebody else's.
func (s *BotDB) DeleteStagedVolunteerEventPhoto(ctx context.Context, photoId string, stagedBy string) error {
	if _, err := s.db.Exec(ctx, `
		DELETE FROM event_photos WHERE id = $1 AND event_id IS NULL AND staged_by = $2;
	`, photoId, stagedBy); err != nil {
		return fmt.Errorf("error deleting staged event photo: %s", err)
	}
	return nil
}

// ExpireStagedVolunteerEventPhotos clears staged photos nobody ever attached —
// a form opened, files chosen, and the tab closed. Without this they would
// accumulate as bytes in Postgres that no event will ever reference.
func (s *BotDB) ExpireStagedVolunteerEventPhotos(ctx context.Context, olderThanUnix int64) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM event_photos WHERE event_id IS NULL AND created_at < $1;
	`, olderThanUnix)
	if err != nil {
		return 0, fmt.Errorf("error expiring staged event photos: %s", err)
	}
	return tag.RowsAffected(), nil
}

func (s *BotDB) DeleteVolunteerEventPhoto(ctx context.Context, photoId string) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM event_photos WHERE id = $1;`, photoId); err != nil {
		return fmt.Errorf("error deleting event photo: %s", err)
	}
	return nil
}

var (
	ErrEventNotFound     = fmt.Errorf("event not found")
	ErrSignupNotInternal = fmt.Errorf("not_internal")
	ErrSignupEventClosed = fmt.Errorf("closed")
	ErrSignupEventFull   = fmt.Errorf("full")
	ErrAlreadySignedUp   = fmt.Errorf("already_signed_up")
)

type VolunteerSignupParams struct {
	EventId   string
	UserId    *string
	Email     string
	FirstName string
	LastName  string
	Source    string
	// RequireConfirmation holds the spot but marks it unconfirmed until the
	// emailed link is followed. True for anonymous portal signups.
	RequireConfirmation bool
}

type VolunteerSignupResult struct {
	SignupId       string
	SpotsRemaining int
	CancelToken    string
	ConfirmToken   string
}

// CreateVolunteerSignup claims a spot.
//
// Capacity is enforced inside the transaction with the event row locked, so two
// people racing for the last spot cannot both win — counting before the insert
// without the lock would let both pass.
func (s *BotDB) CreateVolunteerSignup(ctx context.Context, p *VolunteerSignupParams) (*VolunteerSignupResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())

	var signupMode, reviewStatus string
	var maxParticipants int
	var expiration int64
	err = tx.QueryRow(ctx, `
		SELECT signup_mode, review_status, max_participants, expiration
		FROM events
		WHERE id = $1 AND is_volunteer = TRUE
		FOR UPDATE;
	`, p.EventId).Scan(&signupMode, &reviewStatus, &maxParticipants, &expiration)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrEventNotFound
		}
		return nil, err
	}

	if signupMode != structs.SignupModeInternal {
		return nil, ErrSignupNotInternal
	}
	if reviewStatus != structs.EventReviewApproved {
		return nil, ErrSignupEventClosed
	}
	if expiration != 0 && time.Now().Unix() > expiration {
		return nil, ErrSignupEventClosed
	}

	var taken int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM event_signups WHERE event_id = $1 AND cancelled_at IS NULL;
	`, p.EventId).Scan(&taken); err != nil {
		return nil, err
	}
	if maxParticipants > 0 && taken >= maxParticipants {
		return nil, ErrSignupEventFull
	}

	id := uuid.NewString()
	cancelToken := uuid.NewString()

	// Portal signups are held but not counted as confirmed until the emailed
	// link is followed; an authenticated signup is confirmed on the spot because
	// the account already establishes the address.
	confirmToken := ""
	var confirmedAt any
	if p.RequireConfirmation {
		confirmToken = uuid.NewString()
	} else {
		confirmedAt = time.Now().Unix()
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO event_signups (id, event_id, user_id, email, first_name, last_name, source, cancel_token, confirm_token, confirmed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
	`, id, p.EventId, p.UserId, strings.TrimSpace(p.Email), p.FirstName, p.LastName, p.Source, cancelToken, confirmToken, confirmedAt)
	if err != nil {
		// The partial unique indexes make a duplicate live signup a constraint
		// violation, so it needs no separate pre-check.
		if strings.Contains(err.Error(), "event_signups_event_email_live_idx") ||
			strings.Contains(err.Error(), "event_signups_event_user_live_idx") {
			return nil, ErrAlreadySignedUp
		}
		return nil, fmt.Errorf("error inserting signup: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	remaining := maxParticipants - (taken + 1)
	if remaining < 0 {
		remaining = 0
	}
	return &VolunteerSignupResult{
		SignupId: id, SpotsRemaining: remaining, CancelToken: cancelToken, ConfirmToken: confirmToken,
	}, nil
}

// CancelVolunteerSignup releases a spot. Cancelling sets cancelled_at rather
// than deleting, which keeps the row for reporting while the partial unique
// index lets the same person sign up again later.
func (s *BotDB) CancelVolunteerSignup(ctx context.Context, eventId string, userId string) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE event_signups
		SET cancelled_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE event_id = $1 AND user_id = $2 AND cancelled_at IS NULL;
	`, eventId, userId)
	if err != nil {
		return false, fmt.Errorf("error cancelling signup: %s", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *BotDB) GetMyVolunteerEventIds(ctx context.Context, userId string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.id
		FROM event_signups s
		JOIN events e ON e.id = s.event_id
		WHERE s.user_id = $1 AND s.cancelled_at IS NULL
		ORDER BY e.start_at DESC;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error listing signed-up events: %s", err)
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *BotDB) GetVolunteerEventsByIds(ctx context.Context, ids []string) ([]*VolunteerEventRow, error) {
	if len(ids) == 0 {
		return []*VolunteerEventRow{}, nil
	}

	query := fmt.Sprintf(`
		SELECT %s FROM events e
		WHERE e.id = ANY($1) AND e.is_volunteer = TRUE
		ORDER BY e.start_at DESC;
	`, volunteerEventColumns)

	rows, err := s.db.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("error loading events by id: %s", err)
	}
	defer rows.Close()

	events := []*VolunteerEventRow{}
	for rows.Next() {
		event := &VolunteerEventRow{}
		if err := scanVolunteerEventRow(rows, event, nil); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// CountRecentSignupAttempts backs the rate limiter on the unauthenticated
// signup endpoint.
func (s *BotDB) CountRecentSignupAttempts(ctx context.Context, ip string, email string, since int64) (int, int, error) {
	var byIP, byEmail int
	if err := s.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE ip = $1 AND $1 <> ''),
			COUNT(*) FILTER (WHERE LOWER(email) = LOWER($2) AND $2 <> '')
		FROM event_signup_attempts
		WHERE created_at >= $3;
	`, ip, email, since).Scan(&byIP, &byEmail); err != nil {
		return 0, 0, fmt.Errorf("error counting signup attempts: %s", err)
	}
	return byIP, byEmail, nil
}

func (s *BotDB) RecordSignupAttempt(ctx context.Context, ip string, email string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO event_signup_attempts (ip, email) VALUES ($1, $2);
	`, ip, strings.TrimSpace(email))
	return err
}

// CancelVolunteerEvent marks an event cancelled and returns the emails of
// everyone holding a spot, so they can be told rather than finding out by
// revisiting the page.
//
// The status change and the allocation release are ONE transaction: both live
// in this database, so there is no reason to leave a window where an event is
// cancelled but its faucet allocation is still reserved. The RowsAffected guard
// also makes it idempotent — a second cancel is a no-op, not a second release.
func (s *BotDB) CancelVolunteerEvent(ctx context.Context, eventId string) ([]string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())

	tag, err := tx.Exec(ctx, `
		UPDATE events
		SET review_status = 'cancelled',
			cancelled_at = EXTRACT(EPOCH FROM NOW())::BIGINT,
			updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE id = $1 AND is_volunteer = TRUE AND review_status <> 'cancelled';
	`, eventId)
	if err != nil {
		return nil, fmt.Errorf("error cancelling event: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrEventNotFound
	}

	// Release the faucet allocation: a cancelled event will not pay out.
	if _, err := tx.Exec(ctx, `
		UPDATE event_allocations
		SET active = FALSE, released_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE event_id = $1 AND active = TRUE;
	`, eventId); err != nil {
		return nil, fmt.Errorf("error releasing event allocation: %s", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT email FROM event_signups WHERE event_id = $1 AND cancelled_at IS NULL;
	`, eventId)
	if err != nil {
		return nil, err
	}

	emails := []string{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			rows.Close()
			return nil, err
		}
		emails = append(emails, email)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("error committing event cancellation: %s", err)
	}

	return emails, nil
}

// GetElapsedRecurringEventsNeedingSuccessor finds recurring events whose end
// time has passed and that have no successor yet.
//
// The next occurrence is generated when the previous one completes, mirroring
// the improver workflow series pattern, rather than pre-generating a calendar
// of instances that would each need cancelling if the series changed.
func (s *BotDB) GetElapsedRecurringEventsNeedingSuccessor(ctx context.Context, limit int) ([]*VolunteerEventRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT %s FROM events e
		WHERE e.is_volunteer = TRUE
			AND e.review_status = 'approved'
			AND e.recurrence_frequency <> 'none'
			AND e.series_id IS NOT NULL
			AND e.expiration > 0
			AND e.expiration < EXTRACT(EPOCH FROM NOW())
			AND (e.recurrence_until IS NULL OR e.recurrence_until > EXTRACT(EPOCH FROM NOW()))
			AND NOT EXISTS (
				SELECT 1 FROM events successor
				WHERE successor.series_id = e.series_id
				AND successor.start_at > e.start_at
			)
		ORDER BY e.expiration ASC
		LIMIT $1;
	`, volunteerEventColumns)

	rows, err := s.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("error querying recurring events needing a successor: %s", err)
	}
	defer rows.Close()

	events := []*VolunteerEventRow{}
	for rows.Next() {
		event := &VolunteerEventRow{}
		if err := scanVolunteerEventRow(rows, event, nil); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// CreateRecurringSuccessor clones an elapsed occurrence forward.
//
// When the faucet cannot cover it the event is still created, with no codes and
// funding_status 'awaiting_funding' — the event existing is what lets an admin
// see the shortfall and top up, whereas skipping generation would silently drop
// an occurrence from a published series.
func (s *BotDB) CreateRecurringSuccessor(ctx context.Context, previous *VolunteerEventRow, startAt int64, endAt int64, funded bool) (string, error) {
	id := uuid.NewString()
	qrLiveAt := startAt - 86400

	fundingStatus := structs.FundingStatusFunded
	if !funded {
		fundingStatus = structs.FundingStatusAwaiting
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())

	_, err = tx.Exec(ctx, `
		INSERT INTO events (
			id, title, description, amount, start_at, expiration, owner, organization_id,
			is_volunteer, slug, timezone, max_participants, signup_mode, signup_url,
			review_status, qr_live_at, qr_expires_at, codes_generated, funding_status, location_id,
			recurrence_frequency, recurrence_interval, recurrence_monthly_mode,
			recurrence_day_of_month, recurrence_week_of_month, recurrence_weekday,
			recurrence_until, series_id, series_index
		)
		SELECT
			$1, title, description, amount, $2, $3, owner, organization_id,
			TRUE, slug, timezone, max_participants, signup_mode, signup_url,
			'approved', $4, $5, $6, location_id,
			recurrence_frequency, recurrence_interval, recurrence_monthly_mode,
			recurrence_day_of_month, recurrence_week_of_month, recurrence_weekday,
			recurrence_until, series_id, series_index + 1
		FROM events WHERE id = $7;
	`, id, startAt, endAt, qrLiveAt, funded, fundingStatus, previous.Id)
	if err != nil {
		return "", fmt.Errorf("error creating recurring successor: %s", err)
	}

	if funded {
		for range previous.MaxParticipants {
			if _, err := tx.Exec(ctx, `INSERT INTO codes (id, event, code_number) VALUES ($1, $2, (SELECT COALESCE(MAX(code_number), 0) + 1 FROM codes WHERE event = $2));`, uuid.NewString(), id); err != nil {
				return "", fmt.Errorf("error minting successor codes: %s", err)
			}
		}
	}

	// Cover photos are per-occurrence rows, so without this every generated
	// instance after the first would publish with no images at all.
	if _, err := tx.Exec(ctx, `
		INSERT INTO event_photos (id, event_id, position, file_name, content_type, photo_data, size_bytes, width, height)
		SELECT gen_random_uuid()::text, $1, position, file_name, content_type, photo_data, size_bytes, width, height
		FROM event_photos WHERE event_id = $2;
	`, id, previous.Id); err != nil {
		return "", fmt.Errorf("error cloning successor cover photos: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// MintCodesForEvent generates the QR codes for an event that was created
// underfunded, once the faucet can cover it.
func (s *BotDB) MintCodesForEvent(ctx context.Context, eventId string, count int) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	var alreadyGenerated bool
	if err := tx.QueryRow(ctx, `
		SELECT codes_generated FROM events WHERE id = $1 FOR UPDATE;
	`, eventId).Scan(&alreadyGenerated); err != nil {
		return err
	}
	if alreadyGenerated {
		return nil
	}

	for range count {
		if _, err := tx.Exec(ctx, `INSERT INTO codes (id, event, code_number) VALUES ($1, $2, (SELECT COALESCE(MAX(code_number), 0) + 1 FROM codes WHERE event = $2));`, uuid.NewString(), eventId); err != nil {
			return fmt.Errorf("error minting codes: %s", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE events SET codes_generated = TRUE, funding_status = 'funded', updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE id = $1;
	`, eventId); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetUnfundedVolunteerEvents lists events waiting on a faucet top-up, for the
// admin alert and the retry sweep.
func (s *BotDB) GetUnfundedVolunteerEvents(ctx context.Context, limit int) ([]*VolunteerEventRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT %s FROM events e
		WHERE e.is_volunteer = TRUE
			AND e.funding_status = 'awaiting_funding'
			AND e.review_status = 'approved'
			AND e.expiration > EXTRACT(EPOCH FROM NOW())
		ORDER BY e.start_at ASC
		LIMIT $1;
	`, volunteerEventColumns)

	rows, err := s.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("error querying unfunded volunteer events: %s", err)
	}
	defer rows.Close()

	events := []*VolunteerEventRow{}
	for rows.Next() {
		event := &VolunteerEventRow{}
		if err := scanVolunteerEventRow(rows, event, nil); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// ApproveVolunteerEvent flips a pending request to approved and, when funded,
// mints its codes and reserves the allocation — the same transaction the admin
// create path uses, so an approved affiliate event is indistinguishable from an
// admin-created one.
func (s *BotDB) ApproveVolunteerEvent(ctx context.Context, eventId string, approverId string, funded bool) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	var reviewStatus string
	var maxParticipants int
	var amount uint64
	var recurrence string
	var seriesId *string
	var organizationId *int64
	if err := tx.QueryRow(ctx, `
		SELECT review_status, max_participants, amount, recurrence_frequency, series_id, NULLIF(organization_id, 0)
		FROM events
		WHERE id = $1 AND is_volunteer = TRUE
		FOR UPDATE;
	`, eventId).Scan(&reviewStatus, &maxParticipants, &amount, &recurrence, &seriesId, &organizationId); err != nil {
		if err == pgx.ErrNoRows {
			return ErrEventNotFound
		}
		return err
	}
	if reviewStatus != structs.EventReviewPending {
		return fmt.Errorf("event is not pending approval")
	}

	fundingStatus := structs.FundingStatusFunded
	if !funded {
		fundingStatus = structs.FundingStatusAwaiting
	}

	if _, err := tx.Exec(ctx, `
		UPDATE events
		SET review_status = 'approved',
			approved_by = $2,
			approved_at = EXTRACT(EPOCH FROM NOW())::BIGINT,
			codes_generated = $3,
			funding_status = $4,
			updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE id = $1;
	`, eventId, approverId, funded, fundingStatus); err != nil {
		return fmt.Errorf("error approving event: %s", err)
	}

	if funded {
		for range maxParticipants {
			if _, err := tx.Exec(ctx, `INSERT INTO codes (id, event, code_number) VALUES ($1, $2, (SELECT COALESCE(MAX(code_number), 0) + 1 FROM codes WHERE event = $2));`, uuid.NewString(), eventId); err != nil {
				return fmt.Errorf("error minting codes on approval: %s", err)
			}
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO event_allocations (id, event_id, series_id, organization_id, cycle, amount)
			VALUES ($1, $2, $3, $4, $5, $6);
		`, uuid.NewString(), eventId, seriesId, organizationId,
			allocationCycleForRecurrence(recurrence), int64(amount)*int64(maxParticipants)); err != nil {
			return fmt.Errorf("error recording allocation on approval: %s", err)
		}
	}

	return tx.Commit(ctx)
}

func (s *BotDB) RejectVolunteerEvent(ctx context.Context, eventId string, approverId string, reason string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE events
		SET review_status = 'rejected',
			rejected_reason = $3,
			approved_by = $2,
			updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE id = $1 AND is_volunteer = TRUE AND review_status = 'pending';
	`, eventId, approverId, strings.TrimSpace(reason))
	if err != nil {
		return fmt.Errorf("error rejecting event: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}

// GetOrganizationVolunteerEvents lists an organization's events across all
// review states, so an affiliate can see their own pending requests.
func (s *BotDB) GetOrganizationVolunteerEvents(ctx context.Context, organizationId int64, f *structs.VolunteerEventsFilter) ([]*VolunteerEventRow, int, error) {
	if f.Count <= 0 {
		f.Count = 20
	}
	if f.Count > 100 {
		f.Count = 100
	}

	query := fmt.Sprintf(`
		SELECT %s, COUNT(*) OVER()
		FROM events e
		WHERE e.is_volunteer = TRUE AND e.organization_id = $1
		ORDER BY %s
		LIMIT $2 OFFSET $3;
	`, volunteerEventColumns, managementReviewOrder)

	rows, err := s.db.Query(ctx, query, organizationId, f.Count, f.Page*f.Count)
	if err != nil {
		return nil, 0, fmt.Errorf("error querying organization volunteer events: %s", err)
	}
	defer rows.Close()

	events := []*VolunteerEventRow{}
	total := 0
	for rows.Next() {
		event := &VolunteerEventRow{}
		rowTotal := 0
		if err := scanVolunteerEventRow(rows, event, &rowTotal); err != nil {
			return nil, 0, err
		}
		total = rowTotal
		events = append(events, event)
	}
	return events, total, rows.Err()
}

// GetVolunteerEventCodes returns the redemption codes for an event, for the QR
// download. Codes are downloadable as soon as they exist even though they only
// become spendable 24h before the event.
func (s *BotDB) GetVolunteerEventCodes(ctx context.Context, eventId string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id FROM codes WHERE event = $1 ORDER BY id;
	`, eventId)
	if err != nil {
		return nil, fmt.Errorf("error loading event codes: %s", err)
	}
	defer rows.Close()

	codes := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		codes = append(codes, id)
	}
	return codes, rows.Err()
}

// GetVolunteerEventForManagement loads any volunteer event regardless of review
// state, for admin/affiliate surfaces. The public getter deliberately refuses
// non-approved events, so management reads need their own path.
func (s *BotDB) GetVolunteerEventForManagement(ctx context.Context, id string) (*VolunteerEventRow, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM events e WHERE e.id = $1 AND e.is_volunteer = TRUE;
	`, volunteerEventColumns)

	event := &VolunteerEventRow{}
	if err := scanVolunteerEventRow(s.db.QueryRow(ctx, query, id), event, nil); err != nil {
		return nil, err
	}
	return event, nil
}

// UpcomingVolunteerSignup is a live signup for an event starting soon.
type UpcomingVolunteerSignup struct {
	EventId    string
	EventTitle string
	StartAt    int64
	Email      string
	UserId     *string
}

// GetUpcomingVolunteerSignups returns live signups for approved, non-cancelled
// events starting inside the given window.
//
// Reminder eligibility is resolved in two steps because the data spans two
// databases: signups and events live here, while accounts, verified emails and
// reminder preferences live in the app database.
func (s *BotDB) GetUpcomingVolunteerSignups(ctx context.Context, fromUnix int64, toUnix int64) ([]UpcomingVolunteerSignup, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.id, COALESCE(e.title, ''), e.start_at, su.email, su.user_id
		FROM event_signups su
		JOIN events e ON e.id = su.event_id
		WHERE su.cancelled_at IS NULL
			AND e.is_volunteer = TRUE
			AND e.review_status = 'approved'
			AND e.start_at BETWEEN $1 AND $2
		ORDER BY e.start_at ASC;
	`, fromUnix, toUnix)
	if err != nil {
		return nil, fmt.Errorf("error loading upcoming volunteer signups: %s", err)
	}
	defer rows.Close()

	signups := []UpcomingVolunteerSignup{}
	for rows.Next() {
		signup := UpcomingVolunteerSignup{}
		if err := rows.Scan(&signup.EventId, &signup.EventTitle, &signup.StartAt, &signup.Email, &signup.UserId); err != nil {
			return nil, err
		}
		signups = append(signups, signup)
	}
	return signups, rows.Err()
}

// ConfirmVolunteerSignup completes a portal signup via its emailed token.
// Single-use: the token is cleared, so a replayed link is a no-op rather than a
// second confirmation.
func (s *BotDB) ConfirmVolunteerSignup(ctx context.Context, token string) (string, string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", fmt.Errorf("token is required")
	}

	var eventId, email string
	err := s.db.QueryRow(ctx, `
		UPDATE event_signups
		SET confirmed_at = COALESCE(confirmed_at, EXTRACT(EPOCH FROM NOW())::BIGINT),
			confirm_token = ''
		WHERE confirm_token = $1 AND confirm_token <> '' AND cancelled_at IS NULL
		RETURNING event_id, email;
	`, token).Scan(&eventId, &email)
	if err != nil {
		return "", "", err
	}
	return eventId, email, nil
}

// PeekVolunteerSignupByToken is the read half of the confirm flow, so a
// prefetching mail scanner cannot confirm on the user's behalf.
func (s *BotDB) PeekVolunteerSignupByToken(ctx context.Context, token string) (string, string, error) {
	var email, title string
	err := s.db.QueryRow(ctx, `
		SELECT su.email, COALESCE(e.title, '')
		FROM event_signups su
		JOIN events e ON e.id = su.event_id
		WHERE su.confirm_token = $1 AND su.confirm_token <> '';
	`, strings.TrimSpace(token)).Scan(&email, &title)
	if err != nil {
		return "", "", err
	}
	return email, title, nil
}

// ExpireUnconfirmedVolunteerSignups releases spots held by portal signups that
// were never confirmed, so an abandoned or bot-submitted form cannot hold a
// place indefinitely.
func (s *BotDB) ExpireUnconfirmedVolunteerSignups(ctx context.Context, olderThan int64) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE event_signups
		SET cancelled_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE confirmed_at IS NULL
			AND cancelled_at IS NULL
			AND created_at < $1;
	`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("error expiring unconfirmed signups: %s", err)
	}
	return tag.RowsAffected(), nil
}

// EventBlastRecipient is one person to notify. UserId is set when the signup
// resolves to an account, which is what decides push versus email.
type EventBlastRecipient struct {
	Email     string
	FirstName string
	UserId    *string
}

// GetEventBlastRecipients returns everyone holding a confirmed spot. Unconfirmed
// signups are excluded: they have not proven the address, and blasting them
// would turn the event form into an open mail relay.
func (s *BotDB) GetEventBlastRecipients(ctx context.Context, eventId string) ([]EventBlastRecipient, error) {
	rows, err := s.db.Query(ctx, `
		SELECT email, first_name, user_id
		FROM event_signups
		WHERE event_id = $1 AND cancelled_at IS NULL AND confirmed_at IS NOT NULL;
	`, eventId)
	if err != nil {
		return nil, fmt.Errorf("error loading blast recipients: %s", err)
	}
	defer rows.Close()

	recipients := []EventBlastRecipient{}
	for rows.Next() {
		recipient := EventBlastRecipient{}
		if err := rows.Scan(&recipient.Email, &recipient.FirstName, &recipient.UserId); err != nil {
			return nil, err
		}
		recipients = append(recipients, recipient)
	}
	return recipients, rows.Err()
}

func (s *BotDB) RecordEventBlast(ctx context.Context, eventId string, sentBy string, subject string, message string, pushCount int, emailCount int) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO event_blasts (id, event_id, sent_by, subject, message, push_count, email_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7);
	`, uuid.NewString(), eventId, sentBy, subject, message, pushCount, emailCount)
	if err != nil {
		return fmt.Errorf("error recording event blast: %s", err)
	}
	return nil
}

// ErrEventElapsed is returned when an edit targets an occurrence that has
// already finished.
var ErrEventElapsed = errors.New("event has already ended")

// UpdateVolunteerEvent applies an edit to a single occurrence.
//
// Recurrence semantics: this only ever touches the row it is given, and edits
// are refused once an occurrence has ended. Past instances therefore keep the
// title, description, reward and photos they actually ran with — their QR codes
// and redemptions describe that version of the event, so rewriting them would
// falsify history. Future occurrences pick the change up automatically because
// each successor is cloned from the one before it.
func (s *BotDB) UpdateVolunteerEvent(ctx context.Context, eventId string, p *CreateVolunteerEventParams) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	var expiration int64
	var maxParticipants int
	var codesGenerated bool
	if err := tx.QueryRow(ctx, `
		SELECT expiration, max_participants, codes_generated
		FROM events WHERE id = $1 AND is_volunteer = TRUE
		FOR UPDATE;
	`, eventId).Scan(&expiration, &maxParticipants, &codesGenerated); err != nil {
		if err == pgx.ErrNoRows {
			return ErrEventNotFound
		}
		return err
	}
	if expiration > 0 && expiration < time.Now().Unix() {
		return ErrEventElapsed
	}

	if _, err := tx.Exec(ctx, `
		UPDATE events SET
			title = $2,
			description = $3,
			slug = $4,
			amount = $5,
			start_at = $6,
			expiration = $7,
			qr_live_at = $6 - 86400,
			qr_expires_at = $8,
			timezone = $9,
			max_participants = $10,
			signup_mode = $11,
			signup_url = $12,
			location_id = $13,
			recurrence_frequency = $14,
			recurrence_monthly_mode = $15,
			recurrence_day_of_month = $16,
			recurrence_week_of_month = $17,
			recurrence_weekday = $18,
			recurrence_until = $19,
			updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE id = $1;
	`, eventId, p.Title, p.Description, p.Slug, p.RewardAmount, p.StartAt, p.EndAt,
		nullableUnix(p.QRExpiresAt), p.Timezone, p.MaxParticipants, p.SignupMode, p.SignupURL,
		p.LocationId, p.RecurrenceFrequency, p.RecurrenceMonthlyMode,
		p.RecurrenceDayOfMonth, p.RecurrenceWeekOfMonth, p.RecurrenceWeekday, p.RecurrenceUntil,
	); err != nil {
		return fmt.Errorf("error updating volunteer event: %s", err)
	}

	// Raising the participant cap needs more codes. Lowering it deliberately
	// does NOT destroy codes: one may already be printed or in someone's hand,
	// and invalidating it would strand a volunteer at the event.
	if codesGenerated && p.MaxParticipants > maxParticipants {
		for range p.MaxParticipants - maxParticipants {
			if _, err := tx.Exec(ctx, `INSERT INTO codes (id, event, code_number) VALUES ($1, $2, (SELECT COALESCE(MAX(code_number), 0) + 1 FROM codes WHERE event = $2));`, uuid.NewString(), eventId); err != nil {
				return fmt.Errorf("error minting additional codes: %s", err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE event_allocations
		SET amount = $2
		WHERE event_id = $1 AND active = TRUE;
	`, eventId, int64(p.RewardAmount)*int64(p.MaxParticipants)); err != nil {
		return fmt.Errorf("error updating event allocation: %s", err)
	}

	return tx.Commit(ctx)
}

// CreateEventEditRequest parks an affiliate's proposed changes for review.
func (s *BotDB) CreateEventEditRequest(ctx context.Context, eventId string, requestedBy string, payload string) (string, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO event_edit_requests (id, event_id, requested_by, payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (event_id) WHERE status = 'pending'
		DO UPDATE SET payload = EXCLUDED.payload,
			requested_by = EXCLUDED.requested_by,
			created_at = EXTRACT(EPOCH FROM NOW())::BIGINT;
	`, id, eventId, requestedBy, payload)
	if err != nil {
		return "", fmt.Errorf("error recording edit request: %s", err)
	}
	return id, nil
}

type EventEditRequest struct {
	Id          string
	EventId     string
	RequestedBy string
	Payload     string
	CreatedAt   int64
}

func (s *BotDB) GetPendingEventEditRequest(ctx context.Context, eventId string) (*EventEditRequest, error) {
	request := &EventEditRequest{}
	err := s.db.QueryRow(ctx, `
		SELECT id, event_id, requested_by, payload, created_at
		FROM event_edit_requests
		WHERE event_id = $1 AND status = 'pending';
	`, eventId).Scan(&request.Id, &request.EventId, &request.RequestedBy, &request.Payload, &request.CreatedAt)
	if err != nil {
		return nil, err
	}
	return request, nil
}

func (s *BotDB) ResolveEventEditRequest(ctx context.Context, eventId string, decidedBy string, approved bool, reason string) error {
	status := "rejected"
	if approved {
		status = "approved"
	}
	_, err := s.db.Exec(ctx, `
		UPDATE event_edit_requests
		SET status = $2, decided_by = $3, reject_reason = $4,
			decided_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE event_id = $1 AND status = 'pending';
	`, eventId, status, decidedBy, strings.TrimSpace(reason))
	if err != nil {
		return fmt.Errorf("error resolving edit request: %s", err)
	}
	return nil
}

// AddEventBlastImage stores an inline image for an organizer's message.
func (s *BotDB) AddEventBlastImage(ctx context.Context, eventId string, data []byte, contentType string) (string, error) {
	id := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO event_blast_images (id, event_id, content_type, image_data, size_bytes)
		VALUES ($1, $2, $3, $4, $5);
	`, id, eventId, contentType, data, len(data)); err != nil {
		return "", fmt.Errorf("error storing blast image: %s", err)
	}
	return id, nil
}

func (s *BotDB) GetEventBlastImage(ctx context.Context, imageId string) (*StoredPhoto, error) {
	image := &StoredPhoto{}
	if err := s.db.QueryRow(ctx, `
		SELECT image_data, content_type FROM event_blast_images WHERE id = $1;
	`, imageId).Scan(&image.Data, &image.ContentType); err != nil {
		return nil, err
	}
	return image, nil
}

// OrganizationHasPublishedEvents reports whether an organization has any event
// the public portal would show. Used to scope the public organizer-logo
// endpoint, so an org that has never had an event approved is not discoverable.
func (s *BotDB) OrganizationHasPublishedEvents(ctx context.Context, organizationId int64) (bool, error) {
	var exists bool
	if err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM events
			WHERE organization_id = $1
				AND is_volunteer = TRUE
				AND review_status IN ('approved', 'cancelled')
		);
	`, organizationId).Scan(&exists); err != nil {
		return false, fmt.Errorf("error checking published events for organization: %s", err)
	}
	return exists, nil
}

// VolunteerLocationExists reports whether an id refers to a real, active
// volunteer location. Create/edit payloads carry a client-supplied location_id,
// and an unvalidated foreign reference should not be stored just because the
// read path happens to filter it out later.
func (a *AppDB) VolunteerLocationExists(ctx context.Context, locationId int64) (bool, error) {
	var exists bool
	if err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM locations
			WHERE id = $1 AND active = TRUE AND location_kind = 'volunteer'
		);
	`, locationId).Scan(&exists); err != nil {
		return false, fmt.Errorf("error validating volunteer location: %s", err)
	}
	return exists, nil
}

// NumberedCode is a redemption code with the stable label printed on its QR
// sheet.
type NumberedCode struct {
	Id     string
	Number int
}

// GetVolunteerEventCodesWithNumbers returns codes in printed order.
func (s *BotDB) GetVolunteerEventCodesWithNumbers(ctx context.Context, eventId string) ([]NumberedCode, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, COALESCE(code_number, 0)
		FROM codes
		WHERE event = $1
		ORDER BY code_number ASC NULLS LAST, id ASC;
	`, eventId)
	if err != nil {
		return nil, fmt.Errorf("error loading numbered event codes: %s", err)
	}
	defer rows.Close()

	codes := []NumberedCode{}
	for rows.Next() {
		code := NumberedCode{}
		if err := rows.Scan(&code.Id, &code.Number); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}
