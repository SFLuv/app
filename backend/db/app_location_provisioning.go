package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// LocationProvisioningContext is what the caller needs in order to work out
// whether a location needs its own wallets, and what to derive them from.
type LocationProvisioningContext struct {
	OwnerID string
	// OwnerEOA is the signer every one of this merchant's smart accounts is
	// derived from. Empty when the merchant has no wallet at all yet.
	OwnerEOA string
	// Street names the derived wallets. A shop can be renamed; a different
	// address is a different shop, so the label stays meaningful.
	Street string
	// NextSmartIndex is the first unused smart-account index for this owner.
	NextSmartIndex int
	// AlreadyAssigned counts the owner's locations that already hold a payment
	// wallet. Zero means this is their first, which keeps the primary wallet.
	AlreadyAssigned int
	// HasPaymentWallet is true when this location is already provisioned, either
	// by an earlier approval or because the merchant chose one themselves.
	HasPaymentWallet bool
	// PrimaryWallet is the owner's existing wallet, used for a first location.
	PrimaryWallet string
}

// NeedsDerivedWallets reports whether approval must mint new addresses for this
// location, rather than recording the owner's existing primary.
func (c *LocationProvisioningContext) NeedsDerivedWallets() bool {
	return !c.HasPaymentWallet && c.AlreadyAssigned > 0
}

// GetLocationProvisioningContext gathers everything needed to provision, outside
// any transaction, so the caller can do its RPC work before taking write locks.
func (a *AppDB) GetLocationProvisioningContext(ctx context.Context, locationID uint) (*LocationProvisioningContext, error) {
	result := &LocationProvisioningContext{}

	err := a.db.QueryRow(ctx, `
		SELECT
			COALESCE(l.owner_id, ''),
			COALESCE(NULLIF(TRIM(l.street), ''), 'Location ' || l.id::text),
			COALESCE((
				SELECT NULLIF(TRIM(w.eoa_address), '')
				FROM wallets w
				WHERE w.owner = l.owner_id AND w.active = TRUE
				AND NULLIF(TRIM(w.eoa_address), '') IS NOT NULL
				ORDER BY w.id ASC LIMIT 1
			), ''),
			COALESCE((
				SELECT MAX(w.smart_index) + 1
				FROM wallets w
				WHERE w.owner = l.owner_id AND w.is_eoa = FALSE
			), 0),
			(
				SELECT COUNT(*)
				FROM location_payment_wallets p
				JOIN locations other ON other.id = p.location_id
				WHERE other.owner_id = l.owner_id AND other.active = TRUE AND p.active = TRUE
			),
			EXISTS (
				SELECT 1 FROM location_payment_wallets p
				WHERE p.location_id = l.id AND p.active = TRUE
			),
			COALESCE((
				SELECT COALESCE(
					NULLIF(TRIM(u.primary_wallet_address), ''),
					NULLIF(TRIM(legacy.smart_address), '')
				)
				FROM users u
				LEFT JOIN LATERAL (
					SELECT w.smart_address FROM wallets w
					WHERE w.owner = u.id AND w.active = TRUE AND w.is_eoa = FALSE
					AND NULLIF(TRIM(w.smart_address), '') IS NOT NULL
					ORDER BY w.smart_index ASC NULLS LAST, w.id ASC LIMIT 1
				) legacy ON TRUE
				WHERE u.id = l.owner_id
			), '')
		FROM locations l
		WHERE l.id = $1;
	`, locationID).Scan(
		&result.OwnerID, &result.Street, &result.OwnerEOA,
		&result.NextSmartIndex, &result.AlreadyAssigned,
		&result.HasPaymentWallet, &result.PrimaryWallet,
	)
	if err != nil {
		return nil, fmt.Errorf("error loading provisioning context for location %d: %w", locationID, err)
	}

	return result, nil
}

// DerivedLocationWallets are the addresses a second or later location will be
// paid into, already computed from the account factory.
type DerivedLocationWallets struct {
	PaymentAddress string
	PaymentIndex   int
	TippingAddress string
	TippingIndex   int
	Street         string
}

// provisionLocationWallets gives a newly approved location its own addresses,
// inside the approving transaction.
//
// Two shops must never share a payment wallet. On chain a transfer records only
// an address, so if two locations resolve to the same one there is nothing that
// says which shop a payment belongs to — takings, tips and the merchant-mode day
// view all become unsplittable, and no later migration can separate history that
// was already mixed.
//
// The owner's first location keeps their existing primary wallet: that is where
// merchants are already paid, and moving it would orphan their history for no
// benefit. Every location after the first gets freshly derived addresses.
func (a *AppDB) provisionLocationWallets(
	ctx context.Context,
	tx pgx.Tx,
	locationID uint,
	provisioning *LocationProvisioningContext,
	derived *DerivedLocationWallets,
) error {
	if provisioning == nil || provisioning.HasPaymentWallet {
		// Already provisioned, or the merchant chose one themselves. Approval is
		// not the place to overrule that.
		return nil
	}

	if provisioning.AlreadyAssigned == 0 {
		if err := a.assignExistingWallet(ctx, tx, locationID, provisioning.PrimaryWallet); err != nil {
			return err
		}
		return syncLocationPaymentWalletAddress(ctx, tx, uint64(locationID))
	}

	if derived == nil {
		// The caller could not derive addresses — almost always the chain RPC
		// being unreachable. Failing here rolls the approval back so the location
		// is never published without a wallet of its own; the admin retries.
		return fmt.Errorf("cannot approve a second location without deriving its wallets: address derivation unavailable")
	}

	if err := a.insertDerivedWallets(ctx, tx, locationID, provisioning, derived); err != nil {
		return err
	}

	// Approval is the moment the listing becomes payable, so the row has to
	// carry the address by the time this transaction commits — there is no later
	// write that would come back for it.
	return syncLocationPaymentWalletAddress(ctx, tx, uint64(locationID))
}

func (a *AppDB) assignExistingWallet(ctx context.Context, tx pgx.Tx, locationID uint, address string) error {
	if strings.TrimSpace(address) == "" {
		// Nothing to assign. Left unprovisioned rather than failing: an admin
		// should not be blocked from publishing a merchant who has not finished
		// setting up a wallet, and a first location has nothing to collide with.
		return nil
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO location_payment_wallets (location_id, wallet_address, is_default)
		VALUES ($1, $2, TRUE)
		ON CONFLICT DO NOTHING;
	`, locationID, address); err != nil {
		return fmt.Errorf("error assigning existing wallet to location %d: %w", locationID, err)
	}
	return nil
}

func (a *AppDB) insertDerivedWallets(
	ctx context.Context,
	tx pgx.Tx,
	locationID uint,
	provisioning *LocationProvisioningContext,
	derived *DerivedLocationWallets,
) error {
	// The smart accounts stay counterfactual until first use. That is fine for
	// receiving — tokens sit at the address either way — and the paymaster
	// deploys them on the merchant's first outgoing transaction.
	for _, wallet := range []struct {
		name    string
		address string
		index   int
	}{
		{derived.Street + " - Payments", derived.PaymentAddress, derived.PaymentIndex},
		{derived.Street + " - Tips", derived.TippingAddress, derived.TippingIndex},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO wallets (owner, name, is_eoa, eoa_address, smart_address, smart_index, is_hidden, active)
			VALUES ($1, $2, FALSE, $3, $4, $5, FALSE, TRUE)
			ON CONFLICT DO NOTHING;
		`, provisioning.OwnerID, wallet.name, provisioning.OwnerEOA, wallet.address, wallet.index); err != nil {
			return fmt.Errorf("error creating %s for location %d: %w", wallet.name, locationID, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO location_payment_wallets (location_id, wallet_address, is_default)
		VALUES ($1, $2, TRUE);
	`, locationID, derived.PaymentAddress); err != nil {
		return fmt.Errorf("error assigning derived payment wallet to location %d: %w", locationID, err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE locations SET tipping_wallet_address = $1 WHERE id = $2;
	`, derived.TippingAddress, locationID); err != nil {
		return fmt.Errorf("error assigning derived tipping wallet to location %d: %w", locationID, err)
	}

	return nil
}
