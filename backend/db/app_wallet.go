package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
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
		wallets.id, wallets.owner, wallets.name, wallets.is_eoa, wallets.is_redeemer, wallets.eoa_address, wallets.smart_address, wallets.smart_index
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
			&wallet.IsRedeemer,
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

func (a *AppDB) GetWalletByUserAndAddress(ctx context.Context, userId string, address string) (*structs.Wallet, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id,
			owner,
			name,
			is_eoa,
			is_redeemer,
			eoa_address,
			smart_address,
			smart_index
		FROM
			wallets
		WHERE
			owner = $1
		AND (
			LOWER(smart_address) = LOWER($2)
			OR (
				LOWER(eoa_address) = LOWER($3)
				AND
				smart_address IS NULL
			)
		);
	`, userId, address, address)

	var w structs.Wallet
	err := row.Scan(
		&w.Id,
		&w.Owner,
		&w.Name,
		&w.IsEoa,
		&w.IsRedeemer,
		&w.EoaAddress,
		&w.SmartAddress,
		&w.SmartIndex,
	)
	if err != nil {
		return nil, err
	}

	return &w, nil
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

func (a *AppDB) UserHasRedeemerWallet(ctx context.Context, userId string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1
				FROM wallets
				WHERE owner = $1
				AND is_redeemer = TRUE
			);
	`, userId)

	var hasRedeemer bool
	if err := row.Scan(&hasRedeemer); err != nil {
		return false, err
	}

	return hasRedeemer, nil
}

func (a *AppDB) GetSmartWalletByOwnerIndex(ctx context.Context, owner string, index int) (*structs.Wallet, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id,
			owner,
			name,
			is_eoa,
			is_redeemer,
			eoa_address,
			smart_address,
			smart_index
		FROM
			wallets
		WHERE
			owner = $1
		AND
			is_eoa = FALSE
		AND
			smart_index = $2
		ORDER BY id
		LIMIT 1;
	`, owner, index)

	var wallet structs.Wallet
	var smartAddress sql.NullString
	var smartIndex sql.NullInt64
	err := row.Scan(
		&wallet.Id,
		&wallet.Owner,
		&wallet.Name,
		&wallet.IsEoa,
		&wallet.IsRedeemer,
		&wallet.EoaAddress,
		&smartAddress,
		&smartIndex,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if smartAddress.Valid {
		addr := smartAddress.String
		wallet.SmartAddress = &addr
	}
	if smartIndex.Valid {
		idx := int(smartIndex.Int64)
		wallet.SmartIndex = &idx
	}

	return &wallet, nil
}

func (a *AppDB) SetWalletRedeemerStatus(ctx context.Context, walletId int, isRedeemer bool) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			wallets
		SET
			is_redeemer = $1
		WHERE
			id = $2;
	`, isRedeemer, walletId)
	return err
}
