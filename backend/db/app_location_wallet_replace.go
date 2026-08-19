package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// Wallet roles a location can fill. A single address may hold at most one of
// them, across every location the merchant owns.
const (
	LocationWalletRolePayment = "payment"
	LocationWalletRoleTipping = "tipping"
)

// NewLocationWallet is an address the caller has already derived from the
// account factory, ready to be recorded and attached in one transaction.
type NewLocationWallet struct {
	Address string
	Index   int
	Name    string
}

// walletInUseElsewhere reports the name of another location of the same owner
// already using this address, in either role. Empty means the address is free.
//
// Payment wallets live in location_payment_wallets and tipping wallets in a
// column on locations, so no single index can express "one address, one role".
// That is why the check is here rather than in the schema.
func walletInUseElsewhere(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID string, locationID uint64, address string) (string, error) {
	var clashingLocation string
	err := querier.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(TRIM(l.name), ''), 'another location')
		FROM locations l
		WHERE l.active = TRUE
		AND l.id <> $1
		AND l.owner_id = $2
		AND (
			LOWER(TRIM(COALESCE(l.tipping_wallet_address, ''))) = LOWER($3)
			OR EXISTS (
				SELECT 1 FROM location_payment_wallets p
				WHERE p.location_id = l.id AND p.active = TRUE
				AND LOWER(TRIM(p.wallet_address)) = LOWER($3)
			)
		)
		LIMIT 1;
	`, locationID, userID, address).Scan(&clashingLocation)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("error checking wallet uniqueness: %w", err)
	}
	return clashingLocation, nil
}

// GetAssignableWalletsForLocation lists the merchant's wallets and says, for
// each, whether it is free to take on the given role at this location.
//
// The UI needs the unavailable ones too: "in use by Shop B" is a far more
// useful thing to show than an address silently missing from the list.
func (a *AppDB) GetAssignableWalletsForLocation(ctx context.Context, userID string, locationID uint64, role string) ([]structs.AssignableWallet, error) {
	var owns bool
	if err := a.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM locations
			WHERE id = $1 AND owner_id = $2 AND active = TRUE
		);
	`, locationID, userID).Scan(&owns); err != nil {
		return nil, fmt.Errorf("error verifying location ownership: %w", err)
	}
	if !owns {
		return nil, pgx.ErrNoRows
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			TRIM(w.smart_address) AS address,
			COALESCE(NULLIF(TRIM(w.name), ''), 'Wallet') AS name,
			COALESCE((
				SELECT COALESCE(NULLIF(TRIM(other.name), ''), 'another location')
				FROM locations other
				WHERE other.active = TRUE
				AND other.owner_id = w.owner
				AND other.id <> $2
				AND (
					LOWER(TRIM(COALESCE(other.tipping_wallet_address, ''))) = LOWER(TRIM(w.smart_address))
					OR EXISTS (
						SELECT 1 FROM location_payment_wallets p
						WHERE p.location_id = other.id AND p.active = TRUE
						AND LOWER(TRIM(p.wallet_address)) = LOWER(TRIM(w.smart_address))
					)
				)
				LIMIT 1
			), '') AS in_use_by,
			EXISTS (
				SELECT 1 FROM location_payment_wallets p
				WHERE p.location_id = $2 AND p.active = TRUE
				AND LOWER(TRIM(p.wallet_address)) = LOWER(TRIM(w.smart_address))
			) AS is_this_payment,
			EXISTS (
				SELECT 1 FROM locations self
				WHERE self.id = $2
				AND LOWER(TRIM(COALESCE(self.tipping_wallet_address, ''))) = LOWER(TRIM(w.smart_address))
			) AS is_this_tipping
		FROM wallets w
		WHERE w.owner = $1
		AND w.active = TRUE
		AND w.is_hidden = FALSE
		-- Smart accounts only. An EOA can receive the token, but the platform
		-- pays gas through the paymaster and batches through the bundler, both of
		-- which act on smart accounts. A till on a bare EOA would take payments
		-- and then be unable to spend them the way every other merchant does.
		AND w.is_eoa = FALSE
		AND NULLIF(TRIM(w.smart_address), '') IS NOT NULL
		ORDER BY w.smart_index ASC NULLS LAST, w.id ASC;
	`, userID, locationID)
	if err != nil {
		return nil, fmt.Errorf("error querying assignable wallets: %w", err)
	}
	defer rows.Close()

	wallets := []structs.AssignableWallet{}
	for rows.Next() {
		var wallet structs.AssignableWallet
		var isThisPayment, isThisTipping bool
		if err := rows.Scan(&wallet.Address, &wallet.Name, &wallet.InUseBy, &isThisPayment, &isThisTipping); err != nil {
			return nil, fmt.Errorf("error scanning assignable wallet: %w", err)
		}

		// Taking the other role at this same location is just as much a clash as
		// taking a role elsewhere — a till and its tip jar must stay separable.
		switch role {
		case LocationWalletRoleTipping:
			wallet.IsCurrent = isThisTipping
			if isThisPayment {
				wallet.InUseBy = "this location's payments"
			}
		default:
			wallet.IsCurrent = isThisPayment
			if isThisTipping {
				wallet.InUseBy = "this location's tips"
			}
		}
		wallet.Available = wallet.InUseBy == "" && !wallet.IsCurrent

		wallets = append(wallets, wallet)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating assignable wallets: %w", err)
	}

	return wallets, nil
}

// UniqueWalletName keeps a merchant's wallet list readable when they spawn a
// second wallet for the same shop. Two entries both called "900 Innes Avenue -
// Payments" are indistinguishable in a picker, so later ones get a counter.
//
// Names are not keys, so a failure to check is not worth failing the swap over —
// on error the plain name is returned and the wallet is still created.
func (a *AppDB) UniqueWalletName(ctx context.Context, ownerID string, preferred string) string {
	preferred = strings.TrimSpace(preferred)
	if preferred == "" {
		return "Location wallet"
	}

	for attempt := 1; attempt <= 20; attempt++ {
		candidate := preferred
		if attempt > 1 {
			candidate = fmt.Sprintf("%s (%d)", preferred, attempt)
		}

		var taken bool
		if err := a.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM wallets
				WHERE owner = $1 AND active = TRUE AND TRIM(name) = $2
			);
		`, ownerID, candidate).Scan(&taken); err != nil {
			return preferred
		}
		if !taken {
			return candidate
		}
	}

	return preferred
}

// ReplaceLocationWallet swaps the address filling a role at one location.
//
// There is deliberately no way to detach a payment wallet on its own. A location
// with no payment wallet cannot be paid, and since the read path stopped falling
// back to the owner's primary wallet, an empty slot is a shop that silently drops
// money rather than one that quietly shares a till. Replacement is therefore a
// single atomic operation: the old address is retired and the new one attached in
// the same transaction, or neither happens.
//
// Pass newWallet to record a freshly derived address; pass address to point at
// one the merchant already owns. Tipping is the one role that accepts an empty
// address, because a location that takes no tips is a legitimate state.
func (a *AppDB) ReplaceLocationWallet(
	ctx context.Context,
	userID string,
	locationID uint64,
	role string,
	address string,
	newWallet *NewLocationWallet,
) (*structs.Location, error) {
	if role != LocationWalletRolePayment && role != LocationWalletRoleTipping {
		return nil, fmt.Errorf("unknown wallet role %q", role)
	}

	user, err := a.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.IsMerchant {
		return nil, fmt.Errorf("only merchants can change location wallets")
	}

	if newWallet != nil {
		address = newWallet.Address
	}
	address = strings.TrimSpace(address)

	if address == "" {
		if role == LocationWalletRolePayment {
			return nil, fmt.Errorf("a location must always have a payment wallet — choose another wallet or create a new one")
		}
	} else {
		address, err = normalizeEthereumAddressForField(address, role+" wallet")
		if err != nil {
			return nil, err
		}
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error starting wallet replacement transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var owns bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM locations
			WHERE id = $1 AND owner_id = $2 AND active = TRUE
		);
	`, locationID, userID).Scan(&owns); err != nil {
		return nil, fmt.Errorf("error verifying location ownership: %w", err)
	}
	if !owns {
		return nil, pgx.ErrNoRows
	}

	if address != "" {
		// Re-checked inside the transaction: the caller may have listed the
		// assignable wallets minutes ago, and another tab may have claimed one since.
		clash, err := walletInUseElsewhere(ctx, tx, userID, locationID, address)
		if err != nil {
			return nil, err
		}
		if clash != "" {
			return nil, fmt.Errorf("that wallet is already in use by %s — each location needs its own payment and tipping wallets", clash)
		}

		if err := ensureWalletFreeOfOtherRoleHere(ctx, tx, locationID, role, address); err != nil {
			return nil, err
		}
	}

	if newWallet != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO wallets (owner, name, is_eoa, eoa_address, smart_address, smart_index, is_hidden, active)
			SELECT $1, $2, FALSE, COALESCE((
				SELECT NULLIF(TRIM(w.eoa_address), '') FROM wallets w
				WHERE w.owner = $1 AND w.active = TRUE
				AND NULLIF(TRIM(w.eoa_address), '') IS NOT NULL
				ORDER BY w.id ASC LIMIT 1
			), ''), $3, $4, FALSE, TRUE
			ON CONFLICT DO NOTHING;
		`, userID, newWallet.Name, newWallet.Address, newWallet.Index); err != nil {
			return nil, fmt.Errorf("error recording the new wallet: %w", err)
		}
	} else if address != "" {
		// An existing address must be a smart account this merchant owns. Checked
		// here and not only in the picker, because the picker is a suggestion and
		// this endpoint is reachable directly.
		var owned bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM wallets w
				WHERE w.owner = $1 AND w.active = TRUE
				AND w.is_eoa = FALSE
				AND LOWER(TRIM(COALESCE(w.smart_address, ''))) = LOWER($2)
			);
		`, userID, address).Scan(&owned); err != nil {
			return nil, fmt.Errorf("error verifying wallet ownership: %w", err)
		}
		if !owned {
			var isOwnEOA bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM wallets w
					WHERE w.owner = $1 AND w.active = TRUE
					AND LOWER(TRIM(COALESCE(w.eoa_address, ''))) = LOWER($2)
					AND (w.is_eoa = TRUE OR NULLIF(TRIM(w.smart_address), '') IS NULL)
				);
			`, userID, address).Scan(&isOwnEOA); err != nil {
				return nil, fmt.Errorf("error verifying wallet ownership: %w", err)
			}
			if isOwnEOA {
				return nil, fmt.Errorf("that wallet is a signing key, not a smart account — locations must be paid into a smart account")
			}
			return nil, fmt.Errorf("that wallet does not belong to this account")
		}
	}

	switch role {
	case LocationWalletRolePayment:
		if _, err := tx.Exec(ctx, `
			UPDATE location_payment_wallets
			SET active = FALSE, delete_date = $2, delete_reason = $3
			WHERE location_id = $1 AND active = TRUE
			AND LOWER(TRIM(wallet_address)) <> LOWER($4);
		`, locationID, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonWalletSettings, address); err != nil {
			return nil, fmt.Errorf("error retiring the previous payment wallet: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO location_payment_wallets (location_id, wallet_address, is_default)
			VALUES ($1, $2, TRUE)
			ON CONFLICT (location_id, wallet_address) WHERE active = TRUE
			DO UPDATE SET is_default = TRUE, active = TRUE, delete_date = NULL, delete_reason = NULL;
		`, locationID, address); err != nil {
			return nil, fmt.Errorf("error attaching the new payment wallet: %w", err)
		}

		// The whole point of the swap is that the old address stops being paid.
		// Retiring the row without moving the column would leave the location
		// still advertising the wallet the merchant just walked away from.
		if err := syncLocationPaymentWalletAddress(ctx, tx, locationID); err != nil {
			return nil, err
		}

	case LocationWalletRoleTipping:
		if _, err := tx.Exec(ctx, `
			UPDATE locations SET tipping_wallet_address = $1
			WHERE id = $2 AND owner_id = $3;
		`, address, locationID, userID); err != nil {
			return nil, fmt.Errorf("error updating the tipping wallet: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("error committing wallet replacement: %w", err)
	}

	locations, err := a.GetLocationsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, location := range locations {
		if location != nil && location.ID == uint(locationID) {
			return location, nil
		}
	}
	return nil, pgx.ErrNoRows
}

// ensureWalletFreeOfOtherRoleHere stops a location pointing its tips and its
// takings at the same address, which would make the two indistinguishable on
// chain and the day view unsplittable.
func ensureWalletFreeOfOtherRoleHere(ctx context.Context, tx pgx.Tx, locationID uint64, role string, address string) error {
	var clashes bool
	var query string
	if role == LocationWalletRolePayment {
		query = `
			SELECT EXISTS(
				SELECT 1 FROM locations
				WHERE id = $1
				AND LOWER(TRIM(COALESCE(tipping_wallet_address, ''))) = LOWER($2)
			);`
	} else {
		query = `
			SELECT EXISTS(
				SELECT 1 FROM location_payment_wallets
				WHERE location_id = $1 AND active = TRUE
				AND LOWER(TRIM(wallet_address)) = LOWER($2)
			);`
	}

	if err := tx.QueryRow(ctx, query, locationID, address).Scan(&clashes); err != nil {
		return fmt.Errorf("error checking the wallet's other role: %w", err)
	}
	if clashes {
		other := "tipping"
		if role == LocationWalletRoleTipping {
			other = "payment"
		}
		return fmt.Errorf("that wallet is already this location's %s wallet — payments and tips must stay separate", other)
	}
	return nil
}
