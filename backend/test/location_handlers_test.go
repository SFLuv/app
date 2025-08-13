package test

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"net/http"
	//"github.com/SFLuv/app/backend/structs"
)

func GroupLocationHandlers(t *testing.T) {
	t.Run("add location test", ModuleAddLocationHandler)
	t.Run("get location test", ModuleGetLocationHandler)
	t.Run("update location test", ModuleGetLocationsHandler)
	t.Run("get all locations test", ModuleUpdateLocationHandler)
}

func ModuleAddLocationHandler(t *testing.T) {
	Spoofer.SetValue("userDid", TEST_USER_1.Id)

	body_data_1, err := json.Marshal(TEST_LOCATION_1)
	if err != nil {
		t.Fatalf("error marshaling JSON for location 1: %s", err)
	}

	add_request_1, err := http.NewRequest(http.MethodPost, TestServer.URL+"/locations", bytes.NewReader([]byte(body_data_1)))
	if err != nil {
		t.Fatalf("error creating add request 1: %s", err)
	}

	add_request_1.Header.Set("Content-Type", "application/json")

	Spoofer.SetValue("userDid", TEST_USER_2.Id)

	body_data_2, err := json.Marshal(TEST_LOCATION_2)
	if err != nil {
		t.Fatalf("error marshaling JSON for location 2: %s", err)
	}

	add_request_2, err := http.NewRequest(http.MethodPost, TestServer.URL+"/locations", bytes.NewReader([]byte(body_data_2)))
	if err != nil {
		t.Fatalf("error creating add request 2: %s", err)
	}

	add_request_2.Header.Set("Content-Type", "application/json")

	add_request_response_1, err := TestServer.Client().Do(add_request_1)
	if err != nil {
		t.Fatalf("error sending add request: %s", err)
	}
	if add_request_response_1.StatusCode < 200 || add_request_response_1.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", add_request_response_1.StatusCode)
	}

	add_request_response_2, err := TestServer.Client().Do(add_request_2)
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

func ModuleGetLocationHandler(t *testing.T) {
	get_request, err := http.NewRequest(http.MethodGet, TestServer.URL+"/locations/"+"1", nil)
	if err != nil {
		t.Fatalf("error creating get request: %s", err)
	}

	get_request_response, err := TestServer.Client().Do(get_request)
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

func ModuleGetLocationsHandler(t *testing.T) {
	get_request, err := http.NewRequest(http.MethodGet, TestServer.URL+"/locations", nil)
	if err != nil {
		t.Fatalf("error creating get request: %s", err)
	}

	get_request_response, err := TestServer.Client().Do(get_request)
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

func ModuleUpdateLocationHandler(t *testing.T) {
	put_request_1, err := http.NewRequest(http.MethodPut, TestServer.URL+"/locations/"+"1", nil)
	if err != nil {
		t.Fatalf("error creating put request: %s", err)
	}

	put_request_1.Header.Set("Content-Type", "application/json")

	put_request_response_1, err := TestServer.Client().Do(put_request_1)
	if err != nil {
		t.Fatalf("error sending put request: %s", err)
	}
	if put_request_response_1.StatusCode < 200 || put_request_response_1.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", put_request_response_1.StatusCode)
	}

}
