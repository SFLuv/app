package db

import (
	"database/sql"
	"fmt"
)

type MerchantDB struct {
	db *sql.DB
}

func Merchant(db *sql.DB) *MerchantDB {
	return &MerchantDB{db}
}

func (s *MerchantDB) CreateTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS merchants(
			name TEXT PRIMARY KEY NOT NULL,
			googleid TEXT
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating merchants table: %s", err)
		fmt.Println(fmt.Sprintf("error creating tables: %v", err))
		return err
	}

	fmt.Println("tables created")

	return nil
}


func (s *MerchantDB) GetMerchant(name string) bool {
	fmt.Println("Merchant got")
	return true
}

func (s *MerchantDB) AddMerchant(name string) bool {
	fmt.Println("Merchant added")
	return true
}
