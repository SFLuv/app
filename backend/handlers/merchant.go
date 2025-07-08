package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"github.com/faucet-portal/backend/db"
	"github.com/faucet-portal/backend/structs"
)

type MerchantService struct {
	db *db.MerchantDB
}

func NewMerchantService(db *db.MerchantDB) *MerchantService {
	return &MerchantService{db}
}

func (s *MerchantService) GetMerchant(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	merchant := params.Get("merchant")
	merch := s.db.GetMerchant(merchant)
	w.WriteHeader(200)
	w.Write([]byte(fmt.Sprintf(`{ "merchant": %t }`, merch)))
}

func (s *MerchantService) AddMerchant(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var account *structs.MerchantRequest
	err = json.Unmarshal(body, &account)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO SANCHEZ: add merchant to db using GORM

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}
