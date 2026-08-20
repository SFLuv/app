package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ErrLocationWalletIndexMoved says another provisioning run claimed the smart
// account index this one derived an address for. Nothing was written, and the
// caller can read, derive and try again.
var ErrLocationWalletIndexMoved = errors.New("smart account index moved before the derived wallets could be written")

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

	// AlwaysDerive makes even a first location derive its own wallets instead of
	// inheriting users.primary_wallet_address.
	//
	// Set only by the creation path, and deliberately not by approval. The
	// inherit-the-primary rule was written when provisioning happened at
	// approval and nowhere else: by then the merchant had been taking payment
	// into their primary wallet for weeks, and moving them off it would have
	// orphaned that history against an address the map no longer showed. A
	// location created from now on has no history to orphan, so it starts on its
	// own till and a merchant's personal wallet is never also a shop's. Leaving
	// this false on the approval backstop is what keeps locations that predate
	// the change on the rule they were submitted under.
	AlwaysDerive bool
}

// NeedsDerivedWallets reports whether this location must have new addresses
// minted for it, rather than recording the owner's existing primary.
func (c *LocationProvisioningContext) NeedsDerivedWallets() bool {
	return !c.HasPaymentWallet && (c.AlwaysDerive || c.AlreadyAssigned > 0)
}

// DerivationStartIndex is the first smart-account index a location's wallets may
// be derived at.
//
// Index 0 is the account holder's own wallet by convention — the redeemer sync
// asks for it by that number — so a merchant whose smart wallet has not been
// registered with us yet would otherwise have their first till derived onto the
// exact address their personal wallet is going to occupy. Skipping it costs
// nothing and keeps a till and a personal wallet from ever being one address.
func (c *LocationProvisioningContext) DerivationStartIndex() int {
	if c.NextSmartIndex < 1 {
		return 1
	}
	return c.NextSmartIndex
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

// DerivedLocationWallets are the addresses a location will be paid into,
// already computed from the account factory.
type DerivedLocationWallets struct {
	PaymentAddress string
	PaymentIndex   int
	TippingAddress string
	TippingIndex   int
	Street         string
}

// ProvisionLocationWallets gives a location its own addresses in a transaction
// of its own.
//
// Approval provisions inside the transaction that publishes the listing, so
// nothing can become payable without a till. Creation has no such transaction to
// join — the location is already committed by the time the chain has answered —
// so it gets this one.
func (a *AppDB) ProvisionLocationWallets(
	ctx context.Context,
	locationID uint,
	provisioning *LocationProvisioningContext,
	derived *DerivedLocationWallets,
) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error opening transaction to provision location %d: %w", locationID, err)
	}
	defer tx.Rollback(ctx)

	if err := a.provisionLocationWallets(ctx, tx, locationID, provisioning, derived); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// provisionLocationWallets gives a location its own addresses, inside whichever
// transaction the caller is already running.
//
// Two shops must never share a payment wallet. On chain a transfer records only
// an address, so if two locations resolve to the same one there is nothing that
// says which shop a payment belongs to — takings, tips and the merchant-mode day
// view all become unsplittable, and no later migration can separate history that
// was already mixed.
//
// A location that already holds a wallet is left alone, whether it was given one
// at creation, at an earlier approval, or chosen by the merchant. That is what
// makes approval safe to leave in place as a backstop for anything that reached
// it unprovisioned.
func (a *AppDB) provisionLocationWallets(
	ctx context.Context,
	tx pgx.Tx,
	locationID uint,
	provisioning *LocationProvisioningContext,
	derived *DerivedLocationWallets,
) error {
	if provisioning == nil {
		return nil
	}

	if err := lockOwnerWalletDerivation(ctx, tx, provisioning.OwnerID); err != nil {
		return err
	}

	// The context the caller derived against was read before the lock and can be
	// minutes old by the time the chain answers. Everything that decides what to
	// write is re-read here, under the lock, so the decision is made against the
	// state actually being written to.
	state, err := readLocationProvisioningState(ctx, tx, locationID, provisioning.OwnerID)
	if err != nil {
		return err
	}
	if state.HasPaymentWallet {
		return nil
	}

	if state.AlreadyAssigned == 0 && !provisioning.AlwaysDerive {
		if err := a.assignExistingWallet(ctx, tx, locationID, provisioning.PrimaryWallet); err != nil {
			return err
		}
		return syncLocationPaymentWalletAddress(ctx, tx, uint64(locationID))
	}

	if derived == nil {
		// The caller could not derive addresses — almost always the chain RPC
		// being unreachable. Approval fails here so a location is never published
		// without a wallet of its own; the admin retries. Creation never reaches
		// this, because it treats a failed derivation as "no wallet yet" and
		// leaves the location for the backstop rather than refusing the shop.
		return fmt.Errorf("cannot provision location %d without deriving its wallets: address derivation unavailable", locationID)
	}

	// Indexes only ever move forward, so an allocation that has overtaken ours
	// means the addresses we hold are another location's now. Refusing is the
	// whole point of the lock: writing them anyway is what used to put two shops
	// on one till.
	if derived.PaymentIndex < state.NextSmartIndex {
		return ErrLocationWalletIndexMoved
	}

	if err := a.insertDerivedWallets(ctx, tx, locationID, provisioning, derived); err != nil {
		return err
	}

	// The row has to carry the address by the time this transaction commits —
	// there is no later write that would come back for it.
	return syncLocationPaymentWalletAddress(ctx, tx, uint64(locationID))
}

// locationProvisioningState is the part of the context that another transaction
// can move underneath us.
type locationProvisioningState struct {
	HasPaymentWallet bool
	AlreadyAssigned  int
	NextSmartIndex   int
}

func readLocationProvisioningState(ctx context.Context, tx pgx.Tx, locationID uint, ownerID string) (locationProvisioningState, error) {
	var state locationProvisioningState
	if err := tx.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1 FROM location_payment_wallets p
				WHERE p.location_id = $1 AND p.active = TRUE
			),
			(
				SELECT COUNT(*)
				FROM location_payment_wallets p
				JOIN locations other ON other.id = p.location_id
				WHERE other.owner_id = $2 AND other.active = TRUE AND p.active = TRUE
			),
			COALESCE((
				SELECT MAX(w.smart_index) + 1
				FROM wallets w
				WHERE w.owner = $2 AND w.is_eoa = FALSE
			), 0);
	`, locationID, ownerID).Scan(&state.HasPaymentWallet, &state.AlreadyAssigned, &state.NextSmartIndex); err != nil {
		return state, fmt.Errorf("error re-reading provisioning state for location %d: %w", locationID, err)
	}
	return state, nil
}

// lockOwnerWalletDerivation serialises smart-account index allocation for one
// merchant.
//
// The index is MAX(smart_index)+1 read outside the transaction, because the
// chain call that turns it into an address must not hold write locks. Two
// creations for the same owner overlapping there read the same index and derive
// the same address — the address is a pure function of owner and index — and the
// wallets unique key then swallowed the second row while its location still
// recorded the address. The result is a till with no wallet behind it, pointed
// at by two shops. Held to the end of the transaction, so a caller that has to
// re-derive queues behind the one that won rather than racing it again.
//
// Safe to take twice in one transaction — a transaction that already holds it
// re-acquires without waiting — which is what lets the approval path take it up
// front, for lock ordering, and provisioning still take it for itself.
func lockOwnerWalletDerivation(ctx context.Context, tx pgx.Tx, ownerID string) error {
	if strings.TrimSpace(ownerID) == "" {
		return nil
	}

	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtext($1), 0);
	`, "location_wallets:"+ownerID); err != nil {
		return fmt.Errorf("error locking wallet derivation for owner %s: %w", ownerID, err)
	}
	return nil
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
	//
	// No ON CONFLICT clause. Under the derivation lock a collision cannot happen
	// for an honest reason, and quietly dropping the row is precisely how the
	// address got recorded with no wallet behind it. If one ever does collide the
	// transaction should die where it happened.
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
			VALUES ($1, $2, FALSE, $3, $4, $5, FALSE, TRUE);
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
