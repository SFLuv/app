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

	// Before anything status-driven, repair profiles left describing more than
	// one Bridge customer: a stale terms status, a KYC link from the previous
	// customer, payout destinations the current one never issued.
	//
	// It runs first, and separately from the pending-KYB pass below, because
	// that pass cannot reach these. Inherited state pins a profile at 'approved'
	// (never listed as pending) or at 'rejected' (excluded outright), and in the
	// rejected case nothing would ever look at it again.
	a.reconcileBridgeProfiles(ctx)

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

// reconcileBridgeProfiles repairs profiles that describe more than one Bridge
// customer at once.
//
// Everything on a payout profile — the KYC link, the terms status, the payout
// addresses — describes ONE Bridge customer. Replacing the customer id in the
// admin panel used to replace only the id, so the rest carried over and quietly
// described the previous business. Attaching now replaces them together; this
// heals the profiles that were attached before it did, on the first sweep after
// boot and every sweep after that.
//
// Bounded by merchant count, not by traffic: one customer read per attached
// business, and address checks only where local state already looks wrong.
func (a *AppService) reconcileBridgeProfiles(ctx context.Context) {
	if a.bridge == nil || !a.bridge.Enabled() {
		return
	}
	profiles, err := a.db.ListAllMerchantPayoutProfiles(ctx)
	if err != nil {
		if a.logger != nil {
			a.logger.Logf("merchant payout sweep: listing profiles for reconciliation failed: %s", err)
		}
		return
	}
	for _, p := range profiles {
		if ctx.Err() != nil {
			return
		}
		if p == nil || strings.TrimSpace(p.BridgeCustomerID) == "" {
			continue
		}
		a.reconcileProfileTOS(ctx, p)

		// Pull this business's bank accounts every sweep, not only while its
		// verification is still moving.
		//
		// Bridge creates an external account asynchronously, so a connection
		// that looked like it failed can land a minute later. Until now the only
		// things that noticed were a page load and the external_account webhook;
		// a merchant who closed the tab after an error, on a deployment without
		// webhooks, would simply never see their bank.
		if err := a.syncMerchantBankAccounts(ctx, p); err != nil && a.logger != nil {
			a.logger.Logf("merchant payout sweep: bank sync failed for %s: %s", p.OwnerID, err)
		}

		a.reconcileProfileAddresses(ctx, p)
	}
	a.reconcileMismatchedKYCLinks(ctx)
}

// reconcileProfileTOS corrects a terms status inherited from a customer this
// business is no longer attached to.
//
// The inherited value is only dangerous in one direction. A stale 'approved'
// makes the merchant's card hide the terms step and makes the status endpoint
// skip its own refresh — which is gated on the status not already being
// approved — so nothing else in the system would ever notice.
func (a *AppService) reconcileProfileTOS(ctx context.Context, p *structs.MerchantPayoutProfile) {
	customer, err := a.bridge.GetCustomer(ctx, p.BridgeCustomerID)
	if err != nil || customer == nil {
		return
	}
	tos := "pending"
	if customer.HasAcceptedTOS {
		tos = bridge.KYCApproved
	}
	if strings.EqualFold(tos, p.TOSStatus) {
		return
	}
	if err := a.db.SetMerchantKYBStatus(ctx, p.OwnerID, p.KYBStatus, tos); err != nil {
		if a.logger != nil {
			a.logger.Logf("merchant payout sweep: could not correct the terms status for %s: %s", p.OwnerID, err)
		}
		return
	}
	if a.logger != nil && strings.EqualFold(p.TOSStatus, bridge.KYCApproved) {
		a.logger.Logf(
			"merchant payout sweep: %s showed terms accepted but customer %s has not accepted them — corrected. "+
				"Their card will now offer the terms before a bank can be connected.",
			p.OwnerID, p.BridgeCustomerID,
		)
	}
}

// reconcileProfileAddresses removes payout destinations the current Bridge
// customer did not issue.
//
// The local signal comes first: a destination whose bank is no longer one of
// this business's active accounts is the shape a replaced customer leaves
// behind. Only then is Bridge asked, so a settled estate costs nothing and a
// destination is never removed on a guess.
func (a *AppService) reconcileProfileAddresses(ctx context.Context, p *structs.MerchantPayoutProfile) {
	locationIDs, err := a.db.ListApprovedLocationIDsForOwner(ctx, p.OwnerID)
	if err != nil || len(locationIDs) == 0 {
		return
	}
	banks, err := a.db.ListMerchantBankAccounts(ctx, p.OwnerID)
	if err != nil {
		return
	}
	activeBanks := make(map[string]bool, len(banks))
	for _, b := range banks {
		activeBanks[b.BridgeExternalAccountID] = true
	}

	suspect := map[uint64]*structs.LocationLiquidationAddress{}
	for _, locationID := range locationIDs {
		current, err := a.db.GetLocationLiquidationAddress(ctx, locationID)
		if err != nil || current == nil {
			continue
		}
		if current.BridgeExternalAccountID != "" && activeBanks[current.BridgeExternalAccountID] {
			continue
		}
		suspect[locationID] = current
	}
	if len(suspect) == 0 {
		return
	}

	issued, err := a.bridge.ListLiquidationAddresses(ctx, p.BridgeCustomerID)
	if err != nil {
		if a.logger != nil {
			a.logger.Logf(
				"merchant payout sweep: %d payout destination(s) for %s look stale but Bridge could not be read (%s); left untouched",
				len(suspect), p.OwnerID, err,
			)
		}
		return
	}
	valid := make(map[string]bool, len(issued))
	for _, la := range issued {
		valid[la.ID] = true
	}

	for locationID, current := range suspect {
		// Bridge still issues it, so the local signal was only a retired bank
		// mirror. Leave the money alone.
		if current.BridgeLiquidationAddressID != "" && valid[current.BridgeLiquidationAddressID] {
			continue
		}
		if err := a.db.DeleteLocationLiquidationAddress(ctx, locationID); err != nil {
			if a.logger != nil {
				a.logger.Logf("merchant payout sweep: could not clear the stale payout address on location %d: %s", locationID, err)
			}
			continue
		}
		if a.logger != nil {
			a.logger.Logf(
				"merchant payout sweep: cleared payout address %s from location %d — customer %s did not issue it. "+
					"The location needs a bank attached again before it can unwrap.",
				current.Address, locationID, p.BridgeCustomerID,
			)
		}
	}
}

// reconcileMismatchedKYCLinks drops KYC links that belong to a different Bridge
// customer than the profile's own, then re-reads the real status from the
// attached customer.
//
// This exists because an admin attaching a customer by hand used to leave the
// previous link in place. Every sync afterwards read that link and overwrote a
// verified merchant's status with an unrelated business's — which quietly
// un-approved them, and un-approving them is what made payout provisioning
// refuse to attach a bank they had just connected.
//
// Cheap by construction: it only touches profiles holding both a customer and a
// link, and only calls Bridge for those.
func (a *AppService) reconcileMismatchedKYCLinks(ctx context.Context) {
	if a.bridge == nil || !a.bridge.Enabled() {
		return
	}
	profiles, err := a.db.ListAllMerchantPayoutProfiles(ctx)
	if err != nil {
		if a.logger != nil {
			a.logger.Logf("merchant payout sweep: listing profiles for KYC-link reconciliation failed: %s", err)
		}
		return
	}
	for _, p := range profiles {
		if ctx.Err() != nil {
			return
		}
		if p == nil || strings.TrimSpace(p.BridgeKYCLinkID) == "" || strings.TrimSpace(p.BridgeCustomerID) == "" {
			continue
		}
		link, err := a.bridge.GetKYCLink(ctx, p.BridgeKYCLinkID)
		if err != nil {
			if !bridge.IsNotFound(err) {
				continue
			}
			// The link no longer exists on Bridge; keeping the id only risks
			// this same confusion later.
			link = nil
		}
		if link != nil && (link.CustomerID == "" || strings.EqualFold(link.CustomerID, p.BridgeCustomerID)) {
			continue
		}

		if err := a.db.ClearMerchantKYCLink(ctx, p.OwnerID); err != nil {
			if a.logger != nil {
				a.logger.Logf("merchant payout sweep: could not clear the mismatched KYC link for %s: %s", p.OwnerID, err)
			}
			continue
		}
		if a.logger != nil {
			other := "(missing on bridge)"
			if link != nil {
				other = link.CustomerID
			}
			a.logger.Logf(
				"merchant payout sweep: cleared KYC link %s from owner %s — it belonged to customer %s, not the attached %s",
				p.BridgeKYCLinkID, p.OwnerID, other, p.BridgeCustomerID,
			)
		}

		// Now that the wrong link is gone, the attached customer is the only
		// source of truth left, so read the status straight from it.
		refreshed, err := a.db.GetMerchantPayoutProfile(ctx, p.OwnerID)
		if err != nil || refreshed == nil {
			continue
		}
		if _, err := a.syncMerchantKYB(ctx, refreshed); err != nil && a.logger != nil {
			a.logger.Logf("merchant payout sweep: KYB re-sync after clearing the link failed for %s: %s", p.OwnerID, err)
		}
	}
}
