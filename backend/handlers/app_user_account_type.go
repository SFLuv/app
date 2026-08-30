package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// GetMerchantRevertEligibility tells the settings screen whether this account
// can still be turned back into a personal one, and what is stopping it.
//
// A read rather than something the client works out from its own location list:
// the client's copy is whatever the last profile fetch returned, and an admin
// approving a listing in between is exactly the case where the two disagree.
func (a *AppService) GetMerchantRevertEligibility(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	eligibility, err := a.db.GetMerchantRevertEligibility(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error loading merchant revert eligibility for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, eligibility)
}

// UpdateOwnAccountType is the settings screen's switch between a personal and a
// merchant account.
//
// Both directions come through here rather than through two verbs, because the
// two are one decision from the person's side and the interesting rule — that
// merchant is one-way once a listing exists — belongs in one place.
func (a *AppService) UpdateOwnAccountType(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var request struct {
		AccountType string `json:"account_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Could not read the account type."})
		return
	}
	if !structs.IsValidAccountType(request.AccountType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unknown account type."})
		return
	}

	result, err := a.db.SetOwnAccountType(r.Context(), *userDid, request.AccountType)
	if errors.Is(err, db.ErrMerchantAccountIsPermanent) {
		// Re-read so the refusal can say what is actually holding them, rather
		// than restating the rule at somebody who may only need to cancel one
		// pending application to satisfy it.
		eligibility, readErr := a.db.GetMerchantRevertEligibility(r.Context(), *userDid)
		if readErr != nil {
			a.logger.Logf("error re-reading revert eligibility for %s: %s", *userDid, readErr)
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "This merchant account still has locations, so it cannot be turned back into a personal account.",
			})
			return
		}

		message := "This merchant account still has locations, so it cannot be turned back into a personal account."
		if eligibility.ApprovedLocations > 0 {
			message = "You have an approved location on the SFLuv map, so this account stays a merchant account."
		} else if eligibility.PendingLocations > 0 {
			message = "Cancel your pending location applications first, then you can switch back to a personal account."
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":       message,
			"eligibility": eligibility,
		})
		return
	}
	if err != nil {
		a.logger.Logf("error setting account type for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.logger.Logf(
		"user %s changed their own account type from %s to %s",
		result.UserId, result.PreviousAccountType, result.AccountType,
	)

	writeJSON(w, http.StatusOK, result)
}

// MarkWebMerchantPromptSeen records that the web app has put the merchant
// option to somebody who signed up on mobile, so it stops offering it.
//
// Deliberately not tied to the person accepting or declining. The prompt is
// shown once whatever they do with it; the settings screen is where the offer
// lives from then on.
func (a *AppService) MarkWebMerchantPromptSeen(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if err := a.db.MarkWebMerchantPromptSeen(r.Context(), *userDid); err != nil {
		a.logger.Logf("error marking web merchant prompt seen for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
