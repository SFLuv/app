package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func normalizeLocationPageRequest(r *structs.LocationsPageRequest) (uint, uint) {
	if r == nil {
		return 0, 100
	}

	count := r.Count
	if count == 0 {
		count = 100
	}
	if count > 200 {
		count = 200
	}

	return r.Page, count
}

func (a *AppDB) getLocationHoursByIDs(ctx context.Context, ids []uint64) (map[uint64][]string, map[uint64][]structs.LocationDayHours, error) {
	if len(ids) == 0 {
		return map[uint64][]string{}, map[uint64][]structs.LocationDayHours{}, nil
	}

	idParams := make([]int32, 0, len(ids))
	seen := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		idParams = append(idParams, int32(id))
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			location_id,
			hours,
			weekday,
			is_closed,
			intervals
		FROM
			location_hours
		WHERE
			location_id = ANY($1::int4[])
		AND
			active = TRUE
		ORDER BY
			location_id ASC,
			weekday ASC;
	`, idParams)
	if err != nil {
		return nil, nil, fmt.Errorf("error querying location hours: %w", err)
	}
	defer rows.Close()

	hoursByLocation := make(map[uint64][]string, len(idParams))
	daysByLocation := make(map[uint64][]structs.LocationDayHours, len(idParams))
	for rows.Next() {
		var locationID int32
		var hours string
		var day structs.LocationDayHours
		var intervals []byte
		if err := rows.Scan(&locationID, &hours, &day.Weekday, &day.IsClosed, &intervals); err != nil {
			return nil, nil, fmt.Errorf("error scanning location hours: %w", err)
		}
		if len(intervals) > 0 {
			if err := json.Unmarshal(intervals, &day.Intervals); err != nil {
				return nil, nil, fmt.Errorf("error decoding location hour intervals: %w", err)
			}
		}
		hoursByLocation[uint64(locationID)] = append(hoursByLocation[uint64(locationID)], hours)
		daysByLocation[uint64(locationID)] = append(daysByLocation[uint64(locationID)], day)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating location hours: %w", err)
	}

	return hoursByLocation, daysByLocation, nil
}

func (a *AppDB) GetLocation(ctx context.Context, id uint64) (*structs.PublicLocation, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			l.id,
			COALESCE(l.google_id, ''),
			l.name,
			l.approval,
			`+locationPayToAddressExpr+` AS pay_to_address,
			COALESCE(
				NULLIF(TRIM(l.tipping_wallet_address), ''),
				''
			) AS tip_to_address,
			l.description,
			l.type,
			l.street,
			l.city,
			l.state,
			l.zip,
			l.lat,
			l.lng,
			l.phone,
			l.email,
			l.website,
			l.image_url,
			l.icon_updated_at,
			l.photo_updated_at,
			l.rating,
			l.maps_page
		FROM locations l
		LEFT JOIN users u
			ON u.id = l.owner_id
			AND u.active = TRUE
		WHERE l.id = $1
		AND l.active = TRUE
		AND l.location_kind = 'merchant';
	`, id)

	location := structs.PublicLocation{}
	var payToAddress sql.NullString
	var tipToAddress sql.NullString
	var iconUpdatedAt sql.NullTime
	var photoUpdatedAt sql.NullTime
	err := row.Scan(
		&location.ID,
		&location.GoogleID,
		&location.Name,
		&location.Approval,
		&payToAddress,
		&tipToAddress,
		&location.Description,
		&location.Type,
		&location.Street,
		&location.City,
		&location.State,
		&location.ZIP,
		&location.Lat,
		&location.Lng,
		&location.Phone,
		&location.Email,
		&location.Website,
		&location.ImageURL,
		&iconUpdatedAt,
		&photoUpdatedAt,
		&location.Rating,
		&location.MapsPage,
	)
	if err != nil {
		return nil, err
	}
	if payToAddress.Valid {
		location.PayToAddress = payToAddress.String
	}
	if tipToAddress.Valid {
		location.TipToAddress = tipToAddress.String
	}
	if iconUpdatedAt.Valid {
		location.IconURL = LocationIconURL(location.ID, iconUpdatedAt.Time)
	}
	if photoUpdatedAt.Valid {
		location.PhotoURL = LocationPhotoURL(location.ID, photoUpdatedAt.Time)
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			hours
		FROM location_hours
		WHERE location_id = $1
		AND active = TRUE
		ORDER BY weekday;
	`, id)
	if err != nil {
		return nil, fmt.Errorf("error getting location hours: %s", err)
	}
	defer rows.Close()

	curr_hours := ""
	openingHours := []string{}
	for rows.Next() {
		err = rows.Scan(
			&curr_hours,
		)
		if err != nil {
			return nil, err
		}
		openingHours = append(openingHours, curr_hours)
	}
	location.OpeningHours = openingHours

	return &location, nil
}

func (s *AppDB) GetLocations(ctx context.Context, r *structs.LocationsPageRequest) ([]*structs.PublicLocation, error) {
	page, count := normalizeLocationPageRequest(r)
	offset := page * count

	rows, err := s.db.Query(ctx, `
		SELECT
			l.id,
			COALESCE(l.google_id, ''),
			l.name,
			`+locationPayToAddressExpr+` AS pay_to_address,
			COALESCE(
				NULLIF(TRIM(l.tipping_wallet_address), ''),
				''
			) AS tip_to_address,
			l.description,
			l.type,
			l.street,
			l.city,
			l.state,
			l.zip,
			l.lat,
			l.lng,
			l.phone,
			l.email,
			l.website,
			l.image_url,
			l.icon_updated_at,
			l.photo_updated_at,
			l.rating,
			l.maps_page
		FROM locations l
		LEFT JOIN users u
			ON u.id = l.owner_id
			AND u.active = TRUE
		WHERE l.approval = TRUE
		AND l.active = TRUE
		AND l.location_kind = 'merchant'
		ORDER BY l.id
		LIMIT $1
		OFFSET $2;
	`, count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying for locations: %w", err)
	}
	defer rows.Close()

	locations := []*structs.PublicLocation{}
	locationIDs := []uint64{}

	for rows.Next() {
		location := structs.PublicLocation{}
		var payToAddress sql.NullString
		var tipToAddress sql.NullString
		var iconUpdatedAt sql.NullTime
		var photoUpdatedAt sql.NullTime

		err = rows.Scan(
			&location.ID,
			&location.GoogleID,
			&location.Name,
			&payToAddress,
			&tipToAddress,
			&location.Description,
			&location.Type,
			&location.Street,
			&location.City,
			&location.State,
			&location.ZIP,
			&location.Lat,
			&location.Lng,
			&location.Phone,
			&location.Email,
			&location.Website,
			&location.ImageURL,
			&iconUpdatedAt,
			&photoUpdatedAt,
			&location.Rating,
			&location.MapsPage,
		)

		if err != nil {
			return nil, fmt.Errorf("error scanning location row: %w", err)
		}
		if payToAddress.Valid {
			location.PayToAddress = payToAddress.String
		}
		if tipToAddress.Valid {
			location.TipToAddress = tipToAddress.String
		}
		if iconUpdatedAt.Valid {
			location.IconURL = LocationIconURL(location.ID, iconUpdatedAt.Time)
		}
		if photoUpdatedAt.Valid {
			location.PhotoURL = LocationPhotoURL(location.ID, photoUpdatedAt.Time)
		}
		locations = append(locations, &location)
		locationIDs = append(locationIDs, uint64(location.ID))
	}

	hoursByLocation, daysByLocation, err := s.getLocationHoursByIDs(ctx, locationIDs)
	if err != nil {
		return nil, err
	}

	for _, loc := range locations {
		loc.OpeningHours = hoursByLocation[uint64(loc.ID)]
		loc.Hours = daysByLocation[uint64(loc.ID)]
	}

	return locations, nil
}

func (s *AppDB) GetAuthedLocations(ctx context.Context, r *structs.LocationsPageRequest) ([]*structs.Location, error) {
	page, count := normalizeLocationPageRequest(r)
	offset := page * count

	rows, err := s.db.Query(ctx, `
		SELECT
			l.id,
			COALESCE(l.google_id, ''),
			COALESCE(l.listing_source, 'google_place'),
			l.owner_id,
			l.name,
			l.description,
			l.type,
			l.approval,
			l.street,
			l.city,
			l.state,
			l.zip,
			l.lat,
			l.lng,
			l.phone,
			l.email,
			l.admin_phone,
			l.admin_email,
			l.website,
			l.image_url,
			l.icon_updated_at,
			l.photo_updated_at,
			l.rating,
			l.maps_page,
			l.contact_firstname,
			l.contact_lastname,
			l.contact_phone,
			l.pos_system,
			l.sole_proprietorship,
			l.tipping_policy,
			l.tipping_division,
			l.table_coverage,
			l.service_stations,
			l.tablet_model,
			l.messaging_service,
			`+locationPayToAddressExpr+` AS pay_to_address,
			COALESCE(
				NULLIF(TRIM(l.tipping_wallet_address), ''),
				''
			) AS tip_to_address,
			l.reference,
			l.hours_manual
		FROM locations l
		LEFT JOIN users u
			ON u.id = l.owner_id
			AND u.active = TRUE
		WHERE l.active = TRUE
		AND l.location_kind = 'merchant'
		ORDER BY l.id
		LIMIT $1
		OFFSET $2;
	`, count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying for locations: %w", err)
	}
	defer rows.Close()

	authedLocations := []*structs.Location{}
	locationIDs := []uint64{}

	for rows.Next() {
		location := structs.Location{}
		var payToAddress sql.NullString
		var tipToAddress sql.NullString
		var iconUpdatedAt sql.NullTime
		var photoUpdatedAt sql.NullTime

		err = rows.Scan(
			&location.ID,
			&location.GoogleID,
			&location.ListingSource,
			&location.OwnerID,
			&location.Name,
			&location.Description,
			&location.Type,
			&location.Approval,
			&location.Street,
			&location.City,
			&location.State,
			&location.ZIP,
			&location.Lat,
			&location.Lng,
			&location.Phone,
			&location.Email,
			&location.AdminPhone,
			&location.AdminEmail,
			&location.Website,
			&location.ImageURL,
			&iconUpdatedAt,
			&photoUpdatedAt,
			&location.Rating,
			&location.MapsPage,
			&location.ContactFirstName,
			&location.ContactLastName,
			&location.ContactPhone,
			&location.PosSystem,
			&location.SoleProprietorship,
			&location.TippingPolicy,
			&location.TippingDivision,
			&location.TableCoverage,
			&location.ServiceStations,
			&location.TabletModel,
			&location.MessagingService,
			&payToAddress,
			&tipToAddress,
			&location.Reference,
			&location.HoursManual,
		)

		if err != nil {
			return nil, fmt.Errorf("error scanning authed location row: %w", err)
		}
		if payToAddress.Valid {
			location.PayToAddress = payToAddress.String
		}
		if tipToAddress.Valid {
			location.TipToAddress = tipToAddress.String
		}
		if iconUpdatedAt.Valid {
			location.IconURL = LocationIconURL(location.ID, iconUpdatedAt.Time)
		}
		if photoUpdatedAt.Valid {
			location.PhotoURL = LocationPhotoURL(location.ID, photoUpdatedAt.Time)
		}
		authedLocations = append(authedLocations, &location)
		locationIDs = append(locationIDs, uint64(location.ID))
	}

	hoursByLocation, daysByLocation, err := s.getLocationHoursByIDs(ctx, locationIDs)
	if err != nil {
		return nil, err
	}

	for _, loc := range authedLocations {
		loc.OpeningHours = hoursByLocation[uint64(loc.ID)]
		loc.Hours = daysByLocation[uint64(loc.ID)]
	}

	if err := s.attachLocationPaymentWallets(ctx, authedLocations); err != nil {
		return nil, err
	}

	return authedLocations, nil
}

// ErrDuplicateGoogleLocation is returned when a Google place is already
// registered as an active location. Callers surface it as a 409.
var ErrDuplicateGoogleLocation = errors.New("a location for this google place already exists")

// LocationApprovalContact is the merchant-side addressee for approval mail.
type LocationApprovalContact struct {
	Name             string
	AdminEmail       string
	ContactFirstName string
	ContactLastName  string
}

// GetLocationApprovalContact loads who to notify when a location is approved.
func (a *AppDB) GetLocationApprovalContact(ctx context.Context, id uint) (*LocationApprovalContact, error) {
	contact := &LocationApprovalContact{}
	err := a.db.QueryRow(ctx, `
		SELECT
			COALESCE(name, ''),
			COALESCE(admin_email, ''),
			COALESCE(contact_firstname, ''),
			COALESCE(contact_lastname, '')
		FROM locations
		WHERE id = $1;
	`, id).Scan(
		&contact.Name,
		&contact.AdminEmail,
		&contact.ContactFirstName,
		&contact.ContactLastName,
	)
	if err != nil {
		return nil, fmt.Errorf("error loading approval contact for location %d: %w", id, err)
	}

	return contact, nil
}

// replaceLocationHours rewrites the weekday rows for a location. Hours arrive as
// an ordered slice of Google weekday descriptions, so the existing rows are
// dropped and reinserted rather than updated in place — updating in place
// requires matching on weekday, and getting that wrong overwrites every row
// with the last weekday's hours.
// replaceLocationDayHours writes structured hours and derives the display text
// from them, so the text column can never disagree with the times a picker
// shows. It is the only place that decides what the text says.
func replaceLocationDayHours(ctx context.Context, tx pgx.Tx, locationID uint, days []structs.LocationDayHours) error {
	if _, err := tx.Exec(ctx, `DELETE FROM location_hours WHERE location_id = $1;`, locationID); err != nil {
		return fmt.Errorf("error clearing location hours: %w", err)
	}

	for _, day := range days {
		day.SortIntervals()
		encoded, err := json.Marshal(day.Intervals)
		if err != nil {
			return fmt.Errorf("error encoding location hour intervals: %w", err)
		}
		// open_minute/close_minute mirror the first stretch so anything still
		// reading the flat columns sees a sane value rather than nothing.
		var firstOpen, firstClose *int
		if len(day.Intervals) > 0 {
			firstOpen = &day.Intervals[0].OpenMinute
			firstClose = &day.Intervals[0].CloseMinute
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO location_hours (
				location_id,
				weekday,
				hours,
				open_minute,
				close_minute,
				is_closed,
				intervals
			) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb);
		`, locationID, day.Weekday, day.Display(), firstOpen, firstClose, day.IsClosed, encoded); err != nil {
			return fmt.Errorf("error adding location hours to hour table: %w", err)
		}
	}

	return nil
}

func replaceLocationHours(ctx context.Context, tx pgx.Tx, locationID uint, hours []string) error {
	_, err := tx.Exec(ctx, `
		DELETE FROM location_hours
		WHERE location_id = $1;
	`, locationID)
	if err != nil {
		return fmt.Errorf("error clearing location hours: %w", err)
	}

	for weekday, entry := range hours {
		_, err := tx.Exec(ctx, `
			INSERT INTO location_hours (
				location_id,
				weekday,
				hours
			) VALUES ($1, $2, $3);
		`, locationID, weekday, entry)
		if err != nil {
			return fmt.Errorf("error adding location hours to hour table: %w", err)
		}
	}

	return nil
}

func (a *AppDB) AddLocation(ctx context.Context, location *structs.Location) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error opening transaction for new location: %w", err)
	}
	defer tx.Rollback(ctx)

	// Checked inside the transaction so two concurrent submissions of the same
	// business cannot both pass. The partial unique index added in schema 1.24
	// is the backstop when it exists.
	//
	// Skipped for manual listings: they have no place id, so there is nothing to
	// deduplicate on. Two manual submissions for the same shop are caught by the
	// admin reviewing the queue, not here — which is why listing_source is stored
	// and surfaced to that review.
	if location.GoogleID != "" {
		var duplicateExists bool
		err = tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM locations
				WHERE google_id = $1
				AND active = TRUE
			);
		`, location.GoogleID).Scan(&duplicateExists)
		if err != nil {
			return fmt.Errorf("error checking for duplicate location: %w", err)
		}
		if duplicateExists {
			return ErrDuplicateGoogleLocation
		}
	}

	var locationID uint
	err = tx.QueryRow(ctx, `
			INSERT INTO locations (
				google_id,
				owner_id,
				name,
				description,
				type,
				approval,
				approved_at,
				street,
				city,
				state,
				zip,
				lat,
				lng,
				phone,
				email,
				admin_phone,
				admin_email,
				website,
				image_url,
				rating,
				maps_page,
				contact_firstname,
				contact_lastname,
				contact_phone,
				pos_system,
				sole_proprietorship,
				tipping_policy,
				tipping_division,
				table_coverage,
				service_stations,
				tablet_model,
				messaging_service,
				reference,
				listing_source
			) VALUES (
				-- NULL, not '': the partial unique index covers google_id IS NOT
				-- NULL, so an empty string would make every manual listing a
				-- duplicate of the previous one.
				NULLIF($1, ''), $2, $3, $4, $5, $6,
				CASE WHEN $6 IS TRUE THEN NOW() ELSE NULL END,
				$7, $8, $9, $10,
				$11, $12, $13, $14, $15, $16, $17, $18,
				$19, $20, $21, $22, $23, $24, $25, $26,
				$27, $28, $29, $30, $31, $32, $33
			)
			RETURNING id;`,
		location.GoogleID,
		location.OwnerID,
		location.Name,
		location.Description,
		location.Type,
		location.Approval,
		location.Street,
		location.City,
		location.State,
		location.ZIP,
		location.Lat,
		location.Lng,
		location.Phone,
		location.Email,
		location.AdminPhone,
		location.AdminEmail,
		location.Website,
		location.ImageURL,
		location.Rating,
		location.MapsPage,
		location.ContactFirstName,
		location.ContactLastName,
		location.ContactPhone,
		location.PosSystem,
		location.SoleProprietorship,
		location.TippingPolicy,
		location.TippingDivision,
		location.TableCoverage,
		location.ServiceStations,
		location.TabletModel,
		location.MessagingService,
		location.Reference,
		location.ListingSource,
	).Scan(&locationID)
	if err != nil {
		return fmt.Errorf("error adding location to locations table: %w", err)
	}

	if err := replaceLocationHours(ctx, tx, locationID, location.OpeningHours); err != nil {
		return err
	}

	// Listing the shop is the act that finishes merchant onboarding, so the
	// timestamp the forced-onboarding gate keys off is stamped here, with the
	// insert, rather than by a separate call that could fail on its own and
	// strand somebody behind a gate they had in fact passed.
	//
	// Only while still NULL. A merchant opening a second location years later
	// has not re-onboarded, and moving the stamp forward would restate when they
	// first finished — the one thing the column exists to remember.
	//
	// Only for account_type = 'merchant'. The gate exists to walk a self-declared
	// merchant through setup; a regular account that happens to submit a listing
	// never saw that flow, and marking it complete would skip it for good if they
	// later say they are a merchant.
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET merchant_onboarding_completed_at = NOW()
		WHERE id = $1
		AND account_type = 'merchant'
		AND merchant_onboarding_completed_at IS NULL;
	`, location.OwnerID); err != nil {
		return fmt.Errorf("error recording merchant onboarding completion for %s: %w", location.OwnerID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing new location: %w", err)
	}

	location.ID = locationID
	return nil
}

// UpdateLocation writes the merchant-authored fields of a location the caller
// owns: the display fields a merchant legitimately controls, plus their contact
// details and onboarding answers.
//
// Deliberately excluded from the SET list:
//   - approval / approved_at — this route is owner-scoped, not admin-scoped, so
//     including them let a merchant publish themselves to the public map.
//     Approval changes only through UpdateLocationApproval.
//   - owner_id — a request cannot hand a location to a different account.
//   - google_id, type, city, state, zip, lat, lng, rating, maps_page and the
//     opening hours — these identify and place the listing, and are written only
//     from a server-verified Places lookup (UpdateLocationGooglePlace), so a
//     client cannot swap the underlying place or move the map pin by hand.
//
// The parameter order below is checked against the SET list on every edit: an
// off-by-one here previously wrote admin_phone into the public website column.
func (a *AppDB) UpdateLocation(ctx context.Context, location *structs.Location) error {
	result, err := a.db.Exec(ctx, `
	    UPDATE locations
	    SET
	        name = $1,
	        description = $2,
	        street = $3,
	        phone = $4,
	        email = $5,
	        website = $6,
	        admin_phone = $7,
	        admin_email = $8,
	        contact_firstname = $9,
	        contact_lastname = $10,
	        contact_phone = $11,
	        pos_system = $12,
	        sole_proprietorship = $13,
	        tipping_policy = $14,
	        tipping_division = $15,
	        table_coverage = $16,
	        service_stations = $17,
	        tablet_model = $18,
	        messaging_service = $19,
	        reference = $20
	    WHERE (id = $21 AND owner_id = $22 AND active = TRUE);
	`,
		location.Name,
		location.Description,
		location.Street,
		location.Phone,
		location.Email,
		location.Website,
		location.AdminPhone,
		location.AdminEmail,
		location.ContactFirstName,
		location.ContactLastName,
		location.ContactPhone,
		location.PosSystem,
		location.SoleProprietorship,
		location.TippingPolicy,
		location.TippingDivision,
		location.TableCoverage,
		location.ServiceStations,
		location.TabletModel,
		location.MessagingService,
		location.Reference,
		location.ID,
		location.OwnerID,
	)
	if err != nil {
		return fmt.Errorf("error updating locations table: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// UpdateLocationGooglePlace rewrites the Google-derived half of a location the
// caller owns, from an already-verified Places lookup. This is the only write
// path for those columns, so a client can never set them directly.
func (a *AppDB) UpdateLocationGooglePlace(ctx context.Context, ownerID string, locationID uint, place *structs.VerifiedGooglePlace) error {
	return a.updateLocationGooglePlace(ctx, ownerID, locationID, place)
}

// updateLocationGooglePlace is the shared implementation. An empty ownerID
// means "any owner" and is only reachable from the admin path; the duplicate
// check and the column list are identical either way, so the two callers cannot
// drift apart in what a re-point actually writes.
func (a *AppDB) updateLocationGooglePlace(ctx context.Context, ownerID string, locationID uint, place *structs.VerifiedGooglePlace) error {
	if place == nil {
		return fmt.Errorf("a verified google place is required")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error opening transaction for google place update: %w", err)
	}
	defer tx.Rollback(ctx)

	// Another active location already claiming this place would break the
	// one-active-location-per-place rule the add path enforces.
	var takenElsewhere bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM locations
			WHERE google_id = $1
			AND id <> $2
			AND active = TRUE
		);
	`, place.GoogleID, locationID).Scan(&takenElsewhere)
	if err != nil {
		return fmt.Errorf("error checking for duplicate location: %w", err)
	}
	if takenElsewhere {
		return ErrDuplicateGoogleLocation
	}

	result, err := tx.Exec(ctx, `
		UPDATE locations
		SET
			-- NULL, not '', to match the INSERT. The partial unique index covers
			-- google_id IS NOT NULL, so two rows storing '' would collide while
			-- two storing NULL do not. No caller can reach this with an empty
			-- value today — the handler rejects a blank id and the verifier falls
			-- back to the requested place id — but the two writers of this column
			-- disagreeing is how a third one gets written wrong.
			google_id = NULLIF($1, ''),
			name = $2,
			type = $3,
			street = $4,
			city = $5,
			state = $6,
			zip = $7,
			lat = $8,
			lng = $9,
			website = $10,
			rating = $11,
			maps_page = $12
		WHERE (id = $13 AND ($14 = '' OR owner_id = $14) AND active = TRUE);
	`,
		place.GoogleID,
		place.Name,
		place.Type,
		place.Street,
		place.City,
		place.State,
		place.ZIP,
		place.Lat,
		place.Lng,
		place.Website,
		place.Rating,
		place.MapsPage,
		locationID,
		ownerID,
	)
	if err != nil {
		return fmt.Errorf("error updating location google data: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	// Structured times when Google gave us usable ones, text otherwise. A place
	// with no readable periods still keeps its display strings rather than
	// losing its hours entirely.
	if len(place.StructuredHours) == len(structs.WeekdayNames) {
		if err := replaceLocationDayHours(ctx, tx, locationID, place.StructuredHours); err != nil {
			return err
		}
	} else if err := replaceLocationHours(ctx, tx, locationID, place.OpeningHours); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing google place update: %w", err)
	}

	return nil
}

func (a *AppDB) GetLocationsByUser(ctx context.Context, userId string) ([]*structs.Location, error) {
	rows, err := a.db.Query(ctx, `
    SELECT
        l.id,
		COALESCE(l.google_id, ''),
		COALESCE(l.listing_source, 'google_place'),
		l.owner_id,
		l.name,
		l.description,
		l.type,
		l.approval,
		l.street,
		l.city,
		l.state,
		l.zip,
		l.lat,
		l.lng,
		l.phone,
		l.email,
		l.admin_phone,
		l.admin_email,
		l.website,
		l.image_url,
		l.icon_updated_at,
		l.photo_updated_at,
		l.rating,
		l.maps_page,
		l.contact_firstname,
		l.contact_lastname,
		l.contact_phone,
		l.pos_system,
		l.sole_proprietorship,
		l.tipping_policy,
		l.tipping_division,
		l.table_coverage,
		l.service_stations,
		l.tablet_model,
		l.messaging_service,
		`+locationPayToAddressExpr+` AS pay_to_address,
		COALESCE(
			NULLIF(TRIM(l.tipping_wallet_address), ''),
			''
		) AS tip_to_address,
		l.reference,
		l.hours_manual
    FROM locations l
	LEFT JOIN users u
		ON u.id = l.owner_id
		AND u.active = TRUE
    WHERE l.owner_id = $1
	AND l.active = TRUE
	AND l.location_kind = 'merchant'
	ORDER BY l.id DESC
	LIMIT 500;
`, userId)

	if err != nil {
		return nil, fmt.Errorf("error querying location table: %s", err)
	}
	defer rows.Close()

	locations := []*structs.Location{}
	locationIDs := []uint64{}

	for rows.Next() {
		var location structs.Location
		var payToAddress sql.NullString
		var tipToAddress sql.NullString
		var iconUpdatedAt sql.NullTime
		var photoUpdatedAt sql.NullTime
		err := rows.Scan(
			&location.ID,
			&location.GoogleID,
			&location.ListingSource,
			&location.OwnerID,
			&location.Name,
			&location.Description,
			&location.Type,
			&location.Approval,
			&location.Street,
			&location.City,
			&location.State,
			&location.ZIP,
			&location.Lat,
			&location.Lng,
			&location.Phone,
			&location.Email,
			&location.AdminPhone,
			&location.AdminEmail,
			&location.Website,
			&location.ImageURL,
			&iconUpdatedAt,
			&photoUpdatedAt,
			&location.Rating,
			&location.MapsPage,
			&location.ContactFirstName,
			&location.ContactLastName,
			&location.ContactPhone,
			&location.PosSystem,
			&location.SoleProprietorship,
			&location.TippingPolicy,
			&location.TippingDivision,
			&location.TableCoverage,
			&location.ServiceStations,
			&location.TabletModel,
			&location.MessagingService,
			&payToAddress,
			&tipToAddress,
			&location.Reference,
			&location.HoursManual,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning location data: %s", err)
		}
		if payToAddress.Valid {
			location.PayToAddress = payToAddress.String
		}
		if tipToAddress.Valid {
			location.TipToAddress = tipToAddress.String
		}
		if iconUpdatedAt.Valid {
			location.IconURL = LocationIconURL(location.ID, iconUpdatedAt.Time)
		}
		if photoUpdatedAt.Valid {
			location.PhotoURL = LocationPhotoURL(location.ID, photoUpdatedAt.Time)
		}
		locations = append(locations, &location)
		locationIDs = append(locationIDs, uint64(location.ID))
	}

	hoursByLocation, daysByLocation, err := a.getLocationHoursByIDs(ctx, locationIDs)
	if err != nil {
		return nil, err
	}

	for _, loc := range locations {
		loc.OpeningHours = hoursByLocation[uint64(loc.ID)]
		loc.Hours = daysByLocation[uint64(loc.ID)]
	}

	if err := a.attachLocationPaymentWallets(ctx, locations); err != nil {
		return nil, err
	}

	return locations, nil
}

// GetVolunteerLocationsByIds resolves volunteer event locations in one round
// trip. Events live in the bot database and locations in the app database, so
// this cannot be a SQL join — the handler batches ids and stitches the result.
// It deliberately reads only location_kind = 'volunteer' rows, so an event can
// never surface a merchant's private address by pointing at the wrong id.
func (s *AppDB) GetVolunteerLocationsByIds(ctx context.Context, ids []int64) (map[int64]*structs.VolunteerEventLocation, error) {
	result := map[int64]*structs.VolunteerEventLocation{}
	if len(ids) == 0 {
		return result, nil
	}

	rows, err := s.db.Query(ctx, `
		SELECT
			id,
			COALESCE(name, ''),
			COALESCE(street, ''),
			COALESCE(city, ''),
			COALESCE(state, ''),
			COALESCE(zip, ''),
			lat,
			lng
		FROM locations
		WHERE id = ANY($1)
			AND active = TRUE
			AND location_kind = 'volunteer';
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		location := &structs.VolunteerEventLocation{}
		if err := rows.Scan(
			&location.Id,
			&location.Name,
			&location.Street,
			&location.City,
			&location.State,
			&location.Zip,
			&location.Lat,
			&location.Lng,
		); err != nil {
			return nil, err
		}
		result[location.Id] = location
	}

	return result, rows.Err()
}
