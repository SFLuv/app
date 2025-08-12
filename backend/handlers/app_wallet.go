package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

func (a *AppService) GetWalletsByUser(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	wallets, err := a.db.GetWalletsByUser(*userDid)
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
	wallet.Owner = *userDid

	err = a.db.AddWallet(&wallet)
	if err != nil {
		a.logger.Logf("error adding wallet:\n  %#v\nfor user %s: %s", wallet, *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
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

	err = a.db.UpdateWallet(&wallet)
	if err != nil {
		a.logger.Logf("error updating wallet:\n  %#v\nfor user %s: %s", wallet, *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
