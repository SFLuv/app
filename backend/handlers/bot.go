package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

type BotService struct {
	db                 *db.BotDB
	appDb              *db.AppDB
	bot                bot.IBot
	affiliateScheduler *AffiliateScheduler
}

func NewBotService(db *db.BotDB, appDb *db.AppDB, bot bot.IBot, affiliateScheduler *AffiliateScheduler) *BotService {
	return &BotService{
		db:                 db,
		appDb:              appDb,
		bot:                bot,
		affiliateScheduler: affiliateScheduler,
	}
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

	if userDid := utils.GetDid(r); userDid != nil {
		event.Owner = *userDid
	}
	if event.Owner == "" && s.appDb != nil {
		if adminId, err := s.appDb.GetFirstAdminId(r.Context()); err == nil && adminId != "" {
			event.Owner = adminId
		}
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

	owner := ""
	if s.appDb != nil {
		if eventOwner, err := s.db.GetEventOwner(r.Context(), event); err == nil {
			owner = eventOwner
		}
	}

	if owner != "" && s.appDb != nil {
		freed, err := s.db.EventUnredeemedValue(r.Context(), event)
		if err == nil && freed > 0 {
			if err := s.appDb.AddAffiliateWeeklyBalance(r.Context(), owner, freed); err != nil {
				fmt.Printf("error refunding affiliate balance for event %s: %s\n", event, err)
			}
		}
	}

	err := s.db.DeleteEvent(r.Context(), event)
	if err != nil {
		fmt.Printf("error deleting event %s: %s\n", event, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *BotService) AffiliateNewEvent(w http.ResponseWriter, r *http.Request) {
	body := EnsureBody(w, r)
	if body == nil {
		return
	}

	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var event *structs.Event
	if !EnsureUnmarshal(w, &event, body) {
		return
	}
	event.Owner = *userDid

	eventTotal := uint64(event.Amount) * uint64(event.Codes)
	reservation, err := s.appDb.ReserveAffiliateBalance(r.Context(), *userDid, eventTotal)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if err.Error() == "insufficient affiliate balance" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("insufficient affiliate balance"))
			return
		}
		fmt.Printf("error reserving affiliate balance: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	decimals, err := strconv.Atoi(os.Getenv("TOKEN_DECIMALS"))
	if err != nil {
		fmt.Println("invalid token decimals in .env")
		_ = s.appDb.RefundAffiliateBalance(r.Context(), *userDid, reservation.WeeklyDeducted, reservation.OneTimeDeducted)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	eventTotalBig := new(big.Int).SetUint64(eventTotal)
	eventTotalBig.Mul(eventTotalBig, big.NewInt(int64(decimals)))

	faucetBalance, err := s.bot.Balance()
	if err != nil {
		fmt.Printf("error getting current bot balance: %s\n", err)
		_ = s.appDb.RefundAffiliateBalance(r.Context(), *userDid, reservation.WeeklyDeducted, reservation.OneTimeDeducted)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	allocatedBalance, err := s.db.AllocatedBalance(r.Context())
	if err != nil {
		fmt.Printf("error getting allocated balance for faucet: %s", err)
		_ = s.appDb.RefundAffiliateBalance(r.Context(), *userDid, reservation.WeeklyDeducted, reservation.OneTimeDeducted)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	bigAllocated := big.NewInt(int64(allocatedBalance))
	bigAllocated.Mul(bigAllocated, big.NewInt(int64(decimals)))

	unallocated := bigAllocated.Sub(faucetBalance, bigAllocated)

	if eventTotalBig.Cmp(unallocated) > 0 {
		fmt.Println("total event rewards should not exceed unallocated balance")
		adminEmail := os.Getenv("AFFILIATE_ADMIN_EMAIL")
		emailSender := utils.NewEmailSender()
		if adminEmail != "" && emailSender != nil {
			fromDomain := os.Getenv("MAILGUN_DOMAIN")
			fromEmail := "no_reply@sfluv.org"
			if fromDomain != "" {
				fromEmail = "no_reply@" + fromDomain
			}

			availableTokens := new(big.Int).Div(unallocated, big.NewInt(int64(decimals)))
			subject := "Failed Affiliate Event Creation (Faucet Balance)"
			details := fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:180px;">Affiliate</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Required Balance</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%d SFLuv</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Available Faucet Balance</td>
    <td style="padding:12px 0; font-size:13px; color:#111827;">%s SFLuv</td>
  </tr>
</table>`, *userDid, eventTotal, availableTokens.String())

			htmlContent := utils.BuildStyledEmail(
				"Failed Affiliate Event Creation",
				"Affiliate event creation failed due to faucet balance.",
				details,
			)

			err = emailSender.SendEmail(adminEmail, "Admin", subject, htmlContent, fromEmail, "SFLuv Affiliates")
			if err != nil {
				fmt.Printf("error sending affiliate faucet balance email: %s\n", err)
			}
		}
		_ = s.appDb.RefundAffiliateBalance(r.Context(), *userDid, reservation.WeeklyDeducted, reservation.OneTimeDeducted)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Not enough balance in faucet. Please try again later, or contact us at admin@sfluv.org."))
		return
	}

	id, err := s.db.NewEvent(r.Context(), event)
	if err != nil {
		fmt.Println(err)
		_ = s.appDb.RefundAffiliateBalance(r.Context(), *userDid, reservation.WeeklyDeducted, reservation.OneTimeDeducted)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if s.affiliateScheduler != nil {
		s.affiliateScheduler.ScheduleEventExpiration(id, *userDid, event.Expiration)
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(id))
}

func (s *BotService) AffiliateGetEvents(w http.ResponseWriter, r *http.Request) {
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

	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	events, err := s.db.GetEventsByOwner(r.Context(), &structs.EventsRequest{
		Page:    page,
		Count:   count,
		Search:  search,
		Expired: expired,
	}, *userDid)
	if err != nil {
		fmt.Printf("error getting affiliate events: page %d, count %d, search %s, expired %t\n: %s", page, count, search, expired, err)
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

func (s *BotService) AffiliateGetCodes(w http.ResponseWriter, r *http.Request) {
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

	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	owner, err := s.db.GetEventOwner(r.Context(), event)
	if err != nil || owner == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if owner != *userDid {
		w.WriteHeader(http.StatusForbidden)
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

func (s *BotService) AffiliateDeleteEvent(w http.ResponseWriter, r *http.Request) {
	event := r.PathValue("event")
	if event == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	owner, err := s.db.GetEventOwner(r.Context(), event)
	if err != nil || owner == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if owner != *userDid {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	freed, err := s.db.EventUnredeemedValue(r.Context(), event)
	if err == nil && freed > 0 {
		fmt.Printf("freed affiliate balance %d for event %s\n", freed, event)
		if s.appDb != nil {
			if err := s.appDb.AddAffiliateWeeklyBalance(r.Context(), owner, freed); err != nil {
				fmt.Printf("error refunding affiliate balance for event %s: %s\n", event, err)
			}
		}
	}

	err = s.db.DeleteEvent(r.Context(), event)
	if err != nil {
		fmt.Printf("error deleting event %s: %s\n", event, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *BotService) AffiliateBalance(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	balance, err := s.getAffiliateBalance(r.Context(), *userDid)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Printf("error getting affiliate balance: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	bytes, err := json.Marshal(balance)
	if err != nil {
		fmt.Printf("error marshalling affiliate balance: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

func (s *BotService) getAffiliateBalance(ctx context.Context, owner string) (*structs.AffiliateBalance, error) {
	if s.appDb == nil {
		return nil, fmt.Errorf("affiliate database unavailable")
	}

	affiliate, err := s.appDb.GetAffiliateByUser(ctx, owner)
	if err != nil {
		return nil, err
	}

	reserved, err := s.db.AllocatedBalanceByOwner(ctx, owner)
	if err != nil {
		return nil, err
	}

	return &structs.AffiliateBalance{
		Available:        affiliate.WeeklyBalance + affiliate.OneTimeBalance,
		WeeklyAllocation: affiliate.WeeklyAllocation,
		WeeklyBalance:    affiliate.WeeklyBalance,
		OneTimeBalance:   affiliate.OneTimeBalance,
		Reserved:         reserved,
	}, nil
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
