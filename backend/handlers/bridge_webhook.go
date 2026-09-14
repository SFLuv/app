package handlers

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bridge"
)

// ReceiveBridgeWebhook takes Bridge's event deliveries.
//
// Same shape as the W-9 webhook, for the same reasons: verify the signature
// before reading anything, acknowledge fast, do the work detached, and never
// act on what the delivery claims — re-read the object from Bridge instead.
// That last rule is what makes the exact event taxonomy unimportant: any
// signed event about a customer, a bank account, or a drain triggers a
// re-sync of that thing from the source of truth.
func (a *AppService) ReceiveBridgeWebhook(w http.ResponseWriter, r *http.Request) {
	if a.bridge == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, webhookBodyLimit))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.bridge.VerifyWebhookSignature(r.Header.Get(bridge.WebhookSignatureHeader), body, time.Now()); err != nil {
		if a.logger != nil {
			a.logger.Logf("bridge webhook: refused a delivery from %s: %s", r.RemoteAddr, err)
		}
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	ev, err := bridge.ParseEvent(body)
	if err != nil {
		// Signed but unreadable; retrying will not make it parse.
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
	go a.handleBridgeEvent(ev)
}

func (a *AppService) handleBridgeEvent(ev *bridge.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), webhookAckDeadline)
	defer cancel()

	category := strings.ToLower(strings.TrimSpace(ev.EventCategory))
	switch {
	case strings.Contains(category, "drain"):
		// A drain changed. The sweep already knows how to walk open unwraps
		// and match them to drains by deposit hash; one pass now is the
		// cheapest correct response.
		a.syncOpenUnwraps(ctx, 200)

	case strings.HasPrefix(category, "kyc_link"), strings.HasPrefix(category, "customer"), strings.Contains(category, "external_account"):
		profile, err := a.lookupProfileForEvent(ctx, ev)
		if err != nil || profile == nil {
			// Not one of ours (another environment, a sample post). Fine.
			return
		}
		if synced, err := a.syncMerchantKYB(ctx, profile); err == nil && synced != nil {
			profile = synced
		} else if err != nil && a.logger != nil {
			a.logger.Logf("bridge webhook: KYB sync failed for %s: %s", profile.OwnerID, err)
		}
		if profile.BridgeCustomerID != "" {
			if err := a.syncMerchantBankAccounts(ctx, profile); err != nil && a.logger != nil {
				a.logger.Logf("bridge webhook: bank sync failed for %s: %s", profile.OwnerID, err)
			}
			// A bank appearing is the moment addresses can be minted. Harmless
			// when nothing changed: provisioning skips locations already set.
			if profile.KYBStatus == bridge.KYCApproved {
				if _, err := a.provisionLiquidationAddresses(ctx, profile.OwnerID, "", 0); err != nil && a.logger != nil {
					a.logger.Logf("bridge webhook: provisioning failed for %s: %s", profile.OwnerID, err)
				}
			}
		}
	}
}

// lookupProfileForEvent finds which business an event is about. Bridge puts
// the customer id in different places for different objects; the KYC link
// id and the customer id are both tried.
func (a *AppService) lookupProfileForEvent(ctx context.Context, ev *bridge.Event) (*structsProfile, error) {
	id := strings.TrimSpace(ev.EventObjectID)
	if id == "" {
		return nil, nil
	}
	if p, err := a.db.GetMerchantPayoutProfileByCustomer(ctx, id); err == nil && p != nil {
		return p, nil
	}
	// The object may carry its customer id in the body.
	var probe struct {
		CustomerID string `json:"customer_id"`
	}
	if len(ev.EventObject) > 0 {
		_ = jsonUnmarshal(ev.EventObject, &probe)
		if probe.CustomerID != "" {
			return a.db.GetMerchantPayoutProfileByCustomer(ctx, probe.CustomerID)
		}
	}
	return nil, nil
}
