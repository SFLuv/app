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

func (s *AppService) GetMerchant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	num, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid id"))
		return
	}
	merch, err := s.db.GetMerchant(num)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("no merchant"))
		return
	}

	jsonBytes, err := json.Marshal(map[string]any{
		"merchant": merch,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error marshalling JSON"))
		return
	}
	w.WriteHeader(200)
	w.Write(jsonBytes)
}

func (s *AppService) GetMerchants(w http.ResponseWriter, r *http.Request) {
	merchants, err := s.db.GetMerchants()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("failed to get merchants"))
		return
	}
	jsonBytes, err := json.Marshal(map[string]any{
		"merchants": merchants,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error marshalling JSON for merchants objects"))
		return
	}
	w.WriteHeader(200)
	w.Write(jsonBytes)
}

func (s *AppService) AddMerchant(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error reading req body"))
		return
	}

	var merchant *structs.MerchantRequest
	err = json.Unmarshal(body, &merchant)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid req body"))
		return
	}

	err = s.db.AddMerchant(merchant)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid merchant body"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}
