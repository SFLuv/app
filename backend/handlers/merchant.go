package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/faucet-portal/backend/db"
	"github.com/faucet-portal/backend/structs"
	"gorm.io/gorm"
)

type MerchantService struct {
	db *gorm.DB
}

func NewMerchantService(db *db.AccountDB) *AccountService {
	return &AccountService{db}
}

func (s *MerchantService) GetMerchant(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	address := params.Get("address")

	// TODO SANCHEZ: get merchant from db using GORM

	w.WriteHeader(200)
	w.Write([]byte(fmt.Sprintf(`{ "merchant": %t }`, acc)))
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
