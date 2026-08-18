package handlers

import (
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// GetW9AdminOverview is the Tax & Escrow panel.
//
// It leads with faucet coverage because that is the number an admin needs
// before doing anything else. Escrowed money is reserved and must not be
// allocated to events; back pay is owed but deliberately not reserved, since it
// only exists after an escrow window lapsed and the money returned to the
// spendable pool. Showing both, with an explicit verdict on whether the faucet
// covers what is owed, is what makes approving a back payment a decision rather
// than a gamble.
func (a *AppService) GetW9AdminOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	escrowed, err := a.db.EscrowedTotalBase(ctx)
	if err != nil {
		a.logger.Logf("error totalling escrow: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	overview := structs.W9AdminOverview{
		EscrowedSfluv: formatSfluvBase(escrowed),
		Rows:          []structs.W9AdminRow{},
	}

	faucet := big.NewInt(0)
	if a.bot != nil && a.bot.bot != nil {
		if balance, err := a.bot.bot.Balance(); err == nil && balance != nil {
			faucet = balance
		}
	}
	overview.FaucetSfluv = formatSfluvBase(faucet)

	allocatedBase := big.NewInt(0)
	if a.bot != nil {
		if allocated, err := a.bot.totalAllocatedBalance(ctx); err == nil {
			multiplier, mErr := getTokenMultiplier()
			if mErr == nil && multiplier != nil {
				allocatedBase = new(big.Int).Mul(multiplier, new(big.Int).SetUint64(allocated))
			}
		}
	}
	overview.AllocatedSfluv = formatSfluvBase(allocatedBase)

	// Available is what may actually be committed to something new. Escrow is
	// subtracted here and nowhere else has to remember to do it.
	available := new(big.Int).Sub(faucet, allocatedBase)
	available.Sub(available, escrowed)
	if available.Sign() < 0 {
		available = big.NewInt(0)
	}
	overview.AvailableSfluv = formatSfluvBase(available)

	rows, err := a.db.ListW9AdminRows(ctx, w9AdminTaxYear(r))
	if err != nil {
		a.logger.Logf("error listing w9 admin rows: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	overview.Rows = rows
	overview.PeopleWithHolds = len(rows)
	for _, row := range rows {
		if row.OldestEscrowAt == nil {
			continue
		}
		age := int(time.Since(*row.OldestEscrowAt).Hours() / 24)
		if age > overview.OldestEscrowAge {
			overview.OldestEscrowAge = age
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(overview)
}

// ClearW9Filing is the override for cases the vendor cannot resolve. A reason
// is required because it bypasses the entire control, and the record of who did
// it and why is the only thing that makes that acceptable.
func (a *AppService) ClearW9Filing(w http.ResponseWriter, r *http.Request) {
	adminDid := utils.GetDid(r)
	userID := chiParam(r, "user_id")
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	var request struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(body, &request)

	adminID := ""
	if adminDid != nil {
		adminID = *adminDid
	}

	taxYear := w9AdminTaxYear(r)
	if err := a.db.ManuallyClearW9Filing(r.Context(), userID, taxYear, adminID, request.Reason); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	// Clearing a filing means the held money should go out now, for the same
	// reason a completed form does.
	released, backPay := 0, int64(0)
	if a.payouts != nil {
		var err error
		released, backPay, err = a.payouts.ReleaseEscrowForUserYear(r.Context(), userID, taxYear)
		if err != nil {
			a.logger.Logf("error releasing escrow after clearing %s: %s", userID, err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"released": released, "back_pay_queued": backPay})
}

// ResendW9Request issues a fresh form link for someone who lost theirs.
func (a *AppService) ResendW9Request(w http.ResponseWriter, r *http.Request) {
	if a.payouts == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	userID := chiParam(r, "user_id")
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	filing, err := a.payouts.EnsureW9Request(r.Context(), userID, w9AdminTaxYear(r))
	if err != nil {
		a.logger.Logf("error resending a w9 request for %s: %s", userID, err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(filing)
}

// PrecheckW9ForRecipient answers whether an admin may send to this address.
//
// Admin sends are signed in the browser from the admin's own wallet, so the
// backend never holds the money and cannot escrow it. The gate is therefore a
// refusal rather than a hold: the send is blocked, the recipient is asked for a
// W-9, and the admin sends again once it is on file.
//
// It replaces /w9/check, whose response the old modal used to offer an admin an
// inline "approve and send" button — which filed and approved a W-9 that never
// existed.
func (a *AppService) PrecheckW9ForRecipient(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	var request struct {
		Address    string `json:"address"`
		AmountBase string `json:"amount_base"`
	}
	_ = json.Unmarshal(body, &request)

	address := strings.TrimSpace(request.Address)
	if address == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	taxYear := time.Now().UTC().Year()

	userID := ""
	if lookup, err := a.db.GetWalletAddressOwnerLookup(ctx, address); err == nil && lookup != nil {
		userID = lookup.UserID
	}

	response := map[string]any{"allowed": true, "tax_year": taxYear}

	// An address with no account cannot be asked for a form, so there is
	// nothing to wait for and the send goes ahead. It is attributed later, if
	// the wallet is ever linked.
	if userID == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	filing, err := a.db.GetW9Filing(ctx, userID, taxYear)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	status := db.W9StatusNotStarted
	if filing != nil {
		status = filing.Status
	}
	if db.W9StatusClears(status) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	earned, err := a.db.SumUserEarnedForYear(ctx, userID, taxYear)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	amount, ok := new(big.Int).SetString(strings.TrimSpace(request.AmountBase), 10)
	if !ok {
		amount = big.NewInt(0)
	}

	// hasOpenEscrow is deliberately false here: an admin send is refused either
	// way, and passing the real value would only change which reason is quoted.
	decision := decidePayout(earned, amount, w9Thresholds(), false, status)
	if decision.Action == payoutActionPay {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	// Blocked. Ask the recipient for a form and tell them why, so the admin
	// does not have to chase them separately.
	if a.payouts != nil {
		if _, err := a.payouts.EnsureW9Request(ctx, userID, taxYear); err != nil {
			a.logger.Logf("w9 precheck: could not prepare a form for %s: %s", userID, err)
		}
		a.pushW9Notice(ctx, userID, "w9_required",
			"A W-9 is needed before we can pay you",
			"You've earned enough this year that we need a tax form on file. Complete it in the app and we'll send your payment.",
		)
	}

	response["allowed"] = false
	response["reason"] = "w9_required"
	response["user_id"] = userID
	response["earned_sfluv"] = formatSfluvBase(earned)
	response["threshold_sfluv"] = formatSfluvBase(w9ThresholdBase())
	response["message"] = "This person needs a W-9 on file before they can be paid. We've asked them for one — send again once it's in."

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(response)
}

// Get1099Report is the year-end view of who we owe a 1099-NEC.
//
// Exposed now so the data is reachable before it is needed. The intent is that
// this becomes an MCP report later; until then it is one admin call, and the
// underlying query is the same either way.
//
// The number to look at is BlockedCount: payees who were paid past the filing
// threshold but have no tax ID on file. Every one of those needs a person to
// chase a W-9, and January is the wrong month to discover them.
func (a *AppService) Get1099Report(w http.ResponseWriter, r *http.Request) {
	taxYear := w9AdminTaxYear(r)
	threshold := w9ThresholdBase()

	rows, err := a.db.List1099Candidates(r.Context(), taxYear, threshold)
	if err != nil {
		a.logger.Logf("error building the 1099 report for %d: %s", taxYear, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	report := structs.Form1099Report{
		TaxYear:        taxYear,
		ThresholdSfluv: formatSfluvBase(threshold),
		Rows:           []structs.Form1099Row{},
	}

	total := big.NewInt(0)
	for _, row := range rows {
		if paid, ok := new(big.Int).SetString(row.PaidSfluv, 10); ok {
			total.Add(total, paid)
		}
		// Amounts are rendered for reading only at the edge; the sum above uses
		// base units so nothing rounds on the way through.
		row.PaidSfluv = formatSfluvBase(mustBase(row.PaidSfluv))
		if row.Reportable {
			report.ReportableCount++
			if row.Fileable {
				report.FileableCount++
			} else {
				report.BlockedCount++
			}
		}
		report.Rows = append(report.Rows, row)
	}
	report.TotalPaidSfluv = formatSfluvBase(total)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(report)
}

func mustBase(raw string) *big.Int {
	parsed, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return big.NewInt(0)
	}
	return parsed
}
