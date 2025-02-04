package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/faucet-portal/backend/bot"
	"github.com/faucet-portal/backend/db"
	"github.com/faucet-portal/backend/structs"
)

type BotService struct {
	db  *db.BotDB
	bot *bot.Bot
}

func NewBotService(db *db.BotDB, bot *bot.Bot) *BotService {
	return &BotService{db, bot}
}

// Create an event with x amount of available codes, y $SFLUV per code, and z expiration date. Responds with event id
func (s *BotService) NewEvent(w http.ResponseWriter, r *http.Request) {
	adminKey := os.Getenv("ADMIN_KEY")
	if r.Header[http.CanonicalHeaderKey("X-API-KEY")][0] != adminKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var event *structs.Event
	err = json.Unmarshal(body, &event)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id, err := s.db.NewEvent(event)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write([]byte(id))
}

// Get event codes by event id x, page y, and amount per page z (up to 100). Responds with array of event codes
func (s *BotService) GetCodes(w http.ResponseWriter, r *http.Request) {
	adminKey := os.Getenv("ADMIN_KEY")
	if r.Header[http.CanonicalHeaderKey("X-API-KEY")][0] != adminKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var request *structs.CodesPageRequest
	err = json.Unmarshal(body, &request)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	codes, err := s.db.GetCodes(request)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	bytes, err := json.Marshal(codes)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(bytes)
}

// Verify requesting address event redemption status, Check code redemption status, Send tokens. Responds with 200 OK, 500 tx error, or 400 status
func (s *BotService) Redeem(w http.ResponseWriter, r *http.Request) {

	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var request *structs.RedeemRequest
	err = json.Unmarshal(body, &request)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	amount, err := s.db.Redeem(request.Code)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = s.bot.Send(amount, request.Address)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
