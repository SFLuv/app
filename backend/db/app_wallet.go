package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) AddWallet(ctx context.Context, wallet *structs.Wallet) (int, error) {
	row := a.db.QueryRow(ctx, `
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
		DO NOTHING
		RETURNING id;
	`,
		wallet.Owner,
		wallet.Name,
		wallet.IsEoa,
		wallet.EoaAddress,
		wallet.SmartAddress,
		wallet.SmartIndex,
	)

	var id int
	err := row.Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (a *AppDB) GetWalletsByUser(ctx context.Context, userId string) ([]*structs.Wallet, error) {
	rows, err := a.db.Query(ctx, `
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
	defer rows.Close()

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

func (a *AppDB) UpdateWallet(ctx context.Context, wallet *structs.Wallet) error {
	_, err := a.db.Exec(ctx, `
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
