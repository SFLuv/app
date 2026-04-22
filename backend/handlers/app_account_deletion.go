package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

func accountPurgeEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ACCOUNT_PURGE_ENABLED")), "true")
}

func (a *AppService) UserIsActive(ctx context.Context, id string) bool {
	active, err := a.db.UserIsActive(ctx, id)
	if err != nil {
		a.logger.Logf("error checking active user state for %s: %s", id, err)
		return false
	}
	return active
}

func (a *AppService) GetDeleteAccountPreview(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	preview, err := a.db.GetAccountDeletionPreview(r.Context(), *userDid, time.Now().UTC())
	if err != nil {
		if err == db.ErrUserPendingDeletion {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error building delete-account preview for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	preview.PurgeEnabled = accountPurgeEnabled()
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(preview)
}

func (a *AppService) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	status, err := a.db.ScheduleAccountDeletion(r.Context(), *userDid, time.Now().UTC())
	if err != nil {
		if err == db.ErrUserPendingDeletion {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error scheduling account deletion for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := a.revokeDeletedUserAppleAccess(r.Context(), *userDid); err != nil {
		a.logger.Logf("error revoking apple access at delete initiation for user %s: %s", *userDid, err)
	}

	status.PurgeEnabled = accountPurgeEnabled()
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(status)
}

func (a *AppService) CancelDeleteAccount(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	status, err := a.db.CancelAccountDeletion(r.Context(), *userDid, time.Now().UTC())
	if err != nil {
		switch err {
		case db.ErrUserDeletionNotScheduled:
			w.WriteHeader(http.StatusConflict)
			return
		case db.ErrUserDeletionWindowExpired:
			w.WriteHeader(http.StatusGone)
			return
		case pgx.ErrNoRows:
			w.WriteHeader(http.StatusNotFound)
			return
		default:
			a.logger.Logf("error canceling account deletion for user %s: %s", *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	status.PurgeEnabled = accountPurgeEnabled()
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}

func (a *AppService) GetDeleteAccountStatus(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	status, err := a.db.GetAccountDeletionStatus(r.Context(), *userDid, time.Now().UTC())
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error getting delete-account status for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	status.PurgeEnabled = accountPurgeEnabled()
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}

func (a *AppService) PurgeDeletedAccounts(ctx context.Context, now time.Time) (int, error) {
	if !accountPurgeEnabled() {
		return 0, nil
	}

	userIDs, err := a.db.ListUsersReadyForPurge(ctx, now.UTC())
	if err != nil {
		return 0, err
	}

	purged := 0
	for _, userID := range userIDs {
		// Apple access should already be revoked and cleared when deletion is initiated,
		// but keep this call as a no-op/idempotent safeguard for older rows.
		if err := a.revokeDeletedUserAppleAccess(ctx, userID); err != nil {
			return purged, err
		}
		if err := a.db.PurgeDeletedUser(ctx, userID, now.UTC()); err != nil {
			return purged, err
		}
		purged++
	}

	return purged, nil
}

func (a *AppService) PurgeDeletedAccountsManual(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	now := time.Now().UTC()

	if userID != "" {
		if err := a.revokeDeletedUserAppleAccess(r.Context(), userID); err != nil {
			a.logger.Logf("error revoking apple access for purged user %s: %s", userID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := a.db.PurgeDeletedUser(r.Context(), userID, now); err != nil {
			switch err {
			case db.ErrUserDeletionNotScheduled, db.ErrUserDeletionWindowExpired:
				w.WriteHeader(http.StatusConflict)
				return
			case pgx.ErrNoRows:
				w.WriteHeader(http.StatusNotFound)
				return
			default:
				a.logger.Logf("error manually purging deleted user %s: %s", userID, err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"purged":   1,
			"user_ids": []string{userID},
		})
		return
	}

	userIDs, err := a.db.ListUsersReadyForPurge(r.Context(), now)
	if err != nil {
		a.logger.Logf("error listing purge-ready users: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	purgedUserIDs := make([]string, 0, len(userIDs))
	for _, candidateID := range userIDs {
		if err := a.revokeDeletedUserAppleAccess(r.Context(), candidateID); err != nil {
			a.logger.Logf("error revoking apple access for purged user %s: %s", candidateID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := a.db.PurgeDeletedUser(r.Context(), candidateID, now); err != nil {
			a.logger.Logf("error manually purging deleted user %s: %s", candidateID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		purgedUserIDs = append(purgedUserIDs, candidateID)
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"purged":   len(purgedUserIDs),
		"user_ids": purgedUserIDs,
	})
}

func (a *AppService) revokeDeletedUserAppleAccess(ctx context.Context, userID string) error {
	return a.revokeStoredAppleCredential(ctx, userID)
}
