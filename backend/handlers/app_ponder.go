package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

const (
	defaultExpoPushAPIURL        = "https://exp.host/--/api/v2/push/send"
	defaultExpoPushReceiptAPIURL = "https://exp.host/--/api/v2/push/getReceipts"
	expoDeviceNotRegistered      = "DeviceNotRegistered"
	pushDeleteReasonDeadToken    = "push_device_not_registered"
)

type expoPushTicket struct {
	Status  string         `json:"status"`
	ID      string         `json:"id,omitempty"`
	Message string         `json:"message,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

type expoPushSendResponse struct {
	Data   json.RawMessage  `json:"data"`
	Errors []expoPushTicket `json:"errors,omitempty"`
}

type expoPushReceipt struct {
	Status  string         `json:"status"`
	Message string         `json:"message,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

type expoPushReceiptResponse struct {
	Data   map[string]expoPushReceipt `json:"data"`
	Errors []expoPushTicket           `json:"errors,omitempty"`
}

func shortenPonderAddress(address string) string {
	address = strings.TrimSpace(address)
	if len(address) <= 12 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}

func ponderPushAccountLabel(wallet *structs.Wallet, address string) string {
	if wallet != nil {
		name := strings.TrimSpace(wallet.Name)
		if name != "" {
			return name
		}
	}
	return shortenPonderAddress(address)
}

func allowedWalletAddresses(wallets []*structs.Wallet) map[string]bool {
	userWallets := make(map[string]bool, len(wallets))
	for _, wallet := range wallets {
		if !wallet.IsEoa && wallet.SmartAddress != nil {
			userWallets[strings.ToLower(strings.TrimSpace(*wallet.SmartAddress))] = true
			continue
		}
		userWallets[strings.ToLower(strings.TrimSpace(wallet.EoaAddress))] = true
	}
	return userWallets
}

func ponderServerBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("PONDER_SERVER_BASE_URL")), "/")
}

func (a *AppService) createPonderHook(ctx context.Context, address string) (*structs.PonderSubscriptionServerRequest, error) {
	ponderUrl := ponderServerBaseURL()
	if ponderUrl == "" {
		return nil, fmt.Errorf("PONDER_SERVER_BASE_URL is required")
	}

	subscriptionBody := structs.PonderSubscriptionServerRequest{
		Id:      0,
		Address: address,
		Url:     os.Getenv("PONDER_CALLBACK_URL"),
	}

	reqBody, err := json.Marshal(subscriptionBody)
	if err != nil {
		return nil, fmt.Errorf("error marshalling ponder subscription body: %w", err)
	}

	subscriptionReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/hooks", ponderUrl), bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error creating ponder subscription request: %w", err)
	}
	subscriptionReq.Header.Add("X-Admin-Key", os.Getenv("PONDER_KEY"))

	res, err := http.DefaultClient.Do(subscriptionReq)
	if err != nil {
		return nil, fmt.Errorf("error sending ponder subscription request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		resBody, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("ponder subscription not created: status %d: %s", res.StatusCode, strings.TrimSpace(string(resBody)))
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading ponder subscription response body: %w", err)
	}

	var newSubscription structs.PonderSubscriptionServerRequest
	if err := json.Unmarshal(resBody, &newSubscription); err != nil {
		return nil, fmt.Errorf("error unmarshalling ponder subscription response: %w", err)
	}

	return &newSubscription, nil
}

func (a *AppService) deletePonderHook(ctx context.Context, hookID int) error {
	ponderUrl := ponderServerBaseURL()
	if ponderUrl == "" {
		return fmt.Errorf("PONDER_SERVER_BASE_URL is required")
	}

	subscriptionReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/hooks?id=%d", ponderUrl, hookID), nil)
	if err != nil {
		return fmt.Errorf("error creating ponder delete subscription request: %w", err)
	}
	subscriptionReq.Header.Add("X-Admin-Key", os.Getenv("PONDER_KEY"))

	res, err := http.DefaultClient.Do(subscriptionReq)
	if err != nil {
		return fmt.Errorf("error sending ponder delete subscription request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		resBody, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ponder subscription not deleted: status %d: %s", res.StatusCode, strings.TrimSpace(string(resBody)))
	}

	return nil
}

func (a *AppService) deletePonderHooksForAddressIfUnused(ctx context.Context, address string) error {
	hasActiveDependency, err := a.db.HasActivePonderNotificationDependency(ctx, address)
	if err != nil {
		return err
	}
	if hasActiveDependency {
		return nil
	}

	hookIDs, err := a.db.GetKnownPonderHookIDsForAddress(ctx, address)
	if err != nil {
		return err
	}

	for _, hookID := range hookIDs {
		if err := a.deletePonderHook(ctx, hookID); err != nil {
			return err
		}
		if err := a.db.ClearMobilePushSubscriptionPonderHook(ctx, hookID); err != nil {
			return err
		}
	}

	return nil
}

func expoPushReceiptURL() string {
	receiptURL := strings.TrimSpace(os.Getenv("EXPO_PUSH_RECEIPT_API_URL"))
	if receiptURL == "" {
		receiptURL = defaultExpoPushReceiptAPIURL
	}
	return receiptURL
}

func expoPushReceiptDelay() time.Duration {
	rawValue := strings.TrimSpace(os.Getenv("EXPO_PUSH_RECEIPT_DELAY_SECONDS"))
	if rawValue == "" {
		return 30 * time.Second
	}
	seconds, err := strconv.Atoi(rawValue)
	if err != nil || seconds < 0 {
		return 30 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func expoDetailsError(details map[string]any) string {
	if len(details) == 0 {
		return ""
	}
	if value, ok := details["error"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func isExpoDeviceNotRegistered(details map[string]any) bool {
	return expoDetailsError(details) == expoDeviceNotRegistered
}

func parseExpoPushTickets(raw json.RawMessage) ([]expoPushTicket, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var tickets []expoPushTicket
		if err := json.Unmarshal(trimmed, &tickets); err != nil {
			return nil, err
		}
		return tickets, nil
	}

	var ticket expoPushTicket
	if err := json.Unmarshal(trimmed, &ticket); err != nil {
		return nil, err
	}
	return []expoPushTicket{ticket}, nil
}

func (a *AppService) deactivatePushTokenAndCleanup(ctx context.Context, token string, reason string) {
	disabledAddresses, err := a.db.DeactivateMobilePushSubscriptionsByToken(ctx, token, reason)
	if err != nil {
		a.logger.Logf("error deactivating mobile push subscriptions for dead Expo token: %s", err)
		return
	}

	for _, address := range disabledAddresses {
		if err := a.deletePonderHooksForAddressIfUnused(ctx, address); err != nil {
			a.logger.Logf("error cleaning up ponder hooks after deactivating dead Expo token for address %s: %s", address, err)
		}
	}
}

func (a *AppService) checkExpoPushReceiptAfterDelay(ticketID string, token string) {
	delay := expoPushReceiptDelay()
	if delay > 0 {
		time.Sleep(delay)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	receipt, err := getExpoPushReceipt(ctx, ticketID)
	if err != nil {
		a.logger.Logf("error getting Expo push receipt %s: %s", ticketID, err)
		return
	}

	errorCode := expoDetailsError(receipt.Details)
	if err := a.db.MarkMobilePushNotificationTicketReceipt(ctx, ticketID, receipt.Status, receipt.Message, errorCode); err != nil {
		a.logger.Logf("error marking Expo push receipt %s: %s", ticketID, err)
	}

	if receipt.Status == "error" && errorCode == expoDeviceNotRegistered {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		a.deactivatePushTokenAndCleanup(cleanupCtx, token, pushDeleteReasonDeadToken)
	}
}

func (a *AppService) handleExpoPushTicket(ctx context.Context, listener *structs.MobilePushSubscription, token string, ticket *expoPushTicket) {
	if listener == nil || ticket == nil {
		return
	}

	if ticket.Status == "error" && isExpoDeviceNotRegistered(ticket.Details) {
		a.deactivatePushTokenAndCleanup(ctx, token, pushDeleteReasonDeadToken)
		return
	}

	if ticket.Status != "ok" || strings.TrimSpace(ticket.ID) == "" {
		return
	}

	if err := a.db.AddMobilePushNotificationTicket(ctx, listener.Owner, token, listener.Address, ticket.ID); err != nil {
		a.logger.Logf("error storing Expo push ticket %s for user %s address %s: %s", ticket.ID, listener.Owner, listener.Address, err)
		return
	}

	go a.checkExpoPushReceiptAfterDelay(ticket.ID, token)
}

func resolvePushSyncState(req structs.PushSubscriptionSyncRequest) (*bool, *bool, bool) {
	if req.PreferenceEnabled != nil || req.DeviceRegistered != nil {
		return req.PreferenceEnabled, req.DeviceRegistered, req.PreferenceEnabled != nil
	}
	if req.Enabled != nil {
		return req.Enabled, req.Enabled, true
	}
	if len(req.Addresses) > 0 {
		enabled := true
		return &enabled, &enabled, true
	}
	return nil, nil, false
}

func expectedPushState(
	subscription *structs.MobilePushSubscription,
	preferenceEnabled *bool,
	deviceRegistered *bool,
) (bool, bool, bool) {
	preference := false
	device := true
	if subscription != nil {
		preference = subscription.PreferenceEnabled
		device = subscription.DeviceRegistered
	}
	if preferenceEnabled != nil {
		preference = *preferenceEnabled
	}
	if deviceRegistered != nil {
		device = *deviceRegistered
	}
	return preference, device, preference && device
}

func (a *AppService) AddPonderMerchantSubscription(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		a.logger.Logf("error reading request body from user %s: %s", *userDid, err)
		return
	}

	var req structs.PonderSubscriptionRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	isVerified, err := a.db.IsVerifiedEmailForUser(r.Context(), *userDid, req.Email)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		a.logger.Logf("error checking verified email for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !isVerified {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("notification email must be verified"))
		return
	}

	user, err := a.db.GetUserById(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting user %s details: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !user.IsMerchant && !user.IsAdmin {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	wallets, err := a.db.GetWalletsByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting wallets for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	userWallets := allowedWalletAddresses(wallets)

	if !userWallets[strings.ToLower(strings.TrimSpace(req.Address))] {
		a.logger.Logf("user %s attempted to add ponder subscription for unowned address %s", *userDid, req.Address)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	newSubscription, err := a.createPonderHook(r.Context(), req.Address)
	if err != nil {
		a.logger.Logf("error creating ponder subscription: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	formattedSubscription := structs.PonderSubscription{
		Id:      newSubscription.Id,
		Address: newSubscription.Address,
		Type:    structs.MerchantSubscription,
		Owner:   *userDid,
		Data:    []byte(req.Email),
	}

	err = a.db.AddPonderSubscription(r.Context(), &formattedSubscription)
	if err != nil {
		a.logger.Logf("error adding new ponder subscription: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (a *AppService) SyncPonderPushSubscriptions(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading push subscription sync body for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.PushSubscriptionSyncRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	wallets, err := a.db.GetWalletsByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting wallets for push sync user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	userWallets := allowedWalletAddresses(wallets)
	normalizedAddresses := make([]string, 0, len(req.Addresses))
	seenAddresses := make(map[string]struct{}, len(req.Addresses))
	for _, rawAddress := range req.Addresses {
		address := strings.ToLower(strings.TrimSpace(rawAddress))
		if address == "" {
			continue
		}
		if !userWallets[address] {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, seen := seenAddresses[address]; seen {
			continue
		}
		seenAddresses[address] = struct{}{}
		normalizedAddresses = append(normalizedAddresses, address)
	}
	preferenceEnabled, deviceRegistered, pruneMissing := resolvePushSyncState(req)
	deviceOnlySync := preferenceEnabled == nil && deviceRegistered != nil
	syncAddresses := normalizedAddresses
	if deviceOnlySync {
		syncAddresses = nil
	}

	existingSubscriptions, err := a.db.GetMobilePushSubscriptionsByOwnerToken(r.Context(), *userDid, req.Token)
	if err != nil {
		a.logger.Logf("error getting existing push subscriptions for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	existingByAddress := make(map[string]*structs.MobilePushSubscription, len(existingSubscriptions))
	for _, subscription := range existingSubscriptions {
		if subscription == nil {
			continue
		}
		existingByAddress[strings.ToLower(strings.TrimSpace(subscription.Address))] = subscription
	}

	hookCandidateAddresses := make([]string, 0, len(normalizedAddresses)+len(existingSubscriptions))
	if len(normalizedAddresses) > 0 && !deviceOnlySync {
		hookCandidateAddresses = append(hookCandidateAddresses, normalizedAddresses...)
	} else if preferenceEnabled != nil || deviceRegistered != nil {
		seenHookCandidates := make(map[string]struct{}, len(existingSubscriptions))
		for _, subscription := range existingSubscriptions {
			if subscription == nil {
				continue
			}
			address := strings.ToLower(strings.TrimSpace(subscription.Address))
			if address == "" {
				continue
			}
			if _, seen := seenHookCandidates[address]; seen {
				continue
			}
			seenHookCandidates[address] = struct{}{}
			hookCandidateAddresses = append(hookCandidateAddresses, address)
		}
	}

	createdHookIDsByAddress := make(map[string]int)
	for _, address := range hookCandidateAddresses {
		subscription := existingByAddress[address]
		_, _, shouldBeActive := expectedPushState(subscription, preferenceEnabled, deviceRegistered)
		if !shouldBeActive {
			continue
		}
		if subscription != nil && subscription.PonderHookId != nil && *subscription.PonderHookId > 0 {
			continue
		}
		newHook, err := a.createPonderHook(r.Context(), address)
		if err != nil {
			for _, hookID := range createdHookIDsByAddress {
				if deleteErr := a.deletePonderHook(r.Context(), hookID); deleteErr != nil {
					a.logger.Logf("error deleting orphaned ponder hook %d after failed push hook creation for user %s: %s", hookID, *userDid, deleteErr)
				}
			}
			a.logger.Logf("error creating ponder hook for push subscription user %s address %s: %s", *userDid, address, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		createdHookIDsByAddress[address] = newHook.Id
	}

	disabledAddresses, err := a.db.SyncMobilePushSubscriptions(
		r.Context(),
		*userDid,
		req.Token,
		syncAddresses,
		createdHookIDsByAddress,
		preferenceEnabled,
		deviceRegistered,
		pruneMissing,
	)
	if err != nil {
		for _, hookID := range createdHookIDsByAddress {
			if deleteErr := a.deletePonderHook(r.Context(), hookID); deleteErr != nil {
				a.logger.Logf("error deleting orphaned ponder hook %d after failed push sync for user %s: %s", hookID, *userDid, deleteErr)
			}
		}
		a.logger.Logf("error syncing push subscriptions for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for address, hookID := range createdHookIDsByAddress {
		if err := a.db.SetMobilePushSubscriptionPonderHook(r.Context(), *userDid, req.Token, address, hookID); err != nil {
			if deleteErr := a.deletePonderHook(r.Context(), hookID); deleteErr != nil {
				a.logger.Logf("error deleting orphaned ponder hook %d after failed hook-id sync for user %s: %s", hookID, *userDid, deleteErr)
			}
			a.logger.Logf("error recording ponder hook %d for push subscription user %s address %s: %s", hookID, *userDid, address, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	for _, address := range disabledAddresses {
		if err := a.deletePonderHooksForAddressIfUnused(r.Context(), address); err != nil {
			a.logger.Logf("error cleaning up ponder hooks for disabled push address %s: %s", address, err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) GetPonderPushSubscriptions(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	var (
		subscriptions []*structs.MobilePushSubscription
		err           error
	)
	if token != "" {
		subscriptions, err = a.db.GetMobilePushSubscriptionsByOwnerToken(r.Context(), *userDid, token)
	} else {
		subscriptions, err = a.db.GetMobilePushSubscriptionsByUser(r.Context(), *userDid)
	}
	if err != nil {
		a.logger.Logf("error getting mobile push subscriptions for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	body, err := json.Marshal(subscriptions)
	if err != nil {
		a.logger.Logf("error marshalling mobile push subscriptions for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func (a *AppService) DeletePonderPushSubscription(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	subscriptionID, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || subscriptionID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	subscription, err := a.db.GetMobilePushSubscription(r.Context(), subscriptionID, *userDid)
	if err != nil {
		a.logger.Logf("error getting mobile push subscription %d for user %s: %s", subscriptionID, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	address, err := a.db.DeleteMobilePushSubscription(r.Context(), subscriptionID, *userDid)
	if err != nil {
		a.logger.Logf("error deleting mobile push subscription %d for user %s: %s", subscriptionID, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if address == "" {
		address = subscription.Address
	}

	if err := a.deletePonderHooksForAddressIfUnused(r.Context(), address); err != nil {
		a.logger.Logf("error cleaning up ponder hooks after deleting push subscription %d for user %s: %s", subscriptionID, *userDid, err)
	}

	w.WriteHeader(http.StatusOK)
}

func (a *AppService) DeletePonderMerchantSubscription(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	hookId, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	subscription, err := a.db.GetPonderSubscription(r.Context(), hookId)
	if err != nil {
		a.logger.Logf("error getting ponder subscription for id %d: %s", hookId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if subscription.Owner != *userDid {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err = a.db.DeletePonderSubscription(r.Context(), hookId, *userDid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := a.deletePonderHooksForAddressIfUnused(r.Context(), subscription.Address); err != nil {
		a.logger.Logf("error cleaning up ponder hooks after deleting subscription %d for user %s: %s", hookId, *userDid, err)
	}

	w.WriteHeader(http.StatusOK)
}

func (a *AppService) GetPonderSubscriptions(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	subscriptions, err := a.db.GetPonderSubscriptionsByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting ponder subscriptions for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	body, err := json.Marshal(subscriptions)
	if err != nil {
		a.logger.Logf("error marshalling ponder subscriptions for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(200)
	w.Write(body)
}

func (a *AppService) PonderPingCallback(w http.ResponseWriter, r *http.Request) {
	key := os.Getenv("PONDER_KEY")
	if key != r.Header.Get("X-Admin-Key") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *AppService) PonderHookHandler(w http.ResponseWriter, r *http.Request) {
	key := os.Getenv("PONDER_KEY")
	if key != r.Header.Get("X-Admin-Key") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading ponder hook body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	tx := structs.PonderHookData{}
	err = json.Unmarshal(body, &tx)
	if err != nil {
		a.logger.Logf("error unmarshalling ponder txs into hook data: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sender := utils.NewEmailSender()
	if sender == nil {
		a.logger.Logf("error initializing new email sender: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	formattedAmount, err := utils.FormatTokenAmountFromStrings(tx.Amount, os.Getenv("TOKEN_DECIMALS"), 2)
	if err != nil {
		a.logger.Logf("error formatting ponder transaction amount %s: %s", tx.Amount, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	listeners, err := a.db.GetPonderSubscriptions(r.Context(), tx.To)
	if err != nil {
		a.logger.Logf("error getting ponder subscriptions for %s: %s", tx.To, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, l := range listeners {
		w, err := a.db.GetWalletByUserAndAddress(r.Context(), l.Owner, l.Address)
		if err != nil {
			a.logger.Logf("error getting wallet for user %s, address %s while sending tx receipt email: %s", l.Owner, l.Address, err)
			continue
		}

		subjectTail := fmt.Sprintf("- %s", tx.Hash[:6])
		toLine := tx.To

		if w.Name != "" {
			subjectTail = fmt.Sprintf("to %s %s", w.Name, subjectTail)
			toLine = fmt.Sprintf("%s (%s)", w.Name, tx.To)
		}

		subject := fmt.Sprintf("%s $SFLuv Incoming %s", formattedAmount, subjectTail)
		sections := fmt.Sprintf(`
            <tr>
              <td style="padding:24px 28px 8px;">
                <p style="margin:0 0 8px; font-size:14px; color:#6b7280;">Summary</p>
                <p style="margin:0; font-size:18px; font-weight:600; color:#111827;">Value: %s SFLuv</p>
              </td>
            </tr>
            <tr>
              <td style="padding:0 28px 24px;">
                <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
                  <tr>
                    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:120px;">From</td>
                    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
                  </tr>
                  <tr>
                    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">To</td>
                    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
                  </tr>
                  <tr>
                    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Hash</td>
                    <td style="padding:12px 0; font-size:13px; color:#111827; word-break:break-all;">%s</td>
                  </tr>
                </table>
              </td>
            </tr>
            <tr>
              <td style="padding:0 28px 24px;">
                <div style="background-color:#f9fafb; border-radius:12px; padding:14px 16px; font-size:12px; color:#6b7280;">
                  If you did not expect this transaction, please contact the SFLuv team.
                </div>
              </td>
            </tr>`,
			utils.EscapeEmailHTML(formattedAmount),
			utils.EscapeEmailHTML(tx.From),
			utils.EscapeEmailHTML(toLine),
			utils.EscapeEmailHTML(tx.Hash),
		)
		htmlContent := utils.BuildStyledEmailWithSections(
			"SFLuv Transaction Alert",
			"A new transaction has been recorded.",
			sections,
			"SFLuv · Transaction Notifications",
		)

		err = sender.SendEmail(
			strings.TrimSpace(string(l.Data)),
			"Merchant",
			subject,
			htmlContent,
			utils.NotificationFromEmail(),
			"SFLuv Transactions",
		)
	}
	if err != nil {
		a.logger.Logf("error sending transaction receipt email: %s", err.Error())
	}

	pushListeners, err := a.db.GetMobilePushSubscriptionsByAddress(r.Context(), tx.To)
	if err != nil {
		a.logger.Logf("error getting mobile push subscriptions for %s: %s", tx.To, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, listener := range pushListeners {
		wallet, walletErr := a.db.GetWalletByUserAndAddress(r.Context(), listener.Owner, listener.Address)
		if walletErr != nil {
			a.logger.Logf("error getting wallet for push notification user %s, address %s: %s", listener.Owner, listener.Address, walletErr)
			continue
		}

		accountLabel := ponderPushAccountLabel(wallet, listener.Address)
		title := fmt.Sprintf("SFLuv received to %s", accountLabel)
		body := fmt.Sprintf("%s SFLUV", formattedAmount)

		token := strings.TrimSpace(string(listener.Data))
		ticket, pushErr := sendExpoPushNotification(r.Context(), token, title, body, map[string]string{
			"hash":    tx.Hash,
			"to":      tx.To,
			"from":    tx.From,
			"amount":  formattedAmount,
			"address": listener.Address,
		})
		a.handleExpoPushTicket(r.Context(), listener, token, ticket)
		if pushErr != nil {
			a.logger.Logf("error sending Expo push notification for user %s, address %s: %s", listener.Owner, listener.Address, pushErr)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func sendExpoPushNotification(ctx context.Context, token string, title string, body string, data map[string]string) (*expoPushTicket, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("empty Expo push token")
	}

	pushURL := strings.TrimSpace(os.Getenv("EXPO_PUSH_API_URL"))
	if pushURL == "" {
		pushURL = defaultExpoPushAPIURL
	}

	payload := map[string]any{
		"to":    token,
		"title": title,
		"body":  body,
		"sound": "default",
		"data":  data,
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pushURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	bodyBytes, readErr := io.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("expo push api returned %d: %s", res.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var sendResponse expoPushSendResponse
	if err := json.Unmarshal(bodyBytes, &sendResponse); err != nil {
		return nil, fmt.Errorf("error parsing Expo push response: %w", err)
	}

	tickets, err := parseExpoPushTickets(sendResponse.Data)
	if err != nil {
		return nil, fmt.Errorf("error parsing Expo push ticket: %w", err)
	}
	if len(tickets) == 0 {
		if len(sendResponse.Errors) > 0 {
			ticket := sendResponse.Errors[0]
			return &ticket, fmt.Errorf("expo push api returned error: %s", ticket.Message)
		}
		return nil, fmt.Errorf("expo push api returned no ticket")
	}

	ticket := tickets[0]
	if ticket.Status == "error" {
		message := strings.TrimSpace(ticket.Message)
		if message == "" {
			message = "unknown Expo push ticket error"
		}
		return &ticket, fmt.Errorf("%s", message)
	}

	return &ticket, nil
}

func getExpoPushReceipt(ctx context.Context, ticketID string) (*expoPushReceipt, error) {
	ticketID = strings.TrimSpace(ticketID)
	if ticketID == "" {
		return nil, fmt.Errorf("empty Expo push ticket id")
	}

	payload := map[string]any{
		"ids": []string{ticketID},
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, expoPushReceiptURL(), bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	bodyBytes, readErr := io.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("expo push receipt api returned %d: %s", res.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var receiptResponse expoPushReceiptResponse
	if err := json.Unmarshal(bodyBytes, &receiptResponse); err != nil {
		return nil, fmt.Errorf("error parsing Expo push receipt response: %w", err)
	}
	if len(receiptResponse.Errors) > 0 {
		errTicket := receiptResponse.Errors[0]
		return nil, fmt.Errorf("expo push receipt api returned error: %s", errTicket.Message)
	}

	receipt, ok := receiptResponse.Data[ticketID]
	if !ok {
		return nil, fmt.Errorf("Expo push receipt %s not found", ticketID)
	}

	return &receipt, nil
}
