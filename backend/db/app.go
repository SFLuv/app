package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type AppDB struct {
	db *pgx.Conn
}

func App(db *pgx.Conn) *AppDB {
	return &AppDB{db}
}

func (s *AppDB) CreateTables() error {
	_, err := s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS users(
			id TEXT PRIMARY KEY,
			is_admin BOOLEAN NOT NULL DEFAULT false,
			is_merchant BOOLEAN NOT NULL DEFAULT false,
			is_organizer BOOLEAN NOT NULL DEFAULT false,
			is_improver BOOLEAN NOT NULL DEFAULT false,
			contact_email TEXT,
			contact_phone TEXT,
			contact_name TEXT
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating users table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS wallets(
			id SERIAL PRIMARY KEY NOT NULL,
			owner TEXT NOT NULL REFERENCES users(id),
			name TEXT NOT NULL,
			is_eoa BOOLEAN NOT NULL,
			eoa_address TEXT NOT NULL,
			smart_address TEXT,
			smart_index INTEGER,
			UNIQUE (owner, is_eoa, eoa_address, smart_index)
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating wallets table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS locations (
    id SERIAL PRIMARY KEY,
    google_id TEXT,
    owner_id TEXT REFERENCES users(id),
    name TEXT,
    description TEXT,
    type TEXT,
    approval BOOLEAN,
    street TEXT,
    city TEXT,
    state TEXT,
    zip TEXT,
    lat NUMERIC,
    lng NUMERIC,
    phone TEXT,
    email TEXT,
    website TEXT,
    image_url TEXT,
    rating NUMERIC,
    maps_page TEXT,
    contact_firstname TEXT,
    contact_lastname TEXT,
    contact_phone TEXT,
    pos_system TEXT,
    sole_proprietorship TEXT,
    tipping_policy TEXT,
    tipping_division TEXT,
    table_coverage TEXT,
    service_stations INTEGER,
    tablet_model TEXT,
    messaging_service TEXT
);


	`)
	if err != nil {
		return fmt.Errorf("error creating locations table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS location_hours(
			location_id INTEGER REFERENCES locations(id),
			weekday INTEGER NOT NULL,
			hours TEXT
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating location_hours table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS contacts(
			id SERIAL PRIMARY KEY NOT NULL,
			owner TEXT NOT NULL REFERENCES users(id),
			name TEXT NOT NULL,
			address TEXT NOT NULL,
			is_favorite BOOLEAN NOT NULL DEFAULT FALSE,
			UNIQUE (owner, address),
			UNIQUE (owner, name)
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating contacts table: %s", err)
	}

	return nil
}
