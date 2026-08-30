package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (a *AppService) GetLocation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	num, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		a.logger.Logf("invalid id, got: %s: %s", id, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	location, err := a.db.GetLocation(r.Context(), num)
	if err != nil {
		a.logger.Logf("no location with id %s: %s", id, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	jsonBytes, err := json.Marshal(map[string]any{
		"location": location,
	})
	if err != nil {
		a.logger.Logf("Error marhalling json %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func (a *AppService) GetLocations(w http.ResponseWriter, r *http.Request) {
	page, count := parsePageAndCount(r.URL.Query(), 100, 200)

	request := structs.LocationsPageRequest{
		Page:  uint(page),
		Count: uint(count),
	}

	locations, err := a.db.GetLocations(r.Context(), &request)
	if err != nil {
		a.logger.Logf("Failed to get locations %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(map[string]any{
		"locations": locations,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		a.logger.Logf("Error marshalling JSON for locations objects %s", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func (a *AppService) GetAuthedLocations(w http.ResponseWriter, r *http.Request) {
	page, count := parsePageAndCount(r.URL.Query(), 100, 200)

	request := structs.LocationsPageRequest{
		Page:  uint(page),
		Count: uint(count),
	}

	locations, err := a.db.GetAuthedLocations(r.Context(), &request)
	if err != nil {
		a.logger.Logf("Failed to get locations %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(map[string]any{
		"locations": locations,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		a.logger.Logf("Error marshalling JSON for locations objects %s", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func (a *AppService) GetLocationsByUser(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		a.logger.Logf("Could not pull user DID")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	locations, err := a.db.GetLocationsByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting locations for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(map[string]any{
		"locations": locations,
	})
	if err != nil {
		a.logger.Logf(("Error marshalling JSON for locations objects"))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func (a *AppService) AddLocation(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userDid := utils.GetDid(r)
	if userDid == nil {
		a.logger.Logf("Could not pull user DID")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading req body: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var location *structs.Location
	err = json.Unmarshal(body, &location)
	if err != nil {
		a.logger.Logf("invalid req body: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if location == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	location.OwnerID = *userDid
	// A submission never carries its own approval state; new locations always
	// start pending and are published only through the admin approval route.
	location.Approval = nil
	location.NormalizeForSubmission()

	// Re-fetch the place from Google server-side so the stored name, address and
	// coordinates are Google's, not the browser's. This is what stops a place
	// that is really a street address from landing on the map as a business.
	//
	// Manual listings have no place id to re-fetch — that is the whole point of
	// the path — so they skip straight to local validation, which is stricter
	// about the merchant-authored name for exactly that reason.
	if location.EffectiveListingSource() == structs.ListingSourceGooglePlace && GooglePlacesVerificationEnabled() {
		verified, err := VerifyGooglePlace(r.Context(), location.GoogleID)
		if err != nil {
			if IsPlaceVerificationError(err) {
				a.logger.Logf("rejected location submission from %s: %s", *userDid, err.Error())
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			// A Places outage should not block onboarding; fall through and let
			// local validation decide on the client-supplied values.
			a.logger.Logf("google place verification unavailable for %s: %s", location.GoogleID, err.Error())
		} else {
			verified.ApplyTo(location)
		}
	}

	if err := location.ValidateForSubmission(); err != nil {
		a.logger.Logf("invalid location submission from %s: %s", *userDid, err.Error())
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	err = a.db.AddLocation(r.Context(), location)
	if err != nil {
		if errors.Is(err, db.ErrDuplicateGoogleLocation) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "This business is already registered with SFLuv.",
			})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		a.logger.Logf("error adding location for %s: %s", *userDid, err.Error())
		return
	}

	// Mint the till now rather than at approval, so the merchant's Locations tab
	// has an address to show them while the listing sits in the review queue.
	// This cannot fail the request: see provisionNewLocationWallets.
	a.provisionNewLocationWallets(r.Context(), location.ID)

	a.sendRoleRequestEmail(
		"MERCHANT_ADMIN_EMAIL",
		"New Location Added",
		"A new merchant location has been submitted for review.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:140px;">Location</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Submitted By</td>
    <td style="padding:12px 0; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
</table>`, utils.EscapeEmailHTML(location.Name), utils.EscapeEmailHTML(location.OwnerID)),
	)

	// The id, not the old "success" text. The stepper needs it: opening hours
	// are optional on the form but are written through their own endpoint, which
	// is addressed by location id, and the confirmation screen links to the
	// listing it has just created. Still a 201 with a body, so a client that
	// only reads the status code is unaffected.
	writeJSON(w, http.StatusCreated, map[string]any{"id": location.ID})
}

// CancelLocationApplication withdraws an application the caller owns while it
// is still waiting to be reviewed.
//
// The merchant's own way out. Before this, an application submitted by mistake
// — the wrong branch, a duplicate, a shop they decided against — sat in the
// admin queue with nothing the merchant could do about it, and it also pinned
// their account as a merchant account, because a pending listing is one of the
// two things that stops a revert to personal.
func (a *AppService) CancelLocationApplication(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = a.db.CancelPendingLocation(r.Context(), *userDid, locationID)
	if errors.Is(err, db.ErrLocationNotCancellable) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "This application has already been reviewed, so it can no longer be cancelled.",
		})
		return
	}
	if err != nil {
		a.logger.Logf("error cancelling location %d for %s: %s", locationID, *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.logger.Logf("user %s cancelled their pending location application %d", *userDid, locationID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userDid := utils.GetDid(r)
	if userDid == nil {
		a.logger.Logf("could not pull user DID")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update location body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// The client historically sent {"location": {...}}; accept both that and a
	// bare location object so neither shape silently unmarshals to a zero value.
	var wrapper struct {
		Location *structs.Location `json:"location"`
	}
	var location structs.Location
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Location != nil {
		location = *wrapper.Location
	} else if err := json.Unmarshal(body, &location); err != nil {
		a.logger.Logf("error unmarshalling update location body: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if location.ID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A location id is required."})
		return
	}

	location.OwnerID = *userDid
	// Approval is admin-only and is never read from this route; see
	// db.UpdateLocation for the full list of columns this route cannot write.
	location.Approval = nil
	location.NormalizeForSubmission()

	if err := location.ValidateForUpdate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	err = a.db.UpdateLocation(r.Context(), &location)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("failed to update location %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// UpdateLocationGooglePlace re-points an owned location at a Google place. The
// place is re-fetched server-side, so the stored name, address, coordinates and
// hours are always Google's own values rather than anything the client sent.
func (a *AppService) UpdateLocationGooglePlace(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var request struct {
		GoogleID string `json:"google_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !GooglePlacesVerificationEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Google verification is not configured, so location details cannot be refreshed right now.",
		})
		return
	}

	verified, err := VerifyGooglePlace(r.Context(), request.GoogleID)
	if err != nil {
		if IsPlaceVerificationError(err) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		a.logger.Logf("google place verification failed for %s: %s", request.GoogleID, err.Error())
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "Could not reach Google to verify this location. Please try again.",
		})
		return
	}

	err = a.db.UpdateLocationGooglePlace(r.Context(), *userDid, uint(locationID), verified)
	if err != nil {
		if errors.Is(err, db.ErrDuplicateGoogleLocation) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "Another SFLuv location is already using this Google listing.",
			})
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error updating google place for location %d: %s", locationID, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, verified)
}

// sendLocationApprovedEmail notifies the merchant contact that their location is
// live. It runs from the admin approval route, which is the only path that can
// actually flip approval.
func (a *AppService) sendLocationApprovedEmail(ctx context.Context, locationID uint) {
	contact, err := a.db.GetLocationApprovalContact(ctx, locationID)
	if err != nil {
		a.logger.Logf("error loading approval contact for location %d: %s", locationID, err.Error())
		return
	}
	if contact.AdminEmail == "" {
		return
	}

	sender := utils.NewEmailSender()
	if sender == nil {
		return
	}

	details := fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:160px;">Location</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Status</td>
    <td style="padding:12px 0; font-size:13px; color:#111827;">Approved</td>
  </tr>
</table>`, utils.EscapeEmailHTML(contact.Name))

	htmlContent := utils.BuildStyledEmail(
		"Location Approved",
		"Your location has been approved.",
		details,
	)

	err = sender.SendEmail(
		contact.AdminEmail,
		contact.ContactName,
		"Location Approved",
		htmlContent,
		utils.NotificationFromEmail(),
		"SFLuv Admin",
	)
	if err != nil {
		a.logger.Logf("error sending location approval email: %s", err.Error())
	}
}

func (a *AppService) UpdateLocationWalletSettings(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	locationIDValue := chi.URLParam(r, "id")
	locationID, err := strconv.ParseUint(locationIDValue, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading location wallet settings body for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var request structs.LocationWalletSettingsUpdateRequest
	if err := json.Unmarshal(body, &request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	location, err := a.db.UpdateLocationWalletSettings(r.Context(), *userDid, locationID, &request)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		errMsg := err.Error()
		if containsLocationWalletValidationError(errMsg) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}

		a.logger.Logf("error updating location wallet settings for user %s and location %d: %s", *userDid, locationID, errMsg)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(location)
}

// Messages that name the offending location or wallet cannot be matched
// exactly, but they are still the merchant's mistake to fix rather than a
// server fault — without this they would surface as a 500 and lose their text.
var locationWalletValidationPrefixes = []string{
	"that wallet is already in use by ",
	"that wallet is already this location's ",
	"that wallet does not belong to this account",
	"that wallet is a signing key, not a smart account",
	"a location must always have a payment wallet",
	"this account has no signing wallet",
	"unknown wallet role ",
}

func containsLocationWalletValidationError(errMsg string) bool {
	for _, prefix := range locationWalletValidationPrefixes {
		if strings.HasPrefix(errMsg, prefix) {
			return true
		}
	}

	switch errMsg {
	case
		"only merchants can update merchant wallet settings",
		"only merchants can change location wallets",
		"payment wallet is required",
		"payment wallet must be a valid ethereum address",
		"default payment wallet requires at least one payment wallet",
		"default payment wallet must be a valid ethereum address",
		"default payment wallet must be one of the payment wallets",
		"tipping wallet must be a valid ethereum address",
		"tipping wallet must be different from every payment wallet",
		"tipping wallet must be different from the default payment wallet":
		return true
	default:
		return false
	}
}
