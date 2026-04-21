package test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func GroupUsersHandlers(t *testing.T) {
	t.Run("add user handler", ModuleAddUserHandler)
	t.Run("update user info handler", ModuleUpdateUserInfoHandler)
	t.Run("get user authed handler", ModuleGetUserAuthedHandler)
	t.Run("account deletion handlers", ModuleAccountDeletionHandlers)
}

func ModuleAddUserHandler(t *testing.T) {
	Spoofer.SetValue("userDid", TEST_USER_2.Id)

	req, err := http.NewRequest(http.MethodPost, TestServer.URL+"/users", nil)
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

	Spoofer.SetValue("userDid", TEST_USER_1.Id)

	req, err = http.NewRequest(http.MethodPost, TestServer.URL+"/users", nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}

	res, err = TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending request: %s", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", res.StatusCode)
	}
}

func ModuleUpdateUserInfoHandler(t *testing.T) {
	Spoofer.SetValue("userDid", TEST_USER_2.Id)

	body, err := json.Marshal(TEST_USER_2)
	if err != nil {
		t.Fatalf("error marshalling user for request body: %s", err)
	}

	req, err := http.NewRequest(http.MethodPut, TestServer.URL+"/users", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("error creating update user request: %s", err)
	}

	res, err := TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending update user request: %s", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", res.StatusCode)
	}
}

func ModuleGetUserAuthedHandler(t *testing.T) {
	Spoofer.SetValue("userDid", TEST_USER_2.Id)

	req, err := http.NewRequest(http.MethodGet, TestServer.URL+"/users", nil)
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

	var user structs.AuthedUserResponse
	err = json.Unmarshal(body, &user)
	if err != nil {
		t.Fatalf("error unmarshalling get user body")
	}

	if user.User.Id != TEST_USER_2.Id {
		t.Fatalf("got incorrect user id %s, expected %s", user.User.Id, TEST_USER_2.Id)
	}

	if user.User.Email == nil {
		t.Fatal("expected to get user email")
	}

	if *user.User.Email != *TEST_USER_2.Email {
		t.Fatalf("got incorrect user email %s, expected %s", *user.User.Email, *TEST_USER_2.Email)
	}
}

func ModuleAccountDeletionHandlers(t *testing.T) {
	userID := "test-delete-handler"
	Spoofer.SetValue("userDid", userID)

	req, err := http.NewRequest(http.MethodPost, TestServer.URL+"/users", nil)
	if err != nil {
		t.Fatalf("error creating add-user request: %s", err)
	}
	res, err := TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending add-user request: %s", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected add-user 201, got %d", res.StatusCode)
	}

	req, err = http.NewRequest(http.MethodGet, TestServer.URL+"/users/delete-account/preview", nil)
	if err != nil {
		t.Fatalf("error creating preview request: %s", err)
	}
	res, err = TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending preview request: %s", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected preview 200, got %d", res.StatusCode)
	}

	req, err = http.NewRequest(http.MethodPost, TestServer.URL+"/users/delete-account", nil)
	if err != nil {
		t.Fatalf("error creating delete-account request: %s", err)
	}
	res, err = TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending delete-account request: %s", err)
	}
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("expected delete-account 202, got %d", res.StatusCode)
	}

	req, err = http.NewRequest(http.MethodGet, TestServer.URL+"/users", nil)
	if err != nil {
		t.Fatalf("error creating inactive user request: %s", err)
	}
	res, err = TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending inactive user request: %s", err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected inactive user lookup 403, got %d", res.StatusCode)
	}

	req, err = http.NewRequest(http.MethodGet, TestServer.URL+"/users/delete-account/status", nil)
	if err != nil {
		t.Fatalf("error creating delete-account status request: %s", err)
	}
	res, err = TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending delete-account status request: %s", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected delete-account status 200, got %d", res.StatusCode)
	}

	req, err = http.NewRequest(http.MethodPost, TestServer.URL+"/users/delete-account/cancel", nil)
	if err != nil {
		t.Fatalf("error creating cancel delete-account request: %s", err)
	}
	res, err = TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending cancel delete-account request: %s", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected cancel delete-account 200, got %d", res.StatusCode)
	}

	req, err = http.NewRequest(http.MethodGet, TestServer.URL+"/users", nil)
	if err != nil {
		t.Fatalf("error creating restored user request: %s", err)
	}
	res, err = TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending restored user request: %s", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected restored user lookup 200, got %d", res.StatusCode)
	}
}
