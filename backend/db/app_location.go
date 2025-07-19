package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) GetLocation(id uint64) (*structs.LocationRequest, error) {
	row := a.db.QueryRow(context.Background(), `
		SELECT name, googleid, description, id FROM locations WHERE id = $1
	`, id)

	location := structs.LocationRequest{}
	err := row.Scan(&location.Name, &location.GoogleID, &location.Description, &location.ID)
	if err != nil {
		return nil, fmt.Errorf("error scanning location data: %s", err)
	}

	return &location, nil
}

func (a *AppDB) GetLocations() (*[]structs.LocationRequest, error) {
	rows, err := a.db.Query(context.Background(), `
    	SELECT name, googleid, description, id FROM locations
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying locations: %w", err)
	}
	defer rows.Close()

	locations := []structs.LocationRequest{}
	for rows.Next() {
		location := structs.LocationRequest{}
		err := rows.Scan(&location.Name, &location.GoogleID, &location.Description, &location.ID)
		if err != nil {
			return nil, fmt.Errorf("error scanning location data: %s", err)
		}
		locations = append(locations, location)
	}

	return &locations, nil
}

func (a *AppDB) AddLocation(location *structs.LocationRequest) error {
	_, err := a.db.Exec(context.Background(), `
		INSERT INTO locations
			(name, googleid, description, id)
		VALUES
			($1, $2, $3, $4);
		`, location.Name, location.GoogleID, location.Description, location.ID)
	return err
}
