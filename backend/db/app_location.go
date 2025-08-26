package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func (a *AppDB) GetLocation(ctx context.Context, id uint64) (*structs.PublicLocation, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id,
			google_id,
			name,
			description,
			type,
			street,
			city,
			state,
			zip,
			lat,
			lng,
			phone,
			email,
			website,
			image_url,
			rating,
			maps_page
		FROM locations
		WHERE id = $1;
	`, id)

	location := structs.PublicLocation{}
	err := row.Scan(
		&location.ID,
		&location.GoogleID,
		&location.Name,
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

	rows, err := a.db.Query(ctx, `
		SELECT
			hours
		FROM location_hours
		WHERE location_id = $1
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
	offset := r.Page * r.Count

	rows, err := s.db.Query(ctx, `
		SELECT
			id,
			google_id,
			name,
			description,
			type,
			street,
			city,
			state,
			zip,
			lat,
			lng,
			phone,
			email,
			website,
			image_url,
			rating,
			maps_page
		FROM locations
		WHERE approval = TRUE
		ORDER BY id
		LIMIT $1
		OFFSET $2;
	`, r.Count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying for locations: %w", err)
	}
	defer rows.Close()

	locations := []*structs.PublicLocation{}

	for rows.Next() {
		location := structs.PublicLocation{}

		err = rows.Scan(
			&location.ID,
			&location.GoogleID,
			&location.Name,
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
		locations = append(locations, &location)
	}

	for _, loc := range locations {
		curr_hours := ""
		openingHours := []string{}
		rows2, err2 := s.db.Query(ctx, `
			SELECT
				hours
			FROM location_hours
			WHERE location_id = $1
			ORDER BY weekday;
		`, loc.ID)
		if err2 != nil {
			s.logger.Logf("error querying location hours table: %s", err2)
			continue
		}

		for rows2.Next() {
			err2 = rows2.Scan(
				&curr_hours,
			)
			if err2 != nil {
				rows2.Close()
				break
			}
			openingHours = append(openingHours, curr_hours)
		}
		if err2 != nil {
			s.logger.Logf("error scanning hours rows for get locations: %s", err2)
			continue
		}

		loc.OpeningHours = openingHours
	}

	return locations, nil
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
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23, $24, $25, $26,
			$27, $28, $29, $30, $31, $32
			);
		`,
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
		WHERE google_id = $1;
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
    WHERE (id = $33 AND owner_id = $34);
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
		WHERE location_id = $3;
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
        id,
		google_id,
		owner_id,
		name,
		description,
		type,
		approval,
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
    FROM locations
    WHERE owner_id = $1;
`, userId)

	if err != nil {
		return nil, fmt.Errorf("error querying location table: %s", err)
	}
	defer rows.Close()

	locations := []*structs.Location{}

	for rows.Next() {
		var location structs.Location
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
			&location.Reference,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning location data: %s", err)
		}
		locations = append(locations, &location)
	}

	finalLocations := []*structs.Location{}
	for _, loc := range locations {
		curr_hours := ""
		openingHours := []string{}
		rows2, err2 := a.db.Query(ctx, `
			SELECT
				hours
			FROM location_hours
			WHERE location_id = $1
			ORDER BY weekday;
		`, loc.ID)
		if err2 != nil {
			a.logger.Logf("error querying location hours table: %s", err2)
			continue
		}

		for rows2.Next() {
			err2 = rows2.Scan(
				&curr_hours,
			)
			if err2 != nil {
				rows2.Close()
				break
			}
			openingHours = append(openingHours, curr_hours)
		}
		if err2 != nil {
			a.logger.Logf("error scanning hours rows for get locations by id: %s", err2)
			continue
		}

		loc.OpeningHours = openingHours
		finalLocations = append(finalLocations, loc)
	}

	return finalLocations, nil
}
