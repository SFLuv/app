package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"

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

	userWallets := map[string]bool{}
	for _, wallet := range wallets {
		if !wallet.IsEoa && wallet.SmartAddress != nil {
			userWallets[*wallet.SmartAddress] = true
			continue
		}
		userWallets[wallet.EoaAddress] = true
	}

	if !userWallets[req.Address] {
		fmt.Println("no address %s", req.Address)
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
	subscriptionReq.Header.Add("X-Admin-Key", os.Getenv("PONDER_KEY"))

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

	amount := new(big.Float)
	decimals := new(big.Float)

	_, ok := decimals.SetPrec(6).SetString(os.Getenv("TOKEN_DECIMALS"))
	if !ok {
		a.logger.Logf("error setting token decimals amount bigint from string: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, ok = amount.SetPrec(6).SetString(tx.Amount)
	if !ok {
		a.logger.Logf("error setting tx amount bigint from string: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	formatted := new(big.Float)
	formatted.Quo(amount, decimals)
	formattedAmount := formatted.Text('f', 2)

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
			formattedAmount,
			tx.From,
			toLine,
			tx.Hash,
		)
		htmlContent := utils.BuildStyledEmailWithSections(
			"SFLuv Transaction Alert",
			"A new transaction has been recorded.",
			sections,
			"SFLuv · Transaction Notifications",
		)

		err = sender.SendEmail(
			string(l.Data),
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

	w.WriteHeader(http.StatusOK)
}
