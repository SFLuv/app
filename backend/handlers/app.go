package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
)

type AppService struct {
	db *db.AppDB
}

func NewAppService(db *db.AppDB) *AppService {
	return &AppService{db}
}

func (s *AppService) GetLocation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	num, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid id"))
		return
	}
	merch, err := s.db.GetLocation(num)
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

func (s *AppService) GetLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := s.db.GetLocations()
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

func (s *AppService) AddLocation(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error reading req body"))
		return
	}

	var location *structs.LocationRequest
	err = json.Unmarshal(body, &location)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid req body"))
		return
	}

	err = s.db.AddLocation(location)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid location body"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}
