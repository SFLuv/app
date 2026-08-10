package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// adminLocationUpdateRequest is the admin-editable surface of a listing.
//
// Deliberately not structs.Location: accepting the full struct here would let a
// request carry approval, owner, wallets or place identity and rely on the
// handler to remember to strip each one. Naming the editable fields means a
// field added to Location later is not silently admin-writable.
type adminLocationUpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Street      string `json:"street"`
	City        string `json:"city"`
	State       string `json:"state"`
	ZIP         string `json:"zip"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Website     string `json:"website"`
	// Absent leaves hours untouched; present replaces all seven days.
	// Structured times win when both are sent — the picker is the real input and
	// the text is only its rendering.
	OpeningHours *[]string                   `json:"opening_hours"`
	Hours        *[]structs.LocationDayHours `json:"hours"`
	// Absent leaves the mode alone, so an edit that does not mention it cannot
	// silently re-enable the nightly overwrite.
	HoursManual *bool `json:"hours_manual"`
}

const weekdaysPerWeek = 7

// AdminUpdateLocation corrects a merchant listing's details, for any location.
//
// Admins can reach fields the owner route cannot — type, hours, and the
// city/state/zip half of the address — because those are otherwise written only
// by the Google sync, leaving no way to fix a listing Google has wrong.
func (a *AppService) AdminUpdateLocation(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || locationID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A valid location id is required."})
		return
	}

	var request adminLocationUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Could not read the update."})
		return
	}

	location := structs.Location{
		ID:          uint(locationID),
		Name:        request.Name,
		Description: request.Description,
		Type:        request.Type,
		Street:      request.Street,
		City:        request.City,
		State:       request.State,
		ZIP:         request.ZIP,
		Phone:       request.Phone,
		Email:       request.Email,
		Website:     request.Website,
	}
	// Same normalisation and rules the owner-facing edit runs. An admin edit is
	// still an edit: it should not be able to write a listing the merchant
	// themselves would have been refused.
	location.NormalizeForSubmission()
	if err := location.ValidateForUpdate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	structuredHours, err := resolveRequestedHours(request.Hours, request.OpeningHours)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := a.db.AdminUpdateLocationDetails(r.Context(), &location); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "That location no longer exists."})
			return
		}
		a.logger.Logf("admin failed to update location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if structuredHours != nil {
		if err := a.db.SetLocationHours(r.Context(), uint(locationID), structuredHours); err != nil {
			a.logger.Logf("admin failed to set hours for location %d: %s", locationID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	if request.HoursManual != nil {
		if err := a.db.SetLocationHoursManual(r.Context(), uint(locationID), *request.HoursManual); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "That location no longer exists."})
				return
			}
			a.logger.Logf("admin failed to set manual hours mode for location %d: %s", locationID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// resolveRequestedHours turns whatever shape the client sent into a validated
// week, or nil to mean "leave hours alone".
//
// Text is still accepted so older clients keep working, but it is parsed
// through the same reader the migration used rather than stored verbatim — the
// database's display column is derived from the times, never the other way
// round.
func resolveRequestedHours(structured *[]structs.LocationDayHours, text *[]string) ([]structs.LocationDayHours, error) {
	var days []structs.LocationDayHours

	switch {
	case structured != nil:
		days = append(days, *structured...)
	case text != nil:
		for index, entry := range *text {
			days = append(days, structs.ParseDisplayHours(index, entry))
		}
	default:
		return nil, nil
	}

	if len(days) != weekdaysPerWeek {
		return nil, fmt.Errorf("opening hours must cover all seven days, Monday first")
	}
	for index := range days {
		// Trust position over any weekday the client supplied, so a mislabelled
		// payload cannot shuffle a merchant's week.
		days[index].Weekday = index
		if err := days[index].Validate(); err != nil {
			return nil, err
		}
	}

	return days, nil
}

// AdminUpdateLocationGooglePlace re-points any location at a Google listing.
//
// The place is re-fetched server-side, so name, address, coordinates, type and
// hours all come back as Google's own values — a client cannot smuggle its own
// address in through this route.
func (a *AppService) AdminUpdateLocationGooglePlace(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || locationID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A valid location id is required."})
		return
	}

	var request struct {
		GoogleID string `json:"google_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Could not read the update."})
		return
	}
	if strings.TrimSpace(request.GoogleID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A Google place is required."})
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
		a.logger.Logf("admin google place verification failed for %s: %s", request.GoogleID, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "Could not reach Google to verify this location. Please try again.",
		})
		return
	}

	if err := a.db.AdminUpdateLocationGooglePlace(r.Context(), uint(locationID), verified); err != nil {
		if errors.Is(err, db.ErrDuplicateGoogleLocation) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "Another SFLuv location is already using this Google listing.",
			})
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "That location no longer exists."})
			return
		}
		a.logger.Logf("admin error updating google place for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
