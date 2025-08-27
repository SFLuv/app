package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
)

type AccountService struct {
	db *db.AccountDB
}

func NewAccountService(db *db.AccountDB) *AccountService {
	return &AccountService{db}
}

func (s *AccountService) GetAccount(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	address := params.Get("address")

	acc := s.db.GetAccount(r.Context(), address)

	w.WriteHeader(200)
	w.Write([]byte(fmt.Sprintf(`{ "account": %t }`, acc)))
}

func (s *AccountService) AddAccount(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var account *structs.AccountRequest
	err = json.Unmarshal(body, &account)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = s.db.NewAccount(r.Context(), account)
	if err != nil {
		fmt.Println("error adding new account:", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}
