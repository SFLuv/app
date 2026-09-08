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
// Off by default. Wallets are minted during the approval itself, which is what
// the Location Approval Form's flow assumes: an application is something a
// merchant can still cancel while it is pending, and a cancelled application
// should not have left a wallet and a smart-account index behind it. It also
// puts the tipping decision where the answer is known to be final — the form's
// "do you accept tips" is what decides whether a tipping wallet exists at all,
// and a merchant can still edit that answer up until an admin acts on it.
//
// The switch stays because the trade-off it was added for is real: with minting
// at creation the merchant's Locations tab has a till address to show them while
// the listing sits in the queue. Turned on, creation provisions and approval is
// the backstop, exactly as before.
func locationWalletMintOnCreateEnabled() bool {
	return envBool("LOCATION_WALLET_MINT_ON_CREATE", false)
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

	// Indexes are handed out in order from the first free one, and only for the
	// halves that are actually wanted. A shop inheriting the merchant's primary
	// wallet for takings but minting a tipping wallet takes one index, not two:
	// burning the payment index anyway would leave a gap that nothing owns.
	derived := &db.DerivedLocationWallets{Street: provisioning.Street}
	index := provisioning.DerivationStartIndex()

	if provisioning.NeedsDerivedPaymentWallet() {
		address, err := a.deriveSmartAccountAddress(ctx, provisioning.OwnerEOA, index)
		if err != nil {
			return nil, err
		}
		derived.PaymentAddress = address
		derived.PaymentIndex = index
		index += 1
	}

	if provisioning.NeedsDerivedTippingWallet() {
		address, err := a.deriveSmartAccountAddress(ctx, provisioning.OwnerEOA, index)
		if err != nil {
			return nil, err
		}
		derived.TippingAddress = address
		derived.TippingIndex = index
	}

	return derived, nil
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
