package db

import (
	"context"
	"fmt"
	"strings"
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

func (s *BotDB) CreateTables(defaultOwner string) error {

	//	Event Table
	_, err := s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS events(
			id TEXT PRIMARY KEY NOT NULL,
			title TEXT,
			description TEXT,
			expiration INTEGER,
			amount INTEGER NOT NULL,
			owner TEXT
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating events table: %s", err)
		return err
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE events
		ADD COLUMN IF NOT EXISTS owner TEXT;

		CREATE INDEX IF NOT EXISTS events_owner_idx ON events(owner);
	`)
	if err != nil {
		err = fmt.Errorf("error altering events table for owner column: %s", err)
		return err
	}

	if defaultOwner != "" {
		escapedOwner := strings.ReplaceAll(defaultOwner, "'", "''")
		defaultQuery := fmt.Sprintf(
			"ALTER TABLE events ALTER COLUMN owner SET DEFAULT '%s';",
			escapedOwner,
		)
		_, err = s.db.Exec(context.Background(), defaultQuery)
		if err != nil {
			err = fmt.Errorf("error setting default event owner: %s", err)
			return err
		}

		_, err = s.db.Exec(context.Background(), `
			UPDATE events
			SET owner = $1
			WHERE owner IS NULL OR owner = '';
		`, defaultOwner)
		if err != nil {
			err = fmt.Errorf("error backfilling event owners: %s", err)
			return err
		}
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

		CREATE INDEX IF NOT EXISTS redemption_address ON redemptions(address);
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
			(id, title, description, amount, expiration, owner)
		VALUES
		 ($1, $2, $3, $4, $5, $6);
	`, id, e.Title, e.Description, e.Amount, e.Expiration, e.Owner)
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

func (s *BotDB) GetEvents(ctx context.Context, e *structs.EventsRequest) ([]*structs.Event, error) {
	offset := e.Page * e.Count

	rows, err := s.db.Query(ctx, `
		SELECT
			e.id,
			e.title,
			e.description,
			e.amount,
			e.expiration,
			e.owner,
			COUNT(c.id)
		FROM
			events e
		LEFT JOIN
			codes c
		ON
			e.id = c.event

		WHERE
			CASE
				WHEN ($1 AND e.expiration != 0) THEN e.expiration > EXTRACT(EPOCH from NOW())
				ELSE TRUE
			END
		AND
			(
				e.title ~* $2
				OR
				e.description ~* $3
			)
		GROUP BY
			e.id,
			e.owner
		ORDER BY
			e.expiration
		LIMIT $4
		OFFSET $5;
	`, !e.Expired, e.Search, e.Search, e.Count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying for events: %s", err)
	}

	events := []*structs.Event{}
	for rows.Next() {
		event := structs.Event{}
		err = rows.Scan(
			&event.Id,
			&event.Title,
			&event.Description,
			&event.Amount,
			&event.Expiration,
			&event.Owner,
			&event.Codes,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning event row: %s", err)
		}

		events = append(events, &event)
	}

	return events, nil
}

func (s *BotDB) GetEventsByOwner(ctx context.Context, e *structs.EventsRequest, owner string) ([]*structs.Event, error) {
	offset := e.Page * e.Count

	rows, err := s.db.Query(ctx, `
		SELECT
			e.id,
			e.title,
			e.description,
			e.amount,
			e.expiration,
			e.owner,
			COUNT(c.id)
		FROM
			events e
		LEFT JOIN
			codes c
		ON
			e.id = c.event
		WHERE
			e.owner = $1
		AND
			CASE
				WHEN ($2 AND e.expiration != 0) THEN e.expiration > EXTRACT(EPOCH from NOW())
				ELSE TRUE
			END
		AND
			(
				e.title ~* $3
				OR
				e.description ~* $4
			)
		GROUP BY
			e.id,
			e.owner
		ORDER BY
			e.expiration
		LIMIT $5
		OFFSET $6;
	`, owner, !e.Expired, e.Search, e.Search, e.Count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying for events: %s", err)
	}
	defer rows.Close()

	events := []*structs.Event{}
	for rows.Next() {
		event := structs.Event{}
		err = rows.Scan(
			&event.Id,
			&event.Title,
			&event.Description,
			&event.Amount,
			&event.Expiration,
			&event.Owner,
			&event.Codes,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning event row: %s", err)
		}

		events = append(events, &event)
	}

	return events, nil
}

func (s *BotDB) GetActiveEvents(ctx context.Context) ([]*structs.Event, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			id,
			expiration,
			owner
		FROM
			events
		WHERE
			expiration > EXTRACT(EPOCH from NOW());
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying for active events: %s", err)
	}
	defer rows.Close()

	events := []*structs.Event{}
	for rows.Next() {
		event := structs.Event{}
		err = rows.Scan(
			&event.Id,
			&event.Expiration,
			&event.Owner,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning active event row: %s", err)
		}

		events = append(events, &event)
	}

	return events, nil
}

func (s *BotDB) GetEventOwner(ctx context.Context, id string) (string, error) {
	row := s.db.QueryRow(ctx, `
		SELECT
			owner
		FROM
			events
		WHERE
			id = $1;
	`, id)
	var owner string
	err := row.Scan(&owner)
	if err != nil {
		return "", err
	}
	return owner, nil
}

func (s *BotDB) EventUnredeemedValue(ctx context.Context, id string) (uint64, error) {
	row := s.db.QueryRow(ctx, `
		SELECT COALESCE(
			(
				(
					SELECT
						COUNT(id)
					FROM
						codes
					WHERE
						event = e.id
				) - (
					SELECT
						COUNT(r.id)
					FROM
						redemptions r
					JOIN
						codes c
					ON
						r.code = c.id
					WHERE
						c.event = e.id
					)
				) * e.amount
			), 0)
		FROM
			events e
		WHERE
			e.id = $1;
	`, id)
	var value uint64
	err := row.Scan(&value)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func (s *BotDB) DeleteEvent(ctx context.Context, id string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error beginning delete event transaction: %s", err)
	}
	defer tx.Rollback(ctx)

	fmt.Println(id)

	_, err = tx.Exec(ctx, `
		DELETE FROM
			codes
		USING
			events
		WHERE
			codes.event = events.id
		AND
			events.id = $1;
	`, id)
	if err != nil {
		return fmt.Errorf("error deleting event %s codes: %s", id, err)
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM
			events
		WHERE
			id = $1;
	`, id)
	if err != nil {
		return fmt.Errorf("error deleting event %s: %s", id, err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("error committing delete event %s transaction: %s", id, err)
	}

	return nil
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
			event
		FROM
			codes c
		JOIN
			redemptions r
		ON
			c.id = r.code
		WHERE
			r.address = $1
		AND
			c.event = (
				SELECT
					event
				FROM
					codes
				WHERE
					id = $2
			);
	`, account, id)

	var redeemed string
	err = row.Scan(&redeemed)
	if err != pgx.ErrNoRows {
		fmt.Println(err)
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

func (s *BotDB) AllocatedBalance(ctx context.Context) (uint64, error) {
	row := s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(
			(
				(
					SELECT
						COUNT(id)
					FROM
						codes
					WHERE
						event = e.id
				) - (
					SELECT
						COUNT(r.id)
					FROM
						redemptions r
					JOIN
						codes c
					ON
						r.code = c.id
					WHERE
						c.event = e.id
				)
			) * (
			SELECT
				amount
			FROM
				events
			WHERE
				id = e.id
			)
		), 0)
		FROM
			events e
		WHERE
			e.expiration > EXTRACT(EPOCH from NOW());
	`)
	var allocated uint64
	err := row.Scan(&allocated)
	if err != nil {
		return 0, err
	}

	return allocated, nil
}

func (s *BotDB) AllocatedBalanceByOwner(ctx context.Context, owner string) (uint64, error) {
	row := s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(
			(
				(
					SELECT
						COUNT(id)
					FROM
						codes
					WHERE
						event = e.id
				) - (
					SELECT
						COUNT(r.id)
					FROM
						redemptions r
					JOIN
						codes c
					ON
						r.code = c.id
					WHERE
						c.event = e.id
					)
				) * (
			SELECT
				amount
			FROM
				events
			WHERE
				id = e.id
			)
		), 0)
		FROM
			events e
		WHERE
			e.expiration > EXTRACT(EPOCH from NOW())
		AND
			e.owner = $1;
	`, owner)
	var allocated uint64
	err := row.Scan(&allocated)
	if err != nil {
		return 0, err
	}

	return allocated, nil
}
