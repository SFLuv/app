package db

import (
	"database/sql"
	"fmt"

	"github.com/faucet-portal/backend/structs"
	"github.com/google/uuid"
)

type BotDB struct {
	db *sql.DB
}

func Bot(db *sql.DB) *BotDB {
	return &BotDB{db}
}

func (s *BotDB) CreateTables() error {

	//	Event Table
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS events(
			id TEXT PRIMARY KEY NOT NULL,
			title TEXT,
			description TEXT,
			expiration INTEGER,
			amount INTEGER NOT NULL
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating events table: %s", err)
		return err
	}

	// Codes Table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS codes(
			id TEXT PRIMARY KEY NOT NULL,
			redeemed INTEGER NOT NULL DEFAULT 0,
			event TEXT,
			FOREIGN KEY (event) REFERENCES events(id)
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating codes table: %s", err)
		return err
	}

	// Accounts Table (for foreign key lookup in redemptions)
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS accounts(
			address TEXT PRIMARY KEY
		)
	`)
	if err != nil {
		err = fmt.Errorf("error creating accounts table: %s", err)
		return err
	}

	// Redemptions Table (accounts - events join table)
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS redemptions(
			account TEXT,
			event TEXT,
			code TEXT,
			FOREIGN KEY (account) REFERENCES accounts(address),
			FOREIGN KEY (event) REFERENCES events(id),
			FOREIGN KEY (code) REFERENCES codes(id)
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating redemptions table: %s", err)
		return err
	}

	return nil
}

func (s *BotDB) NewEvent(e *structs.Event) (string, error) {
	id := uuid.NewString()

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(`
		INSERT INTO events
			(id, title, description, amount, expiration)
		VALUES
		 ($1, $2, $3, $4, $5);
	`, id, e.Title, e.Description, e.Amount, e.Expiration)
	if err != nil {
		tx.Rollback()
		err = fmt.Errorf("error inserting event object: %s", err)
		return "", err
	}

	for range e.Codes {
		codeId := uuid.NewString()

		_, err = tx.Exec(`
				INSERT INTO codes
					(id, event)
				VALUES
					($1, $2);
			`, codeId, id)
		if err != nil {
			err = fmt.Errorf("error inserting event codes: %s", err)
			tx.Rollback()
			return "", err
		}
	}

	err = tx.Commit()
	if err != nil {
		err = fmt.Errorf("error committing db transaction: %s", err)
		tx.Rollback()
		return "", err
	}

	return id, nil
}

func (s *BotDB) GetCodes(r *structs.CodesPageRequest) ([]*structs.Code, error) {
	offset := r.Page * r.Count

	rows, err := s.db.Query(`
		SELECT
			(id, redeemed, event)
		FROM codes
		WHERE event = $1
		LIMIT $2
		OFFSET $3
	`, r.Event, r.Count, offset)
	if err != nil {
		err = fmt.Errorf("error querying for event codes: %s", err)
		return nil, err
	}

	codes := []*structs.Code{}

	for rows.Next() {
		code := structs.Code{}

		err = rows.Scan(&code.Id, &code.Redeemed, &code.Event)
		if err != nil {
			err = fmt.Errorf("error unpacking event codes: %s", err)
			return nil, err
		}

		codes = append(codes, &code)
	}

	return codes, nil
}

func (s *BotDB) Redeem(id string) (uint64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		err = fmt.Errorf("error creating db tx: %s", err)
		return 0, err
	}

	row := tx.QueryRow(`
		SELECT
			(amount)
		FROM events
		WHERE
			id = (
				SELECT
					(event)
				FROM codes
				WHERE
					(id = $1 AND redeemed = 0)
			);
	`, id)

	var amount uint64
	err = row.Scan(&amount)
	if err != nil {
		err = fmt.Errorf("error getting code redemption amount: %s", err)
		tx.Rollback()
		return 0, err
	}

	_, err = tx.Exec(`
		UPDATE codes
		SET redeemed = 1
		WHERE id = $1;
	`, id)
	if err != nil {
		err = fmt.Errorf("error updating code redemption status: %s", err)
		tx.Rollback()
		return 0, err
	}

	err = tx.Commit()
	if err != nil {
		err = fmt.Errorf("error committing code redemption: %s", err)
		return 0, err
	}

	return amount, nil
}
