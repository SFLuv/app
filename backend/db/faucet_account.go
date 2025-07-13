package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

type AccountDB struct {
	db *pgx.Conn
}

func Account(db *pgx.Conn) *AccountDB {
	return &AccountDB{db}
}

func (s *AccountDB) CreateTables() error {
	_, err := s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS accounts(
			address TEXT PRIMARY KEY NOT NULL,
			email TEXT,
			name TEXT
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating accounts table: %s", err)
		return err
	}

	return nil
}

func (s *AccountDB) NewAccount(account *structs.AccountRequest) error {

	_, err := s.db.Exec(context.Background(), `
		INSERT INTO accounts
			(address, email, name)
		VALUES
			($1, $2, $3);
	`, account.Address, account.Email, account.Name)

	return err
}

func (s *AccountDB) GetAccount(address string) bool {
	row := s.db.QueryRow(context.Background(), `
		SELECT * FROM accounts WHERE address = $1;
	`, address)

	err := row.Scan()

	return err != sql.ErrNoRows
}
