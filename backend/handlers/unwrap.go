package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

const minimumFollowupUnwrapAmountSFLUV int64 = 100

func minimumFollowupUnwrapAmountWei() (*big.Int, error) {
	multiplier, err := getTokenMultiplier()
	if err != nil {
		return nil, fmt.Errorf("error reading token multiplier for unwrap threshold: %w", err)
	}
	return new(big.Int).Mul(multiplier, big.NewInt(minimumFollowupUnwrapAmountSFLUV)), nil
}

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
	minimumFollowupAmountWei, err := minimumFollowupUnwrapAmountWei()
	if err != nil {
		a.logger.Logf("error loading unwrap threshold: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
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
			MinimumFollowupAmountWei: minimumFollowupAmountWei.String(),
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
	if wallet.LastUnwrapAt != nil && isSameUTCMonth(*wallet.LastUnwrapAt, now) && amountWei.Cmp(minimumFollowupAmountWei) < 0 {
		allowed = false
		reason = "You already unwrapped this month. Additional unwraps this month must be at least $100."
	}

	resp := structs.UnwrapEligibilityResponse{
		Allowed:                  allowed,
		Reason:                   reason,
		LastUnwrapAt:             wallet.LastUnwrapAt,
		MinimumFollowupAmountWei: minimumFollowupAmountWei.String(),
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

	resp := structs.UnwrapRecordResponse{Recorded: true, RecordedAt: recordedAt}

	// A ledger row needs the hash and the amount; a client that sends only
	// the wallet (the pre-ledger shape) still gets its timestamp stamped.
	if req.TxHash != "" && req.AmountWei != "" {
		amountWei := new(big.Int)
		if _, ok := amountWei.SetString(req.AmountWei, 10); !ok || amountWei.Sign() <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// The destination is trusted only from our own records. A location
		// reports which till it unwrapped from; the address it should have
		// gone to is what we provisioned, and a mismatch is logged loudly.
		destination := strings.ToLower(strings.TrimSpace(req.DestinationAddress))
		var locationID *int64
		if req.LocationID != nil {
			owned, err := a.db.LocationOwnedBy(r.Context(), *req.LocationID, *userDid)
			if err != nil || !owned {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			id := int64(*req.LocationID)
			locationID = &id
			if la, err := a.db.GetLocationLiquidationAddress(r.Context(), *req.LocationID); err == nil && la != nil {
				if destination != "" && !strings.EqualFold(destination, la.Address) {
					a.logger.Logf("unwrap record: wallet %s reported destination %s but location %d is provisioned to %s", req.WalletAddress, destination, *req.LocationID, la.Address)
				}
				destination = strings.ToLower(la.Address)
			}
		}
		role := strings.ToLower(strings.TrimSpace(req.WalletRole))
		if role != "tipping" {
			role = "payment"
		}
		walletID := int64(*wallet.Id)
		entry := structs.Unwrap{
			OwnerID:            *userDid,
			LocationID:         locationID,
			WalletID:           &walletID,
			WalletAddress:      req.WalletAddress,
			WalletRole:         role,
			DestinationAddress: destination,
			AmountWei:          amountWei.String(),
			TxHash:             req.TxHash,
		}
		id, err := a.db.InsertUnwrap(r.Context(), &entry)
		if err != nil {
			a.logger.Logf("error inserting unwrap ledger row wallet=%s tx=%s: %s", req.WalletAddress, req.TxHash, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		resp.UnwrapID = id
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetUnwrapHistory is the merchant's own ledger, newest first.
func (a *AppService) GetUnwrapHistory(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	entries, err := a.db.ListUnwrapsByOwner(r.Context(), *userDid, 100)
	if err != nil {
		a.logger.Logf("error loading unwrap history for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
