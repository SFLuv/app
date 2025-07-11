package db

import (
	"database/sql"
	"fmt"

	"github.com/faucet-portal/backend/structs"
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
			name TEXT,
			googleid TEXT,
			description TEXT,
			id INTEGER
		);
	`)
	if err != nil {
		err = fmt.Errorf("error creating merchants table: %s", err)
		fmt.Println(fmt.Sprintf("error creating tables: %v", err))
		return err
	}

	return nil
}


func (s *MerchantDB) GetMerchant(id uint64) structs.MerchantRequest {
	row := s.db.QueryRow(`
		SELECT name, googleid, description, id FROM merchants WHERE id = $1
	`, id)

	merchant := structs.MerchantRequest{}
	err := row.Scan(&merchant.Name, &merchant.GoogleID, &merchant.Description, &merchant.ID)
	if err != nil {
        fmt.Printf("error scanning merchant data: %s\n", err)
    }

	return merchant
}

func (s *MerchantDB) GetMerchants() []structs.MerchantRequest {
	rows, err := s.db.Query(`
    	SELECT name, googleid, description, id FROM merchants
	`)
	if err != nil {
    	fmt.Println("error querying merchants: %w", err)
	}
	defer rows.Close()

	merchants := []structs.MerchantRequest{}
	for rows.Next() {
		merchant := structs.MerchantRequest{}
		err := rows.Scan(&merchant.Name, &merchant.GoogleID, &merchant.Description, &merchant.ID)
		if err != nil {
        fmt.Printf("error scanning merchant data: %s\n", err)
    }
		merchants = append(merchants, merchant)
	}

	return merchants
}

func (s *MerchantDB) AddMerchant(merchant *structs.MerchantRequest) error {
	_, err := s.db.Exec(`
		INSERT INTO merchants
		(name, googleid, description, id)
	VALUES
		($1, $2, $3, $4);
		`, merchant.Name, merchant.GoogleID, merchant.Description, merchant.ID)
	return err
}
