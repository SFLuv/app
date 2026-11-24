package handlers

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/mail"
	"os"
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

	// Keep the device's recorded app version current. The app sends its version
	// on every request; without this, a till updated while in merchant mode kept
	// the version it was enabled on (Niebla's iPad showed 1.0.3 after updating to
	// 2.0.1). A failure here only leaves a stale label, so it doesn't fail the poll.
	if status.Device != nil {
		if version := strings.TrimSpace(r.Header.Get(clientVersionHeader)); version != "" && version != status.Device.AppVersion {
			if err := a.db.UpdateMerchantModeDeviceAppVersion(r.Context(), status.Device.ID, version); err != nil {
				a.logger.Logf("error recording merchant mode device app version for user %s: %s", *userDid, err.Error())
			} else {
				status.Device.AppVersion = version
			}
		}
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

// AdminResetMerchantModePIN sets a merchant's merchant-mode PIN on their behalf,
// for when they have forgotten it. The body's pin is optional; without one a
// random 6-digit PIN is generated. The PIN is returned once so the admin can
// pass it on, and is never logged.
func (a *AppService) AdminResetMerchantModePIN(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(chiParam(r, "user_id"))
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var request struct {
		PIN string `json:"pin"`
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	pin := strings.TrimSpace(request.PIN)
	generated := false
	if pin == "" {
		pin, err = randomMerchantModePIN()
		if err != nil {
			a.logger.Logf("error generating merchant mode PIN for user %s: %s", userID, err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		generated = true
	}

	if err := a.db.AdminSetMerchantModePIN(r.Context(), userID, pin); err != nil {
		a.logger.Logf("error resetting merchant mode PIN for user %s: %s", userID, err.Error())
		writeMerchantModeError(w, err)
		return
	}

	adminID := "unknown admin"
	if did := utils.GetDid(r); did != nil {
		adminID = *did
	}
	a.logger.Logf("merchant mode PIN reset for user %s by %s", userID, adminID)

	writeJSON(w, http.StatusOK, map[string]any{"pin": pin, "generated": generated})
}

func randomMerchantModePIN() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
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

	// The hub is the record of the request now, and decides who hears about
	// it; a team that wants these in the support inbox gives itself that
	// address. One live alert per merchant, so pressing the button twice does
	// not ask twice.
	contactEmail := ""
	if parsed, err := mail.ParseAddress(strings.TrimSpace(request.ContactEmail)); err == nil {
		contactEmail = parsed.Address
	}
	who := *userDid
	if identities, err := a.db.GetUserIdentities(r.Context(), []string{*userDid}); err == nil {
		if identity, ok := identities[*userDid]; ok {
			if name := strings.TrimSpace(identity.Name); name != "" {
				who = name
			}
			if contactEmail == "" {
				contactEmail = strings.TrimSpace(identity.Email)
			}
		}
	}
	payload := map[string]string{"user_id": *userDid, "email": contactEmail, "subject": "Merchant Mode PIN help"}
	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/"); base != "" {
		payload["url"] = base + "/admin"
	}
	if a.notifications == nil {
		a.logger.Logf("merchant mode PIN help requested by %s, but the notification hub is not running", *userDid)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := a.notifications.Post(r.Context(), Alert{
		Topic: structs.NotificationTopicPinHelp,
		Key:   "pin_help:" + *userDid,
		Kind:  structs.InternalKindNotice,
		Title: "Merchant PIN help: " + who,
		Body:  "A merchant is locked out of merchant mode and needs their PIN reset. Reset it in the admin panel's merchant PIN tools.",
		Fields: []structs.NotificationField{
			{Label: "Merchant", Value: who},
			{Label: "Contact email", Value: contactEmail},
			{Label: "Account", Value: *userDid},
		},
		Payload: payload,
		ReplyTo: contactEmail,
	}); err != nil {
		a.logger.Logf("error posting merchant mode PIN help for user %s: %s", *userDid, err)
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

// ListMerchantModeLocations backs the picker shown when a device is put into
// merchant mode, and the location toggle on the till.
func (a *AppService) ListMerchantModeLocations(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	response, err := a.db.ListMerchantModeLocations(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error listing merchant mode locations for user %s: %s", *userDid, err.Error())
		writeMerchantModeError(w, err)
		return
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		a.logger.Logf("error marshalling merchant mode locations for user %s: %s", *userDid, err.Error())
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
