package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func (a *AppDB) AddWallet(ctx context.Context, wallet *structs.Wallet) (int, error) {
	row := a.db.QueryRow(ctx, `
		INSERT INTO wallets (
			owner,
			name,
			is_eoa,
			is_hidden,
			eoa_address,
			smart_address,
			smart_index
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7
		)
		ON CONFLICT (owner, is_eoa, eoa_address, smart_index)
		DO UPDATE
		SET
			name = wallets.name
		RETURNING id;
	`,
		wallet.Owner,
		wallet.Name,
		wallet.IsEoa,
		wallet.IsHidden,
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
		wallets.id, wallets.owner, wallets.name, wallets.is_eoa, wallets.is_hidden, wallets.is_redeemer, wallets.is_minter, wallets.eoa_address, wallets.smart_address, wallets.smart_index, wallets.last_unwrap_at
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
		var smartAddress sql.NullString
		var smartIndex sql.NullInt64
		var lastUnwrapAt sql.NullTime
		err := rows.Scan(
			&wallet.Id,
			&wallet.Owner,
			&wallet.Name,
			&wallet.IsEoa,
			&wallet.IsHidden,
			&wallet.IsRedeemer,
			&wallet.IsMinter,
			&wallet.EoaAddress,
			&smartAddress,
			&smartIndex,
			&lastUnwrapAt,
		)
		if err != nil {
			continue
		}
		if smartAddress.Valid {
			addr := smartAddress.String
			wallet.SmartAddress = &addr
		}
		if smartIndex.Valid {
			idx := int(smartIndex.Int64)
			wallet.SmartIndex = &idx
		}
		if lastUnwrapAt.Valid {
			unwrapAt := lastUnwrapAt.Time
			wallet.LastUnwrapAt = &unwrapAt
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
			is_hidden,
			is_redeemer,
			is_minter,
			eoa_address,
			smart_address,
			smart_index,
			last_unwrap_at
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
	var smartAddress sql.NullString
	var smartIndex sql.NullInt64
	var lastUnwrapAt sql.NullTime
	err := row.Scan(
		&w.Id,
		&w.Owner,
		&w.Name,
		&w.IsEoa,
		&w.IsHidden,
		&w.IsRedeemer,
		&w.IsMinter,
		&w.EoaAddress,
		&smartAddress,
		&smartIndex,
		&lastUnwrapAt,
	)
	if err != nil {
		return nil, err
	}
	if smartAddress.Valid {
		addr := smartAddress.String
		w.SmartAddress = &addr
	}
	if smartIndex.Valid {
		idx := int(smartIndex.Int64)
		w.SmartIndex = &idx
	}
	if lastUnwrapAt.Valid {
		unwrapAt := lastUnwrapAt.Time
		w.LastUnwrapAt = &unwrapAt
	}

	return &w, nil
}

func (a *AppDB) GetWalletAddressOwnerLookup(ctx context.Context, address string) (*structs.WalletAddressOwnerLookup, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			u.id AS user_id,
			u.is_merchant,
			COALESCE(
				NULLIF(TRIM(first_approved_location.name), ''),
				NULLIF(TRIM(u.contact_name), ''),
				NULLIF(TRIM(w.name), ''),
				''
			) AS merchant_name,
			w.name AS wallet_name,
			CASE
				WHEN LOWER(w.smart_address) = LOWER($1) THEN COALESCE(w.smart_address, '')
				ELSE w.eoa_address
			END AS matched_address
		FROM
			wallets w
		JOIN
			users u
		ON
			u.id = w.owner
		LEFT JOIN LATERAL (
			SELECT
				l.name
			FROM
				locations l
			WHERE
				l.owner_id = u.id
			AND
				l.approval = TRUE
			ORDER BY
				l.approved_at ASC NULLS LAST,
				l.id ASC
			LIMIT 1
		) first_approved_location
		ON TRUE
		WHERE
			LOWER(w.smart_address) = LOWER($1)
		OR
			LOWER(w.eoa_address) = LOWER($1)
		ORDER BY
			CASE
				WHEN LOWER(w.smart_address) = LOWER($1) THEN 0
				WHEN w.is_eoa = TRUE AND LOWER(w.eoa_address) = LOWER($1) THEN 1
				ELSE 2
			END,
			w.id ASC
		LIMIT 1;
	`, address)

	var lookup structs.WalletAddressOwnerLookup
	err := row.Scan(
		&lookup.UserID,
		&lookup.IsMerchant,
		&lookup.MerchantName,
		&lookup.WalletName,
		&lookup.MatchedAddress,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &lookup, nil
}

func (a *AppDB) GetOwnedWalletAddressSet(ctx context.Context, userID string) (map[string]struct{}, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			LOWER(eoa_address),
			LOWER(smart_address)
		FROM
			wallets
		WHERE
			owner = $1;
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("error querying wallet addresses for user %s: %w", userID, err)
	}
	defer rows.Close()

	addressSet := map[string]struct{}{}
	for rows.Next() {
		var eoa string
		var smart sql.NullString
		if err := rows.Scan(&eoa, &smart); err != nil {
			return nil, fmt.Errorf("error scanning wallet addresses for user %s: %w", userID, err)
		}
		eoa = strings.TrimSpace(strings.ToLower(eoa))
		if eoa != "" {
			addressSet[eoa] = struct{}{}
		}
		if smart.Valid {
			smartAddress := strings.TrimSpace(strings.ToLower(smart.String))
			if smartAddress != "" {
				addressSet[smartAddress] = struct{}{}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating wallet addresses for user %s: %w", userID, err)
	}

	return addressSet, nil
}

func (a *AppDB) UserOwnsAnyWalletAddress(ctx context.Context, userID string, addresses []string) (bool, error) {
	normalized := make([]string, 0, len(addresses))
	seen := map[string]struct{}{}
	for _, address := range addresses {
		addr := strings.TrimSpace(strings.ToLower(address))
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		normalized = append(normalized, addr)
	}

	if len(normalized) == 0 {
		return false, nil
	}

	row := a.db.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1
				FROM wallets
				WHERE owner = $1
				AND (
					LOWER(eoa_address) = ANY($2)
					OR
					LOWER(COALESCE(smart_address, '')) = ANY($2)
				)
			);
	`, userID, normalized)

	var ownsAny bool
	if err := row.Scan(&ownsAny); err != nil {
		return false, fmt.Errorf("error checking wallet address ownership for user %s: %w", userID, err)
	}

	return ownsAny, nil
}

func (a *AppDB) UpdateWallet(ctx context.Context, wallet *structs.Wallet) error {
	if wallet.Id == nil {
		return fmt.Errorf("wallet id is required")
	}

	_, err := a.db.Exec(ctx, `
		UPDATE
			wallets
		SET
			name = $1,
			is_hidden = $2
		WHERE
			(id = $3 AND owner = $4);
	`, wallet.Name, wallet.IsHidden, *wallet.Id, wallet.Owner)
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
			is_hidden,
			is_redeemer,
			is_minter,
			eoa_address,
			smart_address,
			smart_index,
			last_unwrap_at
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
	var lastUnwrapAt sql.NullTime
	err := row.Scan(
		&wallet.Id,
		&wallet.Owner,
		&wallet.Name,
		&wallet.IsEoa,
		&wallet.IsHidden,
		&wallet.IsRedeemer,
		&wallet.IsMinter,
		&wallet.EoaAddress,
		&smartAddress,
		&smartIndex,
		&lastUnwrapAt,
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
	if lastUnwrapAt.Valid {
		unwrapAt := lastUnwrapAt.Time
		wallet.LastUnwrapAt = &unwrapAt
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

func (a *AppDB) SetWalletMinterStatus(ctx context.Context, walletId int, isMinter bool) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			wallets
		SET
			is_minter = $1
		WHERE
			id = $2;
	`, isMinter, walletId)
	return err
}

func (a *AppDB) SetWalletLastUnwrapAt(ctx context.Context, walletId int, lastUnwrapAt time.Time) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			wallets
		SET
			last_unwrap_at = $1
		WHERE
			id = $2;
	`, lastUnwrapAt.UTC(), walletId)
	return err
}

func (a *AppDB) GetAllWallets(ctx context.Context) ([]*structs.Wallet, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			owner,
			name,
			is_eoa,
			is_hidden,
			is_redeemer,
			is_minter,
			eoa_address,
			smart_address,
			smart_index,
			last_unwrap_at
		FROM
			wallets
		ORDER BY id;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	wallets := make([]*structs.Wallet, 0)
	for rows.Next() {
		var wallet structs.Wallet
		var smartAddress sql.NullString
		var smartIndex sql.NullInt64
		var lastUnwrapAt sql.NullTime
		err := rows.Scan(
			&wallet.Id,
			&wallet.Owner,
			&wallet.Name,
			&wallet.IsEoa,
			&wallet.IsHidden,
			&wallet.IsRedeemer,
			&wallet.IsMinter,
			&wallet.EoaAddress,
			&smartAddress,
			&smartIndex,
			&lastUnwrapAt,
		)
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
		if lastUnwrapAt.Valid {
			unwrapAt := lastUnwrapAt.Time
			wallet.LastUnwrapAt = &unwrapAt
		}

		wallets = append(wallets, &wallet)
	}

	return wallets, nil
}
