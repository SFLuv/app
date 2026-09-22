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
		// A KYC link belongs to the customer that created it. When an admin
		// attaches a different customer by hand, the link left on the profile is
		// about a DIFFERENT business, and trusting it overwrites the attached
		// customer's real status with a stranger's — which is how a verified
		// merchant kept falling back to "verification in progress" and lost
		// their payout provisioning every time the sweep ran.
		//
		// So the link is only believed when it is a link for this customer.
		// Otherwise fall through to reading the customer itself, below.
		if link != nil && profile.BridgeCustomerID != "" && link.CustomerID != "" &&
			!strings.EqualFold(link.CustomerID, profile.BridgeCustomerID) {
			a.logger.Logf(
				"merchant payout: ignoring KYC link %s for owner %s — it belongs to customer %s, not the attached customer %s",
				profile.BridgeKYCLinkID, profile.OwnerID, link.CustomerID, profile.BridgeCustomerID,
			)
			link = nil
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
		// Read ToS from the customer too. It was only ever read off the KYC
		// link, so a customer attached by an admin carried a blank ToS status
		// forever — and a blank one reads as "fine" everywhere while Bridge is
		// refusing to attach their bank because of it.
		if customer.HasAcceptedTOS {
			tos = bridge.KYCApproved
		} else {
			tos = "pending"
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
	moved, err := a.db.SyncMerchantBankAccounts(ctx, profile.OwnerID, mirror)
	if err != nil {
		return err
	}
	// Loud on purpose. A bank account changing owners means two of our accounts
	// were pointed at one Bridge customer, which is a data problem a person has
	// to untangle — and until this sync learned to move the row, it was the
	// reason a merchant's bank could never appear no matter how many times they
	// reconnected.
	for _, m := range moved {
		a.logger.Logf(
			"merchant payout: bank account %s moved from owner %s to %s — two accounts were attached to one Bridge customer; check which is correct",
			m.BridgeExternalAccountID, m.PreviousOwnerID, m.NewOwnerID,
		)
	}
	return nil
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
		// Terms are tracked separately from verification: a business can be
		// fully KYB-approved and still have terms outstanding, and that gap is
		// invisible in kyb_status. Refreshed only while it is unresolved, so a
		// settled business costs no extra call.
		if profile.BridgeCustomerID != "" && !strings.EqualFold(profile.TOSStatus, bridge.KYCApproved) {
			if customer, err := a.bridge.GetCustomer(ctx, profile.BridgeCustomerID); err == nil && customer != nil {
				tos := "pending"
				if customer.HasAcceptedTOS {
					tos = bridge.KYCApproved
				}
				if !strings.EqualFold(tos, profile.TOSStatus) {
					if err := a.db.SetMerchantKYBStatus(ctx, profile.OwnerID, profile.KYBStatus, tos); err == nil {
						if refreshed, err := a.db.GetMerchantPayoutProfile(ctx, *userDid); err == nil && refreshed != nil {
							profile = refreshed
						}
					}
				}
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

// bridgeBlockerForBank asks Bridge what, if anything, stops this business
// attaching a bank right now, and phrases it for the merchant. "" means nothing
// known is in the way.
//
// It exists because the failure it catches is invisible from our side: a
// customer can be active and KYB-approved while Bridge still refuses every
// external account because the terms of service were never accepted. That path
// produced a bare 502 and a "connection could not be completed" toast, which
// tells the merchant to retry the one thing that cannot work.
func (a *AppService) bridgeBlockerForBank(ctx context.Context, customerID string) (string, string) {
	if a.bridge == nil || !a.bridge.Enabled() || strings.TrimSpace(customerID) == "" {
		return "", ""
	}
	customer, err := a.bridge.GetCustomer(ctx, customerID)
	if err != nil || customer == nil {
		return "", ""
	}
	if !customer.HasAcceptedTOS {
		// Hand back the page where they can fix it, rather than describing a
		// dead end. Without the link this is a message the merchant cannot act
		// on: the terms live with Bridge and we have no other way to reach them.
		tosURL, linkErr := a.bridge.TOSAcceptanceLink(ctx, customerID)
		if linkErr != nil {
			a.logger.Logf("merchant payout: could not get a ToS link for customer %s: %s", customerID, linkErr)
		}
		return "Before connecting a bank, this business has to accept our banking partner's terms of service.", tosURL
	}
	if !strings.EqualFold(customer.Status, "active") {
		switch strings.ToLower(customer.Status) {
		case "under_review":
			return "Your business verification is still under review with our banking partner. You can connect a bank once it clears.", ""
		case "rejected", "offboarded":
			return "Our banking partner cannot approve this business. Please contact support.", ""
		case "":
			return "", ""
		default:
			return "Your business verification is not complete with our banking partner yet (" + customer.Status + ").", ""
		}
	}
	return "", ""
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

	// Cheaper to say this now than after they have logged into their bank, and
	// the ToS link rides along so the client can put the terms in front of them
	// instead of reporting a dead end.
	if blocker, tosURL := a.bridgeBlockerForBank(ctx, profile.BridgeCustomerID); blocker != "" {
		body := map[string]string{"error": blocker}
		if tosURL != "" {
			body["tos_url"] = tosURL
		}
		writeJSON(w, http.StatusConflict, body)
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
		if err != nil {
			a.logger.Logf("merchant payout: profile load failed before plaid exchange for %s: %s", *userDid, err)
		}
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "This account is not set up with our banking partner yet. Verify your business first.",
		})
		return
	}

	// Ownership is checked BEFORE the exchange, because the exchange is not
	// reversible: once Bridge has the public token the bank is linked, and
	// refusing afterwards tells the merchant it failed while it actually
	// succeeded — so they try again and link the same account twice.
	target := uint64(0)
	if req.LocationID != nil {
		owned, ownErr := a.db.LocationOwnedBy(ctx, *req.LocationID, *userDid)
		if ownErr != nil {
			a.logger.Logf("merchant payout: location ownership check failed for %s location %d: %s", *userDid, *req.LocationID, ownErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Could not confirm which location this bank is for. Please try again.",
			})
			return
		}
		if !owned {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "That location does not belong to this account.",
			})
			return
		}
		target = *req.LocationID
	}

	// How many banks we had going in, so a failed-looking exchange can be told
	// apart from one that actually landed.
	banksBefore := 0
	if existing, listErr := a.db.ListMerchantBankAccounts(ctx, *userDid); listErr == nil {
		banksBefore = len(existing)
	}

	if err := client.ExchangePlaidPublicToken(ctx, strings.TrimSpace(req.LinkToken), strings.TrimSpace(req.PublicToken)); err != nil {
		// The full Bridge status and body are in this line — the merchant-facing
		// message stays generic, but nobody should have to guess what Bridge
		// said.
		a.logger.Logf("merchant payout: plaid exchange failed for %s (link_token=%s): %s", *userDid, strings.TrimSpace(req.LinkToken), err)

		// A rejected exchange is not proof that nothing happened. A retried or
		// already-exchanged token is refused by Bridge while the bank is linked
		// perfectly well, and reporting failure there is what sends a merchant
		// round the loop again to link the same account twice. So ask Bridge
		// what it actually holds before believing the error.
		recovered := false
		if syncErr := a.syncMerchantBankAccounts(ctx, profile); syncErr == nil {
			if after, listErr := a.db.ListMerchantBankAccounts(ctx, *userDid); listErr == nil && len(after) > banksBefore {
				recovered = true
				a.logger.Logf("merchant payout: plaid exchange reported an error for %s but a new bank account is present; continuing", *userDid)
			}
		}
		if !recovered {
			message, tosURL := a.bridgeBlockerForBank(ctx, profile.BridgeCustomerID)
			if message == "" {
				message = "Your bank could not be linked. If you just tried this, wait a moment and reload before trying again — the connection may still be on its way."
			}
			body := map[string]string{"error": message}
			if tosURL != "" {
				body["tos_url"] = tosURL
			}
			writeJSON(w, http.StatusBadGateway, body)
			return
		}
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
			// The exchange already succeeded, so this is a slow confirmation,
			// not a failed connection. Saying "failed" here would send the
			// merchant back through Plaid to link the same account again.
			writeJSON(w, http.StatusOK, structs.PlaidExchangeResponse{
				ProvisionLiquidationAddressesResponse: structs.ProvisionLiquidationAddressesResponse{
					Provisioned: []structs.LocationLiquidationAddress{},
					Skipped:     []uint64{},
					Message:     "Bank submitted. It can take a minute to appear — this page will pick it up.",
				},
				BankConnected: false,
			})
			return
		case <-time.After(2 * time.Second):
		}
	}

	// Attach the location the merchant started from (validated above). Other
	// locations are untouched and still have to be attached deliberately, which
	// is the point of attaching per location.
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
	// One Bridge customer, one owner. Two accounts sharing a customer id is how
	// a merchant's bank ends up mirrored under the wrong owner and becomes
	// invisible to them, so the collision is refused here rather than cleaned up
	// afterwards. Re-attaching the same owner is fine — that is a resync.
	if existing, err := a.db.GetMerchantPayoutProfileByCustomer(ctx, customer.ID); err == nil && existing != nil &&
		existing.OwnerID != strings.TrimSpace(req.OwnerID) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":          "That Bridge customer is already attached to a different account. Detach it there first, or attach the customer that belongs to this merchant.",
			"attached_owner": existing.OwnerID,
		})
		return
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

// RequestMerchantTOSLink hands back the hosted page where this business accepts
// Bridge's terms, and the live acceptance state alongside it.
//
// Terms are their own gate. A business can be verified and still be unable to
// attach a bank, and until this existed the only place that surfaced was a
// failed Plaid exchange at the very end of the flow.
func (a *AppService) RequestMerchantTOSLink(w http.ResponseWriter, r *http.Request) {
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

	profile, err := a.db.GetMerchantPayoutProfile(ctx, *userDid)
	if err != nil || profile == nil || profile.BridgeCustomerID == "" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "Verify your business before accepting the banking terms.",
		})
		return
	}

	// Read acceptance from Bridge rather than our mirror: the merchant may have
	// just signed in another tab, and telling them to sign again would be both
	// wrong and the exact loop this is meant to end.
	customer, err := a.bridge.GetCustomer(ctx, profile.BridgeCustomerID)
	if err != nil || customer == nil {
		a.logger.Logf("merchant payout: could not read customer %s for ToS state: %s", profile.BridgeCustomerID, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not reach our banking partner. Try again in a moment."})
		return
	}

	tos := "pending"
	if customer.HasAcceptedTOS {
		tos = bridge.KYCApproved
	}
	if !strings.EqualFold(tos, profile.TOSStatus) {
		if err := a.db.SetMerchantKYBStatus(ctx, profile.OwnerID, profile.KYBStatus, tos); err != nil {
			a.logger.Logf("merchant payout: could not record ToS status for %s: %s", profile.OwnerID, err)
		}
	}
	if customer.HasAcceptedTOS {
		writeJSON(w, http.StatusOK, map[string]string{"tos_status": tos})
		return
	}

	tosURL, err := a.bridge.TOSAcceptanceLink(ctx, profile.BridgeCustomerID)
	if err != nil {
		a.logger.Logf("merchant payout: could not get a ToS link for %s: %s", profile.BridgeCustomerID, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not open the terms right now. Try again in a moment."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"tos_status": tos, "url": tosURL})
}
