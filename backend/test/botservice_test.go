package main

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"net/http"
	"net/http/httptest"

	"github.com/faucet-portal/backend/db"
	"github.com/faucet-portal/backend/handlers"
	"github.com/faucet-portal/backend/structs"
	"github.com/stretchr/testify/assert"
)

func MakeBotService() *handlers.BotService {
	bdb := db.InitDB("bot")
	botDb := db.Bot(bdb)
	botDb.CreateTables()
	return handlers.NewBotService(botDb, nil)
}

func TestBotServiceLogin(t *testing.T) {
	t.Setenv("ADMIN_KEY", "0123456789")

	bot_service := MakeBotService()
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("X-API-KEY", "abcdefghijklmnop")
	w := httptest.NewRecorder()
	bot_service.GetCodes(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status code 401 got %v", res.StatusCode)
	}
}

func TestBotServiceNewCodes(t *testing.T) {
	t.Setenv("DB_FOLDER_PATH", "./test_data")
	t.Setenv("ADMIN_KEY", "0123456789")

	bot_service := MakeBotService()
	post_body := map[string]interface{}{
		"Event": "EVENT1",
		"Count": 10,
	}
	body, _ := json.Marshal(post_body)
	req := httptest.NewRequest(http.MethodPost, "/events/abcdef/codes", bytes.NewReader(body))
	req.Header.Set("X-API-KEY", "0123456789")
	w := httptest.NewRecorder()
	bot_service.NewCodes(w, req)
	res := w.Result()
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status code 200 got %v", res.StatusCode)
	}
	var response []structs.Code
	err = json.Unmarshal(data, &response)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}
	assert.Equal(t, len(response), 10)
}
