package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

func (a *AppService) UserHasAcceptedPrivacyPolicy(ctx context.Context, id string) bool {
	accepted, err := a.db.UserHasAcceptedPrivacyPolicy(ctx, id)
	if err != nil {
		a.logger.Logf("error checking privacy-policy acceptance for user %s: %s", id, err)
		return false
	}

	return accepted
}

func (a *AppService) GetUserPolicyStatus(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	status, err := a.db.GetUserPolicyStatus(r.Context(), *userDid)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error getting user policy status for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.recordClientVersionObservation(r.Context(), *userDid, "user_policy_status", r)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}

func (a *AppService) AcceptUserPolicies(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading user policy acceptance request for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.UserPolicyAcceptanceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !req.AcceptedPrivacyPolicy {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("privacy policy acceptance is required"))
		return
	}

	// Empty is allowed and means "unchanged": clients that predate the signup
	// question send nothing, and so does every re-acceptance forced by a policy
	// version bump. Anything else must be one of the two we store — a value the
	// CHECK constraint would reject should fail here, as a 400 naming the field,
	// rather than as a 500 from the database.
	accountType := strings.TrimSpace(req.AccountType)
	if accountType != "" && !structs.IsValidAccountType(accountType) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("account_type must be 'regular' or 'merchant'"))
		return
	}

	currentStatus, err := a.db.GetUserPolicyStatus(r.Context(), *userDid)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading current user policy status for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !currentStatus.Active {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("inactive users must reactivate before accepting policies"))
		return
	}

	a.recordClientVersionObservation(r.Context(), *userDid, "user_policy_accept", r)

	status, err := a.db.AcceptUserPolicies(r.Context(), *userDid, req.MailingListOptIn, accountType, time.Now().UTC())
	if err != nil {
		switch err {
		case pgx.ErrNoRows:
			w.WriteHeader(http.StatusNotFound)
		case db.ErrUserPendingDeletion:
			w.WriteHeader(http.StatusConflict)
		default:
			a.logger.Logf("error accepting policies for user %s: %s", *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}
