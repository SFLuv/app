package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppService) GetUsers(w http.ResponseWriter, r *http.Request) {

}

func (a *AppService) UpdateUserRole(w http.ResponseWriter, r *http.Request) {

}

func (a *AppService) UpdateLocationApproval(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update location approval body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var u structs.UpdateLocationApprovalRequest
	err = json.Unmarshal(body, &u)
	if err != nil {
		a.logger.Logf("error unmarshalling update location approval body: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ownerID, wasApproved, err := a.db.GetLocationOwnerAndApproval(r.Context(), u.Id)
	if err != nil {
		a.logger.Logf("error loading location %d owner/approval state: %s", u.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	hadOtherApprovedLocations := false
	if u.Approval {
		hadOtherApprovedLocations, err = a.db.OwnerHasApprovedLocationExcluding(r.Context(), ownerID, u.Id)
		if err != nil {
			a.logger.Logf("error checking existing approved locations for owner %s: %s", ownerID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	err = a.db.UpdateLocationApproval(r.Context(), u.Id, u.Approval)
	if err != nil {
		a.logger.Logf("error updating location approval for location %d: %t", u.Id, u.Approval)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if a.redeemer != nil && a.redeemer.IsEnabled() && u.Approval && !wasApproved && !hadOtherApprovedLocations {
		if err := a.redeemer.EnsureMerchantHasRedeemerWallet(r.Context(), ownerID); err != nil {
			a.logger.Logf("error auto-granting redeemer role for user %s after location %d approval: %s", ownerID, u.Id, err)
		}
	}

	w.WriteHeader(http.StatusCreated)
}

func (a *AppService) IsAdmin(ctx context.Context, id string) bool {
	isAdmin, err := a.db.IsAdmin(ctx, id)
	if err != nil {
		a.logger.Logf("error getting admin state for user %s: %s", id, err)
		return false
	}

	return isAdmin
}
