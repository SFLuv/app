package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
				start_at INTEGER NOT NULL DEFAULT 0,
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

			ALTER TABLE events
			ADD COLUMN IF NOT EXISTS start_at INTEGER NOT NULL DEFAULT 0;

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

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE redemptions
		ADD COLUMN IF NOT EXISTS event TEXT;

		UPDATE redemptions r
		SET event = c.event
		FROM codes c
		WHERE r.code = c.id
		AND (r.event IS NULL OR r.event = '');

		UPDATE redemptions
		SET address = LOWER(TRIM(address))
		WHERE address IS NOT NULL;

		DELETE FROM redemptions
		WHERE address IS NULL
		OR address = ''
		OR code IS NULL
		OR code = ''
		OR event IS NULL
		OR event = '';

		DELETE FROM redemptions r
		USING redemptions dup
		WHERE r.code = dup.code
		AND r.id > dup.id;

		DELETE FROM redemptions r
		USING redemptions dup
		WHERE r.id > dup.id
		AND r.address = dup.address
		AND r.event = dup.event;

		ALTER TABLE redemptions
		ALTER COLUMN address SET NOT NULL;

		ALTER TABLE redemptions
		ALTER COLUMN code SET NOT NULL;

		ALTER TABLE redemptions
		ALTER COLUMN event SET NOT NULL;

		CREATE INDEX IF NOT EXISTS redemption_event_idx ON redemptions(event);
		CREATE INDEX IF NOT EXISTS redemption_address_event_idx ON redemptions(address, event);
		CREATE UNIQUE INDEX IF NOT EXISTS redemptions_code_unique_idx ON redemptions(code);
		CREATE UNIQUE INDEX IF NOT EXISTS redemptions_address_event_unique_idx ON redemptions(address, event);
	`)
	if err != nil {
		err = fmt.Errorf("error migrating redemptions table constraints: %s", err)
		return err
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE codes
		ALTER COLUMN redeemed DROP DEFAULT;

		ALTER TABLE codes
		ALTER COLUMN redeemed TYPE BOOLEAN
		USING CASE
			WHEN redeemed IS NULL THEN FALSE
			WHEN redeemed::text ~ '^-?[0-9]+$' THEN (redeemed::text)::bigint <> 0
			WHEN LOWER(redeemed::text) IN ('t', 'true', 'y', 'yes', 'on') THEN TRUE
			ELSE FALSE
		END;

		ALTER TABLE codes
		ALTER COLUMN redeemed SET DEFAULT FALSE;
	`)
	if err != nil {
		err = fmt.Errorf("error normalizing codes.redeemed column type: %s", err)
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
			(id, title, description, amount, start_at, expiration, owner)
		VALUES
		 ($1, $2, $3, $4, $5, $6, $7);
	`, id, e.Title, e.Description, e.Amount, e.StartAt, e.Expiration, e.Owner)
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
	if e.Count <= 0 {
		e.Count = 10
	}
	if e.Count > 100 {
		e.Count = 100
	}
	if e.Page < 0 {
		e.Page = 0
	}
	offset := e.Page * e.Count
	likeSearch := "%" + strings.TrimSpace(e.Search) + "%"

	rows, err := s.db.Query(ctx, `
		SELECT
			e.id,
			e.title,
			e.description,
			e.amount,
			e.start_at,
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
				COALESCE(e.title, '') ILIKE $2
				OR
				COALESCE(e.description, '') ILIKE $2
			)
		GROUP BY
			e.id,
			e.owner
		ORDER BY
			e.expiration ASC,
			e.id ASC
		LIMIT $3
		OFFSET $4;
	`, !e.Expired, likeSearch, e.Count, offset)
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
			&event.StartAt,
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
	if e.Count <= 0 {
		e.Count = 10
	}
	if e.Count > 100 {
		e.Count = 100
	}
	if e.Page < 0 {
		e.Page = 0
	}
	offset := e.Page * e.Count
	likeSearch := "%" + strings.TrimSpace(e.Search) + "%"

	rows, err := s.db.Query(ctx, `
		SELECT
			e.id,
			e.title,
			e.description,
			e.amount,
			e.start_at,
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
				COALESCE(e.title, '') ILIKE $3
				OR
				COALESCE(e.description, '') ILIKE $3
			)
		GROUP BY
			e.id,
			e.owner
		ORDER BY
			e.expiration ASC,
			e.id ASC
		LIMIT $4
		OFFSET $5;
	`, owner, !e.Expired, likeSearch, e.Count, offset)
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
			&event.StartAt,
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
						COUNT(*)
					FROM
						codes
					WHERE
						event = $1
				) - (
					SELECT
						COUNT(*)
					FROM
						redemptions r
					JOIN
						codes c
					ON
						r.code = c.id
					WHERE
						c.event = $1
				)
			) * COALESCE((
				SELECT
					amount
				FROM
					events
				WHERE
					id = $1
			), 0),
			0
		);
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
	if r.Count == 0 {
		r.Count = 100
	}
	if r.Count > 200 {
		r.Count = 200
	}
	offset := r.Page * r.Count

	rows, err := s.db.Query(ctx, `
			SELECT
				id,
				CASE
					WHEN redeemed IS NULL THEN false
					WHEN redeemed::text ~ '^-?[0-9]+$' THEN (redeemed::text)::bigint <> 0
					WHEN LOWER(redeemed::text) IN ('t', 'true', 'y', 'yes', 'on') THEN true
					ELSE false
				END AS redeemed,
				event
		FROM codes
		WHERE event = $1
		ORDER BY id ASC
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

func (s *BotDB) Redeem(ctx context.Context, id string, account string) (uint64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		err = fmt.Errorf("error creating db tx: %s", err)
		return 0, err
	}
	defer tx.Rollback(context.Background())

	row := tx.QueryRow(ctx, `
		SELECT
			c.event,
			c.redeemed,
			e.amount,
			e.start_at,
			e.expiration
		FROM
			codes c
		JOIN
			events e
		ON
			e.id = c.event
		WHERE
			c.id = $1
		FOR UPDATE;
	`, id)

	var eventID string
	var codeRedeemed bool
	var amount uint64
	var startAt int64
	var expiration int64
	err = row.Scan(&eventID, &codeRedeemed, &amount, &startAt, &expiration)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("code redeemed")
		}
		return 0, fmt.Errorf("error loading redemption code: %w", err)
	}
	if codeRedeemed {
		return 0, fmt.Errorf("code redeemed")
	}

	currentTime := time.Now().Unix()
	if startAt > currentTime && startAt != 0 {
		return 0, fmt.Errorf("code not started")
	}
	if expiration < currentTime && expiration != 0 {
		return 0, fmt.Errorf("code expired")
	}

	tag, err := tx.Exec(ctx, `
		UPDATE codes
		SET redeemed = true
		WHERE id = $1
		AND redeemed = false;
	`, id)
	if err != nil {
		return 0, fmt.Errorf("error updating code redemption status: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return 0, fmt.Errorf("code redeemed")
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO redemptions(address, code, event)
		VALUES ($1, $2, $3);
	`, account, id, eventID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "redemptions_address_event_unique_idx":
				return 0, fmt.Errorf("user redeemed")
			case "redemptions_code_unique_idx":
				return 0, fmt.Errorf("code redeemed")
			}
		}
		return 0, fmt.Errorf("error inserting code redemption: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("error committing code redemption: %w", err)
	}

	return amount, nil
}

func (s *BotDB) GetCodeAmount(ctx context.Context, id string) (uint64, error) {
	row := s.db.QueryRow(ctx, `
		SELECT
			e.amount
		FROM
			codes c
		JOIN
			events e
		ON
			e.id = c.event
		WHERE
			c.id = $1
		AND
			c.redeemed = false;
	`, id)

	var amount uint64
	if err := row.Scan(&amount); err != nil {
		return 0, err
	}

	return amount, nil
}

func (s *BotDB) UndoRedeem(ctx context.Context, id string, account string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error creating db tx for redeem undo: %w", err)
	}
	defer tx.Rollback(context.Background())

	row := tx.QueryRow(ctx, `
		SELECT
			redeemed
		FROM
			codes
		WHERE
			id = $1
		FOR UPDATE;
	`, id)

	var redeemed bool
	if err := row.Scan(&redeemed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("error locking code for redeem undo: %w", err)
	}
	if !redeemed {
		return nil
	}

	tag, err := tx.Exec(ctx, `
		DELETE FROM
			redemptions
		WHERE
			code = $1
		AND
			address = $2;
	`, id, account)
	if err != nil {
		return fmt.Errorf("error deleting redemption during undo: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("no matching redemption found to undo for code %s", id)
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			codes
		SET
			redeemed = false
		WHERE
			id = $1
		AND
			NOT EXISTS (
				SELECT
					1
				FROM
					redemptions
				WHERE
					code = $1
			);
	`, id)
	if err != nil {
		return fmt.Errorf("error updating code status during redeem undo: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing redeem undo: %w", err)
	}

	return nil
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
