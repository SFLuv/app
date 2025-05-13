package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/faucet-portal/backend/structs"
	"github.com/google/uuid"
)

type BotDB struct {
	db *sql.DB
}

func Bot(db *sql.DB) *BotDB {
	return &BotDB{db}
}

func (s *BotDB) Begin() (*sql.Tx, error) {
	return s.db.Begin()
}

func (s *BotDB) CreateTables() error {

	//	Event Table
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS events(
			id TEXT PRIMARY KEY NOT NULL,
			title TEXT,
			description TEXT,
			expiration INTEGER,
			amount INTEGER NOT NULL,
			creator TEXT
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
			code TEXT,
			FOREIGN KEY (account) REFERENCES accounts(address),
			FOREIGN KEY (code) REFERENCES codes(id)
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating redemptions table: %s", err)
		return err
	}

	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS admins(
			key TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL,
			limit_amount INTEGER,
			limit_refresh INTEGER,
			current_balance INTEGER,
			last_refresh INTEGER
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating admins table: %s", err)
		return err
	}

	return nil
}

func (s *BotDB) UpdateTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS admins(
			key TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL,
			limit_amount INTEGER,
			limit_refresh INTEGER,
			current_balance INTEGER,
			last_refresh INTEGER
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating admins table: %s", err)
		return err
	}

	_, err = s.db.Exec(`
		ALTER TABLE events
			ADD creator TEXT;
	`)
	if err != nil {
		return fmt.Errorf("error updating tables: %s", err)
	}

	return nil
}

func (s *BotDB) NewEvent(tx *sql.Tx, e *structs.Event) (*sql.Tx, string, error) {
	id := uuid.NewString()

	_, err := tx.Exec(`
		INSERT INTO events
			(id, title, description, amount, expiration, creator)
		VALUES
		 ($1, $2, $3, $4, $5, $6);
	`, id, e.Title, e.Description, e.Amount, e.Expiration, e.Creator)
	if err != nil {
		err = fmt.Errorf("error inserting event object: %s", err)
		return tx, "", err
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
			return tx, "", err
		}
	}

	err = tx.Commit()
	if err != nil {
		err = fmt.Errorf("error committing db transaction: %s", err)
		tx.Rollback()
		return tx, "", err
	}

	return tx, id, nil
}

func (s *BotDB) GetCodes(r *structs.CodesPageRequest) ([]*structs.Code, error) {
	offset := r.Page * r.Count

	fmt.Println(r)

	rows, err := s.db.Query(`
		SELECT
			id, redeemed, event
		FROM codes
		WHERE event = $1
		LIMIT $2
		OFFSET $3;
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

func (s *BotDB) Redeem(id string, account string) (uint64, *sql.Tx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		err = fmt.Errorf("error creating db tx: %s", err)
		return 0, nil, err
	}

	row := tx.QueryRow(`
		SELECT
			id
		FROM events
		WHERE
			(
				SELECT (event)
				FROM codes
				WHERE id = $1
			) = (
				SELECT
					(event)
				FROM codes
				WHERE
					id = (
						SELECT
							(code)
						FROM redemptions
						WHERE account = $2
					)
			);
	`, id, account)

	var redeemed string
	err = row.Scan(&redeemed)
	if err != sql.ErrNoRows {
		err = fmt.Errorf("user redeemed")
		tx.Rollback()
		return 0, nil, err
	}

	time := time.Now().Unix()

	row = tx.QueryRow(`
		SELECT
			amount, expiration
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
	var expiration int64
	err = row.Scan(&amount, &expiration)
	if err != nil {
		if err == sql.ErrNoRows {
			err = fmt.Errorf("code redeemed")
		}
		tx.Rollback()
		return 0, nil, err
	}
	fmt.Println(expiration, time)
	if expiration < time && expiration != 0 {
		err = fmt.Errorf("code expired")
		tx.Rollback()
		return 0, nil, err
	}

	_, err = tx.Exec(`
		UPDATE codes
		SET redeemed = 1
		WHERE id = $1;
	`, id)
	if err != nil {
		err = fmt.Errorf("error updating code redemption status: %s", err)
		tx.Rollback()
		return 0, nil, err
	}

	_, err = tx.Exec(`
		INSERT INTO redemptions(account, code)
		VALUES ($1, $2);
	`, account, id)
	if err != nil {
		err = fmt.Errorf("error inserting code redemption: %s", err)
		tx.Rollback()
		return 0, nil, err
	}

	return amount, tx, nil
}

func (s *BotDB) NewAdmin(key string, name string, limit int, refresh int, refreshStart int) error {
	if refreshStart == 0 {
		refreshStart = int(time.Now().Unix())
	}

	_, err := s.db.Exec(`
		INSERT INTO admins(key, name, limit_amount, limit_refresh, current_balance, last_refresh)
		VALUES($1, $2, $3, $4, $5, $6);
	`, key, name, limit, refresh, 0, refreshStart)

	if err != nil {
		return fmt.Errorf("error inserting admin: %s", err)
	}

	return nil
}

func (s *BotDB) GetAdmin(key string) (*structs.Admin, error) {
	fmt.Println(key)
	a := new(structs.Admin)
	row := s.db.QueryRow(`
		SELECT key, name, limit_amount, limit_refresh, current_balance, last_refresh
		FROM admins
		WHERE key = $1;
	`, key)
	err := row.Scan(&a.Key, &a.Name, &a.Limit, &a.Refresh, &a.Balance, &a.LastRefresh)
	if err != nil {
		return nil, fmt.Errorf("error getting admin: %s", err)
	}

	return a, nil
}

func (s *BotDB) UpdateAdmin(tx *sql.Tx, admin *structs.Admin) (*sql.Tx, error) {
	_, err := tx.Exec(`
		UPDATE admins
		SET
			name = $1,
			limit_amount = $2,
			limit_refresh = $3,
			current_balance = $4,
			last_refresh = $5
		WHERE
			key = $6;
	`, &admin.Name, &admin.Limit, &admin.Refresh, &admin.Balance, &admin.LastRefresh, &admin.Key)
	if err != nil {
		return tx, fmt.Errorf("error updating admin %s: %s", admin.Name, err)
	}

	return tx, nil
}
