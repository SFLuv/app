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
		CREATE TABLE IF NOT EXISTS locations(
			id SERIAL PRIMARY KEY,
			google_id TEXT NOT NULL,
			owner_id TEXT NOT NULL REFERENCES users(id),
			name TEXT NOT NULL,
			description TEXT,
			type TEXT NOT NULL,
			approval BOOLEAN NOT NULL DEFAULT FALSE,
			street TEXT NOT NULL,
			city TEXT NOT NULL,
			state TEXT NOT NULL,
			zip TEXT NOT NULL,
			lat NUMERIC NOT NULL,
			lng NUMERIC NOT NULL,
			phone TEXT NOT NULL,
			email TEXT NOT NULL,
			website TEXT,
			image_url TEXT,
			rating NUMERIC,
			maps_page TEXT NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating locations table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS location_hours(
			location_id INTEGER REFERENCES locations(id),
			weekday INTEGER NOT NULL,
			open_time NUMERIC,
			close_time NUMERIC
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
			is_favorite BOOLEAN NOT NULL DEFAULT FALSE
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating contacts table: %s", err)
	}

	return nil
}
