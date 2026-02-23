package handlers

import (
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

var minimumFollowupUnwrapAmountWei = func() *big.Int {
	value, _ := new(big.Int).SetString("100000000000000000000", 10)
	return value
}()

func isSameUTCMonth(t1 time.Time, t2 time.Time) bool {
	a := t1.UTC()
	b := t2.UTC()
	return a.Year() == b.Year() && a.Month() == b.Month()
}

func (a *AppService) CheckUnwrapEligibility(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req structs.UnwrapEligibilityRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.WalletAddress == "" || req.AmountWei == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	amountWei := new(big.Int)
	if _, ok := amountWei.SetString(req.AmountWei, 10); !ok || amountWei.Sign() <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	wallet, err := a.db.GetWalletByUserAndAddress(r.Context(), *userDid, req.WalletAddress)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		a.logger.Logf("error loading wallet for unwrap eligibility user=%s wallet=%s: %s", *userDid, req.WalletAddress, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !wallet.IsRedeemer {
		resp := structs.UnwrapEligibilityResponse{
			Allowed:                  false,
			Reason:                   "Wallet is not unwrap-enabled",
			LastUnwrapAt:             wallet.LastUnwrapAt,
			MinimumFollowupAmountWei: minimumFollowupUnwrapAmountWei.String(),
		}
		bytes, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write(bytes)
		return
	}

	now := time.Now().UTC()
	allowed := true
	reason := ""
	if wallet.LastUnwrapAt != nil && isSameUTCMonth(*wallet.LastUnwrapAt, now) && amountWei.Cmp(minimumFollowupUnwrapAmountWei) < 0 {
		allowed = false
		reason = "You already unwrapped this month. Additional unwraps this month must be at least $100."
	}

	resp := structs.UnwrapEligibilityResponse{
		Allowed:                  allowed,
		Reason:                   reason,
		LastUnwrapAt:             wallet.LastUnwrapAt,
		MinimumFollowupAmountWei: minimumFollowupUnwrapAmountWei.String(),
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !allowed {
		w.WriteHeader(http.StatusForbidden)
		w.Write(bytes)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

func (a *AppService) RecordUnwrap(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req structs.UnwrapRecordRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.WalletAddress == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	wallet, err := a.db.GetWalletByUserAndAddress(r.Context(), *userDid, req.WalletAddress)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		a.logger.Logf("error loading wallet for unwrap record user=%s wallet=%s: %s", *userDid, req.WalletAddress, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !wallet.IsRedeemer {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if wallet.Id == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	recordedAt := time.Now().UTC()
	if err := a.db.SetWalletLastUnwrapAt(r.Context(), *wallet.Id, recordedAt); err != nil {
		a.logger.Logf("error setting wallet last_unwrap_at wallet_id=%d user=%s: %s", *wallet.Id, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := structs.UnwrapRecordResponse{
		Recorded:   true,
		RecordedAt: recordedAt,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}
