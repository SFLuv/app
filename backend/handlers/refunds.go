package handlers

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
)

// Merchant refunds.
//
// A refund is the till sending tokens back to the customer — an ordinary
// transfer, signed by the merchant's own wallet, exactly like an unwrap. The
// backend does not move the money; it decides whether the amount was allowed and
// records what the transfer was for, because the chain cannot say that.
//
// The rule the whole feature rests on: a payment may be refunded up to what is
// left of it. The original's amount comes from the chain index, never from the
// client, and what has already been refunded comes from our own ledger. A client
// that asks for more than the remainder is refused.

// refundableRemainder returns the original transfer, what has been refunded so
// far, and what is left. A nil transfer means the hash is not an indexed
// transfer into this till.
func (a *AppService) refundableRemainder(
	ctx context.Context, originalHash string, till string,
) (*structs.PonderTransaction, *big.Int, *big.Int, error) {
	original, err := a.ponderDb.GetTransferByHash(ctx, originalHash)
	if err != nil {
		return nil, nil, nil, err
	}
	if original == nil {
		return nil, nil, nil, nil
	}
	// Only money that came INTO this till can be sent back out of it. Without
	// this a merchant could "refund" a payment made to somebody else.
	if !strings.EqualFold(strings.TrimSpace(original.To), strings.TrimSpace(till)) {
		return nil, nil, nil, nil
	}

	total, ok := new(big.Int).SetString(original.Amount, 10)
	if !ok {
		return nil, nil, nil, nil
	}

	totals, err := a.db.RefundedTotals(ctx, []string{originalHash})
	if err != nil {
		return nil, nil, nil, err
	}
	refunded := totals[strings.ToLower(strings.TrimSpace(originalHash))]
	if refunded == nil {
		refunded = big.NewInt(0)
	}
	remaining := new(big.Int).Sub(total, refunded)
	if remaining.Sign() < 0 {
		remaining = big.NewInt(0)
	}
	return original, refunded, remaining, nil
}

// resolveRefundTill finds the payment wallet a refund must come from, and checks
// the caller owns the location it belongs to.
func (a *AppService) resolveRefundTill(ctx context.Context, userDid string, locationID uint64) (string, bool, error) {
	owned, err := a.db.LocationOwnedBy(ctx, locationID, userDid)
	if err != nil {
		return "", false, err
	}
	if !owned {
		return "", false, nil
	}
	wallets, err := a.db.GetMerchantDayWallets(ctx, uint(locationID), "")
	if err != nil {
		return "", false, err
	}
	if wallets == nil || strings.TrimSpace(wallets.Payment) == "" {
		return "", false, nil
	}
	return strings.ToLower(strings.TrimSpace(wallets.Payment)), true, nil
}

// GetRefundability answers "may this be refunded, and for how much" before the
// merchant is shown a refund form, so the form can be bounded rather than the
// submission rejected.
func (a *AppService) GetRefundability(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.ponderDb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "transaction index is unavailable"})
		return
	}
	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	hash := strings.TrimSpace(r.URL.Query().Get("tx_hash"))
	if hash == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	till, ok, err := a.resolveRefundTill(r.Context(), *userDid, locationID)
	if err != nil {
		a.logger.Logf("refund: could not resolve till for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	original, refunded, remaining, err := a.refundableRemainder(r.Context(), hash, till)
	if err != nil {
		a.logger.Logf("refund: could not read refundability for %s: %s", hash, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if original == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "That transaction is not a payment to this location.",
		})
		return
	}

	refunds, err := a.db.ListRefundsForOriginals(r.Context(), []string{hash})
	if err != nil {
		a.logger.Logf("refund: could not list refunds for %s: %s", hash, err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"original_tx_hash":     original.Hash,
		"original_amount_base": original.Amount,
		"refunded_base":        refunded.String(),
		"remaining_base":       remaining.String(),
		"status":               refundStatusFor(original.Amount, refunded),
		"refunds":              refunds[strings.ToLower(hash)],
		"till_address":         till,
		"customer_address":     original.From,
	})
}

// RecordRefund records a refund the merchant's wallet has already sent.
//
// Ordering matters: everything that can refuse the refund is checked BEFORE the
// row is written, and the row is written only once the chain has the transfer.
// The amount is validated against the remainder computed here, so a client
// cannot widen its own limit by sending a different number.
func (a *AppService) RecordRefund(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.ponderDb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "transaction index is unavailable"})
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var req structs.RecordRefundRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req.OriginalTxHash = strings.TrimSpace(req.OriginalTxHash)
	req.RefundTxHash = strings.TrimSpace(req.RefundTxHash)
	if req.OriginalTxHash == "" || req.RefundTxHash == "" || req.LocationID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "The refund is missing its transaction or location."})
		return
	}
	amount, ok := new(big.Int).SetString(strings.TrimSpace(req.Amount), 10)
	if !ok || amount.Sign() <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That refund amount is not a number."})
		return
	}

	till, owned, err := a.resolveRefundTill(r.Context(), *userDid, *req.LocationID)
	if err != nil {
		a.logger.Logf("refund: could not resolve till for location %d: %s", *req.LocationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !owned {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	original, _, remaining, err := a.refundableRemainder(r.Context(), req.OriginalTxHash, till)
	if err != nil {
		a.logger.Logf("refund: could not read refundability for %s: %s", req.OriginalTxHash, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if original == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "That transaction is not a payment to this location.",
		})
		return
	}
	if amount.Cmp(remaining) > 0 {
		// The money has already moved on chain, so this is a record refused, not
		// a refund prevented. Said plainly rather than silently truncated: the
		// two figures are the merchant's evidence for what went wrong.
		a.logger.Logf(
			"refund: REFUSED to record %s against %s — only %s remained refundable (till %s, location %d)",
			amount.String(), req.OriginalTxHash, remaining.String(), till, *req.LocationID,
		)
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":          "That is more than is left to refund on this payment.",
			"remaining_base": remaining.String(),
		})
		return
	}

	// The refund itself has to exist on chain and have come out of this till.
	refundTx, err := a.ponderDb.GetTransferByHash(r.Context(), req.RefundTxHash)
	if err != nil {
		a.logger.Logf("refund: could not read refund transfer %s: %s", req.RefundTxHash, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if refundTx != nil && !strings.EqualFold(strings.TrimSpace(refundTx.From), till) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "That refund did not come from this location's wallet.",
		})
		return
	}

	locationID := int64(*req.LocationID)
	entry := structs.Refund{
		OriginalTxHash: req.OriginalTxHash,
		RefundTxHash:   req.RefundTxHash,
		LocationID:     &locationID,
		OwnerID:        *userDid,
		FromAddress:    till,
		ToAddress:      original.From,
		Amount:         amount.String(),
	}
	id, err := a.db.InsertRefund(r.Context(), &entry)
	if err != nil {
		a.logger.Logf("refund: could not record %s against %s: %s", req.RefundTxHash, req.OriginalTxHash, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, refunded, nowRemaining, err := a.refundableRemainder(r.Context(), req.OriginalTxHash, till)
	if err != nil || refunded == nil {
		refunded, nowRemaining = amount, new(big.Int).Sub(remaining, amount)
	}
	writeJSON(w, http.StatusOK, structs.RecordRefundResponse{
		Recorded:  true,
		RefundID:  id,
		Refunded:  refunded.String(),
		Remaining: nowRemaining.String(),
	})
}

// refundStatusFor turns an original's amount and what has been refunded against
// it into the mark a merchant sees.
func refundStatusFor(originalAmount string, refunded *big.Int) string {
	if refunded == nil || refunded.Sign() == 0 {
		return structs.RefundStatusNone
	}
	total, ok := new(big.Int).SetString(originalAmount, 10)
	if !ok {
		return structs.RefundStatusPartial
	}
	if refunded.Cmp(total) >= 0 {
		return structs.RefundStatusFull
	}
	return structs.RefundStatusPartial
}

// GetLocationTransactions is the location page's history: everything that moved
// through this shop's wallets, with the refund ledger applied.
//
// The refund marks are computed here rather than in each client so the web panel
// and the till app cannot disagree about whether a payment is fully refunded —
// which is the difference between offering a refund button and hiding it.
func (a *AppService) GetLocationTransactions(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.ponderDb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "transaction index is unavailable"})
		return
	}
	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	owned, err := a.db.LocationOwnedBy(r.Context(), locationID, *userDid)
	if err != nil {
		a.logger.Logf("refund: ownership check failed for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !owned {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	wallets, err := a.db.GetMerchantDayWallets(r.Context(), uint(locationID), "")
	if err != nil || wallets == nil {
		a.logger.Logf("refund: could not resolve wallets for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	till := strings.ToLower(strings.TrimSpace(wallets.Payment))
	tips := strings.ToLower(strings.TrimSpace(wallets.Tipping))

	addresses := []string{}
	if till != "" {
		addresses = append(addresses, till)
	}
	if tips != "" {
		addresses = append(addresses, tips)
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 {
		count = 50
	}

	transfers, total, err := a.ponderDb.GetTransfersForAddresses(r.Context(), addresses, page, count)
	if err != nil {
		a.logger.Logf("refund: could not load transactions for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// One pass for the ledger rather than a query per line: payments need what
	// has been refunded off them, refunds need the payment they undid.
	incoming := make([]string, 0, len(transfers))
	outgoing := make([]string, 0, len(transfers))
	for _, t := range transfers {
		if strings.EqualFold(t.To, till) {
			incoming = append(incoming, t.Hash)
		} else if strings.EqualFold(t.From, till) {
			outgoing = append(outgoing, t.Hash)
		}
	}
	totals, err := a.db.RefundedTotals(r.Context(), incoming)
	if err != nil {
		a.logger.Logf("refund: could not sum refunds for location %d: %s", locationID, err)
		totals = map[string]*big.Int{}
	}
	refundsByOriginal, err := a.db.ListRefundsForOriginals(r.Context(), incoming)
	if err != nil {
		refundsByOriginal = map[string][]structs.Refund{}
	}
	originalsByRefund, err := a.db.RefundOriginalsByRefundHash(r.Context(), outgoing)
	if err != nil {
		originalsByRefund = map[string]string{}
	}

	rows := make([]structs.LocationTransaction, 0, len(transfers))
	for _, t := range transfers {
		row := structs.LocationTransaction{
			Hash:       t.Hash,
			From:       t.From,
			To:         t.To,
			AmountBase: t.Amount,
			Timestamp:  int64(t.Timestamp),
			Refund:     structs.RefundState{Status: structs.RefundStatusNone},
		}
		switch {
		case strings.EqualFold(t.To, till):
			row.Direction, row.Wallet = "in", "payment"
		case strings.EqualFold(t.From, till):
			row.Direction, row.Wallet = "out", "payment"
		case strings.EqualFold(t.To, tips):
			row.Direction, row.Wallet = "in", "tipping"
		default:
			row.Direction, row.Wallet = "out", "tipping"
		}

		key := strings.ToLower(t.Hash)
		if row.Direction == "in" && row.Wallet == "payment" {
			refunded := totals[key]
			if refunded == nil {
				refunded = big.NewInt(0)
			}
			row.Refund.Status = refundStatusFor(t.Amount, refunded)
			if refunded.Sign() > 0 {
				row.Refund.RefundedBase = refunded.String()
				row.Refund.Refunds = refundsByOriginal[key]
			}
			if original, ok := new(big.Int).SetString(t.Amount, 10); ok {
				remaining := new(big.Int).Sub(original, refunded)
				if remaining.Sign() < 0 {
					remaining = big.NewInt(0)
				}
				row.Refund.RemainingBase = remaining.String()
			}
		} else if originalHash, ok := originalsByRefund[key]; ok {
			// This transfer IS a refund. Point it at what it undid so the two
			// lines cross-reference instead of reading as unrelated movements.
			row.Refund.Status = structs.RefundStatusIsRefund
			row.Refund.RefundsOriginalHash = originalHash
		}
		rows = append(rows, row)
	}

	writeJSON(w, http.StatusOK, structs.LocationTransactionsResponse{
		LocationID:    locationID,
		TokenDecimals: merchantTokenDecimals(),
		Transactions:  rows,
		Total:         total,
		Page:          page,
	})
}
