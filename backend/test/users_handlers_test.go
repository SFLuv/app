package test

import (
	"context"
	"net/http"
	"testing"
)

func GroupUsersHandlers(t *testing.T) {
	t.Run("add user handler", ModuleAddUserHandler)
	t.Run("get user authed handler", ModuleGetUserAuthedHandler)
	t.Run("update user info handler", ModuleUpdateUserInfoHandler)
}

func ModuleAddUserHandler(t *testing.T) {
	ctx := context.WithValue(context.Background(), "userDid", TEST_USER_1.Id)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TestServer.URL+"/users", nil)
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

}

func ModuleUpdateUserInfoHandler(t *testing.T) {

}
