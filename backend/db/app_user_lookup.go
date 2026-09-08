package db

// Wallet and contact lookups. These began life in the W9 code but are not
// about tax at all — they answer "who owns this address" and "how do we reach
// them", which several unrelated paths need.

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
)

func (a *AppDB) GetUserIdByWalletAddress(ctx context.Context, address string) (*string, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			owner
		FROM
			wallets
		WHERE
			LOWER(eoa_address) = LOWER($1)
		OR
			LOWER(smart_address) = LOWER($1)
		LIMIT 1;
	`, address)

	var owner string
	err := row.Scan(&owner)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &owner, nil
}

func (a *AppDB) GetUserContactEmail(ctx context.Context, userId string) (*string, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			contact_email
		FROM
			users
		WHERE
			id = $1;
	`, userId)

	var email sql.NullString
	err := row.Scan(&email)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if email.Valid {
		return &email.String, nil
	}
	return nil, nil
}
