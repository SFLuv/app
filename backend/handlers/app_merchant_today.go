package handlers

import (
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// merchantDayBounds returns [start, end) for the business day containing now, in
// the location's zone. Midnight to midnight local, computed here rather than on
// the device so every client agrees on when the till rolls over.
func merchantDayBounds(now time.Time, zoneName string) (int64, int64, string) {
	location, err := time.LoadLocation(zoneName)
	if err != nil {
		location = time.UTC
		zoneName = "UTC"
	}

	local := now.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	// AddDate rather than +24h: it stays correct across a daylight-saving change,
	// where a local day is 23 or 25 hours long.
	end := start.AddDate(0, 0, 1)

	return start.Unix(), end.Unix(), start.Format("2006-01-02")
}

func tipPairingWindowSeconds() int64 {
	if raw := strings.TrimSpace(os.Getenv("MERCHANT_TIP_PAIRING_WINDOW_SECONDS")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return structs.DefaultTipPairingWindowSeconds
}

// GetMerchantToday serves the whole merchant-mode home screen: the day's takings,
// the day's tips, and the paired line items behind them.
//
// Scoped to the calling user's own enabled device, so an employee holding the
// till can only ever see that till.
func (a *AppService) GetMerchantToday(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	status, err := a.db.GetMerchantModeStatus(r.Context(), *userDid, r.URL.Query().Get("installation_id"))
	if err != nil {
		a.logger.Logf("error resolving merchant mode device for %s: %s", *userDid, err.Error())
		writeMerchantModeError(w, err)
		return
	}
	if status == nil || status.Device == nil || !status.Device.MerchantModeEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "this device is not in merchant mode",
		})
		return
	}
	if a.ponderDb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "transaction index is unavailable",
		})
		return
	}

	wallets, err := a.db.GetMerchantDayWallets(r.Context(), status.Device.LocationID, status.Device.WalletAddress)
	if err != nil {
		a.logger.Logf("error resolving merchant day wallets for %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	start, end, businessDate := merchantDayBounds(time.Now(), wallets.TimeZone)

	payments, err := a.ponderDb.TransfersInto(r.Context(), []string{wallets.Payment}, start, end)
	if err != nil {
		a.logger.Logf("error loading merchant payments for %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	refunds, err := a.ponderDb.TransfersOutOf(r.Context(), []string{wallets.Payment}, start, end)
	if err != nil {
		a.logger.Logf("error loading merchant refunds for %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var tips []structs.MerchantTransfer
	if wallets.Tipping != "" {
		tips, err = a.ponderDb.TransfersInto(r.Context(), []string{wallets.Tipping}, start, end)
		if err != nil {
			a.logger.Logf("error loading merchant tips for %s: %s", *userDid, err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	// Totals come from the raw streams, never from the paired rows. Pairing is a
	// heuristic and can attribute a tip to the wrong line; the day's figures must
	// not be able to drift because of that.
	paymentsTotal := structs.SumTransfers(payments)
	paymentsTotal.Sub(paymentsTotal, structs.SumTransfers(refunds))

	response := structs.MerchantTodayResponse{
		BusinessDate:         businessDate,
		TimeZone:             wallets.TimeZone,
		PaymentsBase:         paymentsTotal.String(),
		TipsBase:             structs.SumTransfers(tips).String(),
		TokenDecimals:        merchantTokenDecimals(),
		Transactions:         structs.PairPaymentsAndTips(payments, tips, refunds, tipPairingWindowSeconds()),
		TipsWalletConfigured: wallets.Tipping != "",
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		a.logger.Logf("error marshalling merchant today for %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

// merchantTokenDecimals reports how many decimal places the client should shift.
// TOKEN_DECIMALS holds the multiplier (1000000), not the exponent, so it is
// counted rather than parsed.
func merchantTokenDecimals() int {
	multiplier := strings.TrimSpace(os.Getenv("TOKEN_DECIMALS"))
	value, ok := new(big.Int).SetString(multiplier, 10)
	if !ok || value.Sign() <= 0 {
		return 6
	}
	decimals := 0
	ten := big.NewInt(10)
	for value.Cmp(big.NewInt(1)) > 0 {
		value.Div(value, ten)
		decimals++
	}
	return decimals
}
