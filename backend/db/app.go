package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/logger"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AppDB struct {
	db     *pgxpool.Pool
	logger *logger.LogCloser
}

func App(db *pgxpool.Pool, logger *logger.LogCloser) *AppDB {
	return &AppDB{db, logger}
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
			contact_name TEXT,
			paypal_eth TEXT NOT NULL DEFAULT '',
			last_redemption INTEGER NOT NULL DEFAULT 0
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
				admin_phone TEXT,
				admin_email TEXT,
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
				messaging_service TEXT,
				reference TEXT,
				UNIQUE (google_id)
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

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS ponder_subscriptions(
			id INTEGER PRIMARY KEY,
			address TEXT NOT NULL,
			type TEXT NOT NULL,
			owner TEXT NOT NULL REFERENCES users(id),
			data TEXT
		);

		CREATE INDEX IF NOT EXISTS ponder_subscription_address ON ponder_subscriptions(address);
	`)
	if err != nil {
		return fmt.Errorf("error creating ponder subscriptions table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS w9_wallet_earnings(
			wallet_address TEXT NOT NULL,
			year INTEGER NOT NULL,
			amount_received NUMERIC(78, 0) NOT NULL DEFAULT 0,
			user_id TEXT,
			w9_required BOOLEAN NOT NULL DEFAULT false,
			w9_required_at TIMESTAMP,
			last_tx_hash TEXT,
			last_tx_timestamp INTEGER,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (wallet_address, year)
		);

		CREATE INDEX IF NOT EXISTS w9_wallet_earnings_user_id_idx ON w9_wallet_earnings(user_id);
		CREATE INDEX IF NOT EXISTS w9_wallet_earnings_year_idx ON w9_wallet_earnings(year);
		CREATE INDEX IF NOT EXISTS w9_wallet_earnings_required_idx ON w9_wallet_earnings(w9_required);
	`)
	if err != nil {
		return fmt.Errorf("error creating w9 wallet earnings table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS w9_submissions(
			id SERIAL PRIMARY KEY,
			wallet_address TEXT NOT NULL,
			year INTEGER NOT NULL,
			email TEXT NOT NULL,
			submitted_at TIMESTAMP NOT NULL DEFAULT NOW(),
			pending_approval BOOLEAN NOT NULL DEFAULT TRUE,
			approved_at TIMESTAMP NULL,
			approved_by_user_id TEXT NULL,
			rejected_at TIMESTAMP NULL,
			rejected_by_user_id TEXT NULL,
			rejection_reason TEXT NULL,
			w9_url TEXT NULL,
			UNIQUE (wallet_address, year)
		);

		ALTER TABLE w9_submissions ADD COLUMN IF NOT EXISTS rejected_at TIMESTAMP NULL;
		ALTER TABLE w9_submissions ADD COLUMN IF NOT EXISTS rejected_by_user_id TEXT NULL;
		ALTER TABLE w9_submissions ADD COLUMN IF NOT EXISTS rejection_reason TEXT NULL;

		CREATE INDEX IF NOT EXISTS w9_submissions_pending_idx ON w9_submissions(pending_approval);
		CREATE INDEX IF NOT EXISTS w9_submissions_year_idx ON w9_submissions(year);
	`)
	if err != nil {
		return fmt.Errorf("error creating w9 submissions table: %s", err)
	}

	return nil
}
