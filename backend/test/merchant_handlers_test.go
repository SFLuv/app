package test

import (
	"bytes"
	"context"
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
	testrouter := chi.NewRouter()

	mdb, err := db.PgxDB("test_merchant")
	if err != nil {
		t.Fatalf("error intializing database: %s", err)
	}
	defer mdb.Close(context.Background())

	appDb := db.App(mdb)
	err = appDb.CreateTables()
	if err != nil {
		t.Fatalf("error creating tables: %s", err)
	}

	appService := handlers.NewAppService(appDb)

	router.AddMerchantRoutes(testrouter, appService)
	testserver = httptest.NewServer(testrouter)
	defer testserver.Close()

	t.Run("add merchant test", UnitAddMerchant)
	t.Run("get merchant test", UnitGetMerchant)
	t.Run("get all merchants test", UnitGetMerchants)

}

func UnitAddMerchant(t *testing.T) {
	body_data_1 := []byte(`{"name": "Bob's Burgers", "googleid": "abc123", "description": "a homestyle burger place", "id": 1}`)
	add_request_1, err := http.NewRequest(http.MethodPost, testserver.URL+"/merchants", bytes.NewReader(body_data_1))
	if err != nil {
		t.Fatalf("error creating add request 1: %s", err)
	}

	body_data_2 := []byte(`{"name": "Krusty Crab", "googleid": "def345", "description": "delicious Krabby Patties", "id": 2}`)
	add_request_2, err := http.NewRequest(http.MethodPost, testserver.URL+"/merchants", bytes.NewReader(body_data_2))
	if err != nil {
		t.Fatalf("error creating add request 2: %s", err)
	}

	add_request_1.Header.Set("Content-Type", "application/json")
	add_request_2.Header.Set("Content-Type", "application/json")

	add_request_response_1, err := testserver.Client().Do(add_request_1)
	if err != nil {
		t.Fatalf("error sending add request: %s", err)
	}
	if add_request_response_1.StatusCode < 200 || add_request_response_1.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", add_request_response_1.StatusCode)
	}

	add_request_response_2, err := testserver.Client().Do(add_request_2)
	if err != nil {
		t.Fatalf("error sending add request: %s", err)
	}
	if add_request_response_2.StatusCode < 200 || add_request_response_2.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", add_request_response_2.StatusCode)
	}

	_, err = io.ReadAll(add_request_response_1.Body)
	if err != nil {
		t.Fatalf("error reading response 1 body %s", err)
	}
	_, err = io.ReadAll(add_request_response_2.Body)
	if err != nil {
		t.Fatalf("error reading response 2 body %s", err)
	}
}

func UnitGetMerchant(t *testing.T) {
	get_request, err := http.NewRequest(http.MethodGet, testserver.URL+"/merchants/"+"1", nil)
	if err != nil {
		t.Fatalf("error creating get request: %s", err)
	}

	get_request_response, err := testserver.Client().Do(get_request)
	if err != nil {
		t.Fatalf("error sending get request: %s", err)
	}
	if get_request_response.StatusCode < 200 || get_request_response.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", get_request_response.StatusCode)
	}

	_, err = io.ReadAll(get_request_response.Body)
	if err != nil {
		t.Fatalf("error reading response body %s", err)
	}
}

func UnitGetMerchants(t *testing.T) {
	get_request, err := http.NewRequest(http.MethodGet, testserver.URL+"/merchants", nil)
	if err != nil {
		t.Fatalf("error creating get request: %s", err)
	}

	get_request_response, err := testserver.Client().Do(get_request)
	if err != nil {
		t.Fatalf("error sending get request: %s", err)
	}
	if get_request_response.StatusCode < 200 || get_request_response.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", get_request_response.StatusCode)
	}

	_, err = io.ReadAll(get_request_response.Body)
	if err != nil {
		t.Fatalf("error reading response body %s", err)
	}

}
