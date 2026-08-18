package handlers

import (
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
)

// GetW9Status is what the escrow panel reads: whether a form is owed, how much
// is waiting on it, and how long the automatic window has left.
func (a *AppService) GetW9Status(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	response, err := a.buildW9Status(r, *userDid)
	if err != nil {
		a.logger.Logf("error building w9 status for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (a *AppService) buildW9Status(r *http.Request, userID string) (*structs.W9StatusResponse, error) {
	ctx := r.Context()
	taxYear := time.Now().UTC().Year()

	filing, err := a.db.GetW9Filing(ctx, userID, taxYear)
	if err != nil {
		return nil, err
	}
	escrowed, escrowedCount, backPay, backPayCount, err := a.db.SumUserEscrowAndBackPay(ctx, userID, taxYear)
	if err != nil {
		return nil, err
	}
	earned, err := a.db.SumUserEarnedForYear(ctx, userID, taxYear)
	if err != nil {
		return nil, err
	}

	status := db.W9StatusNotStarted
	if filing != nil {
		status = filing.Status
	}

	response := &structs.W9StatusResponse{
		TaxYear:        taxYear,
		FilingStatus:   status,
		Cleared:        db.W9StatusClears(status),
		ThresholdSfluv: formatSfluvBase(w9ThresholdBase()),
		EarnedSfluv:    formatSfluvBase(earned),
		ThresholdBase:  w9ThresholdBase().String(),
		EarnedBase:     earned.String(),
		EscrowedSfluv:  formatSfluvBase(escrowed),
		EscrowedCount:  escrowedCount,
		BackPaySfluv:   formatSfluvBase(backPay),
		BackPayCount:   backPayCount,
		Items:          []structs.W9DayItem{},
	}
	// Required means "we are holding money on this", not merely "past the
	// threshold" — that is the thing a person can act on.
	response.Required = !response.Cleared && (escrowedCount > 0 || backPayCount > 0)

	// The outstanding tier drives the modal. A cleared filing has none by
	// definition — the notices are deleted when it clears — but the guard keeps
	// a stale row from resurrecting a warning somebody has already answered.
	if !response.Cleared {
		if outstanding, err := a.db.GetOutstandingW9Tier(ctx, userID, taxYear); err == nil && outstanding != nil {
			response.Tier = outstanding.Tier
			response.TierAcknowledged = outstanding.AcknowledgedAt != nil
			response.Blocked = outstanding.Tier == db.W9TierBlocked
		}
	}

	if filing != nil && !response.Cleared {
		response.FormURL = filing.FormURL
		response.FormURLExpiresAt = filing.FormURLExpiresAt
	}

	rows, err := a.db.ListPayoutsForUserYear(ctx, userID, taxYear,
		[]string{db.PayoutStateEscrowed, db.PayoutStateExpired, db.PayoutStateBackPayRequested})
	if err != nil {
		return nil, err
	}
	window := escrowWindow()
	for _, row := range rows {
		amount, _ := new(big.Int).SetString(row.AmountBase, 10)
		item := structs.W9DayItem{
			Source:      row.Source,
			SourceLabel: payoutSourceLabel(row.Source),
			AmountSfluv: formatSfluvBase(amount),
			State:       row.State,
			EscrowedAt:  row.EscrowedAt,
		}
		if row.State == db.PayoutStateEscrowed && row.EscrowedAt != nil {
			expires := row.EscrowedAt.Add(window)
			item.ExpiresAt = &expires
			if response.EscrowExpiresAt == nil || expires.Before(*response.EscrowExpiresAt) {
				response.EscrowExpiresAt = &expires
			}
		}
		response.Items = append(response.Items, item)
	}

	return response, nil
}

// payoutSourceLabel turns an internal source into something worth reading on a
// phone.
func payoutSourceLabel(source string) string {
	switch source {
	case db.PayoutSourceRedemptionCode:
		return "Volunteer reward"
	case db.PayoutSourceWorkflowStep:
		return "Project bounty"
	case db.PayoutSourceWorkflowManager:
		return "Project management bounty"
	case db.PayoutSourceAdminManual:
		return "Payment from SFLuv"
	default:
		return "Reward"
	}
}

// StartW9 hands back a link to the form, creating the vendor request if this is
// the first time. Called when someone taps "complete your tax form".
func (a *AppService) StartW9(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.payouts == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	taxYear := time.Now().UTC().Year()
	filing, err := a.payouts.EnsureW9Request(r.Context(), *userDid, taxYear)
	if err != nil {
		a.logger.Logf("error starting a w9 for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("The tax form service is unavailable right now. Please try again shortly."))
		return
	}
	if filing == nil || strings.TrimSpace(filing.FormURL) == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("No tax form is available right now. Please try again shortly."))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"form_url":            filing.FormURL,
		"form_url_expires_at": filing.FormURLExpiresAt,
		"tax_year":            taxYear,
	})
}

// The webhook receiver that used to live here is gone.
//
// The vendor publishes no webhook, callback or notification — verified against
// its docs and every changelog entry from 0.1.0 to 0.7.0 — so nothing would
// ever have called it. It also invented a signature header and an HMAC scheme
// to validate deliveries that cannot arrive. Completion is discovered by the
// sweeper polling GetW9Status; see PayoutService.pollProviderFilings.

// AttributeUnlinkedPayoutsForUser claims past payouts made to an address before
// it was linked to an account.
//
// Redemption is unauthenticated, so a QR can be scanned to an unknown address
// and the money goes out ungated — there is nobody to ask for a form. Without
// this, doing that and linking the wallet afterwards would be a permanent way
// around the threshold.
func (a *AppService) AttributeUnlinkedPayoutsForUser(r *http.Request, userID string) {
	wallets, err := a.db.GetWalletsByUser(r.Context(), userID)
	if err != nil || len(wallets) == 0 {
		return
	}
	addresses := make([]string, 0, len(wallets)*2)
	for _, wallet := range wallets {
		if wallet == nil {
			continue
		}
		if wallet.SmartAddress != nil {
			addresses = append(addresses, *wallet.SmartAddress)
		}
		addresses = append(addresses, wallet.EoaAddress)
	}
	if claimed, err := a.db.AttributeUnlinkedPayouts(r.Context(), userID, addresses); err == nil && claimed > 0 {
		a.logger.Logf("w9: attributed %d previously unlinked payouts to %s", claimed, userID)
	}
}

func w9AdminTaxYear(r *http.Request) int {
	if raw := strings.TrimSpace(r.URL.Query().Get("year")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 2000 {
			return parsed
		}
	}
	return time.Now().UTC().Year()
}

func chiParam(r *http.Request, name string) string {
	return strings.TrimSpace(chi.URLParam(r, name))
}

// AcknowledgeW9Tier records that someone dismissed a tier's modal.
//
// For the first three tiers that is the end of it. The blocked modal is
// re-armed by the next refused payout, so dismissing it buys quiet rather than
// silence — being unable to receive money is not a state somebody should be
// able to put away and forget.
func (a *AppService) AcknowledgeW9Tier(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	tier := chiParam(r, "tier")
	if db.W9TierSeverity(tier) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("unknown tier"))
		return
	}

	if err := a.db.AcknowledgeW9Tier(r.Context(), *userDid, time.Now().UTC().Year(), tier); err != nil {
		a.logger.Logf("error acknowledging w9 tier %s for %s: %s", tier, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
