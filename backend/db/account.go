package db

import (
	"database/sql"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

type AccountDB struct {
	db *sql.DB
}

func Account(db *sql.DB) *AccountDB {
	return &AccountDB{db}
}

func (s *AccountDB) CreateTables() error {
	_, err := s.db.Exec(`
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

	_, err := s.db.Exec(`
		INSERT INTO accounts
			(address, email, name)
		VALUES
			($1, $2, $3);
	`, account.Address, account.Email, account.Name)

	return err
}

func (s *AccountDB) GetAccount(address string) bool {
	row := s.db.QueryRow(`
		SELECT * FROM accounts WHERE address = $1;
	`, address)

	err := row.Scan()

	return err != sql.ErrNoRows
}
