package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bridge"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
)

// structsProfile and jsonUnmarshal exist so the webhook file reads cleanly
// without importing two packages for one lookup.
type structsProfile = structs.MerchantPayoutProfile

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

// The sweep is the fallback for webhooks. Webhooks are faster, but a missed
// delivery must not strand a merchant on "processing" forever, and a KYB
// approval must show up whether or not Bridge told us. Both worklists are
// small and both calls are idempotent, so running it on a timer is cheap.
const merchantPayoutSweepInterval = 10 * time.Minute

func StartMerchantPayoutSweep(ctx context.Context, a *AppService, appLogger *logger.LogCloser) {
	if ctx == nil || a == nil || a.bridge == nil || !a.bridge.Enabled() {
		return
	}
	go func() {
		ticker := time.NewTicker(merchantPayoutSweepInterval)
		defer ticker.Stop()
		a.RunMerchantPayoutSweep(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.RunMerchantPayoutSweep(ctx)
			}
		}
	}()
	if appLogger != nil {
		appLogger.Logf("merchant payout sweep started (every %s)", merchantPayoutSweepInterval)
	}
}

func (a *AppService) RunMerchantPayoutSweep(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 4*time.Minute)
	defer cancel()

	pending, err := a.db.ListMerchantPayoutProfilesPendingKYB(ctx, 100)
	if err != nil && a.logger != nil {
		a.logger.Logf("merchant payout sweep: listing pending KYB failed: %s", err)
	}
	for _, p := range pending {
		if ctx.Err() != nil {
			return
		}
		synced, err := a.syncMerchantKYB(ctx, p)
		if err != nil {
			if a.logger != nil {
				a.logger.Logf("merchant payout sweep: KYB sync failed for %s: %s", p.OwnerID, err)
			}
			continue
		}
		if synced != nil && synced.KYBStatus == bridge.KYCApproved && synced.BridgeCustomerID != "" {
			_ = a.syncMerchantBankAccounts(ctx, synced)
			if _, err := a.provisionLiquidationAddresses(ctx, synced.OwnerID, "", 0); err != nil && a.logger != nil {
				a.logger.Logf("merchant payout sweep: provisioning failed for %s: %s", synced.OwnerID, err)
			}
		}
	}

	a.syncOpenUnwraps(ctx, 100)
}

// syncOpenUnwraps matches every unfinished ledger row to its Bridge drain by
// the on-chain deposit hash and carries the state across.
func (a *AppService) syncOpenUnwraps(ctx context.Context, limit int) {
	open, err := a.db.ListOpenUnwraps(ctx, limit)
	if err != nil {
		if a.logger != nil {
			a.logger.Logf("merchant payout sweep: listing open unwraps failed: %s", err)
		}
		return
	}
	if len(open) == 0 {
		return
	}

	// Drains are listed per liquidation address, so group the work by
	// destination and fetch each address's history once.
	type key struct{ customerID, addressID string }
	byKey := map[key][]*structs.Unwrap{}
	for _, u := range open {
		if u.LocationID == nil {
			continue
		}
		la, err := a.db.GetLocationLiquidationAddress(ctx, uint64(*u.LocationID))
		if err != nil || la == nil || la.BridgeLiquidationAddressID == "" {
			continue
		}
		profile, err := a.db.GetMerchantPayoutProfile(ctx, la.OwnerID)
		if err != nil || profile == nil || profile.BridgeCustomerID == "" {
			continue
		}
		k := key{profile.BridgeCustomerID, la.BridgeLiquidationAddressID}
		byKey[k] = append(byKey[k], u)
	}

	for k, rows := range byKey {
		if ctx.Err() != nil {
			return
		}
		drains, err := a.bridge.ListDrains(ctx, k.customerID, k.addressID)
		if err != nil {
			if a.logger != nil {
				a.logger.Logf("merchant payout sweep: listing drains for %s failed: %s", k.addressID, err)
			}
			continue
		}
		byHash := map[string]bridge.Drain{}
		byID := map[string]bridge.Drain{}
		for _, d := range drains {
			if h := strings.ToLower(strings.TrimSpace(d.DepositTxHash)); h != "" {
				byHash[h] = d
			}
			byID[d.ID] = d
		}
		for _, u := range rows {
			// A row already linked to a drain keeps following that drain;
			// hashes only matter for the first match.
			var d bridge.Drain
			ok := false
			if u.BridgeDrainID != "" {
				d, ok = byID[u.BridgeDrainID]
			}
			if !ok {
				d, ok = byHash[strings.ToLower(u.TxHash)]
			}
			if !ok {
				// The web app submits through an ERC-4337 bundler and records
				// the hash it gets back, which can be the user operation's
				// rather than the transaction's (verified on Celo: Bridge
				// reports the bundle tx). So fall back to the deposit itself:
				// same address, same amount, landed within minutes of the
				// row, and not already claimed by another row. Exactly one
				// candidate or nothing.
				if fb, found := a.fallbackDrainForUnwrap(ctx, u, drains); found {
					d, ok = fb, true
				}
			}
			if !ok {
				// Not seen by Bridge yet — a transaction still confirming, or
				// one that never landed. Touch it so it is not retried before
				// everything else, and leave it open.
				_ = a.db.TouchUnwrapSynced(ctx, u.ID)
				continue
			}
			status := db.UnwrapStatusFromDrainState(d.State)
			if err := a.db.UpdateUnwrapFromDrain(ctx, u.ID, d.DepositTxHash, status, d.ID, d.State, d.TraceNumber); err != nil && a.logger != nil {
				a.logger.Logf("merchant payout sweep: updating unwrap %d failed: %s", u.ID, err)
			}
		}
	}
}

// fallbackDrainMatchWindow bounds how far a drain's on-chain deposit time may
// sit from the ledger row's creation for the two to be treated as the same
// event. Rows are written right after the bundler accepts the operation.
const fallbackDrainMatchWindow = 30 * time.Minute

func (a *AppService) fallbackDrainForUnwrap(ctx context.Context, u *structs.Unwrap, drains []bridge.Drain) (bridge.Drain, bool) {
	want, ok := new(big.Int).SetString(u.AmountWei, 10)
	if !ok {
		return bridge.Drain{}, false
	}
	var matches []bridge.Drain
	for _, d := range drains {
		amt, err := drainAmountBaseUnits(d.Amount)
		if err != nil || amt.Cmp(want) != 0 {
			continue
		}
		ts, err := time.Parse(time.RFC3339, d.DepositTxTimestamp)
		if err != nil {
			continue
		}
		if diff := ts.Sub(u.CreatedAt); diff < -fallbackDrainMatchWindow || diff > fallbackDrainMatchWindow {
			continue
		}
		claimed, err := a.db.UnwrapExistsForDrain(ctx, d.ID)
		if err != nil || claimed {
			continue
		}
		matches = append(matches, d)
	}
	if len(matches) != 1 {
		return bridge.Drain{}, false
	}
	if a.logger != nil {
		a.logger.Logf("merchant payout sweep: unwrap %d matched drain %s by amount+time (ledger hash %s, bridge hash %s)", u.ID, matches[0].ID, u.TxHash, matches[0].DepositTxHash)
	}
	return matches[0], true
}

// drainAmountBaseUnits turns Bridge's decimal USDC amount ("5.0") into token
// base units (6 decimals), the unit the ledger stores.
func drainAmountBaseUnits(amount string) (*big.Int, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, fmt.Errorf("empty amount")
	}
	whole, frac, _ := strings.Cut(amount, ".")
	if len(frac) > 6 {
		frac = frac[:6]
	}
	for len(frac) < 6 {
		frac += "0"
	}
	if whole == "" {
		whole = "0"
	}
	n, ok := new(big.Int).SetString(whole+frac, 10)
	if !ok {
		return nil, fmt.Errorf("bad amount %q", amount)
	}
	return n, nil
}
