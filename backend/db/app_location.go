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
			maps_page
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
	)
	if err != nil {
		return nil, err
	}
	hours := [][2]float64{}
	opening_time := float64(100)
	closing_time := float64(100)
	rows, err := a.db.Query(context.Background(), `
		SELECT
			open_time,
			close_time
		FROM location_hours
		WHERE location_id = $1
		ORDER BY weekday;
	`, id)
	if err != nil {
		return nil, fmt.Errorf("error getting location hours: %s", err)
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(
			&opening_time,
			&closing_time,
		)
		if err != nil {
			return nil, err
		}
		hour_pair := [2]float64{opening_time, closing_time}
		hours = append(hours, hour_pair)
	}
	location.OpeningHours = hours

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
			maps_page
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
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning location row: %w", err)
		}

		locations = append(locations, &location)
	}

	finalLocations := []*structs.Location{}
	for _, loc := range locations {
		hours := [][2]float64{}
		opening_time := float64(100)
		closing_time := float64(100)
		rows2, err2 := s.db.Query(context.Background(), `
			SELECT
				open_time,
				close_time
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
				&opening_time,
				&closing_time,
			)
			hour_pair := [2]float64{opening_time, closing_time}
			if err2 != nil {
				rows2.Close()
				break
			}
			hours = append(hours, hour_pair)
		}
		if err2 != nil {
			fmt.Printf("error scanning hours rows for get locations: %s\n", err2)
			continue
		}

		loc.OpeningHours = hours
		finalLocations = append(finalLocations, loc)
	}

	return finalLocations, nil
}

func (a *AppDB) AddLocation(location *structs.Location) error {
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
			maps_page
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18);
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
	)

	for i := 0; i < len(location.OpeningHours); i++ {
		hours := location.OpeningHours[i]
		_, err := a.db.Exec(context.Background(), `
		INSERT INTO location_hours (
			location_id,
			weekday,
			open_time,
			close_time
		) VALUES ($1, $2, $3, $4);
		`,
			location.ID,
			i,
			hours[0],
			hours[1],
		)
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
			maps_page = $18
		WHERE id = $19;
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
		location.ID,
	)

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
			maps_page
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
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning location data: %s", err)
		}
		locations = append(locations, &location)
	}

	return locations, nil
}
