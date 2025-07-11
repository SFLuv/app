package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"testing"

	"net/http"
	"net/http/httptest"

	"github.com/faucet-portal/backend/db"

	"github.com/faucet-portal/backend/handlers"
	"github.com/faucet-portal/backend/router"
	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	//"github.com/faucet-portal/backend/structs"
)
var testserver *httptest.Server

func TestMain(m *testing.M) {
    // setup code
    testrouter := chi.NewRouter()
    godotenv.Load("../test.env", "../.env")
    mdb := db.InitDB("merchants")
	merchantDb := db.Merchant(mdb)
    err := merchantDb.CreateTables()
    if err != nil {
        fmt.Printf("error creating tables: %s\n", err)
        os.Exit(1)
    }
    merchant_service := handlers.NewMerchantService(merchantDb)
    router.AddMerchantRoutes(testrouter, merchant_service)
    testserver = httptest.NewServer(testrouter)
    defer testserver.Close()

    //run tests
    code := m.Run()

    // teardown code
    err = os.RemoveAll("/Users/sanchezoleary/Projects/app/backend/test/data")
    if err != nil {
        log.Fatalf("Failed to delete directory: %v", err)
    }
    os.Exit(code)
}

func TestAddMerchant(t *testing.T) {
    body_data_1 := []byte(`{"name": "Bob's Burgers", "googleid": "abc123", "description": "a homestyle burger place", "id": 1}`)
    add_request_1, err := http.NewRequest(http.MethodPost, testserver.URL + "/merchants", bytes.NewReader(body_data_1))
    if err != nil {
        fmt.Printf("error creating add request 1: %s\n", err)
        os.Exit(1)
    }
    body_data_2 := []byte(`{"name": "Krusty Crab", "googleid": "def345", "description": "delicious Krabby Patties", "id": 2}`)
    add_request_2, err := http.NewRequest(http.MethodPost, testserver.URL + "/merchants", bytes.NewReader(body_data_2))
    if err != nil {
        fmt.Printf("error creating add request 2: %s\n", err)
        os.Exit(1)
    }
    add_request_1.Header.Set("Content-Type", "application/json")
    add_request_2.Header.Set("Content-Type", "application/json")
    add_request_response_1, err := testserver.Client().Do(add_request_1)
    if err != nil {
        fmt.Printf("error sending add request: %s\n", err)
        os.Exit(1)
    }
    add_request_response_2, err := testserver.Client().Do(add_request_2)
    if err != nil {
        fmt.Printf("error sending add request: %s\n", err)
        os.Exit(1)
    }
    addbody1, _ := io.ReadAll(add_request_response_1.Body)
    addbody2, _ := io.ReadAll(add_request_response_2.Body)
    fmt.Println(string(addbody1))
    fmt.Println(string(addbody2))
}

func TestGetMerchant(t *testing.T) {
    get_request, err := http.NewRequest(http.MethodGet, testserver.URL + "/merchants/" + "1", nil)
    if err != nil {
        fmt.Printf("error creating get request: %s\n", err)
        os.Exit(1)
    }
    get_request_response, err := testserver.Client().Do(get_request)
    if err != nil {
        fmt.Printf("error sending get request: %s\n", err)
        os.Exit(1)
    }
    getbody, _ := io.ReadAll(get_request_response.Body)
    fmt.Println(string(getbody))
    }

func TestGetMerchants(t *testing.T) {
    get_request, err := http.NewRequest(http.MethodGet, testserver.URL + "/merchants", nil)
    if err != nil {
        fmt.Printf("error creating get request: %s\n", err)
        os.Exit(1)
    }
    get_request_response, err := testserver.Client().Do(get_request)
    if err != nil {
        fmt.Printf("error sending get request: %s\n", err)
        os.Exit(1)
    }
    getbody, _ := io.ReadAll(get_request_response.Body)
    fmt.Println(string(getbody))
    }
