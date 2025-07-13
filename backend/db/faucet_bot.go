package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type BotDB struct {
	db *pgx.Conn
}

func Bot(db *pgx.Conn) *BotDB {
	return &BotDB{db}
}

func (s *BotDB) CreateTables() error {

	//	Event Table
	_, err := s.db.Exec(context.Background(), `
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
	_, err = s.db.Exec(context.Background(), `
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
	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS accounts(
			address TEXT PRIMARY KEY
		)
	`)
	if err != nil {
		err = fmt.Errorf("error creating accounts table: %s", err)
		return err
	}

	// Redemptions Table (accounts - events join table)
	_, err = s.db.Exec(context.Background(), `
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

	return nil
}

func (s *BotDB) NewEvent(e *structs.Event) (string, error) {
	id := uuid.NewString()

	tx, err := s.db.Begin(context.Background())
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(context.Background(), `
		INSERT INTO events
			(id, title, description, amount, expiration)
		VALUES
		 ($1, $2, $3, $4, $5);
	`, id, e.Title, e.Description, e.Amount, e.Expiration)
	if err != nil {
		tx.Rollback(context.Background())
		err = fmt.Errorf("error inserting event object: %s", err)
		return "", err
	}

	for range e.Codes {
		codeId := uuid.NewString()

		_, err = tx.Exec(context.Background(), `
				INSERT INTO codes
					(id, event)
				VALUES
					($1, $2);
			`, codeId, id)
		if err != nil {
			err = fmt.Errorf("error inserting event codes: %s", err)
			tx.Rollback(context.Background())
			return "", err
		}
	}

	err = tx.Commit(context.Background())
	if err != nil {
		err = fmt.Errorf("error committing db transaction: %s", err)
		tx.Rollback(context.Background())
		return "", err
	}

	return id, nil
}

func (s *BotDB) NewCode(code *structs.Code) (string, error) {
	id := uuid.NewString()

	tx, err := s.db.Begin(context.Background())
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(context.Background(), `
		INSERT INTO codes
			(id, redeemed, event)
		VALUES
		 ($1, $2, $3);
	`, id, code.Redeemed, code.Event)
	if err != nil {
		tx.Rollback(context.Background())
		err = fmt.Errorf("error inserting event object: %s", err)
		return "", err
	}

	err = tx.Commit(context.Background())
	if err != nil {
		err = fmt.Errorf("error committing db transaction: %s", err)
		tx.Rollback(context.Background())
		return "", err
	}

	return id, nil
}

func (s *BotDB) GetCodes(r *structs.CodesPageRequest) ([]*structs.Code, error) {
	offset := r.Page * r.Count

	fmt.Println(r)

	rows, err := s.db.Query(context.Background(), `
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

func (s *BotDB) NewCodes(r *structs.NewCodesRequest) ([]*structs.Code, error) {
	results := make([]*structs.Code, r.Count)

	tx, err := s.db.Begin(context.Background())
	if err != nil {
		return nil, err
	}

	for i := 0; i < int(r.Count); i++ {
		codeId := uuid.NewString()

		_, err = tx.Exec(context.Background(), `
			INSERT INTO codes
				(id, event)
			VALUES
				($1, $2);
		`, codeId, r.Event)
		if err != nil {
			err = fmt.Errorf("error inserting event codes: %s", err)
			tx.Rollback(context.Background())
			return nil, err
		}

		results[i] = &structs.Code{
			Id:       codeId,
			Redeemed: false,
			Event:    r.Event,
		}
	}

	err = tx.Commit(context.Background())
	if err != nil {
		err = fmt.Errorf("error committing db transaction: %s", err)
		tx.Rollback(context.Background())
		return nil, err
	}

	return results, nil
}

func (s *BotDB) Redeem(id string, account string) (uint64, pgx.Tx, error) {
	tx, err := s.db.Begin(context.Background())
	if err != nil {
		err = fmt.Errorf("error creating db tx: %s", err)
		return 0, nil, err
	}

	row := tx.QueryRow(context.Background(), `
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
		tx.Rollback(context.Background())
		return 0, nil, err
	}

	time := time.Now().Unix()

	row = tx.QueryRow(context.Background(), `
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
		tx.Rollback(context.Background())
		return 0, nil, err
	}
	fmt.Println(expiration, time)
	if expiration < time && expiration != 0 {
		err = fmt.Errorf("code expired")
		tx.Rollback(context.Background())
		return 0, nil, err
	}

	_, err = tx.Exec(context.Background(), `
		UPDATE codes
		SET redeemed = 1
		WHERE id = $1;
	`, id)
	if err != nil {
		err = fmt.Errorf("error updating code redemption status: %s", err)
		tx.Rollback(context.Background())
		return 0, nil, err
	}

	_, err = tx.Exec(context.Background(), `
		INSERT INTO redemptions(account, code)
		VALUES ($1, $2);
	`, account, id)
	if err != nil {
		err = fmt.Errorf("error inserting code redemption: %s", err)
		tx.Rollback(context.Background())
		return 0, nil, err
	}

	return amount, tx, nil
}
