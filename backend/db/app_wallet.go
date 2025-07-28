package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
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
	rows, err := a.db.Query(context.Background(), `
	SELECT
		wallets.id, wallets.owner, wallets.name, wallets.is_eoa, wallets.eoa_address, wallets.smart_address, wallets.smart_index
	FROM
		wallets JOIN users ON wallets.owner = users.id
	WHERE
		users.id = $1;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error querying user wallets: %s", err)
	}

	wallets := []*structs.Wallet{}
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
			continue
		}

		wallets = append(wallets, &wallet)
	}

	return wallets, nil
}

func (a *AppDB) UpdateWallet(wallet *structs.Wallet) error {
	_, err := a.db.Exec(context.Background(), `
		UPDATE
			wallets
		SET
			name = $1
		WHERE
			(id = $2 AND owner = $3);
	`, wallet.Name, *wallet.Id, wallet.Owner)
	if err != nil {
		return err
	}

	return nil
}
