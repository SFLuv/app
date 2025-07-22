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

	appService := handlers.NewAppService(appDb)

	router.AddLocationRoutes(testrouter, appService)
	testserver = httptest.NewServer(testrouter)
	defer testserver.Close()

	t.Run("add location test", UnitAddLocation)
	t.Run("get location test", UnitGetLocation)
	t.Run("get all locations test", UnitGetLocations)

}

func UnitAddLocation(t *testing.T) {
	body_data_1 := []byte(`{
		"id": 1,
		"google_id": "abc123",
		"owner_id": "user-001",
		"name": "Bob's Burgers",
		"description": "A homestyle burger place",
		"type": "Restaurant",
		"approval": true,
		"street": "123 Ocean Ave",
		"city": "Seymour's Bay",
		"state": "CA",
		"zip": "90210",
		"lat": 34.0522,
		"lng": -118.2437,
		"phone": "555-1234",
		"email": "bob@example.com",
		"website": "https://bobsburgers.com",
		"image_url": "https://images.example.com/bobs.jpg",
		"rating": 4.6,
		"maps_page": "https://maps.google.com/?cid=abc123"
	}`)

	body_data_2 := []byte(`{
		"id": 2,
		"google_id": "def345",
		"owner_id": "user-002",
		"name": "Krusty Krab",
		"description": "Delicious Krabby Patties",
		"type": "Fast Food",
		"approval": false,
		"street": "124 Bikini Bottom Blvd",
		"city": "Bikini Bottom",
		"state": "HI",
		"zip": "96815",
		"lat": 21.3069,
		"lng": -157.8583,
		"phone": "555-5678",
		"email": "krabs@krustykrab.com",
		"website": "https://krustykrab.com",
		"image_url": "https://images.example.com/krusty.jpg",
		"rating": 4.9,
		"maps_page": "https://maps.google.com/?cid=def345"
	}`)

	add_request_1, err := http.NewRequest(http.MethodPost, testserver.URL+"/locations", bytes.NewReader(body_data_1))
	if err != nil {
		t.Fatalf("error creating add request 1: %s", err)
	}
	add_request_2, err := http.NewRequest(http.MethodPost, testserver.URL+"/locations", bytes.NewReader(body_data_2))
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

func UnitGetLocation(t *testing.T) {
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

func UnitGetLocations(t *testing.T) {
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

func UnitUpdateLocation(t *testing.T) {
	body_data_1 := []byte(`{
		"id": 1,
		"google_id": "abc123",
		"owner_id": "user-001",
		"name": "Bob's Burgers",
		"description": "This description has been updated",
		"type": "Restaurant",
		"approval": true,
		"street": "456 New Street Location",
		"city": "New Updated City",
		"state": "CA",
		"zip": "90210",
		"lat": 34.0522,
		"lng": -118.2437,
		"phone": "555-1234",
		"email": "bob@example.com",
		"website": "https://bobsburgers.com",
		"image_url": "https://images.example.com/bobs.jpg",
		"rating": 4.6,
		"maps_page": "https://maps.google.com/?cid=abc123"
	}`)

	put_request_1, err := http.NewRequest(http.MethodPut, testserver.URL+"/locations", bytes.NewReader(body_data_1))
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
