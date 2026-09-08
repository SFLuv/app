package db

import (
	"context"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// MerchantOnboardingPending answers the read-only gate's predicate in one
// round trip: a self-declared merchant who has not yet listed a shop.
//
// The two columns are read together rather than through the existing profile
// loaders because this runs on every mutating request — GetUserById pulls the
// wallets, locations and role flags with it, none of which the gate looks at.
//
// A missing user row is not pending. Somebody with no row has not accepted the
// privacy policy either, and that gate — which runs first and knows why it is
// refusing — is the one that should be answering them.
func (a *AppDB) MerchantOnboardingPending(ctx context.Context, userId string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			account_type = $2
		AND
			merchant_onboarding_completed_at IS NULL
		FROM
			users
		WHERE
			id = $1;
	`, userId, structs.AccountTypeMerchant)

	var pending bool
	if err := row.Scan(&pending); err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	return pending, nil
}
