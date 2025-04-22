package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/faucet-portal/backend/bot"
	"github.com/faucet-portal/backend/db"
	"github.com/faucet-portal/backend/structs"
)

type BotService struct {
	db  *db.BotDB
	bot bot.IBot
}

func NewBotService(db *db.BotDB, bot bot.IBot) *BotService {
	return &BotService{db, bot}
}

func EnsureLogin(w http.ResponseWriter, r *http.Request) bool {
	adminKey := os.Getenv("ADMIN_KEY")
	header := r.Header[http.CanonicalHeaderKey("X-API-KEY")]
	if len(header) == 0 {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	if header[0] != adminKey {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func EnsureBody(w http.ResponseWriter, r *http.Request) []byte {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return nil
	}
	return body
}

func EnsureUnmarshal(w http.ResponseWriter, obj any, body []byte) bool {
	err := json.Unmarshal(body, obj)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return false
	}
	return true
}

// Create an event with x amount of available codes, y $SFLUV per code, and z expiration date. Responds with event id
func (s *BotService) NewEvent(w http.ResponseWriter, r *http.Request) {
	if !EnsureLogin(w, r) {
		return
	}

	body := EnsureBody(w, r)
	if body == nil {
		return
	}

	var event *structs.Event
	if !EnsureUnmarshal(w, &event, body) {
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

func (s *BotService) NewCodes(w http.ResponseWriter, r *http.Request) {
	if !EnsureLogin(w, r) {
		return
	}

	body := EnsureBody(w, r)
	if body == nil {
		return
	}

	var new_codes *structs.NewCodesRequest
	if !EnsureUnmarshal(w, &new_codes, body) {
		return
	}

	codes, err := s.db.NewCodes(new_codes)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(codes)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
	}
}

// Get event codes by event id x, page y, and amount per page z (up to 100). Responds with array of event codes
func (s *BotService) GetCodes(w http.ResponseWriter, r *http.Request) {
	if !EnsureLogin(w, r) {
		return
	}

	params := r.URL.Query()
	event := params.Get("event")
	count, err := strconv.Atoi(params.Get("count"))
	if err != nil {
		count = 100
	}
	page, err := strconv.Atoi(params.Get("page"))
	if err != nil {
		page = 0
	}

	if event == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	request := structs.CodesPageRequest{
		Event: event,
		Count: uint32(count),
		Page:  uint32(page),
	}

	codes, err := s.db.GetCodes(&request)
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

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

// Verify requesting address event redemption status, Check code redemption status, Send tokens. Responds with 200 OK, 500 tx error, or 400 status
func (s *BotService) Redeem(w http.ResponseWriter, r *http.Request) {

	body := EnsureBody(w, r)
	if body == nil {
		return
	}

	var request *structs.RedeemRequest
	if !EnsureUnmarshal(w, &request, body) {
		return
	}

	amount, tx, err := s.db.Redeem(request.Code, request.Address)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		switch err.Error() {
		case "code expired":
			w.Write([]byte("code expired"))
		case "code redeemed":
			w.Write([]byte("code redeemed"))
		case "user redeemed":
			w.Write([]byte("user redeemed"))
		}

		fmt.Println(err)
		return
	}

	err = s.bot.Send(amount, request.Address)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = tx.Commit()
	if err != nil {
		fmt.Printf("error committing code redemption: %s\n", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
}
