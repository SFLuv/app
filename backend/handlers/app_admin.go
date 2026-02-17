package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

func (a *AppService) GetUsers(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if !a.IsAdmin(r.Context(), *userDid) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	params := r.URL.Query()
	page, err := strconv.Atoi(params.Get("page"))
	if err != nil || page < 0 {
		page = 0
	}
	count, err := strconv.Atoi(params.Get("count"))
	if err != nil || count <= 0 || count > 500 {
		count = 100
	}

	users, err := a.db.GetUsers(r.Context(), page, count)
	if err != nil {
		a.logger.Logf("error getting users: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(users)
}

func (a *AppService) UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if !a.IsAdmin(r.Context(), *userDid) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update user role body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req struct {
		UserId string `json:"user_id"`
		Role   string `json:"role"`
		Value  bool   `json:"value"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.UserId == "" || req.Role == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.db.UpdateUserRole(r.Context(), req.UserId, req.Role, req.Value); err != nil {
		a.logger.Logf("error updating user role: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
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
