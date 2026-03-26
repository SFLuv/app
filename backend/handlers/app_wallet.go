package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

func (a *AppService) GetWalletsByUser(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	wallets, err := a.db.GetWalletsByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting wallets for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	bytes, err := json.Marshal(wallets)
	if err != nil {
		a.logger.Logf("error marshalling wallets struct:\n  %#v\nfor user %s: %s", wallets, *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

func (a *AppService) AddWallet(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading add wallet request body from user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()

	var wallet structs.Wallet
	err = json.Unmarshal(body, &wallet)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Ignore any client-supplied id for wallet creation; the DB owns identity here.
	wallet.Id = nil
	wallet.Owner = *userDid

	id, err := a.db.AddWallet(r.Context(), &wallet)
	if err != nil {
		a.logger.Logf("error adding wallet:\n  %#v\nfor user %s: %s", wallet, *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if a.redeemer != nil && a.redeemer.IsEnabled() && !wallet.IsEoa && wallet.SmartIndex != nil && *wallet.SmartIndex == 0 {
		hasApprovedLocation, err := a.db.UserHasAnyApprovedLocation(r.Context(), wallet.Owner)
		if err != nil {
			a.logger.Logf("error checking approved locations for user %s after wallet add: %s", wallet.Owner, err)
		} else if hasApprovedLocation {
			if err := a.redeemer.EnsureMerchantHasRedeemerWallet(r.Context(), wallet.Owner); err != nil {
				a.logger.Logf("error auto-granting redeemer role for user %s after wallet add: %s", wallet.Owner, err)
			}
		}
	}
	res := strconv.Itoa(id)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(res))
}

func (a *AppService) UpdateWallet(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update wallet request body from user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var wallet structs.Wallet
	err = json.Unmarshal(body, &wallet)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	wallet.Owner = *userDid

	err = a.db.UpdateWallet(r.Context(), &wallet)
	if err != nil {
		a.logger.Logf("error updating wallet:\n  %#v\nfor user %s: %s", wallet, *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (a *AppService) LookupWalletOwnerByAddress(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	address := strings.TrimSpace(r.PathValue("address"))
	if address == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	lookup, err := a.db.GetWalletAddressOwnerLookup(r.Context(), address)
	if err != nil {
		a.logger.Logf("error looking up wallet owner by address %s: %s", address, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	response := map[string]any{
		"found":   false,
		"address": address,
	}
	if lookup != nil {
		response["found"] = true
		response["user_id"] = lookup.UserID
		response["is_merchant"] = lookup.IsMerchant
		response["wallet_name"] = lookup.WalletName
		response["address"] = lookup.MatchedAddress
		if lookup.IsMerchant && strings.TrimSpace(lookup.MerchantName) != "" {
			response["merchant_name"] = strings.TrimSpace(lookup.MerchantName)
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}
