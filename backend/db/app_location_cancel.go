package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrLocationNotCancellable says an application has already been reviewed. Only
// a listing still waiting in the queue can be withdrawn: once an admin has
// approved it the shop is on the map with a wallet behind it, and once they have
// rejected it there is nothing left to withdraw.
var ErrLocationNotCancellable = errors.New("only a location application still awaiting review can be cancelled")

// CancelPendingLocation withdraws an application the caller owns.
//
// Soft-deleted rather than removed, in step with every other retirement in this
// schema. The row is the record that the application was made, and the partial
// unique index on google_id only covers active rows, so withdrawing frees the
// business to be applied for again — by this merchant or by whoever actually
// runs it.
//
// The hours and payment-wallet rows are retired with it. Leaving them active
// would leave a cancelled listing's till in the assignable-wallets list as "in
// use by" a shop that no longer exists.
func (a *AppDB) CancelPendingLocation(ctx context.Context, ownerID string, locationID uint64) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error opening transaction to cancel location %d: %w", locationID, err)
	}
	defer tx.Rollback(ctx)

	// approval IS NULL is the whole guard, and it is in the UPDATE rather than a
	// SELECT before it: an admin approving this listing in the same moment must
	// win or lose the race outright, not have the cancellation applied on top of
	// an approval it never saw.
	tag, err := tx.Exec(ctx, `
		UPDATE locations
		SET
			active = FALSE,
			delete_date = NOW(),
			delete_reason = 'cancelled by the merchant while pending review'
		WHERE id = $1
		AND owner_id = $2
		AND active = TRUE
		AND approval IS NULL;
	`, locationID, ownerID)
	if err != nil {
		return fmt.Errorf("error cancelling location %d: %w", locationID, err)
	}
	if tag.RowsAffected() == 0 {
		// Either it is not theirs, is already gone, or has been reviewed. The
		// caller cannot act differently on any of those, and saying which would
		// tell a stranger whether somebody else's location id exists.
		return ErrLocationNotCancellable
	}

	for _, statement := range []string{
		`UPDATE location_hours SET active = FALSE, delete_date = NOW(), delete_reason = 'location application cancelled' WHERE location_id = $1 AND active = TRUE;`,
		`UPDATE location_payment_wallets SET active = FALSE, delete_date = NOW(), delete_reason = 'location application cancelled' WHERE location_id = $1 AND active = TRUE;`,
	} {
		if _, err := tx.Exec(ctx, statement, locationID); err != nil {
			return fmt.Errorf("error retiring dependants of cancelled location %d: %w", locationID, err)
		}
	}

	// The location row still carries its till address in a column of its own,
	// and a withdrawn application must not go on advertising one.
	if _, err := tx.Exec(ctx, `
		UPDATE locations
		SET payment_wallet_address = '', tipping_wallet_address = ''
		WHERE id = $1;
	`, locationID); err != nil {
		return fmt.Errorf("error clearing wallet addresses on cancelled location %d: %w", locationID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing cancellation of location %d: %w", locationID, err)
	}

	return nil
}

// LocationIsCancellable answers whether the owner may still withdraw a listing,
// for a caller that wants to check without attempting it.
func (a *AppDB) LocationIsCancellable(ctx context.Context, ownerID string, locationID uint64) (bool, error) {
	var cancellable bool
	err := a.db.QueryRow(ctx, `
		SELECT TRUE
		FROM locations
		WHERE id = $1
		AND owner_id = $2
		AND active = TRUE
		AND approval IS NULL;
	`, locationID, ownerID).Scan(&cancellable)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return cancellable, nil
}
