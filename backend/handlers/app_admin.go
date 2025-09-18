package handlers

import (
	"context"
	"encoding/json"
	"fmt"
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

	err = a.db.UpdateLocationApproval(r.Context(), u.Id, u.Approval)
	if err != nil {
		a.logger.Logf("error updating location approval for location %d: %t", u.Id, u.Approval)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (a *AppService) IsAdmin(ctx context.Context, id string) bool {
	fmt.Println("reached is admin")
	isAdmin, err := a.db.IsAdmin(ctx, id)
	if err != nil {
		a.logger.Logf("error getting admin state for user %s: %s", id, err)
		return false
	}

	return isAdmin
}
