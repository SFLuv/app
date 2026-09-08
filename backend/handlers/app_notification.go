package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// GetImproverNotifications backs the notification bell: the full feed plus the
// counts for the unread bubble and the improver tab dot.
//
// Seen notifications stay in the feed until the thing they describe is actually
// resolved — for a pending payout, until the payout settles. Marking one seen
// only silences the bubble; it never hides work that is still outstanding.
func (a *AppService) GetImproverNotifications(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// A pending payout may have settled since the last read; reconciling first
	// means a resolved notification disappears rather than lingering until the
	// next scheduled sweep.
	a.recoverStalePayoutLocksForImprover(*userDid, staleWorkflowPayoutLockCutoff())

	feed, err := a.db.GetImproverNotifications(r.Context(), *userDid)
	if err != nil {
		if !isClientGone(err) {
			a.logger.Logf("error loading notifications for improver %s: %s", *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(feed)
}

// MarkImproverNotificationsSeen clears the unread state for specific
// notifications, or for the whole feed when the request asks for all of them
// (what opening the bell does).
func (a *AppService) MarkImproverNotificationsSeen(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading notification seen body for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.ImproverNotificationSeenRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	keys := req.Keys
	if req.All || len(keys) == 0 {
		// Resolve "all" against the feed as it is right now, so a notification
		// that appears after this call is still reported as unread.
		feed, err := a.db.GetImproverNotifications(r.Context(), *userDid)
		if err != nil {
			if !isClientGone(err) {
				a.logger.Logf("error loading notifications to mark seen for %s: %s", *userDid, err)
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		keys = make([]string, 0, len(feed.Notifications))
		for _, notification := range feed.Notifications {
			if !notification.Seen {
				keys = append(keys, notification.Key)
			}
		}
	}

	if _, err := a.db.MarkImproverNotificationsSeen(r.Context(), *userDid, keys); err != nil {
		if !isClientGone(err) {
			a.logger.Logf("error marking notifications seen for %s: %s", *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	feed, err := a.db.GetImproverNotifications(r.Context(), *userDid)
	if err != nil {
		if !isClientGone(err) {
			a.logger.Logf("error reloading notifications after marking seen for %s: %s", *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(feed)
}
