package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
)

type MerchantService struct {
	db *db.MerchantDB
}

func NewMerchantService(db *db.MerchantDB) *MerchantService {
	return &MerchantService{db}
}

func (s *MerchantService) GetMerchant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	num, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		fmt.Printf("Error converting string to uint: %v\n", err)
		return
	}
	merch := s.db.GetMerchant(num)
	jsonBytes, err := json.Marshal(map[string]any{
		"merchant": merch,
	})
	if err != nil {
		fmt.Printf("Error marshalling JSON for merchant object: %v\n", err)
		return
	}
	w.WriteHeader(200)
	w.Write(jsonBytes)
}

func (s *MerchantService) GetMerchants(w http.ResponseWriter, r *http.Request) {
	merchants := s.db.GetMerchants()
	jsonBytes, err := json.Marshal(map[string]any{
		"merchants": merchants,
	})
	if err != nil {
		fmt.Printf("Error marshalling JSON for merchant objects: %v\n", err)
		return
	}
	w.WriteHeader(200)
	w.Write(jsonBytes)
}

func (s *MerchantService) AddMerchant(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var merchant *structs.MerchantRequest
	err = json.Unmarshal(body, &merchant)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = s.db.AddMerchant(merchant)
	if err != nil {
		fmt.Printf("error adding merchant %s\n", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}
