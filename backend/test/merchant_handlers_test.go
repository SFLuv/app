package test

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"net/http"
	"net/http/httptest"

	"github.com/SFLuv/app/backend/db"

	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/router"
	"github.com/go-chi/chi/v5"
	//"github.com/SFLuv/app/backend/structs"
)

var testserver *httptest.Server

func TestMerchantHandlers(t *testing.T) {
	// setup code
	testrouter := chi.NewRouter()
	mdb, err := db.InitDB("test_merchant")
	if err != nil {
		t.Fatalf("error intializing database: %s\n", err)
	}
	defer mdb.Close()
	merchantDb := db.Merchant(mdb)
	err = merchantDb.CreateTables()
	if err != nil {
		t.Fatalf("error creating tables: %s\n", err)
	}
	merchant_service := handlers.NewMerchantService(merchantDb)
	router.AddMerchantRoutes(testrouter, merchant_service)
	testserver = httptest.NewServer(testrouter)
	defer testserver.Close()
	t.Run("add merchant test", UnitAddMerchant)
	t.Run("get merchant test", UnitGetMerchant)
	t.Run("get all merchants test", UnitGetMerchants)
	// teardown code
}

func UnitAddMerchant(t *testing.T) {
	body_data_1 := []byte(`{"name": "Bob's Burgers", "googleid": "abc123", "description": "a homestyle burger place", "id": 1}`)
	add_request_1, err := http.NewRequest(http.MethodPost, testserver.URL+"/merchants", bytes.NewReader(body_data_1))
	if err != nil {
		t.Fatalf("error creating add request 1: %s\n", err)
	}
	body_data_2 := []byte(`{"name": "Krusty Crab", "googleid": "def345", "description": "delicious Krabby Patties", "id": 2}`)
	add_request_2, err := http.NewRequest(http.MethodPost, testserver.URL+"/merchants", bytes.NewReader(body_data_2))
	if err != nil {
		t.Fatalf("error creating add request 2: %s\n", err)
	}
	add_request_1.Header.Set("Content-Type", "application/json")
	add_request_2.Header.Set("Content-Type", "application/json")
	add_request_response_1, err := testserver.Client().Do(add_request_1)
	if err != nil {
		t.Fatalf("error sending add request: %s\n", err)
	}
	add_request_response_2, err := testserver.Client().Do(add_request_2)
	if err != nil {
		t.Fatalf("error sending add request: %s\n", err)
	}
	addbody1, _ := io.ReadAll(add_request_response_1.Body)
	addbody2, _ := io.ReadAll(add_request_response_2.Body)
	fmt.Println(string(addbody1))
	fmt.Println(string(addbody2))
}

func UnitGetMerchant(t *testing.T) {
	get_request, err := http.NewRequest(http.MethodGet, testserver.URL+"/merchants/"+"1", nil)
	if err != nil {
		t.Fatalf("error creating get request: %s\n", err)
	}
	get_request_response, err := testserver.Client().Do(get_request)
	if err != nil {
		t.Fatalf("error sending get request: %s\n", err)
	}
	getbody, _ := io.ReadAll(get_request_response.Body)
	fmt.Println(string(getbody))
}

func UnitGetMerchants(t *testing.T) {
	get_request, err := http.NewRequest(http.MethodGet, testserver.URL+"/merchants", nil)
	if err != nil {
		t.Fatalf("error creating get request: %s\n", err)
	}
	get_request_response, err := testserver.Client().Do(get_request)
	if err != nil {
		t.Fatalf("error sending get request: %s\n", err)
	}
	getbody, _ := io.ReadAll(get_request_response.Body)
	fmt.Println(string(getbody))
}
