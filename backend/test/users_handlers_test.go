package test

import (
	"io"
	"net/http"
	"testing"
)

func GroupUsersHandlers(t *testing.T) {
	t.Run("add user handler", ModuleAddUserHandler)
	t.Run("get user authed handler", ModuleGetUserAuthedHandler)
	t.Run("update user info handler", ModuleUpdateUserInfoHandler)
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

	t.Log(string(body))
}

func ModuleUpdateUserInfoHandler(t *testing.T) {

}
