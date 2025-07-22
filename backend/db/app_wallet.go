package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func (a *AppDB) AddWallet(wallet *structs.Wallet) error {
	_, err := a.db.Exec(context.Background(), `
		INSERT INTO wallets (
			owner,
			name,
			is_eoa,
			eoa_address,
			smart_address,
			smart_index
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6
		)
		ON CONFLICT (owner, is_eoa, eoa_address, smart_index)
		DO NOTHING;
	`,
		wallet.Owner,
		wallet.Name,
		wallet.IsEoa,
		wallet.EoaAddress,
		wallet.SmartAddress,
		wallet.SmartIndex,
	)
	if err != nil {
		return err
	}

	return nil
}

func (a *AppDB) GetWalletsByUser(userId string) ([]*structs.Wallet, error) {
	var wallets []*structs.Wallet
	rows, err := a.db.Query(context.Background(), `
		SELECT
			wallets.id, wallets.owner, wallets.name, wallets.is_eoa, wallets.eoa_address, wallets.smart_address, wallets.smart_index
		FROM
			wallets JOIN users ON wallets.owner = users.id
		WHERE
			users.id = $1;
	`, userId)
	if err == pgx.ErrNoRows {
		return wallets, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error querying user wallets: %s", err)
	}

	var scanError error
	for rows.Next() {
		var wallet structs.Wallet
		err := rows.Scan(
			&wallet.Id,
			&wallet.Owner,
			&wallet.Name,
			&wallet.IsEoa,
			&wallet.EoaAddress,
			&wallet.SmartAddress,
			&wallet.SmartIndex,
		)
		if err != nil {
			scanError = err
			continue
		}

		wallets = append(wallets, &wallet)
	}
	if len(wallets) == 0 {
		return nil, fmt.Errorf("error while scanning all rows %s", scanError)
	}

	return wallets, nil
}
