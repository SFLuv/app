package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// locationPaymentWalletExpr is the address a location is paid into, derived the
// way every read path already derives it: the location's active wallet rows,
// the default one first, oldest next.
//
// It is copied from the lateral join in GetLocationById and its siblings,
// including the part that is easy to think of as a bug. A row holding a blank
// address still wins its position and resolves to '' — the readers do not look
// past it, so neither does this. Migration 1.46 did skip blanks and fall through
// to the next row, which means the column it backfilled could name a wallet the
// map never shows. Agreeing with the readers is what makes the column safe to
// swap in for the join later; agreeing with the migration would only preserve
// the disagreement.
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
