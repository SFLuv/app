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
			id TEXT PRIMARY KEY NOT NULL,
			is_admin INTEGER NOT NULL DEFAULT 0,
			is_location INTEGER NOT NULL DEFAULT 0,
			is_organizer INTEGER NOT NULL DEFAULT 0,
			is_improver INTEGER NOT NULL DEFAULT 0,
			contact_email TEXT NOT NULL,
			contact_phone TEXT NOT NULL,
			contact_name TEXT NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating users table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS wallets(
			id INTEGER PRIMARY KEY NOT NULL,
			owner TEXT REFERENCES users(id),
			name TEXT,
			is_eoa INTEGER NOT NULL DEFAULT 0,
			eoa_address TEXT NOT NULL,
			smart_address TEXT,
			smart_index INTEGER
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating wallets table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS locations(
			id SERIAL PRIMARY KEY,
			google_id TEXT NOT NULL,
			owner_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			type TEXT NOT NULL,
			approval BOOLEAN NOT NULL DEFAULT FALSE,
			address TEXT NOT NULL,
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
			open_time TIME WITH TIME ZONE,
			close_time TIME WITH TIME ZONE
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating location_hours table: %s", err)
	}

	return nil
}
