package db

import (
	"context"
	"database/sql"
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

func (a *AppDB) getLocationHoursByIDs(ctx context.Context, ids []uint64) (map[uint64][]string, error) {
	if len(ids) == 0 {
		return map[uint64][]string{}, nil
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
			hours
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
		return nil, fmt.Errorf("error querying location hours: %w", err)
	}
	defer rows.Close()

	hoursByLocation := make(map[uint64][]string, len(idParams))
	for rows.Next() {
		var locationID int32
		var hours string
		if err := rows.Scan(&locationID, &hours); err != nil {
			return nil, fmt.Errorf("error scanning location hours: %w", err)
		}
		hoursByLocation[uint64(locationID)] = append(hoursByLocation[uint64(locationID)], hours)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating location hours: %w", err)
	}

	return hoursByLocation, nil
}

func (a *AppDB) GetLocation(ctx context.Context, id uint64) (*structs.PublicLocation, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			l.id,
			l.google_id,
			l.name,
			l.approval,
			COALESCE(
				NULLIF(TRIM(default_payment_wallet.wallet_address), ''),
				NULLIF(TRIM(u.primary_wallet_address), ''),
				NULLIF(TRIM(legacy_wallet.smart_address), ''),
				''
			) AS pay_to_address,
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
			l.rating,
			l.maps_page
		FROM locations l
		LEFT JOIN users u
			ON u.id = l.owner_id
			AND u.active = TRUE
		LEFT JOIN LATERAL (
			SELECT
				lpw.wallet_address
			FROM
				location_payment_wallets lpw
			WHERE
				lpw.location_id = l.id
			AND
				lpw.active = TRUE
			ORDER BY
				CASE
					WHEN lpw.is_default = TRUE THEN 0
					ELSE 1
				END,
				lpw.id ASC
			LIMIT 1
		) default_payment_wallet
			ON TRUE
		LEFT JOIN LATERAL (
			SELECT
				w.smart_address
			FROM
				wallets w
			WHERE
				w.owner = l.owner_id
			AND
				w.active = TRUE
			AND
				w.is_eoa = FALSE
			AND
				w.smart_index = 0
			AND
				w.smart_address IS NOT NULL
			AND
				TRIM(w.smart_address) <> ''
			ORDER BY
				w.id ASC
			LIMIT 1
		) legacy_wallet
			ON TRUE
		WHERE l.id = $1
		AND l.active = TRUE;
	`, id)

	location := structs.PublicLocation{}
	var payToAddress sql.NullString
	var tipToAddress sql.NullString
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
			l.google_id,
			l.name,
			COALESCE(
				NULLIF(TRIM(default_payment_wallet.wallet_address), ''),
				NULLIF(TRIM(u.primary_wallet_address), ''),
				NULLIF(TRIM(legacy_wallet.smart_address), ''),
				''
			) AS pay_to_address,
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
			l.rating,
			l.maps_page
		FROM locations l
		LEFT JOIN users u
			ON u.id = l.owner_id
			AND u.active = TRUE
		LEFT JOIN LATERAL (
			SELECT
				lpw.wallet_address
			FROM
				location_payment_wallets lpw
			WHERE
				lpw.location_id = l.id
			AND
				lpw.active = TRUE
			ORDER BY
				CASE
					WHEN lpw.is_default = TRUE THEN 0
					ELSE 1
				END,
				lpw.id ASC
			LIMIT 1
		) default_payment_wallet
			ON TRUE
		LEFT JOIN LATERAL (
			SELECT
				w.smart_address
			FROM
				wallets w
			WHERE
				w.owner = l.owner_id
			AND
				w.active = TRUE
			AND
				w.is_eoa = FALSE
			AND
				w.smart_index = 0
			AND
				w.smart_address IS NOT NULL
			AND
				TRIM(w.smart_address) <> ''
			ORDER BY
				w.id ASC
			LIMIT 1
		) legacy_wallet
			ON TRUE
		WHERE l.approval = TRUE
		AND l.active = TRUE
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
		locations = append(locations, &location)
		locationIDs = append(locationIDs, uint64(location.ID))
	}

	hoursByLocation, err := s.getLocationHoursByIDs(ctx, locationIDs)
	if err != nil {
		return nil, err
	}

	for _, loc := range locations {
		loc.OpeningHours = hoursByLocation[uint64(loc.ID)]
	}

	return locations, nil
}

func (s *AppDB) GetAuthedLocations(ctx context.Context, r *structs.LocationsPageRequest) ([]*structs.Location, error) {
	page, count := normalizeLocationPageRequest(r)
	offset := page * count

	rows, err := s.db.Query(ctx, `
		SELECT
			l.id,
			l.google_id,
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
			COALESCE(
				NULLIF(TRIM(default_payment_wallet.wallet_address), ''),
				NULLIF(TRIM(u.primary_wallet_address), ''),
				NULLIF(TRIM(legacy_wallet.smart_address), ''),
				''
			) AS pay_to_address,
			COALESCE(
				NULLIF(TRIM(l.tipping_wallet_address), ''),
				''
			) AS tip_to_address,
			l.reference
		FROM locations l
		LEFT JOIN users u
			ON u.id = l.owner_id
			AND u.active = TRUE
		LEFT JOIN LATERAL (
			SELECT
				lpw.wallet_address
			FROM
				location_payment_wallets lpw
			WHERE
				lpw.location_id = l.id
			AND
				lpw.active = TRUE
			ORDER BY
				CASE
					WHEN lpw.is_default = TRUE THEN 0
					ELSE 1
				END,
				lpw.id ASC
			LIMIT 1
		) default_payment_wallet
			ON TRUE
		LEFT JOIN LATERAL (
			SELECT
				w.smart_address
			FROM
				wallets w
			WHERE
				w.owner = l.owner_id
			AND
				w.active = TRUE
			AND
				w.is_eoa = FALSE
			AND
				w.smart_index = 0
			AND
				w.smart_address IS NOT NULL
			AND
				TRIM(w.smart_address) <> ''
			ORDER BY
				w.id ASC
			LIMIT 1
		) legacy_wallet
			ON TRUE
		WHERE l.active = TRUE
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

		err = rows.Scan(
			&location.ID,
			&location.GoogleID,
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
		authedLocations = append(authedLocations, &location)
		locationIDs = append(locationIDs, uint64(location.ID))
	}

	hoursByLocation, err := s.getLocationHoursByIDs(ctx, locationIDs)
	if err != nil {
		return nil, err
	}

	for _, loc := range authedLocations {
		loc.OpeningHours = hoursByLocation[uint64(loc.ID)]
	}

	if err := s.attachLocationPaymentWallets(ctx, authedLocations); err != nil {
		return nil, err
	}

	return authedLocations, nil
}

func (a *AppDB) AddLocation(ctx context.Context, location *structs.Location) error {
	_, err := a.db.Exec(ctx, `
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
				reference
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				CASE WHEN $6 IS TRUE THEN NOW() ELSE NULL END,
				$7, $8, $9, $10,
				$11, $12, $13, $14, $15, $16, $17, $18,
				$19, $20, $21, $22, $23, $24, $25, $26,
				$27, $28, $29, $30, $31, $32
			)
			ON CONFLICT (google_id) WHERE active = TRUE
			DO UPDATE SET
				owner_id = EXCLUDED.owner_id,
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				type = EXCLUDED.type,
				approval = EXCLUDED.approval,
				approved_at = EXCLUDED.approved_at,
				street = EXCLUDED.street,
				city = EXCLUDED.city,
				state = EXCLUDED.state,
				zip = EXCLUDED.zip,
				lat = EXCLUDED.lat,
				lng = EXCLUDED.lng,
				phone = EXCLUDED.phone,
				email = EXCLUDED.email,
				admin_phone = EXCLUDED.admin_phone,
				admin_email = EXCLUDED.admin_email,
				website = EXCLUDED.website,
				image_url = EXCLUDED.image_url,
				rating = EXCLUDED.rating,
				maps_page = EXCLUDED.maps_page,
				contact_firstname = EXCLUDED.contact_firstname,
				contact_lastname = EXCLUDED.contact_lastname,
				contact_phone = EXCLUDED.contact_phone,
				pos_system = EXCLUDED.pos_system,
				sole_proprietorship = EXCLUDED.sole_proprietorship,
				tipping_policy = EXCLUDED.tipping_policy,
				tipping_division = EXCLUDED.tipping_division,
				table_coverage = EXCLUDED.table_coverage,
				service_stations = EXCLUDED.service_stations,
				tablet_model = EXCLUDED.tablet_model,
				messaging_service = EXCLUDED.messaging_service,
				reference = EXCLUDED.reference,
				active = TRUE,
				delete_date = NULL,
				delete_reason = NULL;`,
		&location.GoogleID,
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
		&location.Reference,
	)

	if err != nil {
		return fmt.Errorf("error adding location to locations table: %s", err)
	}

	row := a.db.QueryRow(ctx, `
		SELECT
			id
		FROM locations
		WHERE google_id = $1
		AND active = TRUE;
	`, location.GoogleID,
	)

	id := 0
	err = row.Scan(&id)

	for i := 0; i < len(location.OpeningHours); i++ {
		hours := location.OpeningHours[i]
		_, err := a.db.Exec(ctx, `
		INSERT INTO location_hours (
			location_id,
			weekday,
			hours
		) VALUES ($1, $2, $3);
		`,
			id,
			i,
			hours,
		)
		if err != nil {
			return fmt.Errorf("error adding location hours to hour table: %s", err)
		}
	}
	return err
}

func (a *AppDB) UpdateLocation(ctx context.Context, location *structs.Location) error {
	result, err := a.db.Exec(ctx, `
	    UPDATE locations
	    SET
        google_id = $1,
        owner_id = $2,
        name = $3,
        description = $4,
	        type = $5,
	        approval = $6,
	        approved_at = CASE
	        	WHEN $6 IS TRUE THEN COALESCE(approved_at, NOW())
	        	ELSE NULL
	        END,
	        street = $7,
        city = $8,
        state = $9,
        zip = $10,
        lat = $11,
        lng = $12,
        phone = $13,
        email = $14,
        website = $15,
        admin_phone = $16,
        admin_email = $17,
        image_url = $18,
        rating = $19,
        maps_page = $20,
        contact_firstname = $21,
        contact_lastname = $22,
        contact_phone = $23,
        pos_system = $24,
        sole_proprietorship = $25,
        tipping_policy = $26,
        tipping_division = $27,
        table_coverage = $28,
        service_stations = $29,
        tablet_model = $30,
        messaging_service = $31,
        reference = $32
    WHERE (id = $33 AND owner_id = $34 AND active = TRUE);
	`,
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
		location.ID,
		location.OwnerID,
	)
	if err != nil {
		return fmt.Errorf("error updating locations table: %s", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	for i := 0; i < len(location.OpeningHours); i++ {
		hours := location.OpeningHours[i]
		_, err := a.db.Exec(ctx, `
		UPDATE location_hours
		SET
			weekday = $1,
			hours = $2
		WHERE location_id = $3
		AND active = TRUE;
		`,
			i,
			hours,
			location.ID,
		)
		if err != nil {
			return fmt.Errorf("error updating location hours table: %s", err)
		}
	}
	return err
}

func (a *AppDB) GetLocationsByUser(ctx context.Context, userId string) ([]*structs.Location, error) {
	rows, err := a.db.Query(ctx, `
    SELECT
        l.id,
		l.google_id,
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
		COALESCE(
			NULLIF(TRIM(default_payment_wallet.wallet_address), ''),
			NULLIF(TRIM(u.primary_wallet_address), ''),
			NULLIF(TRIM(legacy_wallet.smart_address), ''),
			''
		) AS pay_to_address,
		COALESCE(
			NULLIF(TRIM(l.tipping_wallet_address), ''),
			''
		) AS tip_to_address,
		l.reference
    FROM locations l
	LEFT JOIN users u
		ON u.id = l.owner_id
		AND u.active = TRUE
	LEFT JOIN LATERAL (
		SELECT
			lpw.wallet_address
		FROM
			location_payment_wallets lpw
		WHERE
			lpw.location_id = l.id
		AND
			lpw.active = TRUE
		ORDER BY
			CASE
				WHEN lpw.is_default = TRUE THEN 0
				ELSE 1
			END,
			lpw.id ASC
		LIMIT 1
	) default_payment_wallet
		ON TRUE
	LEFT JOIN LATERAL (
		SELECT
			w.smart_address
		FROM
			wallets w
		WHERE
			w.owner = l.owner_id
		AND
			w.active = TRUE
		AND
			w.is_eoa = FALSE
		AND
			w.smart_index = 0
		AND
			w.smart_address IS NOT NULL
		AND
			TRIM(w.smart_address) <> ''
		ORDER BY
			w.id ASC
		LIMIT 1
		) legacy_wallet
			ON TRUE
    WHERE l.owner_id = $1
	AND l.active = TRUE
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
		err := rows.Scan(
			&location.ID,
			&location.GoogleID,
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
		locations = append(locations, &location)
		locationIDs = append(locationIDs, uint64(location.ID))
	}

	hoursByLocation, err := a.getLocationHoursByIDs(ctx, locationIDs)
	if err != nil {
		return nil, err
	}

	for _, loc := range locations {
		loc.OpeningHours = hoursByLocation[uint64(loc.ID)]
	}

	if err := a.attachLocationPaymentWallets(ctx, locations); err != nil {
		return nil, err
	}

	return locations, nil
}
