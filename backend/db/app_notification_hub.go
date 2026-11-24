package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// The notification hub's teams and routes. See structs/notification_hub.go for
// the model.

// ErrNotificationTeamNameTaken is a second team with the same name, ignoring
// case, in one organization.
var ErrNotificationTeamNameTaken = errors.New("a team with that name already exists")

// ── Teams ────────────────────────────────────────────────────────────────────

// ListNotificationTeams returns an organization's teams with their members,
// their topics and their chat's name, alphabetically.
func (a *AppDB) ListNotificationTeams(ctx context.Context, organizationId int64) ([]structs.NotificationTeam, error) {
	rows, err := a.db.Query(ctx, `
		SELECT t.id, t.name, t.description, t.push_members, t.email_members, t.email_address,
			t.chat_conversation_id, COALESCE(c.title, ''), t.created_at
		FROM notification_teams t
		LEFT JOIN conversations c ON c.id = t.chat_conversation_id
		WHERE t.organization_id = $1
		ORDER BY LOWER(t.name), t.id;
	`, organizationId)
	if err != nil {
		return nil, fmt.Errorf("error listing notification teams: %w", err)
	}
	teams := []structs.NotificationTeam{}
	index := map[int64]int{}
	for rows.Next() {
		var team structs.NotificationTeam
		if err := rows.Scan(&team.Id, &team.Name, &team.Description, &team.PushMembers, &team.EmailMembers,
			&team.EmailAddress, &team.ChatConversationId, &team.ChatName, &team.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("error scanning notification team: %w", err)
		}
		team.MemberIds = []string{}
		team.Topics = []string{}
		index[team.Id] = len(teams)
		teams = append(teams, team)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return teams, nil
	}

	memberRows, err := a.db.Query(ctx, `
		SELECT m.team_id, m.user_id
		FROM notification_team_members m
		JOIN notification_teams t ON t.id = m.team_id
		-- Only people still in the organization: somebody who left is not on
		-- the team in any way that matters, and listing them would make the
		-- next save of the team refuse the whole list.
		JOIN organization_members om ON om.user_id = m.user_id AND om.organization_id = t.organization_id
		WHERE t.organization_id = $1
		ORDER BY m.team_id, m.created_at, m.user_id;
	`, organizationId)
	if err != nil {
		return nil, fmt.Errorf("error listing notification team members: %w", err)
	}
	for memberRows.Next() {
		var teamId int64
		var userId string
		if err := memberRows.Scan(&teamId, &userId); err != nil {
			memberRows.Close()
			return nil, err
		}
		if i, ok := index[teamId]; ok {
			teams[i].MemberIds = append(teams[i].MemberIds, userId)
		}
	}
	memberRows.Close()
	if err := memberRows.Err(); err != nil {
		return nil, err
	}

	routeRows, err := a.db.Query(ctx, `
		SELECT r.team_id, r.topic
		FROM notification_routes r
		JOIN notification_teams t ON t.id = r.team_id
		WHERE t.organization_id = $1
		ORDER BY r.team_id, r.topic;
	`, organizationId)
	if err != nil {
		return nil, fmt.Errorf("error listing notification routes: %w", err)
	}
	defer routeRows.Close()
	for routeRows.Next() {
		var teamId int64
		var topic string
		if err := routeRows.Scan(&teamId, &topic); err != nil {
			return nil, err
		}
		if i, ok := index[teamId]; ok {
			teams[i].Topics = append(teams[i].Topics, topic)
		}
	}
	return teams, routeRows.Err()
}

// CreateNotificationTeam adds a team with no members and no topics.
func (a *AppDB) CreateNotificationTeam(
	ctx context.Context, organizationId int64, input structs.NotificationTeamInput, actor string,
) (int64, error) {
	var id int64
	err := a.db.QueryRow(ctx, `
		INSERT INTO notification_teams
			(organization_id, name, description, push_members, email_members, email_address, chat_conversation_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id;
	`, organizationId, input.Name, input.Description, input.PushMembers, input.EmailMembers,
		input.EmailAddress, input.ChatConversationId, actor).Scan(&id)
	if isUniqueViolation(err) {
		return 0, ErrNotificationTeamNameTaken
	}
	if err != nil {
		return 0, fmt.Errorf("error creating notification team: %w", err)
	}
	return id, nil
}

// UpdateNotificationTeam replaces a team's details, and reports whether the team
// exists in this organization.
func (a *AppDB) UpdateNotificationTeam(
	ctx context.Context, organizationId int64, teamId int64, input structs.NotificationTeamInput,
) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE notification_teams
		SET name = $3, description = $4, push_members = $5, email_members = $6,
			email_address = $7, chat_conversation_id = $8, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2;
	`, organizationId, teamId, input.Name, input.Description, input.PushMembers, input.EmailMembers,
		input.EmailAddress, input.ChatConversationId)
	if isUniqueViolation(err) {
		return false, ErrNotificationTeamNameTaken
	}
	if err != nil {
		return false, fmt.Errorf("error updating notification team %d: %w", teamId, err)
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteNotificationTeam removes a team, its members and its routes.
func (a *AppDB) DeleteNotificationTeam(ctx context.Context, organizationId int64, teamId int64) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		DELETE FROM notification_teams WHERE organization_id = $1 AND id = $2;
	`, organizationId, teamId)
	if err != nil {
		return false, fmt.Errorf("error deleting notification team %d: %w", teamId, err)
	}
	return tag.RowsAffected() > 0, nil
}

// SetNotificationTeamMembers replaces a team's members.
//
// Only members of the team's organization are written. The handler refuses a
// request naming anybody else before it gets here; the join is the second
// line, so no caller can put an outsider on a team.
func (a *AppDB) SetNotificationTeamMembers(
	ctx context.Context, organizationId int64, teamId int64, userIds []string, actor string,
) (bool, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("error starting team member update: %w", err)
	}
	defer tx.Rollback(ctx)

	var locked int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM notification_teams WHERE organization_id = $1 AND id = $2 FOR UPDATE;
	`, organizationId, teamId).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("error locking notification team %d: %w", teamId, err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM notification_team_members WHERE team_id = $1;`, teamId); err != nil {
		return false, fmt.Errorf("error clearing team members: %w", err)
	}
	if len(userIds) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO notification_team_members (team_id, user_id, created_by)
			SELECT $2, om.user_id, $4
			FROM organization_members om
			WHERE om.organization_id = $1 AND om.user_id = ANY($3::text[])
			ON CONFLICT DO NOTHING;
		`, organizationId, teamId, userIds, actor); err != nil {
			return false, fmt.Errorf("error saving team members: %w", err)
		}
	}
	return true, tx.Commit(ctx)
}

// SetNotificationRoute replaces which of an organization's teams receive one
// topic. Team ids from another organization are ignored.
func (a *AppDB) SetNotificationRoute(
	ctx context.Context, organizationId int64, topic string, teamIds []int64, actor string,
) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting route update: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM notification_routes r
		USING notification_teams t
		WHERE r.team_id = t.id AND t.organization_id = $1 AND r.topic = $2;
	`, organizationId, topic); err != nil {
		return fmt.Errorf("error clearing routes for %s: %w", topic, err)
	}
	if len(teamIds) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO notification_routes (team_id, topic, created_by)
			SELECT t.id, $2, $4
			FROM notification_teams t
			WHERE t.organization_id = $1 AND t.id = ANY($3::bigint[])
			ON CONFLICT DO NOTHING;
		`, organizationId, topic, teamIds, actor); err != nil {
			return fmt.Errorf("error saving routes for %s: %w", topic, err)
		}
	}
	return tx.Commit(ctx)
}

// GetNotificationRoutes is topic → team ids for an organization.
func (a *AppDB) GetNotificationRoutes(ctx context.Context, organizationId int64) (map[string][]int64, error) {
	rows, err := a.db.Query(ctx, `
		SELECT r.topic, r.team_id
		FROM notification_routes r
		JOIN notification_teams t ON t.id = r.team_id
		WHERE t.organization_id = $1
		ORDER BY r.topic, LOWER(t.name);
	`, organizationId)
	if err != nil {
		return nil, fmt.Errorf("error loading notification routes: %w", err)
	}
	defer rows.Close()
	out := map[string][]int64{}
	for rows.Next() {
		var topic string
		var teamId int64
		if err := rows.Scan(&topic, &teamId); err != nil {
			return nil, err
		}
		out[topic] = append(out[topic], teamId)
	}
	return out, rows.Err()
}

// PushDeviceCounts is how many active push subscriptions each user has.
func (a *AppDB) PushDeviceCounts(ctx context.Context, userIds []string) (map[string]int, error) {
	out := map[string]int{}
	if len(userIds) == 0 {
		return out, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT owner, COUNT(*) FROM mobile_push_subscriptions
		WHERE active = TRUE AND owner = ANY($1::text[])
		GROUP BY owner;
	`, userIds)
	if err != nil {
		return nil, fmt.Errorf("error counting push devices: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var owner string
		var count int
		if err := rows.Scan(&owner, &count); err != nil {
			return nil, err
		}
		out[owner] = count
	}
	return out, rows.Err()
}

// ── Delivery ─────────────────────────────────────────────────────────────────

// NotificationDeliveryPlan is everyone and everywhere one topic goes.
type NotificationDeliveryPlan struct {
	Teams []structs.NotificationDeliveryTeam
	// CatchAll is set when the topic had no team and the catch-all's teams
	// stand in for them.
	CatchAll bool
	// Members is every house-organization member on those teams.
	Members []string
	// Push is the members at least one of whose teams pushes.
	Push map[string]bool
	// Emails is every address to write to, deduplicated: members of emailing
	// teams, and the teams' own addresses.
	Emails []string
	// Chats is every group chat to post in.
	Chats []int64
}

// Empty is whether the plan reaches nobody at all.
func (p *NotificationDeliveryPlan) Empty() bool {
	return p == nil || (len(p.Members) == 0 && len(p.Emails) == 0 && len(p.Chats) == 0)
}

// NotificationDeliveryPlan resolves a topic to its teams, falling back to the
// catch-all when no team takes it or its teams reach nobody, and the teams to
// people, addresses and chats.
func (a *AppDB) NotificationDeliveryPlan(ctx context.Context, organizationId int64, topic string) (*NotificationDeliveryPlan, error) {
	load := func(topic string) ([]notificationTeamRow, error) {
		rows, err := a.db.Query(ctx, `
			SELECT t.id, t.name, t.push_members, t.email_members, t.email_address, t.chat_conversation_id
			FROM notification_routes r
			JOIN notification_teams t ON t.id = r.team_id
			WHERE t.organization_id = $1 AND r.topic = $2
			ORDER BY LOWER(t.name), t.id;
		`, organizationId, topic)
		if err != nil {
			return nil, fmt.Errorf("error loading the teams for %s: %w", topic, err)
		}
		defer rows.Close()
		out := []notificationTeamRow{}
		for rows.Next() {
			var row notificationTeamRow
			if err := rows.Scan(&row.id, &row.name, &row.push, &row.email, &row.address, &row.chat); err != nil {
				return nil, err
			}
			out = append(out, row)
		}
		return out, rows.Err()
	}

	teams, err := load(topic)
	if err != nil {
		return nil, err
	}
	plan, err := a.planForTeams(ctx, organizationId, teams)
	if err != nil {
		return nil, err
	}
	// The catch-all takes a topic with no team, and also one whose teams
	// reach nobody (empty, with no address or chat), so routing a topic to a
	// team nobody has joined yet cannot silence it.
	if plan.Empty() && topic != structs.NotificationTopicUnrouted {
		catchAll, err := load(structs.NotificationTopicUnrouted)
		if err != nil {
			return nil, err
		}
		if len(catchAll) > 0 {
			fallback, err := a.planForTeams(ctx, organizationId, catchAll)
			if err != nil {
				return nil, err
			}
			if !fallback.Empty() {
				fallback.CatchAll = true
				return fallback, nil
			}
		}
	}
	return plan, nil
}

// notificationTeamRow is one team as delivery needs it.
type notificationTeamRow struct {
	id          int64
	name        string
	push, email bool
	address     string
	chat        *int64
}

// planForTeams resolves teams to the people, addresses and chats they reach.
// Members are bounded by the organization here, at delivery time.
func (a *AppDB) planForTeams(ctx context.Context, organizationId int64, teams []notificationTeamRow) (*NotificationDeliveryPlan, error) {
	plan := &NotificationDeliveryPlan{Push: map[string]bool{}, Teams: []structs.NotificationDeliveryTeam{}}
	if len(teams) == 0 {
		return plan, nil
	}

	ids := make([]int64, 0, len(teams))
	byId := map[int64]notificationTeamRow{}
	emails := map[string]string{}
	chats := map[int64]bool{}
	for _, team := range teams {
		ids = append(ids, team.id)
		byId[team.id] = team
		plan.Teams = append(plan.Teams, structs.NotificationDeliveryTeam{Id: team.id, Name: team.name})
		if address := strings.TrimSpace(team.address); address != "" {
			emails[strings.ToLower(address)] = address
		}
		if team.chat != nil && *team.chat > 0 && !chats[*team.chat] {
			chats[*team.chat] = true
			plan.Chats = append(plan.Chats, *team.chat)
		}
	}

	rows, err := a.db.Query(ctx, `
		SELECT m.team_id, m.user_id, COALESCE(u.contact_email, '')
		FROM notification_team_members m
		JOIN organization_members om ON om.user_id = m.user_id AND om.organization_id = $1
		JOIN users u ON u.id = m.user_id
		WHERE m.team_id = ANY($2::bigint[])
		ORDER BY m.user_id;
	`, organizationId, ids)
	if err != nil {
		return nil, fmt.Errorf("error loading team members: %w", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var teamId int64
		var userId, email string
		if err := rows.Scan(&teamId, &userId, &email); err != nil {
			return nil, err
		}
		team := byId[teamId]
		if !seen[userId] {
			seen[userId] = true
			plan.Members = append(plan.Members, userId)
		}
		if team.push {
			plan.Push[userId] = true
		}
		if email = strings.TrimSpace(email); team.email && email != "" {
			emails[strings.ToLower(email)] = email
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, address := range emails {
		plan.Emails = append(plan.Emails, address)
	}
	sort.Strings(plan.Emails)
	return plan, nil
}

// SetInternalAlertDelivery records where a notification went.
func (a *AppDB) SetInternalAlertDelivery(ctx context.Context, notificationId int64, delivery structs.NotificationDelivery) error {
	if delivery.Teams == nil {
		delivery.Teams = []structs.NotificationDeliveryTeam{}
	}
	encoded, err := json.Marshal(delivery)
	if err != nil {
		return fmt.Errorf("error encoding delivery for alert %d: %w", notificationId, err)
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE internal_notifications SET delivery = $2::jsonb WHERE id = $1;
	`, notificationId, string(encoded)); err != nil {
		return fmt.Errorf("error recording delivery for alert %d: %w", notificationId, err)
	}
	return nil
}

// ResolveInternalAlertInformational retires a notification the moment it has
// been delivered, for one that never asked for anything.
func (a *AppDB) ResolveInternalAlertInformational(ctx context.Context, notificationId int64) error {
	_, err := a.ResolveInternalAlertById(ctx, notificationId, structs.ResolutionInformational, "", "")
	return err
}

// UserReceivedInternalAlert is the authorization check for an alert's buttons:
// this person was delivered it and is still in its organization.
func (a *AppDB) UserReceivedInternalAlert(ctx context.Context, notificationId int64, userId string) (bool, error) {
	var received bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM user_notifications un
			JOIN internal_notifications n ON n.id = un.internal_notification_id
			JOIN organization_members om ON om.user_id = un.user_id AND om.organization_id = n.organization_id
			WHERE un.internal_notification_id = $1 AND un.user_id = $2
		);
	`, notificationId, userId).Scan(&received)
	if err != nil {
		return false, fmt.Errorf("error checking who received alert %d: %w", notificationId, err)
	}
	return received, nil
}

// GetUserNotificationTeams returns the groups one user receives through their
// teams, and the teams' names.
func (a *AppDB) GetUserNotificationTeams(ctx context.Context, userId string) ([]string, []string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT t.name, COALESCE(array_agg(r.topic) FILTER (WHERE r.topic IS NOT NULL), '{}')
		FROM notification_team_members m
		JOIN notification_teams t ON t.id = m.team_id
		JOIN organization_members om ON om.user_id = m.user_id AND om.organization_id = t.organization_id
		LEFT JOIN notification_routes r ON r.team_id = t.id
		WHERE m.user_id = $1
		GROUP BY t.id, t.name
		ORDER BY LOWER(t.name);
	`, userId)
	if err != nil {
		return nil, nil, fmt.Errorf("error loading notification teams for %s: %w", userId, err)
	}
	defer rows.Close()

	teams := []string{}
	setSeen := map[string]bool{}
	for rows.Next() {
		var name string
		var topics []string
		if err := rows.Scan(&name, &topics); err != nil {
			return nil, nil, err
		}
		teams = append(teams, name)
		for _, topic := range topics {
			if topic == structs.NotificationTopicUnrouted {
				continue
			}
			if def, ok := structs.NotificationTopic(topic); ok {
				setSeen[def.Set] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	sets := []string{}
	for _, set := range structs.NotificationSetCatalog() {
		if setSeen[set.Id] {
			sets = append(sets, set.Id)
		}
	}
	return sets, teams, nil
}

// ── The hub's own view ───────────────────────────────────────────────────────

// LiveInternalAlertCountsByTopic is how many unresolved notifications each
// topic holds.
func (a *AppDB) LiveInternalAlertCountsByTopic(ctx context.Context, organizationId int64) (map[string]int, error) {
	rows, err := a.db.Query(ctx, `
		SELECT topic, COUNT(*)
		FROM internal_notifications
		WHERE organization_id = $1 AND resolved_at IS NULL
		GROUP BY topic;
	`, organizationId)
	if err != nil {
		return nil, fmt.Errorf("error counting live alerts: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var topic string
		var count int
		if err := rows.Scan(&topic, &count); err != nil {
			return nil, err
		}
		out[topic] = count
	}
	return out, rows.Err()
}

// NotificationTopicActivity is, per topic, when it last fired, how many it
// raised in the last day, and how many in the last 30 days reached nobody.
func (a *AppDB) NotificationTopicActivity(ctx context.Context, organizationId int64) (map[string]structs.NotificationTopicActivity, error) {
	rows, err := a.db.Query(ctx, `
		SELECT topic,
			MAX(created_at),
			COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '24 hours'),
			COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '30 days'
				AND (delivery->>'reached_nobody')::boolean IS TRUE)
		FROM internal_notifications
		WHERE organization_id = $1 AND topic <> ''
		GROUP BY topic;
	`, organizationId)
	if err != nil {
		return nil, fmt.Errorf("error loading topic activity: %w", err)
	}
	defer rows.Close()
	out := map[string]structs.NotificationTopicActivity{}
	for rows.Next() {
		var topic string
		var activity structs.NotificationTopicActivity
		var last time.Time
		if err := rows.Scan(&topic, &last, &activity.Last24h, &activity.ReachedNobody30d); err != nil {
			return nil, err
		}
		activity.LastAt = &last
		out[topic] = activity
	}
	return out, rows.Err()
}

// RecentInternalAlerts is the hub's latest notifications, newest first.
func (a *AppDB) RecentInternalAlerts(ctx context.Context, organizationId int64, limit int) ([]structs.NotificationHubEntry, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, topic, notification_set, kind, title, body, created_at, resolved_at, resolution, outcome, delivery
		FROM internal_notifications
		WHERE organization_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2;
	`, organizationId, limit)
	if err != nil {
		return nil, fmt.Errorf("error loading recent notifications: %w", err)
	}
	defer rows.Close()
	out := []structs.NotificationHubEntry{}
	for rows.Next() {
		var entry structs.NotificationHubEntry
		var delivery []byte
		if err := rows.Scan(&entry.Id, &entry.Topic, &entry.Set, &entry.Kind, &entry.Title, &entry.Body,
			&entry.CreatedAt, &entry.ResolvedAt, &entry.Resolution, &entry.Outcome, &delivery); err != nil {
			return nil, err
		}
		if len(delivery) > 0 {
			var parsed structs.NotificationDelivery
			if json.Unmarshal(delivery, &parsed) == nil {
				entry.Delivery = &parsed
			}
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// CountReachedNobody counts notifications since a time that reached no person,
// address or chat.
func (a *AppDB) CountReachedNobody(ctx context.Context, organizationId int64, since time.Time) (int, error) {
	var count int
	err := a.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM internal_notifications
		WHERE organization_id = $1 AND created_at >= $2
		AND (delivery->>'reached_nobody')::boolean IS TRUE;
	`, organizationId, since).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error counting undelivered notifications: %w", err)
	}
	return count, nil
}
