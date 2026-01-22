package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

func (a *AppService) AddUser(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err := a.db.AddUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error adding user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (a *AppService) GetUserAuthed(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	user, err := a.db.GetUserById(r.Context(), *userDid)
	if err == pgx.ErrNoRows {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		a.logger.Logf("error getting user by id %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	wallets, err := a.db.GetWalletsByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting wallets for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	locations, err := a.db.GetLocationsByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting locations for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	contacts, err := a.db.GetContacts(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting contacts for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	response := structs.AuthedUserResponse{
		User:      *user,
		Wallets:   wallets,
		Locations: locations,
		Contacts:  contacts,
	}

	bytes, err := json.Marshal(response)
	if err != nil {
		a.logger.Logf("error marshalling user response struct:\n  %#v\nfor user %s: %s", response, *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

func (a *AppService) UpdateUserInfo(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading add wallet request body from user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var user structs.User
	err = json.Unmarshal(body, &user)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	user.Id = *userDid

	err = a.db.UpdateUserInfo(r.Context(), &user)
	if err != nil {
		a.logger.Logf("error updating user info with struct:\n  %#v\nfor user %s: %s", user, *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (a *AppService) UpdateUserPayPalEth(w http.ResponseWriter, r *http.Request) {
	fmt.Println("update paypal handler reached")
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading paypal address from body from user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = a.db.UpdateUserPayPalEth(r.Context(), *userDid, body)
	if err != nil {
		a.logger.Logf("error updating user paypal address for user: " + *userDid)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
