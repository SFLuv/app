package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
)

func (a *AppService) GetLocation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	num, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		a.logger.Logf("invalid id, got: %s: %s", id, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	location, err := a.db.GetLocation(r.Context(), num)
	if err != nil {
		a.logger.Logf("no location with id %s: %s", id, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	jsonBytes, err := json.Marshal(map[string]any{
		"location": location,
	})
	if err != nil {
		a.logger.Logf("Error marhalling json %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(200)
	w.Write(jsonBytes)
}

func (a *AppService) GetLocations(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	count, err := strconv.Atoi(params.Get("count"))
	if err != nil {
		count = 1000
	}
	page, err := strconv.Atoi(params.Get("page"))
	if err != nil {
		page = 0
	}

	request := structs.LocationsPageRequest{
		Page:  uint(page),
		Count: uint(count),
	}

	locations, err := a.db.GetLocations(r.Context(), &request)
	if err != nil {
		a.logger.Logf("Failed to get locations %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(map[string]any{
		"locations": locations,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		a.logger.Logf("Error marshalling JSON for locations objects %s", err.Error())
		return
	}
	w.WriteHeader(200)
	w.Write(jsonBytes)
}

func (a *AppService) GetLocationsByUser(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		a.logger.Logf("Could not pull user DID")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	locations, err := a.db.GetLocationsByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting locations for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(map[string]any{
		"locations": locations,
	})
	if err != nil {
		a.logger.Logf(("Error marshalling JSON for locations objects"))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(200)
	w.Write(jsonBytes)
}

func (a *AppService) AddLocation(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userDid := utils.GetDid(r)
	if userDid == nil {
		a.logger.Logf("Could not pull user DID")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading req body: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var location *structs.Location
	err = json.Unmarshal(body, &location)
	if err != nil {
		a.logger.Logf("invalid req body: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	location.OwnerID = *userDid
	err = a.db.AddLocation(r.Context(), location)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		a.logger.Logf("invalid location body: %s", err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

func (a *AppService) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userDid := utils.GetDid(r)
	if userDid == nil {
		a.logger.Logf("could not pull user DID")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	id := chi.URLParam(r, "id")
	num, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		a.logger.Logf("error, invalid ID %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	location, err := a.db.GetLocation(r.Context(), num)
	if err != nil {
		a.logger.Logf("no location %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if location.OwnerID != *userDid {
		a.logger.Logf("location Owner ID does not match user DID")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err = a.db.UpdateLocation(r.Context(), location)
	if err != nil {
		a.logger.Logf("failed to update location %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("update successful"))
}
