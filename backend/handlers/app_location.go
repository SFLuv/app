package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
)

func (a *AppService) GetLocation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	num, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid id"))
		return
	}
	merch, err := a.db.GetLocation(num)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("no location"))
		return
	}

	jsonBytes, err := json.Marshal(map[string]any{
		"location": merch,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error marshalling JSON"))
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

	locations, err := a.db.GetLocations(&request)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("failed to get locations"))
		return
	}
	jsonBytes, err := json.Marshal(map[string]any{
		"locations": locations,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error marshalling JSON for locations objects"))
		return
	}
	w.WriteHeader(200)
	w.Write(jsonBytes)
}

func (a *AppService) AddLocation(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error reading req body"))
		return
	}

	var location *structs.Location
	err = json.Unmarshal(body, &location)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid req body"))
		return
	}

	err = a.db.AddLocation(location)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid location body"))
		a.logger.Logf("error was caused calling controller: %s", err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

func (a *AppService) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error reading request body"))
		return
	}

	var location *structs.Location
	err = json.Unmarshal(body, &location)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid request body"))
		return
	}

	err = a.db.UpdateLocation(location)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("failed to update location"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("update successful"))
}
