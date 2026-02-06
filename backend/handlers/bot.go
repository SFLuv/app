package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/common"
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
	body := EnsureBody(w, r)
	if body == nil {
		return
	}

	var event *structs.Event
	if !EnsureUnmarshal(w, &event, body) {
		return
	}

	eventTotal := big.NewInt(int64(event.Amount) * int64(event.Codes))
	decimals, err := strconv.Atoi(os.Getenv("TOKEN_DECIMALS"))
	if err != nil {
		fmt.Println("invalid token decimals in .env")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	eventTotal.Mul(eventTotal, big.NewInt(int64(decimals)))

	balance, err := s.bot.Balance()
	if err != nil {
		fmt.Printf("error getting current bot balance: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	allocatedBalance, err := s.db.AllocatedBalance(r.Context())
	if err != nil {
		fmt.Printf("error getting allocated balance for faucet: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	bigAllocated := big.NewInt(int64(allocatedBalance))
	bigAllocated.Mul(bigAllocated, big.NewInt(int64(decimals)))

	unallocated := bigAllocated.Sub(balance, bigAllocated)

	if eventTotal.Cmp(unallocated) > 0 {
		fmt.Println("total event rewards should not exceed unallocated balance")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("insufficient balance"))
		return
	}

	id, err := s.db.NewEvent(r.Context(), event)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write([]byte(id))
}

func (s *BotService) RemainingBalance(w http.ResponseWriter, r *http.Request) {
	decimals, err := strconv.Atoi(os.Getenv("TOKEN_DECIMALS"))
	if err != nil {
		fmt.Println("invalid token decimals in .env")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	balance, err := s.bot.Balance()
	if err != nil {
		fmt.Printf("error getting current bot balance: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	allocatedBalance, err := s.db.AllocatedBalance(r.Context())
	if err != nil {
		fmt.Printf("error getting allocated balance for faucet: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	bigAllocated := big.NewInt(int64(allocatedBalance))
	bigAllocated.Mul(bigAllocated, big.NewInt(int64(decimals)))

	unallocated := bigAllocated.Sub(balance, bigAllocated)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(unallocated.String()))
}

func (s *BotService) NewCodesRequest(w http.ResponseWriter, r *http.Request) {
	body := EnsureBody(w, r)
	if body == nil {
		return
	}

	var new_codes *structs.NewCodesRequest
	if !EnsureUnmarshal(w, &new_codes, body) {
		return
	}

	new_codes.Event = r.PathValue("event_id")
	if new_codes.Event == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	codes, err := s.db.NewCodes(r.Context(), new_codes)
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

func (s *BotService) GetEvents(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	count, err := strconv.Atoi(params.Get("count"))
	if err != nil {
		count = 10
	}
	page, err := strconv.Atoi(params.Get("page"))
	if err != nil {
		page = 0
	}
	search := params.Get("search")
	expired := params.Get("expired") == "true"

	events, err := s.db.GetEvents(r.Context(), &structs.EventsRequest{
		Page:    page,
		Count:   count,
		Search:  search,
		Expired: expired,
	})
	if err != nil {
		fmt.Printf("error getting events: page %d, count %d, search %s, expired %t\n: %s", page, count, search, expired, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	bytes, err := json.Marshal(events)
	if err != nil {
		fmt.Printf("error marshalling events bytes: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

// Get event codes by event id x, page y, and amount per page z (up to 100). Responds with array of event codes
func (s *BotService) GetCodesRequest(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	event := r.PathValue("event")
	if event == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
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

	codes, err := s.GetCodes(event, count, page)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if len(codes) == 0 {
		w.WriteHeader(http.StatusNotFound)
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

func (s *BotService) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	event := r.PathValue("event")
	if event == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := s.db.DeleteEvent(r.Context(), event)
	if err != nil {
		fmt.Printf("error deleting event %s: %s\n", event, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *BotService) GetCodes(event string, count, page int) ([]*structs.Code, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	request := structs.CodesPageRequest{
		Event: event,
		Count: uint32(count),
		Page:  uint32(page),
	}

	codes, err := s.db.GetCodes(ctx, &request)
	if err != nil {
		return nil, err
	}

	return codes, nil
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

	amount, tx, err := s.db.Redeem(r.Context(), request.Code, request.Address)
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
		fmt.Println("this is the bot error:")
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = tx.Commit(context.Background())
	if err != nil {
		fmt.Printf("error committing code redemption: %s\n", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *BotService) Drain(w http.ResponseWriter, r *http.Request) {
	a := os.Getenv("ADMIN_ADDRESS")
	if a == "" || a == "x" || a == "0x" {
		fmt.Println("WARNING: be sure to specify an admin address in .env")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	adminAddress := common.HexToAddress(a)
	err := s.bot.Drain(adminAddress)
	if err != nil {
		fmt.Printf("error draining faucet: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
