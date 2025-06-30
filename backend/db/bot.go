package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/faucet-portal/backend/structs"
	"github.com/google/uuid"
)

type BotDB struct {
	db *SFLuvDB
}

func Bot(db *SFLuvDB) *BotDB {
	return &BotDB{db}
}

func (s *BotDB) CreateTables() error {

	err := s.db.GetGormDB().AutoMigrate(&structs.Redemption{}, &structs.Account{}, &structs.Code{}, &structs.Event{})
	if err != nil {
		panic(err)
	}

	return nil
}

func (s *BotDB) NewEvent(e *structs.Event) (string, error) {
	id := uuid.NewString()

	e.UUID = id

	s.db.GetGormDB().Create(e)

	for range e.Codes {
		codeId := uuid.NewString()

		code := &structs.Code{
			UUID:     codeId,
			Redeemed: false,
			Event:    id,
		}
		s.db.GetGormDB().Create(code)
	}

	return id, nil
}

func (s *BotDB) NewCode(code *structs.Code) (string, error) {
	id := uuid.NewString()

	code.UUID = id

	result := s.db.GetGormDB().Create(code)
	if result.Error != nil {
		err := fmt.Errorf("error creating code: %s", result.Error)
		return "", err
	}

	return id, nil
}

func (s *BotDB) GetCodes(r *structs.CodesPageRequest) ([]*structs.Code, error) {
	offset := r.Page * r.Count

	fmt.Println(r)

	rows, err := s.db.GetDB().Query(`
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

		err = rows.Scan(&code.UUID, &code.Redeemed, &code.Event)
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

	tx, err := s.db.GetDB().Begin()
	if err != nil {
		return nil, err
	}

	for i := 0; i < int(r.Count); i++ {
		codeId := uuid.NewString()

		_, err = tx.Exec(`
			INSERT INTO codes
				(id, event)
			VALUES
				($1, $2);
		`, codeId, r.Event)
		if err != nil {
			err = fmt.Errorf("error inserting event codes: %s", err)
			tx.Rollback()
			return nil, err
		}

		results[i] = &structs.Code{
			UUID:     codeId,
			Redeemed: false,
			Event:    r.Event,
		}
	}

	err = tx.Commit()
	if err != nil {
		err = fmt.Errorf("error committing db transaction: %s", err)
		tx.Rollback()
		return nil, err
	}

	return results, nil
}

func (s *BotDB) Redeem(id string, account string) (uint64, *sql.Tx, error) {

	tx, err := s.db.GetDB().Begin()
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
				WHERE uuid = $1
			) = (
				SELECT
					(event)
				FROM codes
				WHERE
					uuid = (
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
			uuid = (
				SELECT
					(event)
				FROM codes
				WHERE
					(uuid = $1 AND not redeemed)
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
		SET redeemed = true
		WHERE uuid = $1;
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
