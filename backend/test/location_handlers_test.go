package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logs"

	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/router"
	"github.com/go-chi/chi/v5"
	//"github.com/SFLuv/app/backend/structs"
)

var testserver *httptest.Server

func TestLocationHandlers(t *testing.T) {
	testrouter := chi.NewRouter()

	mdb, err := db.PgxDB("test_location")
	if err != nil {
		t.Fatalf("error intializing database: %s", err)
	}
	defer mdb.Close(context.Background())

	appDb := db.App(mdb)
	err = appDb.CreateTables()
	if err != nil {
		t.Fatalf("error creating tables: %s", err)
	}

	timeString := time.Now().Format(time.RFC3339)

	appLogger, err := logs.New(fmt.Sprintf("./test/logs/app_test_%s.log", timeString), "APP_TEST: ")
	if err != nil {
		t.Fatalf("error initializing app logger")
	}
	defer appLogger.File.Close()

	appService := handlers.NewAppService(appDb, appLogger.Logger)

	router.AddLocationRoutes(testrouter, appService)
	testserver = httptest.NewServer(testrouter)
	defer testserver.Close()

	t.Run("add location test", ModuleAddLocation)
	t.Run("get location test", ModuleGetLocation)
	t.Run("get all locations test", ModuleGetLocations)

}

func ModuleAddLocation(t *testing.T) {
	body_data_1, err := json.Marshal(TEST_LOCATION_1)
	if err != nil {
		t.Fatalf("error marshaling JSON for location 1: %s", err)
	}

	body_data_2, err := json.Marshal(TEST_LOCATION_2)
	if err != nil {
		t.Fatalf("error marshaling JSON for location 2: %s", err)
	}

	add_request_1, err := http.NewRequest(http.MethodPost, testserver.URL+"/locations", bytes.NewReader([]byte(body_data_1)))
	if err != nil {
		t.Fatalf("error creating add request 1: %s", err)
	}
	add_request_2, err := http.NewRequest(http.MethodPost, testserver.URL+"/locations", bytes.NewReader([]byte(body_data_2)))
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

func ModuleGetLocation(t *testing.T) {
	get_request, err := http.NewRequest(http.MethodGet, testserver.URL+"/locations/"+"1", nil)
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

func ModuleGetLocations(t *testing.T) {
	get_request, err := http.NewRequest(http.MethodGet, testserver.URL+"/locations", nil)
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

func ModuleUpdateLocation(t *testing.T) {
	body_data_2, err := json.Marshal(TEST_LOCATION_2A)
	if err != nil {
		t.Fatalf("error marshaling JSON for location 1: %s", err)
	}

	put_request_1, err := http.NewRequest(http.MethodPut, testserver.URL+"/locations/2", bytes.NewReader(body_data_2))
	if err != nil {
		t.Fatalf("error creating put request 1: %s", err)
	}

	put_request_1.Header.Set("Content-Type", "application/json")

	put_request_response_1, err := testserver.Client().Do(put_request_1)
	if err != nil {
		t.Fatalf("error sending put request: %s", err)
	}
	if put_request_response_1.StatusCode < 200 || put_request_response_1.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", put_request_response_1.StatusCode)
	}

}
