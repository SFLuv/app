package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func GroupW9Handlers(t *testing.T) {
	t.Run("submit w9 is public", ModuleSubmitW9IsPublic)
}

func ModuleSubmitW9IsPublic(t *testing.T) {
	Spoofer.SetValue("userDid", nil)
	defer Spoofer.SetValue("userDid", TEST_USER_1.Id)

	reqBody := structs.W9SubmitRequest{
		WalletAddress: "0x1111111111111111111111111111111111111111",
		Email:         "submit-auth@test.com",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("error marshalling w9 submit body: %s", err)
	}

	req, err := http.NewRequest(http.MethodPost, TestServer.URL+"/w9/submit", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("error creating w9 submit request: %s", err)
	}

	res, err := TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending w9 submit request: %s", err)
	}

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for public w9 submit, got %d", res.StatusCode)
	}
}
