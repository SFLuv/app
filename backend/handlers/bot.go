package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

type BotService struct {
	db            *db.BotDB
	appDb         *db.AppDB
	bot           bot.IBot
	payouts       *PayoutService
	activeChainID int64
	readRPCURL    string
	// app is a back-reference used for shared concerns that live on AppService
	// (styled email, logging). Set after construction because the two services
	// reference each other.
	app *AppService
}

// SetAppService completes the mutual wiring between the bot and app services.
func (s *BotService) SetAppService(a *AppService) {
	if s != nil {
		s.app = a
	}
}

var redeemCodeUUIDPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)

func NewBotService(db *db.BotDB, appDb *db.AppDB, bot bot.IBot, payouts *PayoutService, activeChainID int64, readRPCURL string) *BotService {
	return &BotService{
		db:            db,
		appDb:         appDb,
		bot:           bot,
		payouts:       payouts,
		activeChainID: activeChainID,
		readRPCURL:    readRPCURL,
	}
}

func (s *BotService) chainID() int64 {
	if s != nil && s.activeChainID > 0 {
		return s.activeChainID
	}
	return 80094
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

func normalizeRedeemCode(raw string) string {
	code := strings.TrimSpace(raw)
	if code == "" {
		return ""
	}

	if decoded, err := url.QueryUnescape(code); err == nil {
		code = decoded
	}

	code = strings.ReplaceAll(code, " ", "")

	if match := redeemCodeUUIDPattern.FindString(code); match != "" {
		return strings.ToLower(match)
	}

	return strings.ToLower(code)
}

// redeemPayoutTarget is one resolution of a scanned address: where the money
// would land, and whose account it belongs to. The two have to come out of the
// same pass. resolveRedeemPayoutAddress rewrites a location till to the owner's
// personal wallet, so anything that re-reads the address afterwards is asking
// about a different address than the one that was scanned and would never see
// the shop behind it.
type redeemPayoutTarget struct {
	address     string
	userID      string
	accountType string
	// ownerUnreadable means the address is owned but the account behind it could
	// not be read, which is not the same answer as "nobody owns this". The
	// merchant bar treats the two differently.
	ownerUnreadable bool
}

func (s *BotService) resolveRedeemPayoutAddress(ctx context.Context, requestedAddress string) redeemPayoutTarget {
	normalizedRequestedAddress := strings.ToLower(strings.TrimSpace(requestedAddress))
	if !common.IsHexAddress(normalizedRequestedAddress) {
		return redeemPayoutTarget{address: normalizedRequestedAddress}
	}
	normalizedRequestedAddress = strings.ToLower(common.HexToAddress(normalizedRequestedAddress).Hex())
	target := redeemPayoutTarget{address: normalizedRequestedAddress}

	// A deployment with no app database has no users table, so it has no
	// merchant accounts to bar either. Not a lookup failure.
	if s.appDb == nil {
		return target
	}

	ownerLookup, err := s.appDb.GetWalletAddressOwnerLookup(ctx, normalizedRequestedAddress)
	if err != nil {
		fmt.Printf("error resolving wallet owner for redeem address %s: %s\n", normalizedRequestedAddress, err)
		target.ownerUnreadable = true
		return target
	}
	if ownerLookup == nil || strings.TrimSpace(ownerLookup.UserID) == "" {
		return target
	}
	target.userID = ownerLookup.UserID

	user, err := s.appDb.GetUserById(ctx, ownerLookup.UserID)
	if err == nil {
		target.accountType = user.AccountType
		primaryWalletAddress := strings.TrimSpace(user.PrimaryWalletAddress)
		if common.IsHexAddress(primaryWalletAddress) {
			target.address = strings.ToLower(common.HexToAddress(primaryWalletAddress).Hex())
			return target
		}
	} else {
		fmt.Printf("error loading user primary wallet for owner %s redeem address %s: %s\n", ownerLookup.UserID, normalizedRequestedAddress, err)
		target.ownerUnreadable = true
	}

	primarySmartWallet, err := s.appDb.GetSmartWalletByOwnerIndex(ctx, ownerLookup.UserID, 0)
	if err != nil {
		fmt.Printf("error loading primary smart wallet for owner %s redeem address %s: %s\n", ownerLookup.UserID, normalizedRequestedAddress, err)
		return target
	}
	if primarySmartWallet == nil || primarySmartWallet.SmartAddress == nil {
		return target
	}

	smartWalletAddress := strings.TrimSpace(*primarySmartWallet.SmartAddress)
	if !common.IsHexAddress(smartWalletAddress) {
		return target
	}

	target.address = strings.ToLower(common.HexToAddress(smartWalletAddress).Hex())
	return target
}

const (
	merchantFaucetBarEnvKey = "MERCHANT_FAUCET_BAR_ENABLED"

	redeemRefusalMerchantAccount = "merchant_account"
	redeemRefusalOwnerUnreadable = "account_lookup_failed"
)

// Merchants take payment, they do not draw from the faucet: a shop owner who
// also wants to volunteer signs up a second, regular account. Behind a flag
// because it decides who gets paid, and a wrong call has to be revocable
// without a deploy.
func merchantFaucetBarEnabled() bool {
	return envBool(merchantFaucetBarEnvKey, true)
}

// merchantFaucetRefusal names why a resolved scan may not be paid from the
// faucet, or returns "" if it may.
//
// It FAILS CLOSED. An owned address whose account could not be read is refused
// rather than paid, because the two mistakes are not symmetric: a wrong refusal
// costs a retry, since the caller runs this before the code is consumed and the
// same QR still works a minute later, while tokens that leave the faucet cannot
// be pulled back.
//
// An address that resolves to nobody is paid, and that is the hole in this.
// POST /redeem is anonymous, so a merchant scanning with a fresh wallet that
// has never been linked to their account is indistinguishable from a first-time
// volunteer. Closing it would mean authenticating redemption, which would take
// away the walk-up scan the events run on.
func merchantFaucetRefusal(target redeemPayoutTarget) string {
	if !merchantFaucetBarEnabled() {
		return ""
	}
	if target.ownerUnreadable {
		return redeemRefusalOwnerUnreadable
	}
	if target.accountType == structs.AccountTypeMerchant {
		return redeemRefusalMerchantAccount
	}
	return ""
}

func validateEventTiming(event *structs.Event) error {
	if event == nil {
		return fmt.Errorf("invalid event payload")
	}
	if event.StartAt == 0 {
		return fmt.Errorf("start_at_required")
	}
	if event.Expiration == 0 {
		return fmt.Errorf("expiration_required")
	}

	now := time.Now().Unix()
	const startAtGraceSeconds int64 = 5
	// Allow small clock/network drift so "start now" submissions are not rejected as elapsed.
	if int64(event.StartAt)+startAtGraceSeconds < now {
		return fmt.Errorf("start_at_elapsed")
	}
	if int64(event.Expiration) < now {
		return fmt.Errorf("expiration_elapsed")
	}
	if event.Expiration <= event.StartAt {
		return fmt.Errorf("expiration_before_start_at")
	}

	return nil
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
	if err := validateEventTiming(event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		switch err.Error() {
		case "start_at_required":
			w.Write([]byte("start_at is required"))
		case "expiration_required":
			w.Write([]byte("expiration is required"))
		case "start_at_elapsed":
			w.Write([]byte("start_at must not be in the past"))
		case "expiration_elapsed":
			w.Write([]byte("expiration must not be in the past"))
		case "expiration_before_start_at":
			w.Write([]byte("expiration must be after start_at"))
		default:
			w.Write([]byte("invalid event timing"))
		}
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

	allocatedBalance, err := s.totalAllocatedBalance(r.Context())
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

	allocatedBalance, err := s.totalAllocatedBalance(r.Context())
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
	page, count := parsePageAndCount(params, 10, 100)
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
	page, count := parsePageAndCount(params, 100, 200)

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

// DeleteEvent removes an event and its unredeemed codes.
//
// There is no refund step any more: standing per-cycle organization balances
// were retired along with self-serve event creation. Faucet capacity is now
// measured directly from outstanding codes, so deleting an event releases its
// committed value simply by removing those codes — nothing to credit back.
func (s *BotService) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	event := r.PathValue("event")
	if event == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.db.DeleteEvent(r.Context(), event); err != nil {
		if errors.Is(err, db.ErrEventHasRedemptions) {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("this event has redemptions and cannot be deleted; cancel it instead so the payout record is kept"))
			return
		}
		fmt.Printf("error deleting event %s: %s\n", event, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// requireOrg resolves the caller's organization id. Affiliate event access is
// organization-scoped: callers without an organization are rejected.
func (s *BotService) requireOrg(ctx context.Context, userDid string) (int64, error) {
	if s.appDb == nil {
		return 0, fmt.Errorf("app database unavailable")
	}
	org, _, err := s.appDb.GetOrganizationByUser(ctx, userDid)
	if err != nil {
		return 0, err
	}
	if org == nil {
		return 0, pgx.ErrNoRows
	}
	return org.Id, nil
}

// AdminGetOrganizationEvents lists a specific organization's events for the
// admin panel (org details modal), reusing the same org-scoped query the
// affiliate event list uses.
func (s *BotService) AdminGetOrganizationEvents(w http.ResponseWriter, r *http.Request) {
	orgId, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || orgId <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	params := r.URL.Query()
	page, count := parsePageAndCount(params, 10, 100)

	events, err := s.db.GetEventsByOrganization(r.Context(), &structs.EventsRequest{
		Page:    page,
		Count:   count,
		Search:  params.Get("search"),
		Expired: params.Get("expired") == "true",
	}, orgId)
	if err != nil {
		fmt.Printf("error getting organization events for org %d: %s\n", orgId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	bytes, err := json.Marshal(events)
	if err != nil {
		fmt.Printf("error marshalling organization events: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
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
	if request == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	request.Code = normalizeRedeemCode(request.Code)
	request.Address = strings.ToLower(strings.TrimSpace(request.Address))
	if request.Code == "" || !common.IsHexAddress(request.Address) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	request.Address = strings.ToLower(common.HexToAddress(request.Address).Hex())

	resolveAddressCtx, resolveAddressCancel := context.WithTimeout(context.Background(), 5*time.Second)
	payoutTarget := s.resolveRedeemPayoutAddress(resolveAddressCtx, request.Address)
	resolveAddressCancel()
	request.Address = payoutTarget.address

	// Before db.Redeem, and it has to stay there. A refusal raised after the code
	// is consumed would depend on UndoRedeem to hand it back, and getting that
	// wrong either burns a code nobody was paid for or lets one be claimed twice.
	// Refusing first means the scan is simply never counted.
	if refusal := merchantFaucetRefusal(payoutTarget); refusal != "" {
		fmt.Printf("refusing redemption of code %s for address %s (owner %q): %s\n", request.Code, request.Address, payoutTarget.userID, refusal)

		status := http.StatusConflict
		message := "This is a merchant account. Volunteer rewards go to a personal SFLuv account — sign in with one and scan again."
		if refusal == redeemRefusalOwnerUnreadable {
			// Not the scanner's fault and not permanent, so it is answered as a
			// server problem rather than as a rule they broke.
			status = http.StatusServiceUnavailable
			message = "We couldn't check this account just now. The code has not been used — try scanning it again in a moment."
		}

		writeJSON(w, status, structs.RedeemRefusedResponse{
			Status:  "blocked",
			Reason:  refusal,
			Message: message,
		})
		return
	}

	// The tax check no longer happens here. A volunteer who has earned past the
	// reporting threshold used to be refused at this point, with the code left
	// unredeemed and nothing to show for the shift they had just worked. Now the
	// scan always succeeds: the code is consumed, and PayoutService decides
	// whether the money goes out or is held pending a W-9.
	redeemCtx, redeemCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer redeemCancel()

	amount, err := s.db.Redeem(redeemCtx, request.Code, request.Address, s.chainID())
	if err != nil {
		switch err.Error() {
		case "code not started":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("code not started"))
		case "code expired":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("code expired"))
		case "code redeemed":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("code redeemed"))
		case "user redeemed":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("user redeemed"))
		default:
			fmt.Printf("error reserving redemption for code %s address %s: %s\n", request.Code, request.Address, err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	multiplier, multiplierErr := getTokenMultiplier()
	if multiplierErr != nil {
		fmt.Printf("error reading token decimals for code %s: %s\n", request.Code, multiplierErr)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	amountBase := new(big.Int).Mul(multiplier, new(big.Int).SetUint64(amount))

	payoutCtx, payoutCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer payoutCancel()

	result, payoutErr := s.payouts.Pay(payoutCtx, PayoutRequest{
		IdempotencyKey:   "redeem:" + request.Code + ":" + request.Address,
		RecipientAddress: request.Address,
		AmountBase:       amountBase,
		Source:           db.PayoutSourceRedemptionCode,
		SourceRef:        request.Code,
	})
	if payoutErr != nil {
		fmt.Printf("error sending redeem payout for code %s address %s: %s\n", request.Code, request.Address, payoutErr)
		// Only a genuine send failure releases the code. Escrow is a success —
		// undoing the redemption there would let the same reward be claimed
		// twice, once now and once after the W-9 lands.
		if bot.ShouldRevertRedemption(payoutErr) {
			undoCtx, undoCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if undoErr := s.db.UndoRedeem(undoCtx, request.Code, request.Address, s.chainID()); undoErr != nil {
				fmt.Printf("error undoing redemption for code %s address %s after payout failure: %s\n", request.Code, request.Address, undoErr)
			}
			undoCancel()
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Refused. The code is handed back so the same QR still works once the form
	// is in — nothing is owed, nothing is queued, and the volunteer keeps the
	// only thing they need to claim it. Reuses the same undo the send-failure
	// path uses rather than inventing a second refund route.
	if result != nil && result.Blocked {
		undoCtx, undoCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if undoErr := s.db.UndoRedeem(undoCtx, request.Code, request.Address, s.chainID()); undoErr != nil {
			fmt.Printf("error releasing code %s after a blocked payout: %s\n", request.Code, undoErr)
		}
		undoCancel()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(structs.RedeemEscrowedResponse{
			Status:      "blocked",
			Reason:      "w9_required",
			AmountSfluv: formatSfluvBase(amountBase),
			TaxYear:     result.TaxYear,
			Message:     "We couldn't send this reward yet. Complete your W-9 in the SFLuv app, then scan this code again.",
		})
		return
	}

	// Held money is reported as its own outcome rather than as a plain success,
	// so the app can explain what happened instead of showing a reward that
	// never arrives.
	if result != nil && result.Escrowed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(structs.RedeemEscrowedResponse{
			Status:      "escrowed",
			Reason:      "w9_required",
			AmountSfluv: formatSfluvBase(amountBase),
			TaxYear:     result.TaxYear,
			Message:     "Reward saved. Complete your W-9 in the SFLuv app and we'll send it over.",
		})
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
