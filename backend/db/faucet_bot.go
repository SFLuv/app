package db

import (
	"context"
	"fmt"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BotDB struct {
	db *pgxpool.Pool
}

func Bot(db *pgxpool.Pool) *BotDB {
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
			redeemed BOOLEAN DEFAULT FALSE,
			event TEXT,
			FOREIGN KEY (event) REFERENCES events(id)
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating codes table: %s", err)
		return err
	}

	// Redemptions Table (accounts - events join table)
	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS redemptions(
			id SERIAL PRIMARY KEY,
			address TEXT,
			code TEXT,
			FOREIGN KEY (code) REFERENCES codes(id)
		);

		CREATE INDEX redemption_address ON redemptions(address);
	`)
	if err != nil {
		err = fmt.Errorf("error creating redemptions table: %s", err)
		return err
	}

	return nil
}

func (s *BotDB) NewEvent(ctx context.Context, e *structs.Event) (string, error) {
	id := uuid.NewString()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO events
			(id, title, description, amount, expiration)
		VALUES
		 ($1, $2, $3, $4, $5);
	`, id, e.Title, e.Description, e.Amount, e.Expiration)
	if err != nil {
		tx.Rollback(ctx)
		err = fmt.Errorf("error inserting event object: %s", err)
		return "", err
	}

	for range e.Codes {
		codeId := uuid.NewString()

		_, err = tx.Exec(ctx, `
				INSERT INTO codes
					(id, event)
				VALUES
					($1, $2);
			`, codeId, id)
		if err != nil {
			err = fmt.Errorf("error inserting event codes: %s", err)
			tx.Rollback(ctx)
			return "", err
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		err = fmt.Errorf("error committing db transaction: %s", err)
		tx.Rollback(ctx)
		return "", err
	}

	return id, nil
}

func (s *BotDB) NewCode(ctx context.Context, code *structs.Code) (string, error) {
	id := uuid.NewString()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO codes
			(id, redeemed, event)
		VALUES
		 ($1, $2, $3);
	`, id, code.Redeemed, code.Event)
	if err != nil {
		tx.Rollback(ctx)
		err = fmt.Errorf("error inserting event object: %s", err)
		return "", err
	}

	err = tx.Commit(ctx)
	if err != nil {
		err = fmt.Errorf("error committing db transaction: %s", err)
		tx.Rollback(ctx)
		return "", err
	}

	return id, nil
}

func (s *BotDB) GetCodes(ctx context.Context, r *structs.CodesPageRequest) ([]*structs.Code, error) {
	offset := r.Page * r.Count

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
	defer rows.Close()

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

func (s *BotDB) NewCodes(ctx context.Context, r *structs.NewCodesRequest) ([]*structs.Code, error) {
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

func (s *BotDB) Redeem(ctx context.Context, id string, account string) (uint64, pgx.Tx, error) {
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
						WHERE address = $2
					)
			);
	`, id, account)

	var redeemed string
	err = row.Scan(&redeemed)
	if err != pgx.ErrNoRows {
		err = fmt.Errorf("user redeemed")
		tx.Rollback(context.Background())
		return 0, nil, err
	}

	time := time.Now().Unix()

	row = tx.QueryRow(ctx, `
		SELECT
			amount, expiration
		FROM events
		WHERE
			id = (
				SELECT
					(event)
				FROM codes
				WHERE
					(id = $1 AND redeemed = false)
			);
	`, id)

	var amount uint64
	var expiration int64
	err = row.Scan(&amount, &expiration)
	if err != nil {
		if err == pgx.ErrNoRows {
			err = fmt.Errorf("code redeemed")
		}
		tx.Rollback(ctx)
		return 0, nil, err
	}
	if expiration < time && expiration != 0 {
		err = fmt.Errorf("code expired")
		tx.Rollback(ctx)
		return 0, nil, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE codes
		SET redeemed = true
		WHERE id = $1;
	`, id)
	if err != nil {
		err = fmt.Errorf("error updating code redemption status: %s", err)
		tx.Rollback(ctx)
		return 0, nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO redemptions(address, code)
		VALUES ($1, $2);
	`, account, id)
	if err != nil {
		err = fmt.Errorf("error inserting code redemption: %s", err)
		tx.Rollback(ctx)
		return 0, nil, err
	}

	return amount, tx, nil
}
