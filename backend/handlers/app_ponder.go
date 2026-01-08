package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

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
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user, err := a.db.GetUserById(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting user %s details: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !user.IsMerchant {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	wallets, err := a.db.GetWalletsByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting wallets for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	userWallets := map[string]bool{}
	for _, wallet := range wallets {
		if !wallet.IsEoa {
			userWallets[*wallet.SmartAddress] = true
			continue
		}
		userWallets[*&wallet.EoaAddress] = true
	}

	if !userWallets[req.Address] {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ponderUrl := os.Getenv("PONDER_SERVER_BASE_URL")

	subscriptionBody := structs.PonderSubscriptionServerRequest{
		Id:      0,
		Address: req.Address,
		Url:     os.Getenv("PONDER_CALLBACK_URL"),
	}

	reqBody, err := json.Marshal(subscriptionBody)
	if err != nil {
		a.logger.Logf("error marshalling ponder subscription body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	subscriptionReq, err := http.NewRequest("POST", fmt.Sprintf("%s/hooks", ponderUrl), bytes.NewReader(reqBody))
	if err != nil {
		a.logger.Logf("error creating ponder subscription request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	subscriptionReq.Header.Add("X-Admin-Key", os.Getenv("PONDER_KEY"))

	res, err := http.DefaultClient.Do(subscriptionReq)
	if err != nil {
		a.logger.Logf("error sending ponder subscription request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.StatusCode != 201 {
		a.logger.Logf("ponder subscription not created: check ponder logs")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		a.logger.Logf("error reading ponder subscription response body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var newSubscription structs.PonderSubscriptionServerRequest
	err = json.Unmarshal(resBody, &newSubscription)
	if err != nil {
		a.logger.Logf("error unmarshalling ponder subscription request: %s", err)
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

	ponderUrl := os.Getenv("PONDER_SERVER_BASE_URL")

	subscriptionReq, err := http.NewRequest("DELETE", fmt.Sprintf("%s/hooks?id=%d", ponderUrl, hookId), nil)
	if err != nil {
		a.logger.Logf("error creating ponder delete subscription request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res, err := http.DefaultClient.Do(subscriptionReq)
	if err != nil {
		a.logger.Logf("error sending ponder delete subscription request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.StatusCode != 200 {
		a.logger.Logf("ponder subscription not deleted: check ponder logs")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = a.db.DeletePonderSubscription(r.Context(), hookId, *userDid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	subscriptionReq.Header.Add("X-Admin-Key", os.Getenv("PONDER_KEY"))

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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading ponder hook body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	fmt.Println(body)
	w.WriteHeader(http.StatusOK)
}
