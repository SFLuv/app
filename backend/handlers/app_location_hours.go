package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type locationHoursUpdateRequest struct {
	Hours *[]structs.LocationDayHours `json:"hours"`
	// Absent leaves the mode alone.
	HoursManual *bool `json:"hours_manual"`
}

// UpdateLocationHours lets a merchant set their own opening times and choose
// whether the nightly Google sync may overwrite them.
//
// Setting hours by hand does NOT implicitly switch the listing to manual. That
// would be convenient and wrong in both directions: a merchant fixing one day
// would silently opt out of updates forever, and a merchant who wants Google to
// keep them current would have no way to correct a single day. The toggle is a
// separate, explicit choice.
func (a *AppService) UpdateLocationHours(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || locationID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A valid location id is required."})
		return
	}

	owned, err := a.db.UserOwnsLocation(r.Context(), *userDid, uint(locationID))
	if err != nil {
		a.logger.Logf("error checking location ownership for %s on %d: %s", *userDid, locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !owned {
		// 404 rather than 403: a merchant probing ids should not learn which
		// location ids exist.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "That location could not be found."})
		return
	}

	var request locationHoursUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Could not read the update."})
		return
	}

	days, err := resolveRequestedHours(request.Hours, nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if days != nil {
		if err := a.db.SetLocationHours(r.Context(), uint(locationID), days); err != nil {
			a.logger.Logf("error setting hours for location %d: %s", locationID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	if request.HoursManual != nil {
		if err := a.db.SetLocationHoursManual(r.Context(), uint(locationID), *request.HoursManual); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "That location could not be found."})
				return
			}
			a.logger.Logf("error setting manual hours mode for location %d: %s", locationID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
