package db

import (
	"context"
	"fmt"
)

// Wallet roles a location can attach an address in. Kept alongside
// LocationWalletRolePayment/Tipping, which name the same two things for the
// merchant-facing swap endpoint.
const (
	locationWalletRolePaymentLabel = "payment"
	locationWalletRoleTippingLabel = "tipping"
)

// LocationWallet is one address a location takes money at, with whatever we
// already know about it locally.
type LocationWallet struct {
	LocationID   uint64
	LocationName string
	// Role is "payment" or "tipping".
	Role    string
	Address string
	// WalletID names the wallets row behind the address, when there is one. A
	// merchant can attach an address we hold no row for, and those have nowhere
	// to remember a granted role — see IsRedeemer.
	WalletID *int
	// IsRedeemer is our record of the on-chain role, and only ever a shortcut
	// past a chain read. An address with no wallets row always reads false.
	IsRedeemer bool
}

// GetLocationWallets lists every address attached to an active location, once
// each, payment tills and tipping wallets alike.
//
// Deduplicated by address rather than by location: the role is granted to an
// address, so two locations naming the same one are a single grant. Ordering is
// by address so a run that is cut short resumes over the same list.
func (a *AppDB) GetLocationWallets(ctx context.Context) ([]LocationWallet, error) {
	rows, err := a.db.Query(ctx, `
		WITH attached AS (
			SELECT
				l.id AS location_id,
				COALESCE(NULLIF(TRIM(l.name), ''), 'Location ' || l.id::text) AS location_name,
				$1::text AS role,
				TRIM(p.wallet_address) AS address
			FROM location_payment_wallets p
			JOIN locations l ON l.id = p.location_id
			WHERE p.active = TRUE
			AND l.active = TRUE
			AND NULLIF(TRIM(p.wallet_address), '') IS NOT NULL

			UNION ALL

			SELECT
				l.id,
				COALESCE(NULLIF(TRIM(l.name), ''), 'Location ' || l.id::text),
				$2::text,
				TRIM(l.tipping_wallet_address)
			FROM locations l
			WHERE l.active = TRUE
			AND NULLIF(TRIM(l.tipping_wallet_address), '') IS NOT NULL
		)
		SELECT DISTINCT ON (LOWER(attached.address))
			attached.location_id,
			attached.location_name,
			attached.role,
			attached.address,
			known.id,
			COALESCE(known.is_redeemer, FALSE)
		FROM attached
		LEFT JOIN LATERAL (
			SELECT w.id, w.is_redeemer
			FROM wallets w
			WHERE w.active = TRUE
			AND LOWER(TRIM(COALESCE(w.smart_address, ''))) = LOWER(attached.address)
			ORDER BY w.id ASC
			LIMIT 1
		) known ON TRUE
		ORDER BY LOWER(attached.address), attached.role, attached.location_id;
	`, locationWalletRolePaymentLabel, locationWalletRoleTippingLabel)
	if err != nil {
		return nil, fmt.Errorf("error loading location wallets: %w", err)
	}
	defer rows.Close()

	wallets := []LocationWallet{}
	for rows.Next() {
		var wallet LocationWallet
		if err := rows.Scan(
			&wallet.LocationID,
			&wallet.LocationName,
			&wallet.Role,
			&wallet.Address,
			&wallet.WalletID,
			&wallet.IsRedeemer,
		); err != nil {
			return nil, fmt.Errorf("error scanning location wallet: %w", err)
		}
		wallets = append(wallets, wallet)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading location wallets: %w", err)
	}

	return wallets, nil
}
