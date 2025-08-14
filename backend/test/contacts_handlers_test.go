package test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func GroupContactsHandlers(t *testing.T) {
	t.Run("new contact handler", ModuleNewContactHandler)
	t.Run("update contact handler", ModuleUpdateContactHandler)
	t.Run("get contacts handler", ModuleGetContactsHandler)
	t.Run("delete contract handler", ModuleDeleteContactHandler)
}

func ModuleNewContactHandler(t *testing.T) {

	body_data_1, err := json.Marshal(TEST_CONTACT_1)
	if err != nil {
		t.Fatalf("error marshaling JSON for location 1: %s", err)
	}

	add_request_1, err := http.NewRequest(http.MethodPost, TestServer.URL+"/contacts", bytes.NewReader([]byte(body_data_1)))
	if err != nil {
		t.Fatalf("error creating add request 1: %s", err)
	}

	add_request_1.Header.Set("Content-Type", "application/json")

	body_data_2, err := json.Marshal(TEST_CONTACT_2)
	if err != nil {
		t.Fatalf("error marshaling JSON for location 2: %s", err)
	}

	add_request_2, err := http.NewRequest(http.MethodPost, TestServer.URL+"/contacts", bytes.NewReader([]byte(body_data_2)))
	if err != nil {
		t.Fatalf("error creating add request 2: %s", err)
	}

	add_request_2.Header.Set("Content-Type", "application/json")

	Spoofer.SetValue("userDid", TEST_USER_1.Id)

	add_request_response_1, err := TestServer.Client().Do(add_request_1)
	if err != nil {
		t.Fatalf("error sending add request: %s", err)
	}
	if add_request_response_1.StatusCode < 200 || add_request_response_1.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", add_request_response_1.StatusCode)
	}

	Spoofer.SetValue("userDid", TEST_USER_2.Id)

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

func ModuleUpdateContactHandler(t *testing.T) {
	Spoofer.SetValue("userDid", TEST_USER_2.Id)

	body, err := json.Marshal(TEST_CONTACT_2A)
	if err != nil {
		t.Fatalf("error marshalling contact for request body: %s", err)
	}

	req, err := http.NewRequest(http.MethodPut, TestServer.URL+"/contacts", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("error creating update contact request: %s", err)
	}

	res, err := TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending update contact request: %s", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", res.StatusCode)
	}
}

func ModuleGetContactsHandler(t *testing.T) {
	Spoofer.SetValue("userDid", TEST_USER_2.Id)

	req, err := http.NewRequest(http.MethodGet, TestServer.URL+"/contacts", nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}

	res, err := TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending request: %s", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("error reading response body: %s", err)
	}

	var contact []*structs.Contact
	err = json.Unmarshal(body, &contact)
	if err != nil {
		t.Fatalf("error unmarshalling get contact body")
	}

	if contact[0].Id != TEST_CONTACT_2A.Id {
		t.Fatalf("got incorrect contact id %d, expected %d", contact[0].Id, TEST_CONTACT_2A.Id)
	}

	if contact[0].Name != TEST_CONTACT_2A.Name {
		t.Fatalf("got incorrect contact name %s, expected %s", contact[0].Name, TEST_CONTACT_2A.Name)
	}

	if contact[0].Address != TEST_CONTACT_2A.Address {
		t.Fatalf("got incorrect contact address %s, expected %s", contact[0].Address, TEST_CONTACT_2A.Address)
	}
}

func ModuleDeleteContactHandler(t *testing.T) {
	Spoofer.SetValue("userDid", TEST_USER_2.Id)

	query := "?id=" + strconv.Itoa(TEST_CONTACT_2A.Id)

	req, err := http.NewRequest(http.MethodDelete, TestServer.URL+"/contacts"+query, nil)
	if err != nil {
		t.Fatalf("error creating delete contact request: %s", err)
	}

	res, err := TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending delete contact request: %s", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", res.StatusCode)
	}
}
