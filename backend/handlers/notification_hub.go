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
// to which team. House organization only, and only its admins: the web app's
// Notification Hub page is theirs. Members still see which teams they are on
// with each notification they receive.

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
	access, ok := a.requireHubAccess(w, r)
	if !ok {
		return
	}
	org := access.org
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
		CanEdit:       access.admin,
		ManagedTeams:  access.teams,
		MyUserId:      access.actor,
		InboundEmail:  inboundEmailConfigured(),
		Touchpoints:   structs.NotificationTouchpointCatalog(),
		TopicActivity: activity,
		GeneratedAt:   time.Now().UTC(),
	}
	if !access.admin {
		scopeHubView(&view, access.teams)
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

// UpdateNotificationTeam renames a team or changes its description.
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

// notificationTestResult is what a test send reports back: where the hub sent
// it, worked out the same way a real notification's delivery is.
type notificationTestResult struct {
	Topic         string                             `json:"topic"`
	Teams         []structs.NotificationDeliveryTeam `json:"teams"`
	CatchAll      bool                               `json:"catch_all"`
	People        int                                `json:"people"`
	Emails        int                                `json:"emails"`
	Chats         int                                `json:"chats"`
	ReachedNobody bool                               `json:"reached_nobody"`
}

// SendTestNotification posts a test of one topic through the hub, so it goes
// wherever a real one would: its teams, or the catch-all's, by app, push,
// email and chat. It files itself to history, like activity, so it never sits
// in anybody's list waiting to be handled.
func (a *AppService) SendTestNotification(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireHubAccess(w, r)
	if !ok {
		return
	}
	org, actor := access.org, access.actor
	var req struct {
		Topic string `json:"topic"`
		// TeamId sends it to one team only, through that team's own rules.
		// Leads and team admins test this way; without it the test goes
		// wherever a real one would, which only an organization admin can do.
		TeamId int64 `json:"team_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		http.Error(w, "a topic is required", http.StatusBadRequest)
		return
	}
	topic, known := structs.NotificationTopic(strings.TrimSpace(req.Topic))
	if !known {
		http.Error(w, "unknown topic: "+req.Topic, http.StatusBadRequest)
		return
	}
	if a.notifications == nil {
		http.Error(w, "the notification hub is not running", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()

	if req.TeamId == 0 && !access.admin {
		http.Error(w, "choose one of your teams to test", http.StatusForbidden)
		return
	}
	if req.TeamId != 0 && !access.canManage(req.TeamId) {
		http.Error(w, "only this team's lead and admins, or an organization admin, can test it", http.StatusForbidden)
		return
	}
	var plan *db.NotificationDeliveryPlan
	var err error
	if req.TeamId != 0 {
		plan, err = a.db.NotificationTeamPlan(ctx, org.Id, req.TeamId, topic.Id)
	} else {
		plan, err = a.db.NotificationDeliveryPlan(ctx, org.Id, topic.Id)
	}
	if err != nil {
		a.logger.Logf("notification hub: test of %s: %s", topic.Id, err)
		http.Error(w, "could not work out where it would go", http.StatusInternalServerError)
		return
	}

	sender := "An organization admin"
	if user, err := a.db.GetUserById(ctx, actor); err == nil && user != nil {
		if user.Name != nil && strings.TrimSpace(*user.Name) != "" {
			sender = strings.TrimSpace(*user.Name)
		} else if user.Email != nil && strings.TrimSpace(*user.Email) != "" {
			sender = strings.TrimSpace(*user.Email)
		}
	}

	alert := Alert{
		Topic: topic.Id,
		// Unique per send: two tests are two notifications, not one repeated.
		Key:      "test:" + topic.Id + ":" + strconv.FormatInt(time.Now().UnixNano(), 36),
		Kind:     structs.InternalKindNotice,
		Title:    "Test: " + topic.Label,
		Subtitle: "Test from " + sender,
		Body:     "A test from the Notification Hub, to check where " + topic.Label + " notifications go. Nothing needs doing.",
		Fields: []structs.NotificationField{
			{Label: "Topic", Value: topic.Label},
			{Label: "Sent by", Value: sender},
		},
		Informational: true,
		ChatLine:      "Test notification\nSent by " + sender + " from the Notification Hub",
		OnlyTeamId:    req.TeamId,
	}
	if err := a.notifications.Post(ctx, alert); err != nil {
		a.logger.Logf("notification hub: test of %s: %s", topic.Id, err)
		http.Error(w, "could not send the test", http.StatusInternalServerError)
		return
	}

	result := notificationTestResult{
		Topic:         topic.Id,
		Teams:         plan.Teams,
		CatchAll:      plan.CatchAll,
		People:        len(plan.Members),
		Emails:        len(plan.Emails),
		Chats:         len(plan.Chats),
		ReachedNobody: plan.Empty(),
	}
	if result.Teams == nil {
		result.Teams = []structs.NotificationDeliveryTeam{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// SearchNotificationChats finds group chats a team can post into, by name (or
// by id, typed as a number). Only chats with at least one current member of the
// house organization are offered: a team's chat is an internal one.
func (a *AppService) SearchNotificationChats(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireHubAccess(w, r)
	if !ok {
		return
	}
	org := access.org
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) > 100 {
		query = string([]rune(query)[:100])
	}
	chats, err := a.db.SearchNotificationChats(r.Context(), org.Id, query, 20)
	if err != nil {
		a.logger.Logf("notification hub: chat search: %s", err)
		http.Error(w, "could not search the chats", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(chats)
}

// hubAccess is who is looking at the hub: an admin of the house organization,
// who can change anything, or the lead or an admin of some of its teams, who
// can change where things go inside those teams.
type hubAccess struct {
	org   *structs.Organization
	actor string
	admin bool
	// teams is team id → "lead" or "admin", for the teams the viewer manages.
	// Every team, as "lead", for an organization admin.
	teams map[int64]string
}

func (h *hubAccess) canManage(teamId int64) bool { return h.admin || h.teams[teamId] != "" }

// canChooseAdmins is the one thing a team admin cannot do that its lead can.
func (h *hubAccess) canChooseAdmins(teamId int64) bool {
	return h.admin || h.teams[teamId] == "lead"
}

// requireHubAccess admits the house organization's admins and the leads and
// admins of its teams.
func (a *AppService) requireHubAccess(w http.ResponseWriter, r *http.Request) (*hubAccess, bool) {
	org, role, actor, ok := a.requireOrgRole(w, r, structs.OrgRoleMember)
	if !ok {
		return nil, false
	}
	if !org.IsHouse {
		http.Error(w, "notification teams belong to the SFLuv organization", http.StatusForbidden)
		return nil, false
	}
	access := &hubAccess{
		org:   org,
		actor: actor,
		admin: role == structs.OrgRoleAdmin || role == structs.OrgRoleSuperadmin,
		teams: map[int64]string{},
	}
	roles, err := a.db.NotificationTeamRoles(r.Context(), org.Id, actor)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not check your teams", http.StatusInternalServerError)
		return nil, false
	}
	access.teams = roles
	if !access.admin && len(roles) == 0 {
		http.Error(w, "the notification hub is for SFLuv's organization admins and team leads", http.StatusForbidden)
		return nil, false
	}
	return access, true
}

// scopeHubView narrows the hub to the teams one lead or team admin manages:
// those teams, the topics routed to them, and the notifications they got.
func scopeHubView(view *structs.NotificationHubView, managed map[int64]string) {
	teams := []structs.NotificationTeam{}
	for _, team := range view.Teams {
		if managed[team.Id] != "" {
			teams = append(teams, team)
		}
	}
	view.Teams = teams

	routes := map[string][]int64{}
	for topic, ids := range view.Routes {
		kept := []int64{}
		for _, id := range ids {
			if managed[id] != "" {
				kept = append(kept, id)
			}
		}
		if len(kept) > 0 {
			routes[topic] = kept
		}
	}
	view.Routes = routes

	counts := map[string]int{}
	for topic, count := range view.LiveCounts {
		if len(routes[topic]) > 0 {
			counts[topic] = count
		}
	}
	view.LiveCounts = counts

	recent := []structs.NotificationHubEntry{}
	for _, entry := range view.Recent {
		if entry.Delivery == nil {
			continue
		}
		for _, team := range entry.Delivery.Teams {
			if managed[team.Id] != "" {
				recent = append(recent, entry)
				break
			}
		}
	}
	view.Recent = recent
	// Across the whole hub, so not theirs to see.
	view.ReachedNobody = 0
}

// teamForAccess loads one team for a change by somebody who manages it.
func (a *AppService) teamForAccess(w http.ResponseWriter, r *http.Request, access *hubAccess) (*structs.NotificationTeam, bool) {
	teamId, ok := teamIdParam(w, r)
	if !ok {
		return nil, false
	}
	if !access.canManage(teamId) {
		http.Error(w, "only this team's lead and admins, or an organization admin, can change that", http.StatusForbidden)
		return nil, false
	}
	teams, err := a.db.ListNotificationTeams(r.Context(), access.org.Id)
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not load that team", http.StatusInternalServerError)
		return nil, false
	}
	for i := range teams {
		if teams[i].Id == teamId {
			return &teams[i], true
		}
	}
	http.Error(w, "no such team", http.StatusNotFound)
	return nil, false
}

// SetNotificationTeamLead names a team's lead. Organization admins only.
func (a *AppService) SetNotificationTeamLead(w http.ResponseWriter, r *http.Request) {
	org, _, ok := a.requireHubAdmin(w, r)
	if !ok {
		return
	}
	teamId, ok := teamIdParam(w, r)
	if !ok {
		return
	}
	var req structs.NotificationTeamLeadRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil || strings.TrimSpace(req.UserId) == "" {
		http.Error(w, "choose somebody on the team", http.StatusBadRequest)
		return
	}
	found, err := a.db.SetNotificationTeamLead(r.Context(), org.Id, teamId, strings.TrimSpace(req.UserId))
	if err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not change the lead", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "the lead has to be on the team", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// SetNotificationTeamAdmins replaces a team's admins. The lead, or an
// organization admin.
func (a *AppService) SetNotificationTeamAdmins(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireHubAccess(w, r)
	if !ok {
		return
	}
	team, ok := a.teamForAccess(w, r, access)
	if !ok {
		return
	}
	if !access.canChooseAdmins(team.Id) {
		http.Error(w, "only the team's lead can choose its admins", http.StatusForbidden)
		return
	}
	var req structs.NotificationTeamAdminsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "a list of people is required", http.StatusBadRequest)
		return
	}
	onTeam := map[string]bool{}
	for _, id := range team.MemberIds {
		onTeam[id] = true
	}
	ids := []string{}
	for _, id := range req.UserIds {
		id = strings.TrimSpace(id)
		if id == "" || id == team.LeadUserId {
			continue
		}
		if !onTeam[id] {
			http.Error(w, "team admins have to be on the team", http.StatusBadRequest)
			return
		}
		ids = append(ids, id)
	}
	if _, err := a.db.SetNotificationTeamAdmins(r.Context(), access.org.Id, team.Id, ids); err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not save the admins", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// SetNotificationTeamResources replaces a team's shared address and group
// chats: the places, besides its members, its rules can send to.
func (a *AppService) SetNotificationTeamResources(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireHubAccess(w, r)
	if !ok {
		return
	}
	team, ok := a.teamForAccess(w, r, access)
	if !ok {
		return
	}
	var req structs.NotificationTeamResourcesRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		http.Error(w, "an address and a list of chats are required", http.StatusBadRequest)
		return
	}
	address := strings.TrimSpace(req.EmailAddress)
	if address != "" {
		parsed, err := mail.ParseAddress(address)
		if err != nil {
			http.Error(w, "that email address does not look right", http.StatusBadRequest)
			return
		}
		address = parsed.Address
	}
	chatIds := []int64{}
	seen := map[int64]bool{}
	for _, id := range req.ChatIds {
		if id > 0 && !seen[id] {
			seen[id] = true
			chatIds = append(chatIds, id)
		}
	}
	if len(chatIds) > 25 {
		http.Error(w, "a team can post in at most 25 chats", http.StatusBadRequest)
		return
	}
	if len(chatIds) > 0 {
		valid, err := a.db.ValidNotificationChats(r.Context(), access.org.Id, chatIds)
		if err != nil {
			a.logger.Logf("notification hub: %s", err)
			http.Error(w, "could not check those chats", http.StatusInternalServerError)
			return
		}
		if len(valid) != len(chatIds) {
			http.Error(w, "a team can only post in group chats somebody from SFLuv is in", http.StatusBadRequest)
			return
		}
	}
	if _, err := a.db.SetNotificationTeamResources(r.Context(), access.org.Id, team.Id, address, chatIds, access.actor); err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not save that", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// SetNotificationTeamRule sets where one topic goes inside a team: the team's
// default, what comes through the catch-all, or a topic the team is given.
func (a *AppService) SetNotificationTeamRule(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireHubAccess(w, r)
	if !ok {
		return
	}
	team, ok := a.teamForAccess(w, r, access)
	if !ok {
		return
	}
	var req structs.NotificationTeamRuleRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "a topic is required", http.StatusBadRequest)
		return
	}
	topic := strings.TrimSpace(req.Topic)
	given := topic == structs.NotificationTeamRuleDefault
	for _, routed := range team.Topics {
		if routed == topic {
			given = true
		}
	}
	if !given {
		http.Error(w, "this team is not given that topic", http.StatusBadRequest)
		return
	}

	var rule *structs.NotificationTeamDestinations
	if req.Destinations != nil {
		onTeam := map[string]bool{}
		for _, id := range team.MemberIds {
			onTeam[id] = true
		}
		hasChat := map[int64]bool{}
		for _, chat := range team.Chats {
			hasChat[chat.Id] = true
		}
		clean := structs.NotificationTeamDestinations{
			Everyone:     req.Destinations.Everyone,
			Members:      []structs.NotificationMemberDestination{},
			EmailAddress: req.Destinations.EmailAddress && team.EmailAddress != "",
			Chats:        []int64{},
		}
		seen := map[string]bool{}
		for _, member := range req.Destinations.Members {
			if !onTeam[member.UserId] {
				http.Error(w, "a notification can only go to people on the team", http.StatusBadRequest)
				return
			}
			if !seen[member.UserId] {
				seen[member.UserId] = true
				clean.Members = append(clean.Members, member)
			}
		}
		seenChat := map[int64]bool{}
		for _, id := range req.Destinations.Chats {
			if !hasChat[id] {
				http.Error(w, "add that chat to the team first", http.StatusBadRequest)
				return
			}
			if !seenChat[id] {
				seenChat[id] = true
				clean.Chats = append(clean.Chats, id)
			}
		}
		rule = &clean
	}
	if err := a.db.SetNotificationTeamRule(r.Context(), team.Id, topic, rule, access.actor); err != nil {
		a.logger.Logf("notification hub: %s", err)
		http.Error(w, "could not save that", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// GetNotificationHubAccess says whether the viewer can open the hub, and as
// what, without loading it: the web app's menu asks on every sign-in.
func (a *AppService) GetNotificationHubAccess(w http.ResponseWriter, r *http.Request) {
	access, ok := a.requireHubAccess(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"can_edit":      access.admin,
		"managed_teams": access.teams,
	})
}
