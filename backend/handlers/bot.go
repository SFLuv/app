package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/faucet-portal/backend/bot"
	"github.com/faucet-portal/backend/db"
	"github.com/faucet-portal/backend/structs"
	"github.com/faucet-portal/backend/utils"
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
	var admin *structs.Admin
	var err error
	requestKey := r.Header[http.CanonicalHeaderKey("X-API-KEY")][0]
	adminKey := os.Getenv("ADMIN_KEY")
	fmt.Println(requestKey, adminKey)
	if requestKey == adminKey {
		admin = &structs.Admin{}
		admin.Limit = 0
		admin.Key = requestKey
	} else {
		admin, err = s.db.GetAdmin(requestKey)
		if err != nil {
			fmt.Printf("error getting admin for key %s: %s\n", adminKey, err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
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

	currentTime := time.Now()
	lastRefreshTime := time.Unix(int64(admin.LastRefresh), 0)
	refreshTime := lastRefreshTime.Add(time.Hour * time.Duration(admin.Refresh*24))
	if currentTime.After(refreshTime) {
		admin.Balance = 0
		admin.LastRefresh = int(utils.GetMostRecentReset(&lastRefreshTime, (admin.Refresh)).Unix())
		fmt.Println(admin.LastRefresh)
	}

	tx, err := s.db.Begin()
	defer tx.Rollback()
	if err != nil {
		fmt.Printf("error beginning new event tx: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	fmt.Println(admin)

	totalCost := event.Amount * uint64(event.Codes)
	totalAvailable := uint(admin.Limit) - uint(admin.Balance)
	if totalCost > uint64(totalAvailable) && admin.Limit != 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("not enough balance"))
		return
	}
	admin.Balance += int(totalCost)

	if admin.Key != adminKey {
		tx, err = s.db.UpdateAdmin(tx, admin)
		if err != nil {
			fmt.Printf("error updating admin %s: %s\n", admin.Key, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		newExpiration := uint64(time.Unix(int64(admin.LastRefresh), 0).Add(time.Hour * time.Duration(admin.Refresh*24)).Unix())
		if (event.Expiration > newExpiration) || (event.Expiration == 0) {
			event.Expiration = newExpiration
		}
	}

	event.Creator = admin.Key

	tx, id, err := s.db.NewEvent(tx, event)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	tx.Commit()
	w.WriteHeader(201)
	w.Write([]byte(id))
}

// Get event codes by event id x, page y, and amount per page z (up to 100). Responds with array of event codes
func (s *BotService) GetCodes(w http.ResponseWriter, r *http.Request) {
	adminKey := os.Getenv("ADMIN_KEY")
	if r.Header[http.CanonicalHeaderKey("X-API-KEY")][0] != adminKey {
		w.WriteHeader(http.StatusUnauthorized)
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
		tx.Rollback()
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = tx.Commit()
	if err != nil {
		fmt.Printf("error committing code redemption: %s\n", err)
		tx.Rollback()
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *BotService) NewAdmin(w http.ResponseWriter, r *http.Request) {
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

	var admin *structs.Admin
	err = json.Unmarshal(body, &admin)
	if err != nil {
		fmt.Println("error unmarshalling admin: ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = s.db.NewAdmin(admin.Key, admin.Name, admin.Limit, admin.Refresh, admin.LastRefresh)
	if err != nil {
		fmt.Println("error creating new admin: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(201)
	return
}
