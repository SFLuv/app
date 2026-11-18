package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
)

// The notification hub's settings: teams, who is on them, and which topics go
// to which team. House organization only. Members can read it, so anybody can
// see what they are on and why; admins of the organization can change it.

const (
	notificationTeamMaxName        = 80
	notificationTeamMaxDescription = 300
	notificationHubRecent          = 60
)

// notify posts to the hub and logs a failure. Every touchpoint goes through
// here: a notification that could not be posted must not fail the request that
// raised it, which would make our problem the person's, but it must not
// disappear quietly either.
func (a *AppService) notify(ctx context.Context, alert Alert) {
	if a.notifications == nil {
		a.logger.Logf("notifications: %s not posted — the hub is not running", alert.Key)
		return
	}
	if err := a.notifications.Post(ctx, alert); err != nil {
		a.logger.Logf("notifications: could not post %s (%s): %s", alert.Key, alert.Topic, err)
	}
}

// GetNotificationHub returns the hub's settings and its recent notifications.
func (a *AppService) GetNotificationHub(w http.ResponseWriter, r *http.Request) {
	org, role, _, ok := a.requireOrgRole(w, r, structs.OrgRoleMember)
	if !ok {
		return
	}
	if !org.IsHouse {
		http.Error(w, "notification teams belong to the SFLuv organization", http.StatusForbidden)
		return
	}
	ctx := r.Context()

	teams, err := a.db.ListNotificationTeams(ctx, org.Id)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not load teams", http.StatusInternalServerError)
		return
	}
	members, err := a.db.GetOrganizationMembersWithPermissions(ctx, org.Id)
	if err != nil {
		a.logger.Logf("notification hub: error loading members for org %d: %s", org.Id, err)
		http.Error(w, "could not load members", http.StatusInternalServerError)
		return
	}
	routes, err := a.db.GetNotificationRoutes(ctx, org.Id)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not load routes", http.StatusInternalServerError)
		return
	}

	// The rest is decoration. A failure leaves the settings editable and the
	// missing part empty.
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserId)
	}
	devices, err := a.db.PushDeviceCounts(ctx, ids)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		devices = map[string]int{}
	}
	counts, err := a.db.LiveInternalAlertCountsByTopic(ctx, org.Id)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		counts = map[string]int{}
	}
	recent, err := a.db.RecentInternalAlerts(ctx, org.Id, notificationHubRecent)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		recent = []structs.NotificationHubEntry{}
	}
	for i := range recent {
		if topic, ok := structs.NotificationTopic(recent[i].Topic); ok {
			recent[i].TopicLabel = topic.Label
		}
	}
	nobody, err := a.db.CountReachedNobody(ctx, org.Id, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
	}
	activity, err := a.db.NotificationTopicActivity(ctx, org.Id)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		activity = map[string]structs.NotificationTopicActivity{}
	}

	view := structs.NotificationHubView{
		Sets:          structs.NotificationSetCatalog(),
		Topics:        structs.NotificationTopicCatalog(),
		Teams:         teams,
		Members:       make([]structs.NotificationHubMember, 0, len(members)),
		Routes:        routes,
		LiveCounts:    counts,
		Recent:        recent,
		ReachedNobody: nobody,
		CanEdit:       role == structs.OrgRoleAdmin || role == structs.OrgRoleSuperadmin,
		InboundEmail:  inboundEmailConfigured(),
		Touchpoints:   structs.NotificationTouchpointCatalog(),
		TopicActivity: activity,
		GeneratedAt:   time.Now().UTC(),
	}
	for _, member := range members {
		view.Members = append(view.Members, structs.NotificationHubMember{
			UserId:  member.UserId,
			Name:    member.ContactName,
			Email:   member.ContactEmail,
			Role:    member.Role,
			Devices: devices[member.UserId],
		})
	}
	writeJSON(w, http.StatusOK, view)
}

// requireHubAdmin is the guard for every write: an admin of the house
// organization.
func (a *AppService) requireHubAdmin(w http.ResponseWriter, r *http.Request) (*structs.Organization, string, bool) {
	org, _, actor, ok := a.requireOrgRole(w, r, structs.OrgRoleAdmin)
	if !ok {
		return nil, "", false
	}
	if !org.IsHouse {
		http.Error(w, "notification teams belong to the SFLuv organization", http.StatusForbidden)
		return nil, "", false
	}
	return org, actor, true
}

// readTeamInput decodes and checks a team's details, writing the refusal when
// they are not usable.
func (a *AppService) readTeamInput(w http.ResponseWriter, r *http.Request) (structs.NotificationTeamInput, bool) {
	var input structs.NotificationTeamInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil {
		http.Error(w, "a team needs a name", http.StatusBadRequest)
		return input, false
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.EmailAddress = strings.TrimSpace(input.EmailAddress)
	if input.Name == "" {
		http.Error(w, "a team needs a name", http.StatusBadRequest)
		return input, false
	}
	if len([]rune(input.Name)) > notificationTeamMaxName {
		http.Error(w, "that name is too long", http.StatusBadRequest)
		return input, false
	}
	if len([]rune(input.Description)) > notificationTeamMaxDescription {
		http.Error(w, "that description is too long", http.StatusBadRequest)
		return input, false
	}
	if input.EmailAddress != "" {
		parsed, err := mail.ParseAddress(input.EmailAddress)
		if err != nil {
			http.Error(w, "that email address does not look right", http.StatusBadRequest)
			return input, false
		}
		input.EmailAddress = parsed.Address
	}
	if input.ChatConversationId != nil {
		if *input.ChatConversationId <= 0 {
			input.ChatConversationId = nil
		} else if _, err := a.db.GetGroupConversation(r.Context(), *input.ChatConversationId); err != nil {
			http.Error(w, "there is no group chat with that id", http.StatusBadRequest)
			return input, false
		}
	}
	return input, true
}

// CreateNotificationTeam adds a team.
func (a *AppService) CreateNotificationTeam(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := a.requireHubAdmin(w, r)
	if !ok {
		return
	}
	input, ok := a.readTeamInput(w, r)
	if !ok {
		return
	}
	id, err := a.db.CreateNotificationTeam(r.Context(), org.Id, input, actor)
	if errors.Is(err, db.ErrNotificationTeamNameTaken) {
		http.Error(w, "there is already a team called that", http.StatusConflict)
		return
	}
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not create that team", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func teamIdParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "a team id is required", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// UpdateNotificationTeam changes a team's name, description and channels.
func (a *AppService) UpdateNotificationTeam(w http.ResponseWriter, r *http.Request) {
	org, _, ok := a.requireHubAdmin(w, r)
	if !ok {
		return
	}
	teamId, ok := teamIdParam(w, r)
	if !ok {
		return
	}
	input, ok := a.readTeamInput(w, r)
	if !ok {
		return
	}
	found, err := a.db.UpdateNotificationTeam(r.Context(), org.Id, teamId, input)
	if errors.Is(err, db.ErrNotificationTeamNameTaken) {
		http.Error(w, "there is already a team called that", http.StatusConflict)
		return
	}
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not save that team", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "no such team", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// DeleteNotificationTeam removes a team. Its topics are left with whatever other
// teams they have, and the screen says when that is none.
func (a *AppService) DeleteNotificationTeam(w http.ResponseWriter, r *http.Request) {
	org, _, ok := a.requireHubAdmin(w, r)
	if !ok {
		return
	}
	teamId, ok := teamIdParam(w, r)
	if !ok {
		return
	}
	found, err := a.db.DeleteNotificationTeam(r.Context(), org.Id, teamId)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not delete that team", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "no such team", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// SetNotificationTeamMembers replaces a team's members. Every one must be a
// member of the house organization.
func (a *AppService) SetNotificationTeamMembers(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := a.requireHubAdmin(w, r)
	if !ok {
		return
	}
	teamId, ok := teamIdParam(w, r)
	if !ok {
		return
	}
	var req structs.NotificationTeamMembersRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "a list of members is required", http.StatusBadRequest)
		return
	}

	members, err := a.db.GetOrganizationMembersWithPermissions(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("notification hub: error loading members for org %d: %s", org.Id, err)
		http.Error(w, "could not check those members", http.StatusInternalServerError)
		return
	}
	inOrg := map[string]bool{}
	for _, member := range members {
		inOrg[member.UserId] = true
	}
	userIds := []string{}
	seen := map[string]bool{}
	for _, userId := range req.UserIds {
		userId = strings.TrimSpace(userId)
		if userId == "" || seen[userId] {
			continue
		}
		if !inOrg[userId] {
			http.Error(w, "only members of the SFLuv organization can be on a team", http.StatusBadRequest)
			return
		}
		seen[userId] = true
		userIds = append(userIds, userId)
	}

	found, err := a.db.SetNotificationTeamMembers(r.Context(), org.Id, teamId, userIds, actor)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not save those members", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "no such team", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// SetNotificationRoute replaces which teams receive one topic.
func (a *AppService) SetNotificationRoute(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := a.requireHubAdmin(w, r)
	if !ok {
		return
	}
	var req structs.NotificationRouteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		http.Error(w, "a topic and its teams are required", http.StatusBadRequest)
		return
	}
	req.Topic = strings.TrimSpace(req.Topic)
	if !structs.IsRoutableTopic(req.Topic) {
		http.Error(w, "unknown topic: "+req.Topic, http.StatusBadRequest)
		return
	}

	teams, err := a.db.ListNotificationTeams(r.Context(), org.Id)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not check those teams", http.StatusInternalServerError)
		return
	}
	known := map[int64]bool{}
	for _, team := range teams {
		known[team.Id] = true
	}
	teamIds := []int64{}
	for _, id := range req.TeamIds {
		if !known[id] {
			http.Error(w, "no such team: "+strconv.FormatInt(id, 10), http.StatusBadRequest)
			return
		}
		teamIds = append(teamIds, id)
	}

	if err := a.db.SetNotificationRoute(r.Context(), org.Id, req.Topic, teamIds, actor); err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not save that", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
