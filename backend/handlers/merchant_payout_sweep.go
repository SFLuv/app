package handlers

import (
	"context"
	"encoding/json"
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
		for _, d := range drains {
			if h := strings.ToLower(strings.TrimSpace(d.DepositTxHash)); h != "" {
				byHash[h] = d
			}
		}
		for _, u := range rows {
			d, ok := byHash[strings.ToLower(u.TxHash)]
			if !ok {
				// Not seen by Bridge yet — a transaction still confirming, or
				// one that never landed. Touch it so it is not retried before
				// everything else, and leave it open.
				_ = a.db.TouchUnwrapSynced(ctx, u.ID)
				continue
			}
			status := db.UnwrapStatusFromDrainState(d.State)
			if err := a.db.UpdateUnwrapFromDrain(ctx, u.TxHash, status, d.ID, d.State, d.TraceNumber); err != nil && a.logger != nil {
				a.logger.Logf("merchant payout sweep: updating unwrap %d failed: %s", u.ID, err)
			}
		}
	}
}
