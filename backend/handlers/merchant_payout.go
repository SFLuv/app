package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bridge"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// Merchant bank payouts.
//
// A merchant's SFLUV leaves the system in one move: the location's own wallet
// calls withdrawTo on the token, which burns SFLUV and sends the backing USDC
// to a Bridge liquidation address. Bridge drains that address to the
// merchant's bank. Everything in this file is the bookkeeping around that
// move — who the merchant is to Bridge, which bank, which address per
// location, and what became of each unwrap — and none of it touches funds.
//
// The one thing a merchant can get wrong with money is the destination, so
// merchants never type one. Bridge issues the address, we store it, the
// merchant sees a bank name and four digits. An admin can override an address
// by hand, and even that is checked against the list Bridge issued for the
// same business before it is accepted.

const bridgeAPITimeout = 25 * time.Second

// bridgeReady is the gate every payout handler passes first. A missing client
// is a deploy without credentials, and the honest answer to a merchant is
// "not available yet", not a stack trace.
func (a *AppService) bridgeReady(w http.ResponseWriter) (*bridge.Client, bool) {
	if a == nil || a.bridge == nil || !a.bridge.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Bank payouts are not available yet.",
		})
		return nil, false
	}
	return a.bridge, true
}

func (a *AppService) SetBridgeClient(client *bridge.Client) {
	a.bridge = client
}

// --- KYB ------------------------------------------------------------------

// startMerchantKYB creates the business on Bridge and records the hosted link.
// Idempotent on owner: a business that already has a Bridge customer gets a
// fresh hosted link for that customer rather than a second customer.
func (a *AppService) startMerchantKYB(ctx context.Context, ownerID, businessName, email string) (*structs.MerchantPayoutProfile, string, error) {
	if a.bridge == nil || !a.bridge.Enabled() {
		return nil, "", bridge.ErrDisabled
	}

	profile, err := a.db.GetMerchantPayoutProfile(ctx, ownerID)
	if err != nil {
		return nil, "", err
	}
	if profile != nil && profile.BridgeCustomerID != "" {
		url, err := a.bridge.HostedKYCLinkForCustomer(ctx, profile.BridgeCustomerID)
		if err != nil {
			return profile, "", err
		}
		return profile, url, nil
	}

	businessName = strings.TrimSpace(businessName)
	email = strings.TrimSpace(email)
	if businessName == "" || email == "" {
		return profile, "", fmt.Errorf("business name and email are required to start verification")
	}

	link, err := a.bridge.CreateKYCLink(ctx, bridge.CreateKYCLinkInput{BusinessLegalName: businessName, Email: email})
	if err != nil {
		return profile, "", err
	}
	if err := a.db.StartMerchantKYB(ctx, ownerID, link.CustomerID, link.ID, link.KYCStatus, link.TOSStatus, link.KYCLink); err != nil {
		return profile, "", err
	}
	profile, err = a.db.GetMerchantPayoutProfile(ctx, ownerID)
	return profile, link.KYCLink, err
}

// startMerchantKYBForLocation is the approval-time hook: the admin approved a
// location, so its business is invited to verify. Returns the hosted link for
// the approval email, or "" when there is nothing to add (no Bridge client, a
// business already verified, or a failure — which is logged, never fatal to
// the approval).
func (a *AppService) startMerchantKYBForLocation(ctx context.Context, locationID uint, ownerID string) string {
	if a.bridge == nil || !a.bridge.Enabled() {
		return ""
	}
	existing, err := a.db.GetMerchantPayoutProfile(ctx, ownerID)
	if err == nil && existing != nil && existing.KYBStatus == bridge.KYCApproved {
		return ""
	}
	contact, err := a.db.GetLocationApprovalContact(ctx, locationID)
	if err != nil {
		a.logger.Logf("merchant payout: could not load approval contact for location %d: %s", locationID, err)
		return ""
	}
	_, url, err := a.startMerchantKYB(ctx, ownerID, contact.Name, contact.AdminEmail)
	if err != nil {
		a.logger.Logf("merchant payout: could not start KYB for owner %s (location %d): %s", ownerID, locationID, err)
		return ""
	}
	return url
}

// syncMerchantKYB re-reads the business's status from Bridge. It prefers the
// KYC link (which carries both statuses) and falls back to the customer.
func (a *AppService) syncMerchantKYB(ctx context.Context, profile *structs.MerchantPayoutProfile) (*structs.MerchantPayoutProfile, error) {
	if profile == nil || a.bridge == nil || !a.bridge.Enabled() {
		return profile, nil
	}
	kyb, tos := "", ""
	if profile.BridgeKYCLinkID != "" {
		link, err := a.bridge.GetKYCLink(ctx, profile.BridgeKYCLinkID)
		if err != nil && !bridge.IsNotFound(err) {
			return profile, err
		}
		if link != nil {
			kyb, tos = link.KYCStatus, link.TOSStatus
			if profile.BridgeCustomerID == "" && link.CustomerID != "" {
				if err := a.db.AttachBridgeCustomer(ctx, profile.OwnerID, link.CustomerID, kyb); err != nil {
					return profile, err
				}
			}
		}
	}
	if kyb == "" && profile.BridgeCustomerID != "" {
		customer, err := a.bridge.GetCustomer(ctx, profile.BridgeCustomerID)
		if err != nil {
			return profile, err
		}
		// A customer that is active and owes nothing but a bank account is,
		// for our purposes, verified.
		if strings.EqualFold(customer.Status, "active") {
			kyb = bridge.KYCApproved
		} else {
			kyb = customer.Status
		}
	}
	if kyb == "" {
		return profile, nil
	}
	if err := a.db.SetMerchantKYBStatus(ctx, profile.OwnerID, kyb, tos); err != nil {
		return profile, err
	}
	return a.db.GetMerchantPayoutProfile(ctx, profile.OwnerID)
}

// --- Bank accounts and provisioning -----------------------------------------

func (a *AppService) syncMerchantBankAccounts(ctx context.Context, profile *structs.MerchantPayoutProfile) error {
	if profile == nil || profile.BridgeCustomerID == "" {
		return nil
	}
	accounts, err := a.bridge.ListExternalAccounts(ctx, profile.BridgeCustomerID)
	if err != nil {
		return err
	}
	mirror := make([]structs.MerchantBankAccount, 0, len(accounts))
	for _, acct := range accounts {
		mirror = append(mirror, structs.MerchantBankAccount{
			OwnerID:                 profile.OwnerID,
			BridgeExternalAccountID: acct.ID,
			BankName:                acct.BankName,
			Last4:                   acct.Last4,
			AccountOwnerName:        acct.AccountOwnerName,
			Currency:                acct.Currency,
			Active:                  acct.Active,
		})
	}
	return a.db.SyncMerchantBankAccounts(ctx, profile.OwnerID, mirror)
}

// provisionLiquidationAddresses points ONE location at a bank, or refreshes the
// locations that already have a destination. It never gives a destination to a
// location that has not been explicitly attached.
//
// That last part is the rule. It used to fill in every approved location the
// business had, so linking a bank for one shop silently routed every other
// shop's takings to the same account — a decision the merchant never made,
// about money, discovered after the fact. A second location now shows
// "connect a bank" until somebody attaches one, even when the bank is the same.
//
// The Bridge ADDRESS is still shared, because Bridge allows exactly one per
// (customer, bank, chain, currency, rail) and refuses a second. Sharing the
// address is forced; inheriting the destination is not, and only the second one
// was ever a choice.
//
// It is idempotent from the merchant's side: run it twice and nothing changes.
// Bridge addresses are permanent, so an existing Bridge address bound to the
// same bank is reused rather than minted again.
func (a *AppService) provisionLiquidationAddresses(ctx context.Context, ownerID string, preferredExternalAccountID string, onlyLocationID uint64) (structs.ProvisionLiquidationAddressesResponse, error) {
	resp := structs.ProvisionLiquidationAddressesResponse{Provisioned: []structs.LocationLiquidationAddress{}, Skipped: []uint64{}}

	profile, err := a.db.GetMerchantPayoutProfile(ctx, ownerID)
	if err != nil {
		return resp, err
	}
	if profile == nil || profile.BridgeCustomerID == "" {
		resp.Message = "Business verification has not started."
		return resp, nil
	}
	if profile.KYBStatus != bridge.KYCApproved {
		resp.Message = "Business verification is not complete yet."
		return resp, nil
	}

	banks, err := a.db.ListMerchantBankAccounts(ctx, ownerID)
	if err != nil {
		return resp, err
	}
	if len(banks) == 0 {
		resp.Message = "Connect a bank account first."
		return resp, nil
	}
	targetBank := banks[0].BridgeExternalAccountID
	if preferredExternalAccountID != "" {
		found := false
		for _, b := range banks {
			if b.BridgeExternalAccountID == preferredExternalAccountID {
				found = true
				break
			}
		}
		if !found {
			return resp, fmt.Errorf("that bank account is not linked to this business")
		}
		targetBank = preferredExternalAccountID
	}

	existingOnBridge, err := a.bridge.ListLiquidationAddresses(ctx, profile.BridgeCustomerID)
	if err != nil {
		return resp, err
	}

	// Bridge allows exactly one liquidation address per (customer, bank,
	// chain, currency, rail); a second create for the same bank is a 400
	// (verified against sandbox). So every location paying into the same
	// bank shares one address. That loses nothing: a drain is matched to a
	// location through our ledger (deposit tx hash → unwrap row), never
	// through the address itself.
	shared := findLiquidationAddressForBank(existingOnBridge, targetBank)
	if shared == nil {
		created, err := a.bridge.CreateLiquidationAddress(ctx, bridge.CreateLiquidationAddressInput{
			CustomerID:        profile.BridgeCustomerID,
			ExternalAccountID: targetBank,
			ACHReference:      "SFLUV",
		})
		if err != nil {
			// Lost a race with another provisioning call: the address now
			// exists, so read it back rather than fail the merchant.
			if !strings.Contains(err.Error(), "already exists") {
				return resp, err
			}
			existingOnBridge, err = a.bridge.ListLiquidationAddresses(ctx, profile.BridgeCustomerID)
			if err != nil {
				return resp, err
			}
			shared = findLiquidationAddressForBank(existingOnBridge, targetBank)
			if shared == nil {
				return resp, fmt.Errorf("bridge reports an address for this bank but does not list it")
			}
		} else {
			shared = created
		}
	}

	locationIDs, err := a.db.ListApprovedLocationIDsForOwner(ctx, ownerID)
	if err != nil {
		return resp, err
	}
	for _, locationID := range locationIDs {
		if onlyLocationID != 0 && locationID != onlyLocationID {
			continue
		}
		current, err := a.db.GetLocationLiquidationAddress(ctx, locationID)
		if err != nil {
			return resp, err
		}
		// A location with no destination only gets one when it is the location
		// somebody asked about. Sweeps, webhooks and "finish setup" pass no
		// target, and for them an un-attached location stays un-attached.
		if current == nil && onlyLocationID == 0 {
			resp.Skipped = append(resp.Skipped, locationID)
			continue
		}
		// Keep what exists unless this call is an explicit re-point of one
		// location to a chosen bank. Admin overrides are never replaced by
		// provisioning; an admin put them there on purpose.
		if current != nil {
			repoint := onlyLocationID != 0 && preferredExternalAccountID != "" && current.BridgeExternalAccountID != targetBank
			if !repoint || current.Source == "admin" {
				resp.Skipped = append(resp.Skipped, locationID)
				continue
			}
		}

		row := structs.LocationLiquidationAddress{
			LocationID:                 locationID,
			OwnerID:                    ownerID,
			BridgeLiquidationAddressID: shared.ID,
			Address:                    shared.Address,
			Chain:                      shared.Chain,
			Currency:                   shared.Currency,
			DestinationPaymentRail:     shared.DestinationPaymentRail,
			DestinationCurrency:        shared.DestinationCurrency,
			BridgeExternalAccountID:    shared.ExternalAccountID,
			Source:                     "bridge",
		}
		if err := a.db.UpsertLocationLiquidationAddress(ctx, &row); err != nil {
			return resp, err
		}
		resp.Provisioned = append(resp.Provisioned, row)
	}
	return resp, nil
}

// findLiquidationAddressForBank picks the Celo USDC → ACH address Bridge holds
// for a given bank, or nil when none has been minted yet.
func findLiquidationAddressForBank(all []bridge.LiquidationAddress, externalAccountID string) *bridge.LiquidationAddress {
	for i := range all {
		la := &all[i]
		if la.ExternalAccountID == externalAccountID &&
			strings.EqualFold(la.Chain, bridge.Chain) &&
			strings.EqualFold(la.Currency, bridge.Currency) &&
			strings.EqualFold(la.DestinationPaymentRail, bridge.DestinationRail) {
			return la
		}
	}
	return nil
}

// --- Merchant endpoints -----------------------------------------------------

// GetMerchantPayoutStatus is one read for the whole settings section. It
// refreshes KYB and bank accounts from Bridge when the business is mid-flow,
// so a merchant returning from Plaid or from the hosted KYB page sees the
// result without a manual refresh.
func (a *AppService) GetMerchantPayoutStatus(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	resp := structs.MerchantPayoutStatusResponse{
		Enabled:      a.bridge != nil && a.bridge.Enabled(),
		Production:   a.bridge != nil && a.bridge.Production(),
		BankAccounts: []structs.MerchantBankAccount{},
		Locations:    []structs.LocationLiquidationAddress{},
		Unwraps:      []*structs.Unwrap{},
	}

	ctx, cancel := context.WithTimeout(r.Context(), bridgeAPITimeout)
	defer cancel()

	profile, err := a.db.GetMerchantPayoutProfile(ctx, *userDid)
	if err != nil {
		a.logger.Logf("merchant payout: status load failed for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if profile != nil && resp.Enabled {
		if profile.KYBStatus != bridge.KYCApproved {
			if synced, err := a.syncMerchantKYB(ctx, profile); err == nil && synced != nil {
				profile = synced
			} else if err != nil {
				a.logger.Logf("merchant payout: KYB sync failed for %s: %s", *userDid, err)
			}
		}
		if profile.BridgeCustomerID != "" {
			if err := a.syncMerchantBankAccounts(ctx, profile); err != nil {
				a.logger.Logf("merchant payout: bank sync failed for %s: %s", *userDid, err)
			}
		}
	}
	resp.Profile = profile

	if banks, err := a.db.ListMerchantBankAccounts(ctx, *userDid); err == nil {
		resp.BankAccounts = banks
	}
	if locs, err := a.db.ListLocationLiquidationAddressesByOwner(ctx, *userDid); err == nil {
		resp.Locations = locs
	}
	if unwraps, err := a.db.ListUnwrapsByOwner(ctx, *userDid, 50); err == nil {
		resp.Unwraps = unwraps
	}
	writeJSON(w, http.StatusOK, resp)
}

// RequestMerchantKYBLink hands the merchant a hosted verification link. This
// is the "Verify your business" button and the "resend" path in one.
func (a *AppService) RequestMerchantKYBLink(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if _, ok := a.bridgeReady(w); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bridgeAPITimeout)
	defer cancel()

	user, err := a.db.GetUserById(ctx, *userDid)
	if err != nil || !user.IsMerchant {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	// The business name is the first approved location's name — the same one
	// the approval email used — and the email is the merchant's contact.
	businessName, email := "", ""
	if user.Name != nil {
		businessName = *user.Name
	}
	if user.Email != nil {
		email = *user.Email
	}
	if ids, err := a.db.ListApprovedLocationIDsForOwner(ctx, *userDid); err == nil && len(ids) > 0 {
		if contact, err := a.db.GetLocationApprovalContact(ctx, uint(ids[0])); err == nil {
			if contact.Name != "" {
				businessName = contact.Name
			}
			if contact.AdminEmail != "" {
				email = contact.AdminEmail
			}
		}
	}

	profile, url, err := a.startMerchantKYB(ctx, *userDid, businessName, email)
	if err != nil {
		a.logger.Logf("merchant payout: KYB link failed for %s: %s", *userDid, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start verification right now. Try again in a moment."})
		return
	}
	status := bridge.KYCNotStarted
	if profile != nil {
		status = profile.KYBStatus
	}
	writeJSON(w, http.StatusOK, structs.KYBLinkResponse{KYBStatus: status, URL: url})
}

// CreateMerchantPlaidLinkToken opens the door to Plaid Link. Requires a
// verified business: Bridge will not attach a bank to a customer it has not
// cleared, and asking a merchant to connect a bank that then bounces is worse
// than telling them to finish verification.
func (a *AppService) CreateMerchantPlaidLinkToken(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	client, ok := a.bridgeReady(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bridgeAPITimeout)
	defer cancel()

	profile, err := a.db.GetMerchantPayoutProfile(ctx, *userDid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if profile == nil || profile.BridgeCustomerID == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Verify your business before connecting a bank account."})
		return
	}
	if profile.KYBStatus != bridge.KYCApproved {
		if synced, err := a.syncMerchantKYB(ctx, profile); err == nil && synced != nil {
			profile = synced
		}
	}
	if profile.KYBStatus != bridge.KYCApproved {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Business verification is still in progress."})
		return
	}

	link, err := client.CreatePlaidLinkRequest(ctx, profile.BridgeCustomerID)
	if err != nil {
		a.logger.Logf("merchant payout: plaid link token failed for %s: %s", *userDid, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start the bank connection. Try again in a moment."})
		return
	}
	writeJSON(w, http.StatusOK, structs.PlaidLinkTokenResponse{LinkToken: link.LinkToken, ExpiresAt: link.LinkTokenExpiresAt})
}

// CompleteMerchantPlaidLink exchanges Plaid's public token through Bridge,
// then provisions a liquidation address for every approved location. Bridge
// creates the bank record asynchronously, so a short poll waits for it; if it
// is not there yet the response says so and the status endpoint picks it up
// on the next load.
func (a *AppService) CompleteMerchantPlaidLink(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	client, ok := a.bridgeReady(w)
	if !ok {
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var req structs.PlaidExchangeRequest
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.LinkToken) == "" || strings.TrimSpace(req.PublicToken) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	profile, err := a.db.GetMerchantPayoutProfile(ctx, *userDid)
	if err != nil || profile == nil || profile.BridgeCustomerID == "" {
		w.WriteHeader(http.StatusConflict)
		return
	}

	if err := client.ExchangePlaidPublicToken(ctx, strings.TrimSpace(req.LinkToken), strings.TrimSpace(req.PublicToken)); err != nil {
		a.logger.Logf("merchant payout: plaid exchange failed for %s: %s", *userDid, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "The bank connection could not be completed. Please try again."})
		return
	}

	// Bridge says "a few minutes"; in practice it is seconds. Poll briefly so
	// the common case finishes in this request and the merchant sees their
	// bank appear, then provision in the same breath.
	deadline := time.Now().Add(20 * time.Second)
	bankConnected := false
	for {
		if err := a.syncMerchantBankAccounts(ctx, profile); err != nil {
			a.logger.Logf("merchant payout: bank sync after plaid failed for %s: %s", *userDid, err)
		}
		banks, _ := a.db.ListMerchantBankAccounts(ctx, *userDid)
		if len(banks) > 0 {
			bankConnected = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		case <-time.After(2 * time.Second):
		}
	}

	// Attach the location the merchant started from. The flow is launched from
	// one shop's card, so that shop is the one connecting a bank — finishing
	// with nothing attached sends them back through Plaid to fix what looks
	// like a failure. Other locations are untouched and still have to be
	// attached deliberately, which is the point of attaching per location.
	target := uint64(0)
	if req.LocationID != nil {
		owned, err := a.db.LocationOwnedBy(ctx, *req.LocationID, *userDid)
		if err != nil || !owned {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		target = *req.LocationID
	}

	resp := structs.PlaidExchangeResponse{BankConnected: bankConnected}
	provisioned, err := a.provisionLiquidationAddresses(ctx, *userDid, "", target)
	if err != nil {
		a.logger.Logf("merchant payout: provisioning after plaid failed for %s: %s", *userDid, err)
		// The bank is linked; the address can be provisioned from the location
		// card. Say that rather than fail the whole flow.
		provisioned.Message = "Bank connected. Finish setting up this location's payouts from its card."
	}
	resp.ProvisionLiquidationAddressesResponse = provisioned

	// Say what happened. A 200 here only means the exchange was accepted; the
	// bank record and the payout address are both things Bridge may not have
	// caught up on yet, and telling a merchant they can unwrap when no address
	// exists is what made this look broken.
	if resp.Message == "" {
		switch {
		case !bankConnected:
			resp.Message = "Bank submitted. It can take a minute to appear — this page will pick it up."
		case len(resp.Provisioned) > 0:
			resp.Message = "Bank connected and payouts are set up for this location."
		default:
			resp.Message = "Bank connected. Finish setting up this location's payouts from its card."
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ProvisionMerchantLiquidationAddresses is the explicit "finish setup" call,
// for the case where Plaid completed but provisioning did not.
//
// location_id is optional and names the ONE location being set up. Without it
// the call refreshes locations that already have a destination and attaches
// nothing new — attaching is a per-location decision, so the location has to be
// named. The merchant panel always sends it, because the card that offers this
// button belongs to a location.
func (a *AppService) ProvisionMerchantLiquidationAddresses(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if _, ok := a.bridgeReady(w); !ok {
		return
	}

	// Body is optional: an older client sends none, and gets the refresh-only
	// behaviour rather than an error.
	var req struct {
		LocationID uint64 `json:"location_id"`
	}
	if body, readErr := io.ReadAll(r.Body); readErr == nil && len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}
	defer r.Body.Close()

	ctx, cancel := context.WithTimeout(r.Context(), bridgeAPITimeout)
	defer cancel()

	if req.LocationID != 0 {
		owned, err := a.db.LocationOwnedBy(ctx, req.LocationID, *userDid)
		if err != nil || !owned {
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	if profile, err := a.db.GetMerchantPayoutProfile(ctx, *userDid); err == nil && profile != nil {
		_ = a.syncMerchantBankAccounts(ctx, profile)
	}
	resp, err := a.provisionLiquidationAddresses(ctx, *userDid, "", req.LocationID)
	if err != nil {
		a.logger.Logf("merchant payout: provisioning failed for %s: %s", *userDid, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// SetLocationPayoutBank re-points one location at one of the business's
// linked banks. A second location that should pay a different account is this
// call, not a second onboarding.
func (a *AppService) SetLocationPayoutBank(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if _, ok := a.bridgeReady(w); !ok {
		return
	}
	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var req structs.SetLocationPayoutBankRequest
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.BridgeExternalAccountID) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bridgeAPITimeout)
	defer cancel()

	owned, err := a.db.LocationOwnedBy(ctx, locationID, *userDid)
	if err != nil || !owned {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	resp, err := a.provisionLiquidationAddresses(ctx, *userDid, strings.TrimSpace(req.BridgeExternalAccountID), locationID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Admin endpoints --------------------------------------------------------

// AdminSetLocationLiquidationAddress is the manual override. Merchants never
// see it. Even here the address is checked against what Bridge issued for the
// business, because a valid-looking address that belongs to someone else is
// the failure this whole design exists to prevent.
func (a *AppService) AdminSetLocationLiquidationAddress(w http.ResponseWriter, r *http.Request) {
	adminDid := utils.GetDid(r)
	if adminDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var req structs.AdminSetLiquidationAddressRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bridgeAPITimeout)
	defer cancel()

	ownerID, err := a.db.GetLocationOwnerID(ctx, locationID)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	address := strings.TrimSpace(req.Address)
	if address == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("address is required"))
		return
	}

	row := structs.LocationLiquidationAddress{
		LocationID:  locationID,
		OwnerID:     ownerID,
		Address:     address,
		Source:      "admin",
		SetByUserID: *adminDid,
	}

	// Verify against Bridge when we can. If the business has a Bridge
	// customer, the address must be one of theirs; if it is, the row records
	// which one so provisioning and drains line up with it.
	if a.bridge != nil && a.bridge.Enabled() {
		profile, err := a.db.GetMerchantPayoutProfile(ctx, ownerID)
		if err == nil && profile != nil && profile.BridgeCustomerID != "" {
			issued, err := a.bridge.ListLiquidationAddresses(ctx, profile.BridgeCustomerID)
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not verify the address with Bridge."})
				return
			}
			matched := false
			for _, la := range issued {
				if strings.EqualFold(la.Address, address) {
					matched = true
					row.BridgeLiquidationAddressID = la.ID
					row.Chain, row.Currency = la.Chain, la.Currency
					row.DestinationPaymentRail, row.DestinationCurrency = la.DestinationPaymentRail, la.DestinationCurrency
					row.BridgeExternalAccountID = la.ExternalAccountID
					break
				}
			}
			if !matched {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
					"error": "That address was not issued by Bridge for this business. Refusing to save it.",
				})
				return
			}
		}
	}

	if err := a.db.UpsertLocationLiquidationAddress(ctx, &row); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	saved, _ := a.db.GetLocationLiquidationAddress(ctx, locationID)
	writeJSON(w, http.StatusOK, saved)
}

// AdminAttachBridgeCustomer links a business that was onboarded on Bridge by
// hand (the first merchants were) to its SFLUV owner, then syncs its state.
func (a *AppService) AdminAttachBridgeCustomer(w http.ResponseWriter, r *http.Request) {
	client, ok := a.bridgeReady(w)
	if !ok {
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var req structs.AdminAttachBridgeCustomerRequest
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.BridgeCustomerID) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bridgeAPITimeout)
	defer cancel()

	// Admins know a merchant's email, not their Privy id. Resolve it here,
	// and refuse to guess when several accounts share the address: the
	// caller gets the candidates and resubmits with the owner id.
	if strings.TrimSpace(req.OwnerID) == "" {
		email := strings.TrimSpace(req.OwnerEmail)
		if email == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Enter the merchant's email or owner id."})
			return
		}
		candidates, err := a.db.FindMerchantOwnersByEmail(ctx, email)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch len(candidates) {
		case 0:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "No active account uses that email."})
			return
		case 1:
			req.OwnerID = candidates[0].OwnerID
		default:
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":      "Several accounts use that email. Pick the right one.",
				"candidates": candidates,
			})
			return
		}
	}

	customer, err := client.GetCustomer(ctx, strings.TrimSpace(req.BridgeCustomerID))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Bridge did not recognise that customer id."})
		return
	}
	status := customer.Status
	if strings.EqualFold(status, "active") {
		status = bridge.KYCApproved
	}
	if err := a.db.AttachBridgeCustomer(ctx, strings.TrimSpace(req.OwnerID), customer.ID, status); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	profile, _ := a.db.GetMerchantPayoutProfile(ctx, strings.TrimSpace(req.OwnerID))
	if profile != nil {
		_ = a.syncMerchantBankAccounts(ctx, profile)
	}
	resp, err := a.provisionLiquidationAddresses(ctx, strings.TrimSpace(req.OwnerID), "", 0)
	if err != nil {
		a.logger.Logf("merchant payout: provisioning after attach failed for %s: %s", req.OwnerID, err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": profile, "provisioning": resp})
}

func (a *AppService) AdminListMerchantPayouts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profiles, err := a.db.ListAllMerchantPayoutProfiles(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	businesses := make([]structs.AdminMerchantPayoutBusiness, 0, len(profiles))
	for _, p := range profiles {
		b := structs.AdminMerchantPayoutBusiness{Profile: p, BankAccounts: []structs.MerchantBankAccount{}, Locations: []structs.AdminPayoutLocation{}}
		if banks, err := a.db.ListMerchantBankAccounts(ctx, p.OwnerID); err == nil && banks != nil {
			b.BankAccounts = banks
		}
		locationIDs, err := a.db.ListApprovedLocationIDsForOwner(ctx, p.OwnerID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, id := range locationIDs {
			loc := structs.AdminPayoutLocation{LocationID: id}
			if l, err := a.db.GetLocation(ctx, id); err == nil && l != nil {
				loc.Name = l.Name
			}
			if la, err := a.db.GetLocationLiquidationAddress(ctx, id); err == nil {
				loc.Liquidation = la
			}
			b.Locations = append(b.Locations, loc)
		}
		businesses = append(businesses, b)
	}
	unwraps, err := a.db.ListAllUnwraps(ctx, 200)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if unwraps == nil {
		unwraps = []*structs.Unwrap{}
	}
	writeJSON(w, http.StatusOK, structs.AdminMerchantPayoutsResponse{Businesses: businesses, Unwraps: unwraps})
}
