package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
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

// teamLeadSQL is a team's lead as it stands: the one named, while they are
// still on the team and in the organization, or else whoever was added first.
// It reads the team as t.
const teamLeadSQL = `COALESCE(
	(SELECT m.user_id FROM notification_team_members m
		JOIN organization_members om ON om.user_id = m.user_id AND om.organization_id = t.organization_id
		WHERE m.team_id = t.id AND m.user_id = t.lead_user_id),
	(SELECT m.user_id FROM notification_team_members m
		JOIN organization_members om ON om.user_id = m.user_id AND om.organization_id = t.organization_id
		WHERE m.team_id = t.id
		ORDER BY m.created_at, m.user_id
		LIMIT 1),
	'')`

// ListNotificationTeams returns an organization's teams with their members,
// lead, admins, topics, chats and rules, alphabetically.
func (a *AppDB) ListNotificationTeams(ctx context.Context, organizationId int64) ([]structs.NotificationTeam, error) {
	rows, err := a.db.Query(ctx, `
		SELECT t.id, t.name, t.description, t.email_address, `+teamLeadSQL+`, t.created_at
		FROM notification_teams t
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
		if err := rows.Scan(&team.Id, &team.Name, &team.Description, &team.EmailAddress, &team.LeadUserId, &team.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("error scanning notification team: %w", err)
		}
		team.MemberIds = []string{}
		team.AdminIds = []string{}
		team.Topics = []string{}
		team.Chats = []structs.NotificationTeamChat{}
		team.Rules = map[string]structs.NotificationTeamDestinations{}
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
		SELECT m.team_id, m.user_id, m.is_admin
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
		var admin bool
		if err := memberRows.Scan(&teamId, &userId, &admin); err != nil {
			memberRows.Close()
			return nil, err
		}
		if i, ok := index[teamId]; ok {
			teams[i].MemberIds = append(teams[i].MemberIds, userId)
			if admin && userId != teams[i].LeadUserId {
				teams[i].AdminIds = append(teams[i].AdminIds, userId)
			}
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
	for routeRows.Next() {
		var teamId int64
		var topic string
		if err := routeRows.Scan(&teamId, &topic); err != nil {
			routeRows.Close()
			return nil, err
		}
		if i, ok := index[teamId]; ok {
			teams[i].Topics = append(teams[i].Topics, topic)
		}
	}
	routeRows.Close()
	if err := routeRows.Err(); err != nil {
		return nil, err
	}

	chatRows, err := a.db.Query(ctx, `
		SELECT tc.team_id, c.id, COALESCE(c.title, '')
		FROM notification_team_chats tc
		JOIN notification_teams t ON t.id = tc.team_id
		JOIN conversations c ON c.id = tc.conversation_id AND c.kind = 'group' AND c.deleted_at IS NULL
		WHERE t.organization_id = $1
		ORDER BY tc.team_id, tc.created_at, c.id;
	`, organizationId)
	if err != nil {
		return nil, fmt.Errorf("error listing notification team chats: %w", err)
	}
	for chatRows.Next() {
		var teamId int64
		var chat structs.NotificationTeamChat
		if err := chatRows.Scan(&teamId, &chat.Id, &chat.Title); err != nil {
			chatRows.Close()
			return nil, err
		}
		if i, ok := index[teamId]; ok {
			teams[i].Chats = append(teams[i].Chats, chat)
		}
	}
	chatRows.Close()
	if err := chatRows.Err(); err != nil {
		return nil, err
	}

	ruleRows, err := a.db.Query(ctx, `
		SELECT r.team_id, r.topic, r.destinations
		FROM notification_team_topic_routes r
		JOIN notification_teams t ON t.id = r.team_id
		WHERE t.organization_id = $1;
	`, organizationId)
	if err != nil {
		return nil, fmt.Errorf("error listing notification team rules: %w", err)
	}
	defer ruleRows.Close()
	for ruleRows.Next() {
		var teamId int64
		var topic string
		var raw []byte
		if err := ruleRows.Scan(&teamId, &topic, &raw); err != nil {
			return nil, err
		}
		i, ok := index[teamId]
		if !ok {
			continue
		}
		rule, err := decodeTeamDestinations(raw)
		if err != nil {
			return nil, fmt.Errorf("error reading the rule for %s on team %d: %w", topic, teamId, err)
		}
		teams[i].Rules[topic] = rule
	}
	return teams, ruleRows.Err()
}

func decodeTeamDestinations(raw []byte) (structs.NotificationTeamDestinations, error) {
	var rule structs.NotificationTeamDestinations
	if err := json.Unmarshal(raw, &rule); err != nil {
		return rule, err
	}
	if rule.Members == nil {
		rule.Members = []structs.NotificationMemberDestination{}
	}
	if rule.Chats == nil {
		rule.Chats = []int64{}
	}
	return rule, nil
}

// NotificationTeamRoles returns the teams one person leads ("lead") or is an
// admin of ("admin"), in an organization.
func (a *AppDB) NotificationTeamRoles(ctx context.Context, organizationId int64, userId string) (map[int64]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT t.id, `+teamLeadSQL+` = $2 AS leads
		FROM notification_teams t
		WHERE t.organization_id = $1
			AND (`+teamLeadSQL+` = $2 OR EXISTS (
				SELECT 1 FROM notification_team_members m
				JOIN organization_members om ON om.user_id = m.user_id AND om.organization_id = t.organization_id
				WHERE m.team_id = t.id AND m.user_id = $2 AND m.is_admin
			));
	`, organizationId, userId)
	if err != nil {
		return nil, fmt.Errorf("error loading the teams %s manages: %w", userId, err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var teamId int64
		var leads bool
		if err := rows.Scan(&teamId, &leads); err != nil {
			return nil, err
		}
		if leads {
			out[teamId] = "lead"
		} else {
			out[teamId] = "admin"
		}
	}
	return out, rows.Err()
}

// CreateNotificationTeam adds a team with no members and no topics.
func (a *AppDB) CreateNotificationTeam(
	ctx context.Context, organizationId int64, input structs.NotificationTeamInput, actor string,
) (int64, error) {
	var id int64
	err := a.db.QueryRow(ctx, `
		INSERT INTO notification_teams (organization_id, name, description, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id;
	`, organizationId, input.Name, input.Description, actor).Scan(&id)
	if isUniqueViolation(err) {
		return 0, ErrNotificationTeamNameTaken
	}
	if err != nil {
		return 0, fmt.Errorf("error creating notification team: %w", err)
	}
	return id, nil
}

// UpdateNotificationTeam renames a team, and reports whether the team exists in
// this organization.
func (a *AppDB) UpdateNotificationTeam(
	ctx context.Context, organizationId int64, teamId int64, input structs.NotificationTeamInput,
) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE notification_teams
		SET name = $3, description = $4, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2;
	`, organizationId, teamId, input.Name, input.Description)
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

// SetNotificationTeamMembers replaces a team's members, in the order given.
//
// People already on the team keep their place and whether they are an admin;
// new people are added after them, in order. The lead stays the lead while
// they are on the team; otherwise the first person added becomes it.
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

	if _, err := tx.Exec(ctx, `
		DELETE FROM notification_team_members WHERE team_id = $1 AND NOT (user_id = ANY($2::text[]));
	`, teamId, userIds); err != nil {
		return false, fmt.Errorf("error removing team members: %w", err)
	}
	if len(userIds) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO notification_team_members (team_id, user_id, created_by, created_at)
			SELECT $2, om.user_id, $4, NOW() + (given.ord * INTERVAL '1 microsecond')
			FROM unnest($3::text[]) WITH ORDINALITY AS given(user_id, ord)
			JOIN organization_members om ON om.organization_id = $1 AND om.user_id = given.user_id
			ON CONFLICT (team_id, user_id) DO NOTHING;
		`, organizationId, teamId, userIds, actor); err != nil {
			return false, fmt.Errorf("error saving team members: %w", err)
		}
	}
	if err := setTeamLeadInTx(ctx, tx, teamId, ""); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// setTeamLeadInTx makes userId the lead, or with "" keeps the current lead if
// they are still on the team and otherwise picks the first person added. A
// lead is never also an admin.
func setTeamLeadInTx(ctx context.Context, tx pgx.Tx, teamId int64, userId string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE notification_teams t
		SET lead_user_id = (
			SELECT m.user_id FROM notification_team_members m
			WHERE m.team_id = t.id
			ORDER BY (m.user_id = NULLIF($2, '')) DESC NULLS LAST,
				(m.user_id = t.lead_user_id) DESC NULLS LAST,
				m.created_at, m.user_id
			LIMIT 1
		), updated_at = NOW()
		WHERE t.id = $1;
		`, teamId, userId); err != nil {
		return fmt.Errorf("error choosing the lead of team %d: %w", teamId, err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE notification_team_members m SET is_admin = FALSE
		FROM notification_teams t
		WHERE t.id = m.team_id AND m.team_id = $1 AND m.user_id = t.lead_user_id AND m.is_admin;
	`, teamId); err != nil {
		return fmt.Errorf("error tidying the admins of team %d: %w", teamId, err)
	}
	return nil
}

// SetNotificationTeamLead makes one of a team's members its lead. It reports
// false when the team is not in the organization or they are not on it.
func (a *AppDB) SetNotificationTeamLead(ctx context.Context, organizationId int64, teamId int64, userId string) (bool, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("error starting the lead change: %w", err)
	}
	defer tx.Rollback(ctx)

	var onTeam bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM notification_team_members m
			JOIN notification_teams t ON t.id = m.team_id
			JOIN organization_members om ON om.user_id = m.user_id AND om.organization_id = t.organization_id
			WHERE t.organization_id = $1 AND t.id = $2 AND m.user_id = $3
		);
	`, organizationId, teamId, userId).Scan(&onTeam); err != nil {
		return false, fmt.Errorf("error checking the new lead: %w", err)
	}
	if !onTeam {
		return false, nil
	}
	if err := setTeamLeadInTx(ctx, tx, teamId, userId); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// SetNotificationTeamAdmins makes exactly these members of a team its admins.
// The lead and anybody not on the team are skipped.
func (a *AppDB) SetNotificationTeamAdmins(ctx context.Context, organizationId int64, teamId int64, userIds []string) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE notification_team_members m
		SET is_admin = (m.user_id = ANY($3::text[]) AND m.user_id IS DISTINCT FROM t.lead_user_id)
		FROM notification_teams t
		WHERE t.id = m.team_id AND t.organization_id = $1 AND t.id = $2;
	`, organizationId, teamId, userIds)
	if err != nil {
		return false, fmt.Errorf("error saving the admins of team %d: %w", teamId, err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := a.db.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM notification_teams WHERE organization_id = $1 AND id = $2);
		`, organizationId, teamId).Scan(&exists); err != nil {
			return false, err
		}
		return exists, nil
	}
	return true, nil
}

// SetNotificationTeamResources replaces a team's shared address and group chats.
func (a *AppDB) SetNotificationTeamResources(
	ctx context.Context, organizationId int64, teamId int64, emailAddress string, chatIds []int64, actor string,
) (bool, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("error starting the team resources update: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE notification_teams SET email_address = $3, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2;
	`, organizationId, teamId, emailAddress)
	if err != nil {
		return false, fmt.Errorf("error saving the address of team %d: %w", teamId, err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM notification_team_chats WHERE team_id = $1 AND NOT (conversation_id = ANY($2::bigint[]));
	`, teamId, chatIds); err != nil {
		return false, fmt.Errorf("error removing team chats: %w", err)
	}
	if len(chatIds) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO notification_team_chats (team_id, conversation_id, created_by, created_at)
			SELECT $1, c.id, $3, NOW() + (given.ord * INTERVAL '1 microsecond')
			FROM unnest($2::bigint[]) WITH ORDINALITY AS given(id, ord)
			JOIN conversations c ON c.id = given.id AND c.kind = 'group' AND c.deleted_at IS NULL
			ON CONFLICT (team_id, conversation_id) DO NOTHING;
		`, teamId, chatIds, actor); err != nil {
			return false, fmt.Errorf("error saving team chats: %w", err)
		}
	}
	return true, tx.Commit(ctx)
}

// SetNotificationTeamRule sets, or with nil removes, where one topic goes
// inside a team.
func (a *AppDB) SetNotificationTeamRule(
	ctx context.Context, teamId int64, topic string, rule *structs.NotificationTeamDestinations, actor string,
) error {
	if rule == nil {
		if _, err := a.db.Exec(ctx, `
			DELETE FROM notification_team_topic_routes WHERE team_id = $1 AND topic = $2;
		`, teamId, topic); err != nil {
			return fmt.Errorf("error removing the rule for %s on team %d: %w", topic, teamId, err)
		}
		return nil
	}
	raw, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	if _, err := a.db.Exec(ctx, `
		INSERT INTO notification_team_topic_routes (team_id, topic, destinations, updated_by)
		VALUES ($1, $2, $3::jsonb, $4)
		ON CONFLICT (team_id, topic) DO UPDATE
		SET destinations = EXCLUDED.destinations, updated_at = NOW(), updated_by = EXCLUDED.updated_by;
	`, teamId, topic, string(raw), actor); err != nil {
		return fmt.Errorf("error saving the rule for %s on team %d: %w", topic, teamId, err)
	}
	return nil
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
	// Members is every house-organization member it goes to in the app.
	Members []string
	// Push is the members whose phones are buzzed too.
	Push map[string]bool
	// Emails is every address to write to, deduplicated.
	Emails []string
	// Chats is every group chat to post in.
	Chats []int64
}

// Empty is whether the plan reaches nobody at all.
func (p *NotificationDeliveryPlan) Empty() bool {
	return p == nil || (len(p.Members) == 0 && len(p.Emails) == 0 && len(p.Chats) == 0)
}

// NotificationDeliveryPlan resolves a topic to its teams, falling back to the
// catch-all when no team takes it or its teams reach nobody, and each team to
// the people, addresses and chats its own rule for the topic names.
func (a *AppDB) NotificationDeliveryPlan(ctx context.Context, organizationId int64, topic string) (*NotificationDeliveryPlan, error) {
	teams, err := a.routedTeamIds(ctx, organizationId, topic)
	if err != nil {
		return nil, err
	}
	plan, err := a.planForTeams(ctx, organizationId, teams, topic, false)
	if err != nil {
		return nil, err
	}
	// The catch-all takes a topic with no team, and also one whose teams
	// reach nobody, so routing a topic to a team nobody has joined yet cannot
	// silence it.
	if plan.Empty() && topic != structs.NotificationTopicUnrouted {
		catchAll, err := a.routedTeamIds(ctx, organizationId, structs.NotificationTopicUnrouted)
		if err != nil {
			return nil, err
		}
		if len(catchAll) > 0 {
			fallback, err := a.planForTeams(ctx, organizationId, catchAll, topic, true)
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

// NotificationTeamPlan is where one topic goes inside one team, as it would
// arrive there: through the team's own route, or else through the catch-all.
// For a team's test sends.
func (a *AppDB) NotificationTeamPlan(ctx context.Context, organizationId int64, teamId int64, topic string) (*NotificationDeliveryPlan, error) {
	routed, err := a.routedTeamIds(ctx, organizationId, topic)
	if err != nil {
		return nil, err
	}
	direct := false
	for _, id := range routed {
		if id == teamId {
			direct = true
		}
	}
	return a.planForTeams(ctx, organizationId, []int64{teamId}, topic, !direct)
}

func (a *AppDB) routedTeamIds(ctx context.Context, organizationId int64, topic string) ([]int64, error) {
	rows, err := a.db.Query(ctx, `
		SELECT t.id
		FROM notification_routes r
		JOIN notification_teams t ON t.id = r.team_id
		WHERE t.organization_id = $1 AND r.topic = $2
		ORDER BY LOWER(t.name), t.id;
	`, organizationId, topic)
	if err != nil {
		return nil, fmt.Errorf("error loading the teams for %s: %w", topic, err)
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ResolveTeamRule picks the rule a team applies to a topic: the topic's own,
// then (for what came through the catch-all) the catch-all's, then the team's
// default, then everyone on the team in the app and on their phone.
func ResolveTeamRule(rules map[string]structs.NotificationTeamDestinations, topic string, viaCatchAll bool) structs.NotificationTeamDestinations {
	if rule, ok := rules[topic]; ok {
		return rule
	}
	if viaCatchAll {
		if rule, ok := rules[structs.NotificationTopicUnrouted]; ok {
			return rule
		}
	}
	if rule, ok := rules[structs.NotificationTeamRuleDefault]; ok {
		return rule
	}
	return structs.DefaultNotificationTeamDestinations()
}

// planForTeams resolves teams to the people, addresses and chats their rules
// for one topic name. Members are bounded by the organization here, at
// delivery time, and a rule can only reach the team's own members and chats.
func (a *AppDB) planForTeams(
	ctx context.Context, organizationId int64, teamIds []int64, topic string, viaCatchAll bool,
) (*NotificationDeliveryPlan, error) {
	plan := &NotificationDeliveryPlan{Push: map[string]bool{}, Teams: []structs.NotificationDeliveryTeam{}}
	if len(teamIds) == 0 {
		return plan, nil
	}

	type teamState struct {
		name    string
		address string
		members map[string]string // user id → email
		order   []string
		chats   map[int64]bool
		rules   map[string]structs.NotificationTeamDestinations
	}
	teams := map[int64]*teamState{}
	order := []int64{}

	rows, err := a.db.Query(ctx, `
		SELECT t.id, t.name, t.email_address
		FROM notification_teams t
		WHERE t.organization_id = $1 AND t.id = ANY($2::bigint[])
		ORDER BY LOWER(t.name), t.id;
	`, organizationId, teamIds)
	if err != nil {
		return nil, fmt.Errorf("error loading teams: %w", err)
	}
	for rows.Next() {
		var id int64
		state := &teamState{members: map[string]string{}, chats: map[int64]bool{}, rules: map[string]structs.NotificationTeamDestinations{}}
		if err := rows.Scan(&id, &state.name, &state.address); err != nil {
			rows.Close()
			return nil, err
		}
		teams[id] = state
		order = append(order, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(order) == 0 {
		return plan, nil
	}

	memberRows, err := a.db.Query(ctx, `
		SELECT m.team_id, m.user_id, COALESCE(u.contact_email, '')
		FROM notification_team_members m
		JOIN organization_members om ON om.user_id = m.user_id AND om.organization_id = $1
		JOIN users u ON u.id = m.user_id
		WHERE m.team_id = ANY($2::bigint[])
		ORDER BY m.team_id, m.created_at, m.user_id;
	`, organizationId, order)
	if err != nil {
		return nil, fmt.Errorf("error loading team members: %w", err)
	}
	for memberRows.Next() {
		var teamId int64
		var userId, email string
		if err := memberRows.Scan(&teamId, &userId, &email); err != nil {
			memberRows.Close()
			return nil, err
		}
		if state := teams[teamId]; state != nil {
			state.members[userId] = strings.TrimSpace(email)
			state.order = append(state.order, userId)
		}
	}
	memberRows.Close()
	if err := memberRows.Err(); err != nil {
		return nil, err
	}

	chatRows, err := a.db.Query(ctx, `
		SELECT tc.team_id, tc.conversation_id
		FROM notification_team_chats tc
		JOIN conversations c ON c.id = tc.conversation_id AND c.kind = 'group' AND c.deleted_at IS NULL
		WHERE tc.team_id = ANY($1::bigint[]);
	`, order)
	if err != nil {
		return nil, fmt.Errorf("error loading team chats: %w", err)
	}
	for chatRows.Next() {
		var teamId, chatId int64
		if err := chatRows.Scan(&teamId, &chatId); err != nil {
			chatRows.Close()
			return nil, err
		}
		if state := teams[teamId]; state != nil {
			state.chats[chatId] = true
		}
	}
	chatRows.Close()
	if err := chatRows.Err(); err != nil {
		return nil, err
	}

	ruleRows, err := a.db.Query(ctx, `
		SELECT team_id, topic, destinations
		FROM notification_team_topic_routes
		WHERE team_id = ANY($1::bigint[]) AND topic = ANY($2::text[]);
	`, order, []string{topic, structs.NotificationTopicUnrouted, structs.NotificationTeamRuleDefault})
	if err != nil {
		return nil, fmt.Errorf("error loading team rules: %w", err)
	}
	for ruleRows.Next() {
		var teamId int64
		var key string
		var raw []byte
		if err := ruleRows.Scan(&teamId, &key, &raw); err != nil {
			ruleRows.Close()
			return nil, err
		}
		rule, err := decodeTeamDestinations(raw)
		if err != nil {
			ruleRows.Close()
			return nil, fmt.Errorf("error reading the rule for %s on team %d: %w", key, teamId, err)
		}
		if state := teams[teamId]; state != nil {
			state.rules[key] = rule
		}
	}
	ruleRows.Close()
	if err := ruleRows.Err(); err != nil {
		return nil, err
	}

	emails := map[string]string{}
	seen := map[string]bool{}
	seenChat := map[int64]bool{}
	addEmail := func(address string) {
		if address = strings.TrimSpace(address); address != "" {
			emails[strings.ToLower(address)] = address
		}
	}
	for _, teamId := range order {
		state := teams[teamId]
		plan.Teams = append(plan.Teams, structs.NotificationDeliveryTeam{Id: teamId, Name: state.name})
		rule := ResolveTeamRule(state.rules, topic, viaCatchAll)

		deliver := func(userId string, push, email bool) {
			address, onTeam := state.members[userId]
			if !onTeam {
				return
			}
			if !seen[userId] {
				seen[userId] = true
				plan.Members = append(plan.Members, userId)
			}
			if push {
				plan.Push[userId] = true
			}
			if email {
				addEmail(address)
			}
		}
		if rule.Everyone != nil {
			for _, userId := range state.order {
				deliver(userId, rule.Everyone.Push, rule.Everyone.Email)
			}
		}
		for _, member := range rule.Members {
			deliver(member.UserId, member.Push, member.Email)
		}
		if rule.EmailAddress {
			addEmail(state.address)
		}
		for _, chatId := range rule.Chats {
			if state.chats[chatId] && !seenChat[chatId] {
				seenChat[chatId] = true
				plan.Chats = append(plan.Chats, chatId)
			}
		}
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

// SearchNotificationChats lists group chats whose title matches query, or whose
// id is query, that at least one current member of the organization is in.
// An empty query lists the most recently active ones.
func (a *AppDB) SearchNotificationChats(ctx context.Context, organizationId int64, query string, limit int) ([]structs.NotificationChatOption, error) {
	var id int64
	if n, err := strconv.ParseInt(query, 10, 64); err == nil && n > 0 {
		id = n
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(query) + "%"
	rows, err := a.db.Query(ctx, `
		SELECT c.id, COALESCE(c.title, ''),
			(SELECT COUNT(*) FROM conversation_members cm
				WHERE cm.conversation_id = c.id AND cm.left_at IS NULL) AS members
		FROM conversations c
		WHERE c.kind = 'group' AND c.deleted_at IS NULL
			AND EXISTS (
				SELECT 1 FROM conversation_members cm
				JOIN organization_members om ON om.user_id = cm.user_id AND om.organization_id = $1
				WHERE cm.conversation_id = c.id AND cm.left_at IS NULL
			)
			AND ($2 = '' OR c.title ILIKE $3 OR c.id = $4)
		ORDER BY (c.id = $4) DESC, c.last_message_at DESC, c.id DESC
		LIMIT $5;
	`, organizationId, query, pattern, id, limit)
	if err != nil {
		return nil, fmt.Errorf("error searching group chats: %w", err)
	}
	defer rows.Close()
	out := []structs.NotificationChatOption{}
	for rows.Next() {
		var chat structs.NotificationChatOption
		if err := rows.Scan(&chat.Id, &chat.Title, &chat.Members); err != nil {
			return nil, fmt.Errorf("error scanning a group chat: %w", err)
		}
		out = append(out, chat)
	}
	return out, rows.Err()
}

// ValidNotificationChats returns which of ids are live group chats with at
// least one current member of the organization in them.
func (a *AppDB) ValidNotificationChats(ctx context.Context, organizationId int64, ids []int64) ([]int64, error) {
	rows, err := a.db.Query(ctx, `
		SELECT c.id
		FROM conversations c
		WHERE c.id = ANY($2::bigint[]) AND c.kind = 'group' AND c.deleted_at IS NULL
			AND EXISTS (
				SELECT 1 FROM conversation_members cm
				JOIN organization_members om ON om.user_id = cm.user_id AND om.organization_id = $1
				WHERE cm.conversation_id = c.id AND cm.left_at IS NULL
			);
	`, organizationId, ids)
	if err != nil {
		return nil, fmt.Errorf("error checking group chats: %w", err)
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
