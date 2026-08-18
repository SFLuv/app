package handlers

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/SFLuv/app/backend/w9provider"
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
		EscrowedSfluv:  formatSfluvBase(escrowed),
		EscrowedCount:  escrowedCount,
		BackPaySfluv:   formatSfluvBase(backPay),
		BackPayCount:   backPayCount,
		Items:          []structs.W9DayItem{},
	}
	// Required means "we are holding money on this", not merely "past the
	// threshold" — that is the thing a person can act on.
	response.Required = !response.Cleared && (escrowedCount > 0 || backPayCount > 0)

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

// W9ProviderWebhook receives completion events from the tax vendor.
//
// Unauthenticated by necessity — the vendor calls it — so the signature is the
// only thing standing between a stranger and everybody's held money. It is
// verified before the body is trusted for anything.
//
// The handler records the event and returns immediately. Releasing escrow means
// on-chain transfers, which must never happen inside a webhook: a vendor that
// times out and retries would otherwise start a second release.
func (a *AppService) W9ProviderWebhook(w http.ResponseWriter, r *http.Request) {
	if a.w9Provider == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	event, err := a.w9Provider.VerifyWebhook(r.Header, body)
	if err != nil {
		a.logger.Logf("rejected a w9 webhook: %s", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	fresh, err := a.db.RecordProviderEvent(r.Context(), a.w9Provider.Name(), event.EventID, event.Type, event.ProviderRequestID, body)
	if err != nil {
		a.logger.Logf("error recording w9 webhook %s: %s", event.EventID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Acknowledged either way. A duplicate is not an error, and telling the
	// vendor otherwise only invites more retries.
	w.WriteHeader(http.StatusOK)
	if !fresh {
		return
	}

	go a.processW9ProviderEvent(event)
}

// processW9ProviderEvent turns a verified webhook into released money.
//
// Runs detached from the request so a slow chain cannot make the vendor time
// out and redeliver. Its own idempotency comes from two places: the event was
// already claimed by the unique index before we got here, and
// MarkW9FilingCompleted reports whether it actually changed anything — a filing
// that was already cleared releases nothing a second time.
func (a *AppService) processW9ProviderEvent(event w9provider.WebhookEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	processErr := ""
	defer func() {
		if err := a.db.MarkProviderEventProcessed(ctx, a.w9Provider.Name(), event.EventID, processErr); err != nil {
			a.logger.Logf("error marking w9 event %s processed: %s", event.EventID, err)
		}
	}()

	if event.Status != w9provider.StatusCompleted {
		return
	}

	userID, taxYear, err := a.db.GetUserIDByProviderRequest(ctx, event.ProviderRequestID)
	if err != nil {
		processErr = err.Error()
		return
	}
	if userID == "" {
		// A completion we cannot attribute. Recorded rather than dropped so it
		// can be looked at, because somebody has filed a form and is waiting.
		processErr = "no filing matches provider request " + event.ProviderRequestID
		a.logger.Logf("w9: %s", processErr)
		return
	}

	changed, err := a.db.MarkW9FilingCompleted(ctx, userID, taxYear, "", event.Status)
	if err != nil {
		processErr = err.Error()
		return
	}
	if !changed {
		return
	}

	if a.payouts == nil {
		processErr = "no payout service is available to release escrow"
		return
	}
	released, backPay, err := a.payouts.ReleaseEscrowForUserYear(ctx, userID, taxYear)
	if err != nil {
		processErr = err.Error()
		a.logger.Logf("w9: filing completed for %s but escrow did not release: %s", userID, err)
		return
	}
	a.logger.Logf("w9: %s filed for %d — released %d payouts, %d queued for back pay", userID, taxYear, released, backPay)
}

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
