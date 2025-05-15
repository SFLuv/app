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
	"github.com/faucet-portal/backend/router"
	"github.com/faucet-portal/backend/structs"
	"github.com/go-chi/chi/v5"
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
	bot_service.GetCodesRequest(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status code 401 got %v", res.StatusCode)
	}
}

func TestBotServiceNewCodes(t *testing.T) {
	t.Setenv("DB_FOLDER_PATH", "./test_data")
	t.Setenv("ADMIN_KEY", "0123456789")

	r := chi.NewRouter()

	bot_service := MakeBotService()

	router.AddBotRoutes(r, bot_service)

	ts := httptest.NewServer(r)
	defer ts.Close()

	post_body := map[string]interface{}{
		"Count": 10,
	}
	body, _ := json.Marshal(post_body)
	req := httptest.NewRequest(http.MethodPost, "/events/abcdef/codes", bytes.NewReader(body))
	req.Header.Set("X-API-KEY", "0123456789")
	w := httptest.NewRecorder()
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
		return
	}
	defer resp.Body.Close()
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
	// Check if the codes are in the database
	codes, err := bot_service.GetCodes("abcdef", 10, 0)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}
	assert.Equal(t, len(codes), 10)
	for _, code := range codes {
		assert.Equal(t, code.Redeemed, false)
		assert.Equal(t, code.Event, "abcdef")
	}
}
