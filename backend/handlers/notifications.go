package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
)

// The notification screen's endpoints.
//
// One feed for everything a person was sent, so the app has one list to render
// and one place to look. Ordinary notifications and internal alerts differ only
// in how they retire, and that difference is expressed in the data rather than
// in separate endpoints: an ordinary notification is done once it has been
// looked at, an alert is done when it is dealt with.

// GetNotifications returns the notification screen.
//
// Active and history come back together on the first call, so opening the
// screen is one request. Later pages of history come from this same endpoint
// with a cursor, which re-sends the active list — a few rows, and it keeps the
// two halves consistent with each other as the user scrolls.
func (a *AppService) GetNotifications(w http.ResponseWriter, r *http.Request) {
	did := utils.GetDid(r)
	if did == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	userId := *did

	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	feed, err := a.notificationFeed(r.Context(), userId, cursor, limit)
	if err != nil {
		a.logger.Logf("error building the notification feed for %s: %s", userId, err)
		http.Error(w, "could not load notifications", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, feed)
}

// notificationFeed loads the feed and decorates alerts with the buttons this
// particular viewer may use.
//
// The admin check is made once for the whole feed rather than per alert, and
// the buttons are filtered rather than the alerts: somebody designated for
// admin alerts who is not an admin still sees what is waiting, which is the
// point of designating them, and simply cannot decide it from here.
func (a *AppService) notificationFeed(
	ctx context.Context, userId string, cursor string, limit int,
) (*structs.NotificationFeed, error) {
	feed, err := a.db.GetNotificationFeed(ctx, userId, cursor, limit)
	if err != nil {
		return nil, err
	}

	isAdmin := a.IsAdmin(ctx, userId)
	labels := map[string]string{}
	for _, set := range structs.NotificationSetCatalog() {
		labels[set.Id] = set.Label
	}

	decorate := func(items []structs.UserNotification) {
		for i := range items {
			detail := items[i].Internal
			if detail == nil {
				continue
			}
			detail.SetLabel = labels[detail.Set]
			if detail.Resolved {
				continue
			}
			alert, live, err := a.db.GetInternalAlertById(ctx, detail.Id)
			if err != nil || alert == nil {
				continue
			}
			detail.Actions = actionsForAlert(alert, !live, isAdmin)
		}
	}
	decorate(feed.Active)
	decorate(feed.History)

	return feed, nil
}

// MarkNotificationsSeen is what opening the screen does.
//
// It covers ordinary notifications only — the database enforces that, not this
// handler — so viewing the screen cannot retire an alert that still needs
// somebody to act. That is the asymmetry the whole design rests on: a
// transaction notice has done its job once it has been read, and an affiliate
// waiting on a decision has not.
func (a *AppService) MarkNotificationsSeen(w http.ResponseWriter, r *http.Request) {
	did := utils.GetDid(r)
	if did == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	userId := *did

	var req structs.NotificationSeenRequest
	if r.Body != nil {
		// An empty body means "all", which is the common case from the app.
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req)
	}
	if len(req.Ids) == 0 {
		req.All = true
	}

	if _, err := a.db.MarkNotificationsSeen(r.Context(), userId, req.Ids, req.All); err != nil {
		a.logger.Logf("error marking notifications seen for %s: %s", userId, err)
		http.Error(w, "could not update notifications", http.StatusInternalServerError)
		return
	}

	feed, err := a.notificationFeed(r.Context(), userId, "", 0)
	if err != nil {
		a.logger.Logf("error reloading the notification feed for %s: %s", userId, err)
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, feed)
}

// PressNotificationAction runs one of an alert's buttons.
//
// The order of checks is the whole security story, and each one answers a
// different question:
//
//  1. Does this alert exist, and is it still live? A resolved alert's buttons
//     do nothing, so a stale screen cannot approve something twice.
//  2. Is this person designated for the alert's set? That is what makes it
//     their alert at all.
//  3. Does the action exist for this alert's KIND? Looked up server-side, so
//     the client cannot name an action belonging to a different kind.
//  4. Does the action need admin, and do they have it? Subscription is not
//     authority.
//  5. Claim the alert with a conditional update, then run.
//
// Step five is what makes two people pressing Approve at the same moment safe:
// exactly one claim wins, and the loser is told somebody got there first
// instead of the approval running twice.
func (a *AppService) PressNotificationAction(w http.ResponseWriter, r *http.Request) {
	did := utils.GetDid(r)
	if did == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	userId := *did

	notificationId, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "a notification id is required", http.StatusBadRequest)
		return
	}

	var req structs.NotificationActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		http.Error(w, "an action is required", http.StatusBadRequest)
		return
	}
	req.Action = strings.TrimSpace(req.Action)

	alert, live, err := a.db.GetInternalAlertById(r.Context(), notificationId)
	if err != nil {
		a.logger.Logf("error loading alert %d: %s", notificationId, err)
		http.Error(w, "could not load that notification", http.StatusInternalServerError)
		return
	}
	if alert == nil {
		http.Error(w, "no such notification", http.StatusNotFound)
		return
	}
	if !live {
		a.respondNotificationAction(w, r, userId, structs.NotificationActionResponse{
			Refused: true,
			Message: "That has already been dealt with.",
		})
		return
	}

	// Designation is what makes it theirs. Global admins pass, as they do on
	// every other guard in the app.
	receives, err := a.db.UserReceivesSet(r.Context(), alert.OrganizationId, userId, alert.Set)
	if err != nil {
		a.logger.Logf("error checking notification subscription for %s: %s", userId, err)
		http.Error(w, "could not check your access", http.StatusInternalServerError)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), userId)
	if !receives && !isAdmin {
		http.Error(w, "this notification is not addressed to you", http.StatusForbidden)
		return
	}

	spec, found := findNotificationAction(alert.Kind, req.Action)
	if !found {
		http.Error(w, "unknown action for this notification", http.StatusBadRequest)
		return
	}
	if spec.RequiresAdmin && !isAdmin {
		http.Error(w, "that action needs an administrator", http.StatusForbidden)
		return
	}

	press := notificationPress{
		Alert:  alert,
		UserId: userId,
		Actor:  a.notificationActor(r.Context(), userId),
	}

	// Claimed before the work runs, and released if the work fails. Claiming
	// afterwards would let two presses both run the approval and only disagree
	// about who got the credit.
	claimed, err := a.db.ResolveInternalAlertById(
		r.Context(), alert.Id, structs.ResolutionActed, userId, "",
	)
	if err != nil {
		a.logger.Logf("error claiming alert %d: %s", alert.Id, err)
		http.Error(w, "could not update that notification", http.StatusInternalServerError)
		return
	}
	if !claimed {
		a.respondNotificationAction(w, r, userId, structs.NotificationActionResponse{
			Refused: true,
			Message: "Somebody got there first.",
		})
		return
	}

	outcome, runErr := spec.Run(r.Context(), a, press)
	if runErr != nil {
		// Put it back. The condition is still true, so the alert belongs on the
		// board and its button has to keep working.
		if _, reopenErr := a.db.ReopenInternalAlert(r.Context(), alert.Id); reopenErr != nil {
			a.logger.Logf("error reopening alert %d after a failed action: %s", alert.Id, reopenErr)
		}
		if isRetryable(runErr) {
			a.respondNotificationAction(w, r, userId, structs.NotificationActionResponse{
				Refused: true,
				Message: runErr.Error(),
			})
			return
		}
		a.logger.Logf("error running %s on alert %d: %s", req.Action, alert.Id, runErr)
		http.Error(w, "that did not work — nothing has changed", http.StatusInternalServerError)
		return
	}

	// The outcome sentence is written now rather than at claim time, because
	// until the action has run there is nothing true to say about it.
	if err := a.db.SetInternalAlertOutcome(r.Context(), alert.Id, outcome); err != nil {
		a.logger.Logf("error recording the outcome of alert %d: %s", alert.Id, err)
	}

	a.respondNotificationAction(w, r, userId, structs.NotificationActionResponse{
		Ok:      true,
		Message: outcome,
	})
}

// respondNotificationAction sends the result along with the caller's refreshed
// feed, so a press needs no follow-up read and the list cannot be left showing
// an alert that is gone.
func (a *AppService) respondNotificationAction(
	w http.ResponseWriter, r *http.Request, userId string, response structs.NotificationActionResponse,
) {
	if feed, err := a.notificationFeed(r.Context(), userId, "", 0); err == nil {
		response.Feed = feed
	}
	writeJSON(w, http.StatusOK, response)
}

// notificationActor is how a presser is named in the history.
//
// A name, falling back to an email, falling back to the user id. The id is a
// poor answer but it is never wrong, and "Approved by" with nothing after it
// is worse than "Approved by did:privy:…".
func (a *AppService) notificationActor(ctx context.Context, userId string) string {
	identities, err := a.db.GetUserIdentities(ctx, []string{userId})
	if err != nil {
		return userId
	}
	identity, found := identities[userId]
	if !found {
		return userId
	}
	if name := strings.TrimSpace(identity.Name); name != "" {
		return name
	}
	if email := strings.TrimSpace(identity.Email); email != "" {
		return email
	}
	return userId
}

// ── Organization settings ────────────────────────────────────────────────────

// GetOrganizationNotificationSettings returns who receives which sets.
//
// Readable by any member, so somebody can see what they themselves are
// designated for without being an admin. Writing is admin-only.
func (a *AppService) GetOrganizationNotificationSettings(w http.ResponseWriter, r *http.Request) {
	org, role, _, ok := a.requireOrgRole(w, r, structs.OrgRoleMember)
	if !ok {
		return
	}
	if !org.IsHouse {
		// Deliberately not a 403. Partner organizations will have their own
		// sets later, and until then the honest answer is that there are none
		// rather than that the caller is not allowed to ask.
		writeJSON(w, http.StatusOK, structs.OrganizationNotificationSettings{
			Catalog:    []structs.NotificationSetDefinition{},
			Recipients: []structs.OrganizationNotificationRecipient{},
			LiveCounts: map[string]int{},
		})
		return
	}

	members, err := a.db.GetOrganizationMembersWithPermissions(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("error loading members for org %d: %s", org.Id, err)
		http.Error(w, "could not load members", http.StatusInternalServerError)
		return
	}
	subscriptions, err := a.db.GetOrganizationNotificationSubscriptions(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("error loading notification subscriptions for org %d: %s", org.Id, err)
		http.Error(w, "could not load subscriptions", http.StatusInternalServerError)
		return
	}
	counts, err := a.db.LiveInternalAlertCounts(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("error counting live alerts for org %d: %s", org.Id, err)
		counts = map[string]int{}
	}

	out := structs.OrganizationNotificationSettings{
		Catalog:    structs.NotificationSetCatalog(),
		Recipients: make([]structs.OrganizationNotificationRecipient, 0, len(members)),
		CanEdit:    role == structs.OrgRoleAdmin || role == structs.OrgRoleSuperadmin,
		LiveCounts: counts,
	}
	for _, member := range members {
		sets := subscriptions[member.UserId]
		if sets == nil {
			sets = []string{}
		}
		out.Recipients = append(out.Recipients, structs.OrganizationNotificationRecipient{
			UserId: member.UserId,
			Name:   member.ContactName,
			Email:  member.ContactEmail,
			Role:   member.Role,
			Sets:   sets,
		})
	}

	writeJSON(w, http.StatusOK, out)
}

// SetOrganizationNotificationSets replaces one member's subscriptions.
func (a *AppService) SetOrganizationNotificationSets(w http.ResponseWriter, r *http.Request) {
	org, _, actor, ok := a.requireOrgRole(w, r, structs.OrgRoleAdmin)
	if !ok {
		return
	}
	if !org.IsHouse {
		http.Error(w, "notification sets exist only for the SFLuv organization", http.StatusForbidden)
		return
	}

	var req structs.SetOrganizationNotificationSetsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
		http.Error(w, "a user and their sets are required", http.StatusBadRequest)
		return
	}
	req.UserId = strings.TrimSpace(req.UserId)
	if req.UserId == "" {
		http.Error(w, "a user is required", http.StatusBadRequest)
		return
	}
	for _, set := range req.Sets {
		if !structs.IsNotificationSet(strings.TrimSpace(set)) {
			http.Error(w, "unknown notification set: "+set, http.StatusBadRequest)
			return
		}
	}

	// Only a member of this organization, so an admin cannot subscribe an
	// arbitrary user id to the alerts by posting one.
	role, err := a.db.GetOrganizationMemberRole(r.Context(), org.Id, req.UserId)
	if err != nil {
		a.logger.Logf("error loading member role: %s", err)
		http.Error(w, "could not check that member", http.StatusInternalServerError)
		return
	}
	if role == "" {
		http.Error(w, "that user is not a member of this organization", http.StatusBadRequest)
		return
	}

	if err := a.db.SetOrganizationNotificationSets(r.Context(), org.Id, req.UserId, req.Sets, actor); err != nil {
		a.logger.Logf("error saving notification subscriptions: %s", err)
		http.Error(w, "could not save that", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
