package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) GetMerchant(id uint64) (*structs.MerchantRequest, error) {
	row := a.db.QueryRow(context.Background(), `
		SELECT name, googleid, description, id FROM merchants WHERE id = $1
	`, id)

	merchant := structs.MerchantRequest{}
	err := row.Scan(&merchant.Name, &merchant.GoogleID, &merchant.Description, &merchant.ID)
	if err != nil {
		return nil, fmt.Errorf("error scanning merchant data: %s", err)
	}

	return &merchant, nil
}

func (a *AppDB) GetMerchants() (*[]structs.MerchantRequest, error) {
	rows, err := a.db.Query(context.Background(), `
    	SELECT name, googleid, description, id FROM merchants
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying merchants: %w", err)
	}
	defer rows.Close()

	merchants := []structs.MerchantRequest{}
	for rows.Next() {
		merchant := structs.MerchantRequest{}
		err := rows.Scan(&merchant.Name, &merchant.GoogleID, &merchant.Description, &merchant.ID)
		if err != nil {
			return nil, fmt.Errorf("error scanning merchant data: %s", err)
		}
		merchants = append(merchants, merchant)
	}

	return &merchants, nil
}

func (a *AppDB) AddMerchant(merchant *structs.MerchantRequest) error {
	_, err := a.db.Exec(context.Background(), `
		INSERT INTO merchants
			(name, googleid, description, id)
		VALUES
			($1, $2, $3, $4);
		`, merchant.Name, merchant.GoogleID, merchant.Description, merchant.ID)
	return err
}
