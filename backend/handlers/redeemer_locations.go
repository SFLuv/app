package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// Bounds on one sweep. It sends one transaction per address that does not
// already hold the role, serially, waiting for each to mine, so its cost is a
// function of how far behind the chain is rather than of how many locations
// exist. On a caught-up estate it sends none at all.
//
// The pause keeps a sweep of a long-neglected estate from taking the nonce and
// the block space out from under the faucet and the bundler, which share this
// chain. The cap and the budget are what stop a bad day being an expensive one:
// whichever is reached first ends the run, and the rest is picked up on the next
// start with the count of what was left in the log.
const (
	locationRedeemerRunBudget       = 20 * time.Minute
	locationRedeemerGrantPause      = 500 * time.Millisecond
	locationRedeemerMaxGrantsPerRun = 200
)

// LocationRedeemerSyncSummary is what one sweep did, for the log line at the end
// of it.
type LocationRedeemerSyncSummary struct {
	// Wallets is every address attached to an active location.
	Wallets int
	// AlreadyHeld covers both wallets we had already recorded as redeemers and
	// wallets the chain said already held the role.
	AlreadyHeld int
	Granted     int
	Failed      int
	// Deferred is what the grant cap or the run budget left for the next boot.
	Deferred int
}

// locationRedeemerSyncEnabled gates granting REDEEMER_ROLE to location wallets
// at startup.
//
// It decides who can turn tokens back into money, so it has an off switch that
// does not need a deploy. Off, the sweep does not run and nothing else changes:
// the per-approval grant for a merchant's own wallet is a separate path.
func locationRedeemerSyncEnabled() bool {
	return envBool("REDEEMER_LOCATION_WALLET_SYNC", true)
}

// SyncLocationWallets grants REDEEMER_ROLE to every wallet attached to a
// location.
//
// A location's till is where a shop's takings land, and redeeming is how those
// takings become dollars, so the address that receives has to be an address that
// can redeem. Granting it per owner at approval no longer covers that: tills are
// derived per location now, so a merchant's second shop is paid into an address
// their account-level grant never touched.
//
// Nothing here is fatal. Every failure is logged against the address it happened
// to and the sweep moves on, because the alternative — a chain outage taking the
// server down with it — costs far more than a merchant waiting a restart for a
// role they cannot use until they have takings to redeem anyway.
func (r *RedeemerService) SyncLocationWallets(ctx context.Context) (LocationRedeemerSyncSummary, error) {
	var summary LocationRedeemerSyncSummary

	if !r.IsEnabled() {
		return summary, nil
	}
	if !locationRedeemerSyncEnabled() {
		r.logf("location redeemer sync disabled: REDEEMER_LOCATION_WALLET_SYNC is false")
		return summary, nil
	}
	if r.appDb == nil {
		return summary, fmt.Errorf("app db is not configured for location redeemer sync")
	}

	ctx, cancel := context.WithTimeout(ctx, locationRedeemerRunBudget)
	defer cancel()

	wallets, err := r.appDb.GetLocationWallets(ctx)
	if err != nil {
		return summary, fmt.Errorf("error loading location wallets for redeemer sync: %w", err)
	}
	summary.Wallets = len(wallets)

	for index, wallet := range wallets {
		// Out of time or out of allowance. Everything after this point in the list
		// is untouched, and the list is ordered by address, so the next run walks
		// it in the same order and reaches what this one did not.
		if ctx.Err() != nil || summary.Granted >= locationRedeemerMaxGrantsPerRun {
			summary.Deferred = len(wallets) - index
			break
		}

		// Our own record first. It is the only thing standing between a restart
		// and a chain read per location, and it is only ever used to skip work:
		// a wallet recorded as a redeemer is left alone, never granted again.
		if wallet.IsRedeemer {
			summary.AlreadyHeld++
			continue
		}

		if !common.IsHexAddress(strings.TrimSpace(wallet.Address)) {
			r.logf("skipping %s wallet %q on location %d (%s): not an address", wallet.Role, wallet.Address, wallet.LocationID, wallet.LocationName)
			summary.Failed++
			continue
		}
		address := common.HexToAddress(strings.TrimSpace(wallet.Address))

		hasRole, err := r.contract.HasRole(&bind.CallOpts{Context: ctx}, r.redeemerRole, address)
		if err != nil {
			r.logf("error checking REDEEMER_ROLE for %s wallet %s on location %d: %s", wallet.Role, address.Hex(), wallet.LocationID, err)
			summary.Failed++
			continue
		}

		if hasRole {
			summary.AlreadyHeld++
		} else {
			if err := r.grantRedeemerRole(ctx, address); err != nil {
				r.logf("error granting REDEEMER_ROLE to %s wallet %s on location %d (%s): %s", wallet.Role, address.Hex(), wallet.LocationID, wallet.LocationName, err)
				summary.Failed++
				continue
			}
			summary.Granted++
			r.sleep(ctx, locationRedeemerGrantPause)
		}

		// Recorded only after the chain agrees, so a flag can never claim a role
		// that was never granted. Addresses with no wallets row have nowhere to
		// put this and are re-read from the chain on every boot; that is a read,
		// not a transaction.
		if wallet.WalletID != nil {
			if err := r.appDb.SetWalletRedeemerStatus(ctx, *wallet.WalletID, true); err != nil {
				r.logf("error recording is_redeemer for wallet %d (%s): %s", *wallet.WalletID, address.Hex(), err)
			}
		}
	}

	r.logf(
		"location redeemer sync finished: %d wallets, %d already held, %d granted, %d failed, %d deferred",
		summary.Wallets, summary.AlreadyHeld, summary.Granted, summary.Failed, summary.Deferred,
	)

	return summary, nil
}

func (r *RedeemerService) sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
