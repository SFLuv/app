package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) GetLocation(id uint64) (*structs.Location, error) {
	row := a.db.QueryRow(context.Background(), `
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
			messaging_service
		FROM locations
		WHERE id = $1;
	`, id)

	location := structs.Location{}
	err := row.Scan(
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
	)
	if err != nil {
		return nil, err
	}

	rows, err := a.db.Query(context.Background(), `
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

func (s *AppDB) GetLocations(r *structs.LocationsPageRequest) ([]*structs.Location, error) {
	offset := r.Page * r.Count

	rows, err := s.db.Query(context.Background(), `
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
			messaging_service
		FROM locations
		ORDER BY id
		LIMIT $1
		OFFSET $2;
	`, r.Count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying for locations: %w", err)
	}
	defer rows.Close()

	locations := []*structs.Location{}

	for rows.Next() {
		location := structs.Location{}

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
		)

		if err != nil {
			return nil, fmt.Errorf("error scanning location row: %w", err)
		}
		locations = append(locations, &location)
	}

	finalLocations := []*structs.Location{}
	for _, loc := range locations {
		curr_hours := ""
		openingHours := []string{}
		rows2, err2 := s.db.Query(context.Background(), `
			SELECT
				hours
			FROM location_hours
			WHERE location_id = $1
			ORDER BY weekday;
		`, loc.ID)
		if err2 != nil {
			fmt.Printf("error querying location hours table: %s\n", err2)
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
			fmt.Printf("error scanning hours rows for get locations: %s\n", err2)
			continue
		}

		loc.OpeningHours = openingHours
		finalLocations = append(finalLocations, loc)
	}

	return finalLocations, nil
}

func (a *AppDB) AddLocation(location *structs.Location) error {
	fmt.Println("reached add location controller")
	_, err := a.db.Exec(context.Background(), `
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
				messaging_service
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
				$11, $12, $13, $14, $15, $16, $17, $18,
				$19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29
			);
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
	)

	if err != nil {
		return fmt.Errorf("error adding location to locations table: %s", err)
	}

	row := a.db.QueryRow(context.Background(), `
		SELECT
			id
		FROM locations
		WHERE google_id = $1;
	`, location.GoogleID,
	)

	id := 0
	err = row.Scan(&id)
	fmt.Println(id)

	for i := 0; i < len(location.OpeningHours); i++ {
		hours := location.OpeningHours[i]
		_, err := a.db.Exec(context.Background(), `
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
		fmt.Println(err)
		if err != nil {
			return fmt.Errorf("error adding location hours to hour table: %s", err)
		}
	}
	return err
}

func (a *AppDB) UpdateLocation(location *structs.Location) error {
	_, err := a.db.Exec(context.Background(), `
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
        image_url = $16,
        rating = $17,
        maps_page = $18,
        contact_firstname = $19,
        contact_lastname = $20,
        contact_phone = $21,
        pos_system = $22,
        sole_proprietorship = $23,
        tipping_policy = $24,
        tipping_division = $25,
        table_coverage = $26,
        service_stations = $27,
        tablet_model = $28,
        messaging_service = $29
    WHERE location_id = $30;
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
		location.ID,
	)

	if err != nil {
		return fmt.Errorf("error updating locations table: %s", err)
	}

	for i := 0; i < len(location.OpeningHours); i++ {
		hours := location.OpeningHours[i]
		_, err := a.db.Exec(context.Background(), `
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

func (a *AppDB) GetLocationsByUser(userId string) ([]*structs.Location, error) {
	rows, err := a.db.Query(context.Background(), `
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
        messaging_service
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
		rows2, err2 := a.db.Query(context.Background(), `
			SELECT
				hours
			FROM location_hours
			WHERE location_id = $1
			ORDER BY weekday;
		`, loc.ID)
		if err2 != nil {
			fmt.Printf("error querying location hours table: %s\n", err2)
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
			fmt.Printf("error scanning hours rows for get locations by id: %s\n", err2)
			continue
		}

		loc.OpeningHours = openingHours
		finalLocations = append(finalLocations, loc)
	}

	return locations, nil
}
