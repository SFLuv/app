package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func merchantModeErrorStatus(err error) int {
	switch {
	case errors.Is(err, db.ErrMerchantModeForbidden):
		return http.StatusForbidden
	case errors.Is(err, db.ErrMerchantModePINRequired):
		return http.StatusConflict
	case errors.Is(err, db.ErrMerchantModeOldPINNeeded):
		return http.StatusConflict
	case errors.Is(err, db.ErrMerchantModePINLocked):
		return http.StatusTooManyRequests
	case errors.Is(err, db.ErrMerchantModeBadPIN):
		return http.StatusUnauthorized
	case errors.Is(err, db.ErrMerchantModeInvalidPIN), errors.Is(err, db.ErrMerchantModeDeviceNeeded), errors.Is(err, pgx.ErrNoRows):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeMerchantModeError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), merchantModeErrorStatus(err))
}

func (a *AppService) GetMerchantModeStatus(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	status, err := a.db.GetMerchantModeStatus(r.Context(), *userDid, r.URL.Query().Get("installation_id"))
	if err != nil {
		a.logger.Logf("error getting merchant mode status for user %s: %s", *userDid, err.Error())
		writeMerchantModeError(w, err)
		return
	}

	jsonBytes, err := json.Marshal(status)
	if err != nil {
		a.logger.Logf("error marshalling merchant mode status for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func (a *AppService) SetMerchantModePIN(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading merchant mode PIN body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var request structs.MerchantModeSetPINRequest
	if err := json.Unmarshal(body, &request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	status, err := a.db.SetMerchantModePIN(r.Context(), *userDid, request.PIN, request.CurrentPIN)
	if err != nil {
		a.logger.Logf("error setting merchant mode PIN for user %s: %s", *userDid, err.Error())
		writeMerchantModeError(w, err)
		return
	}

	jsonBytes, err := json.Marshal(status)
	if err != nil {
		a.logger.Logf("error marshalling merchant mode PIN response for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func (a *AppService) RequestMerchantModePINHelp(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading merchant mode PIN help body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var request structs.MerchantModeForgotPINRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		a.logger.Logf("merchant mode PIN help requested by %s, but email sender is not configured", *userDid)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	contactEmail := strings.TrimSpace(request.ContactEmail)
	details := fmt.Sprintf(`<table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">User ID</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; text-align:right;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Contact Email</td>
    <td style="padding:12px 0; font-size:13px; color:#111827; text-align:right;">%s</td>
  </tr>
</table>`, utils.EscapeEmailHTML(*userDid), utils.EscapeEmailHTML(contactEmail))
	htmlContent := utils.BuildStyledEmail(
		"Merchant Mode PIN help requested",
		"A merchant needs help resetting their Merchant Mode PIN.",
		details,
	)

	if err := emailSender.SendEmail("techsupport@sfluv.org", "Tech Support", "Merchant Mode PIN help requested", htmlContent, utils.NotificationFromEmail(), "SFLuv Support"); err != nil {
		a.logger.Logf("error sending merchant mode PIN help email for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) ListMerchantModeDevices(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	response, err := a.db.ListMerchantModeDevices(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error listing merchant mode devices for user %s: %s", *userDid, err.Error())
		writeMerchantModeError(w, err)
		return
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		a.logger.Logf("error marshalling merchant mode devices for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func (a *AppService) UpdateMerchantModeDevice(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	deviceID := chi.URLParam(r, "device_id")
	if deviceID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading merchant mode device update body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var request structs.MerchantModeDeviceUpdateRequest
	if err := json.Unmarshal(body, &request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	device, err := a.db.SetMerchantModeDeviceEnabled(r.Context(), *userDid, deviceID, request.MerchantModeEnabled)
	if err != nil {
		a.logger.Logf("error updating merchant mode device %s for user %s: %s", deviceID, *userDid, err.Error())
		writeMerchantModeError(w, err)
		return
	}

	response := &structs.MerchantModeDeviceUpdateResponse{Device: device}
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		a.logger.Logf("error marshalling merchant mode device update response for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func (a *AppService) EnableMerchantMode(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading merchant mode enable body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var request structs.MerchantModeEnableRequest
	if err := json.Unmarshal(body, &request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	status, err := a.db.EnableMerchantModeDevice(r.Context(), *userDid, &request)
	if err != nil {
		a.logger.Logf("error enabling merchant mode for user %s: %s", *userDid, err.Error())
		writeMerchantModeError(w, err)
		return
	}

	jsonBytes, err := json.Marshal(status)
	if err != nil {
		a.logger.Logf("error marshalling merchant mode enable response for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func (a *AppService) DisableMerchantMode(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading merchant mode disable body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var request structs.MerchantModeDisableRequest
	if err := json.Unmarshal(body, &request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	status, err := a.db.DisableMerchantModeDevice(r.Context(), *userDid, &request)
	if err != nil {
		a.logger.Logf("error disabling merchant mode for user %s: %s", *userDid, err.Error())
		writeMerchantModeError(w, err)
		return
	}

	jsonBytes, err := json.Marshal(status)
	if err != nil {
		a.logger.Logf("error marshalling merchant mode disable response for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}
