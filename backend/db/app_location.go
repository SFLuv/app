package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) GetLocation(id uint64) (*structs.LocationRequest, error) {
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

	location := structs.LocationRequest{}
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

	return &location, nil
}

func (s *AppDB) GetLocations(r *structs.LocationsPageRequest) ([]*structs.LocationRequest, error) {
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

	locations := []*structs.LocationRequest{}

	for rows.Next() {
		location := structs.LocationRequest{}

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

	return locations, nil
}

func (a *AppDB) AddLocation(location *structs.LocationRequest) error {
	_, err := a.db.Exec(context.Background(), `
		INSERT INTO locations (
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
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19);
	`,
		location.ID,
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

	return err
}

func (a *AppDB) UpdateLocation(location *structs.LocationRequest) error {
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

func (a *AppDB) GetLocationsByUser(user *structs.User) (*[]structs.LocationRequest, error) {
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
	`, user.Id)

	if err != nil {
		return nil, fmt.Errorf("error querying location table: %s", err)
	}
	defer rows.Close()

	locations := []structs.LocationRequest{}

	for rows.Next() {
		var location structs.LocationRequest
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
		locations = append(locations, location)
	}

	return &locations, nil
}

// add pagination to get locations
