package handlers

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
)

// A merchant filling in the form should not wait on the chain any longer than
// this. The account factory call is two round trips, each already capped at ten
// seconds; the budget covers the retries as well, and running out means the
// location is created without a till for approval to pick up.
const locationProvisioningBudget = 25 * time.Second

// Two attempts, not more. A retry only happens when another of this merchant's
// locations claimed the index between the read and the write, which needs two
// submissions in flight for one owner at once. A third round would be chasing a
// merchant clicking submit on three shops in the same second.
const locationProvisioningAttempts = 2

// locationWalletMintOnCreateEnabled gates minting a till at creation.
//
// It changes which address a merchant's first location is paid into — their own
// primary wallet before, a wallet of the shop's own now — so it has an off
// switch that does not need a deploy. Turned off, creation leaves the location
// unprovisioned and approval provisions it on the old rule, exactly as before.
func locationWalletMintOnCreateEnabled() bool {
	return envBool("LOCATION_WALLET_MINT_ON_CREATE", true)
}

// deriveLocationWallets asks the account factory for the pair of addresses this
// location's wallets will have. Nil, nil means the location needs none.
//
// Derivation is a read against the chain and is done before any transaction
// opens, so no write locks are held while waiting on an RPC.
func (a *AppService) deriveLocationWallets(
	ctx context.Context,
	provisioning *db.LocationProvisioningContext,
) (*db.DerivedLocationWallets, error) {
	if provisioning == nil || !provisioning.NeedsDerivedWallets() {
		return nil, nil
	}

	paymentIndex := provisioning.DerivationStartIndex()
	tippingIndex := paymentIndex + 1

	paymentAddress, err := a.deriveSmartAccountAddress(ctx, provisioning.OwnerEOA, paymentIndex)
	if err != nil {
		return nil, err
	}
	tippingAddress, err := a.deriveSmartAccountAddress(ctx, provisioning.OwnerEOA, tippingIndex)
	if err != nil {
		return nil, err
	}

	return &db.DerivedLocationWallets{
		PaymentAddress: paymentAddress,
		PaymentIndex:   paymentIndex,
		TippingAddress: tippingAddress,
		TippingIndex:   tippingIndex,
		Street:         provisioning.Street,
	}, nil
}

// provisionNewLocationWallets gives a just-created location its own payment and
// tipping wallets, before it has been anywhere near an admin.
//
// It never reports failure to the caller, and that is the point. Approval used
// to be the only place a till was minted, so a chain outage there cost an admin
// a retry; doing the same at creation would mean an unreachable RPC turns the
// whole merchant sign-up away. So a location that cannot be given a wallet is
// created without one and the approval backstop mints it later.
func (a *AppService) provisionNewLocationWallets(ctx context.Context, locationID uint) {
	if locationID == 0 || !locationWalletMintOnCreateEnabled() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, locationProvisioningBudget)
	defer cancel()

	for attempt := 1; attempt <= locationProvisioningAttempts; attempt++ {
		provisioning, err := a.db.GetLocationProvisioningContext(ctx, locationID)
		if err != nil {
			a.logger.Logf("error loading provisioning context for new location %d: %s", locationID, err)
			return
		}
		// A location created now gets a till of its own rather than inheriting the
		// merchant's personal wallet. Approval leaves this false, so a location
		// submitted before this shipped is still provisioned under the old rule.
		provisioning.AlwaysDerive = true

		if !provisioning.NeedsDerivedWallets() {
			return
		}
		if strings.TrimSpace(provisioning.OwnerEOA) == "" {
			a.logger.Logf("new location %d has no signing wallet to derive a till from; leaving it for approval", locationID)
			return
		}

		derived, err := a.deriveLocationWallets(ctx, provisioning)
		if err != nil {
			a.logger.Logf("could not derive wallets for new location %d, leaving it for approval: %s", locationID, err)
			return
		}

		err = a.db.ProvisionLocationWallets(ctx, locationID, provisioning, derived)
		if err == nil {
			a.logger.Logf("provisioned location %d with payment wallet %s and tipping wallet %s", locationID, derived.PaymentAddress, derived.TippingAddress)
			return
		}
		if errors.Is(err, db.ErrLocationWalletIndexMoved) {
			continue
		}

		a.logger.Logf("error provisioning wallets for new location %d, leaving it for approval: %s", locationID, err)
		return
	}

	a.logger.Logf("gave up provisioning wallets for new location %d after %d attempts; approval will provision it", locationID, locationProvisioningAttempts)
}
