package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func GroupW9Handlers(t *testing.T) {
	t.Run("submit w9 requires auth", ModuleSubmitW9RequiresAuth)
	t.Run("webhook requires secret", ModuleSubmitW9WebhookRequiresSecret)
	t.Run("webhook rejects when secret missing", ModuleSubmitW9WebhookRequiresConfiguredSecret)
}

func ModuleSubmitW9RequiresAuth(t *testing.T) {
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

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthenticated w9 submit, got %d", res.StatusCode)
	}
}

func ModuleSubmitW9WebhookRequiresSecret(t *testing.T) {
	t.Setenv("W9_WEBHOOK_SECRET", "test-webhook-secret")

	reqBody := structs.W9SubmitRequest{
		WalletAddress: "0x2222222222222222222222222222222222222222",
		Email:         "webhook-secret@test.com",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("error marshalling webhook body: %s", err)
	}

	req, err := http.NewRequest(http.MethodPost, TestServer.URL+"/w9/webhook", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("error creating webhook request: %s", err)
	}

	res, err := TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending webhook request: %s", err)
	}

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for webhook without secret, got %d", res.StatusCode)
	}
}

func ModuleSubmitW9WebhookRequiresConfiguredSecret(t *testing.T) {
	t.Setenv("W9_WEBHOOK_SECRET", "")

	reqBody := structs.W9SubmitRequest{
		WalletAddress: "0x3333333333333333333333333333333333333333",
		Email:         "webhook-config@test.com",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("error marshalling webhook body: %s", err)
	}

	req, err := http.NewRequest(http.MethodPost, TestServer.URL+"/w9/webhook", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("error creating webhook request: %s", err)
	}
	req.Header.Set("X-W9-Secret", "unused")

	res, err := TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending webhook request: %s", err)
	}

	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when webhook secret is not configured, got %d", res.StatusCode)
	}
}
