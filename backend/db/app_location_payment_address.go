package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// locationPaymentWalletExpr derives the address a location is paid into from
// its active wallet rows: the default one first, oldest next. It is the only
// place that derivation still happens — read paths take the answer off the
// locations row via locationPayToAddressExpr.
//
// It is a copy of the lateral join those readers used before the cutover,
// including the part that is easy to think of as a bug: a row holding a blank
// address still wins its position and resolves to a blank till. The readers did
// not look past it, so neither does this. Migration 1.46 did skip blanks and
// fall through to the next row, which is why 1.47 re-derived every row — the
// column had to reproduce what the map already showed, or the cutover would
// have moved money on the strength of a diff nobody could reconcile.
//
// The statement embedding this must alias its target table l.
const locationPaymentWalletExpr = `
			COALESCE((
				SELECT NULLIF(TRIM(lpw.wallet_address), '')
				FROM
					location_payment_wallets lpw
				WHERE
					lpw.location_id = l.id
				AND
					lpw.active = TRUE
				ORDER BY
					CASE
						WHEN lpw.is_default = TRUE THEN 0
						ELSE 1
					END,
					lpw.id ASC
				LIMIT 1
			), '')`

// locationPayToAddressExpr is how a read path asks for a location's till. It
// reads the column syncLocationPaymentWalletAddress maintains instead of
// re-deriving it, so a caller cannot get a subtly different answer by writing a
// slightly different join.
//
// It stays a shared constant for the same reason the join was worth replacing:
// this is the address a customer's Pay button sends to, and eight copies of a
// derivation is eight chances for one of them to drift somewhere no test looks.
// A wrong till does not crash anything — the money simply lands elsewhere.
//
// The NULLIF/TRIM wrapper is not defensive about the column, which is NOT NULL
// and only ever written trimmed; it is there so this expression is textually
// interchangeable with the join it replaces.
//
// The statement embedding this must alias its target table l.
const locationPayToAddressExpr = `COALESCE(NULLIF(TRIM(l.payment_wallet_address), ''), '')`

// syncLocationPaymentWalletAddress rewrites locations.payment_wallet_address
// from the wallet rows as they now stand. It takes a tx because it is only
// correct inside the transaction that moved them: a commit that changed which
// wallet is default and left the column behind has published a shop whose
// takings go to an address its owner deliberately stopped using.
//
// It recomputes rather than being handed the new address, so a retire/attach
// pair cannot leave the column in a state the derivation would never produce —
// the column is wrong in exactly the same way as the readers or not at all.
func syncLocationPaymentWalletAddress(ctx context.Context, tx pgx.Tx, locationID uint64) error {
	if _, err := tx.Exec(ctx, `
		UPDATE locations l
		SET payment_wallet_address = `+locationPaymentWalletExpr+`
		WHERE l.id = $1;
	`, locationID); err != nil {
		return fmt.Errorf("error syncing payment wallet address for location %d: %w", locationID, err)
	}
	return nil
}

// syncOwnerLocationPaymentWalletAddresses is the same recompute across one
// owner's whole estate, for the account-deletion paths that retire or restore
// every location's wallet rows in a single statement and so never name the
// locations they touched.
func syncOwnerLocationPaymentWalletAddresses(ctx context.Context, tx pgx.Tx, ownerID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE locations l
		SET payment_wallet_address = `+locationPaymentWalletExpr+`
		WHERE l.owner_id = $1;
	`, ownerID); err != nil {
		return fmt.Errorf("error syncing payment wallet addresses for owner %s: %w", ownerID, err)
	}
	return nil
}
