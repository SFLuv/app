package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

func (a *AppService) NewContact(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var c structs.Contact
	err = json.Unmarshal(body, &c)
	if err != nil {
		a.logger.Logf(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	contact, err := a.db.AddContact(r.Context(), &c, *userDid)
	if err != nil {
		a.logger.Logf("error adding contact for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res, err := json.Marshal(contact)
	if err != nil {
		a.logger.Logf(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write(res)
}

func (a *AppService) GetContacts(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	contacts, err := a.db.GetContacts(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting contacts for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res, err := json.Marshal(contacts)
	if err != nil {
		a.logger.Logf(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(res)
}

func (a *AppService) UpdateContact(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var c structs.Contact
	err = json.Unmarshal(body, &c)
	if err != nil {
		a.logger.Logf(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = a.db.UpdateContact(r.Context(), &c, *userDid)
	if err != nil {
		a.logger.Logf("error updating contact %d for user %s: %s", c.Id, *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (a *AppService) DeleteContact(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	id := r.URL.Query().Get("id")
	cId, err := strconv.Atoi(id)
	if err != nil {
		a.logger.Logf(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = a.db.DeleteContact(r.Context(), cId, *userDid)
	if err != nil {
		a.logger.Logf("error deleting contact %d for user %s: %s", cId, *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
