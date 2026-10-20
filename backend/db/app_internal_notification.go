package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// Internal organization alerts and the per-user notification ledger.
//
// The one rule worth stating up front: posting an alert is idempotent in its
// alert_key. Every caller here runs on a timer and will report the same
// condition over and over for as long as it holds, so "post" has to mean "make
// sure this is on the board" rather than "add this to the board". The partial
// unique index on (organization_id, alert_key) WHERE resolved_at IS NULL is
// what enforces it, and the upsert below is what uses it.

// ErrNoHouseOrganization is returned when an alert is posted before the house
// organization exists. Alerts are addressed to an organization, and on a fresh
// database there is not one yet.
var ErrNoHouseOrganization = errors.New("no house organization")

// HouseOrganizationId returns the id of SFLuv's own organization.
//
// Matched on is_house rather than on the name, for the same reason the
// permission section is: a rename must not be able to move where alerts go.
func (a *AppDB) HouseOrganizationId(ctx context.Context) (int64, error) {
	var id int64
	err := a.db.QueryRow(ctx, `
		SELECT id FROM organizations WHERE is_house = TRUE ORDER BY id ASC LIMIT 1;
	`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoHouseOrganization
	}
	if err != nil {
		return 0, fmt.Errorf("error finding the house organization: %w", err)
	}
	return id, nil
}

// InternalAlert is what a caller posts.
type InternalAlert struct {
	OrganizationId int64
	Set            string
	AlertKey       string
	Kind           string
	Title          string
	Body           string
	Fields         []structs.NotificationField
	Payload        map[string]string
	Urgent         bool
}

// PostInternalAlert puts an alert on the board, or refreshes the one already
// there, and returns its id along with whether this call created it.
//
// The created flag is what callers use to decide whether to push. A condition
// that has been true for an hour should not buzz a phone every minute, so the
// push happens on creation and a later refresh only updates the text. An alert
// whose text has not changed is not even written, so updated_at stays honest:
// it means "this changed", not "this was checked".
func (a *AppDB) PostInternalAlert(ctx context.Context, alert InternalAlert) (int64, bool, error) {
	fields, err := json.Marshal(normalizeFields(alert.Fields))
	if err != nil {
		return 0, false, fmt.Errorf("error encoding alert fields: %w", err)
	}
	payload, err := json.Marshal(normalizePayload(alert.Payload))
	if err != nil {
		return 0, false, fmt.Errorf("error encoding alert payload: %w", err)
	}

	var id int64
	var created bool
	err = a.db.QueryRow(ctx, `
		INSERT INTO internal_notifications
			(organization_id, notification_set, alert_key, kind, title, body, fields, payload, urgent)
		VALUES
			($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9)
		ON CONFLICT (organization_id, alert_key) WHERE resolved_at IS NULL
		DO UPDATE SET
			title = EXCLUDED.title,
			body = EXCLUDED.body,
			fields = EXCLUDED.fields,
			payload = EXCLUDED.payload,
			urgent = EXCLUDED.urgent,
			notification_set = EXCLUDED.notification_set,
			kind = EXCLUDED.kind,
			-- Only when something actually changed. A sweep that finds the same
			-- problem it found a minute ago has not updated anything, and
			-- stamping updated_at would make every alert look like it had just
			-- happened.
			updated_at = CASE
				WHEN internal_notifications.title IS DISTINCT FROM EXCLUDED.title
					OR internal_notifications.body IS DISTINCT FROM EXCLUDED.body
					OR internal_notifications.fields IS DISTINCT FROM EXCLUDED.fields
					OR internal_notifications.urgent IS DISTINCT FROM EXCLUDED.urgent
				THEN NOW()
				ELSE internal_notifications.updated_at
			END
		RETURNING id, (xmax = 0) AS created;
	`,
		alert.OrganizationId, alert.Set, alert.AlertKey, alert.Kind,
		alert.Title, alert.Body, string(fields), string(payload), alert.Urgent,
	).Scan(&id, &created)
	if err != nil {
		return 0, false, fmt.Errorf("error posting internal alert %s: %w", alert.AlertKey, err)
	}
	return id, created, nil
}

// DeliverInternalAlert gives every subscriber of the alert's set a row in their
// own ledger, and returns the users who had not already received it.
//
// Returning only the new recipients is what keeps a push from repeating: a
// member who subscribes to a set while an alert is already live gets the alert
// (which is right — it is still true) and gets it once.
func (a *AppDB) DeliverInternalAlert(ctx context.Context, notificationId int64) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		INSERT INTO user_notifications
			(user_id, kind, title, body, data, internal_notification_id, created_at)
		SELECT
			s.user_id,
			n.kind,
			n.title,
			n.body,
			jsonb_build_object('type', 'internal_' || n.kind, 'notification_set', n.notification_set),
			n.id,
			n.created_at
		FROM internal_notifications n
		JOIN organization_notification_subscriptions s
			ON s.organization_id = n.organization_id
			AND s.notification_set = n.notification_set
		WHERE n.id = $1
		AND n.resolved_at IS NULL
		ON CONFLICT (internal_notification_id, user_id) WHERE internal_notification_id IS NOT NULL
		DO NOTHING
		RETURNING user_id;
	`, notificationId)
	if err != nil {
		return nil, fmt.Errorf("error delivering internal alert %d: %w", notificationId, err)
	}
	defer rows.Close()

	recipients := []string{}
	for rows.Next() {
		var userId string
		if err := rows.Scan(&userId); err != nil {
			return nil, err
		}
		recipients = append(recipients, userId)
	}
	return recipients, rows.Err()
}

// ResolveInternalAlert retires one alert by key, recording how it ended.
//
// Returns false when there was nothing live under that key, which is the
// ordinary case and not an error: a sweep reporting that everything is fine
// calls this for conditions that were never raised.
func (a *AppDB) ResolveInternalAlert(
	ctx context.Context, organizationId int64, alertKey string, resolution string, resolvedBy string, outcome string,
) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE internal_notifications
		SET resolved_at = NOW(), resolution = $3, resolved_by = $4, outcome = $5
		WHERE organization_id = $1 AND alert_key = $2 AND resolved_at IS NULL;
	`, organizationId, alertKey, resolution, resolvedBy, outcome)
	if err != nil {
		return false, fmt.Errorf("error resolving internal alert %s: %w", alertKey, err)
	}
	return tag.RowsAffected() > 0, nil
}

// ResolveInternalAlertById retires one alert by id, and reports whether this
// call was the one that did it.
//
// The WHERE clause carrying resolved_at IS NULL is the whole concurrency story
// for action buttons. Two admins pressing Approve at the same moment both reach
// here; exactly one updates a row, and the loser is told somebody got there
// first rather than running the approval twice.
func (a *AppDB) ResolveInternalAlertById(
	ctx context.Context, notificationId int64, resolution string, resolvedBy string, outcome string,
) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE internal_notifications
		SET resolved_at = NOW(), resolution = $2, resolved_by = $3, outcome = $4
		WHERE id = $1 AND resolved_at IS NULL;
	`, notificationId, resolution, resolvedBy, outcome)
	if err != nil {
		return false, fmt.Errorf("error resolving internal alert %d: %w", notificationId, err)
	}
	return tag.RowsAffected() > 0, nil
}

// LiveInternalAlertKeys lists the unresolved alert keys under a prefix.
//
// This is how a sweep reconciles: it knows what is wrong now, this says what
// was wrong before, and the difference is what to post and what to retire.
// Namespaced by prefix so one source cannot retire another's alerts.
func (a *AppDB) LiveInternalAlertKeys(ctx context.Context, organizationId int64, prefix string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT alert_key FROM internal_notifications
		WHERE organization_id = $1 AND resolved_at IS NULL AND alert_key LIKE $2 || '%';
	`, organizationId, prefix)
	if err != nil {
		return nil, fmt.Errorf("error listing live alert keys: %w", err)
	}
	defer rows.Close()

	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// LiveInternalAlert loads one live alert by key, for a caller that needs to
// know whether it already posted something and what it said.
func (a *AppDB) LiveInternalAlert(ctx context.Context, organizationId int64, alertKey string) (*InternalAlertRow, error) {
	row := &InternalAlertRow{}
	var fields, payload []byte
	err := a.db.QueryRow(ctx, `
		SELECT id, organization_id, notification_set, alert_key, kind, title, body,
			fields, payload, urgent, created_at, updated_at
		FROM internal_notifications
		WHERE organization_id = $1 AND alert_key = $2 AND resolved_at IS NULL;
	`, organizationId, alertKey).Scan(
		&row.Id, &row.OrganizationId, &row.Set, &row.AlertKey, &row.Kind, &row.Title, &row.Body,
		&fields, &payload, &row.Urgent, &row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error loading live alert %s: %w", alertKey, err)
	}
	_ = json.Unmarshal(fields, &row.Fields)
	_ = json.Unmarshal(payload, &row.Payload)
	return row, nil
}

// InternalAlertRow is a stored alert.
type InternalAlertRow struct {
	Id             int64
	OrganizationId int64
	Set            string
	AlertKey       string
	Kind           string
	Title          string
	Body           string
	Fields         []structs.NotificationField
	Payload        map[string]string
	Urgent         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// GetInternalAlertById loads one alert whatever its state, for an action
// handler that needs its payload.
func (a *AppDB) GetInternalAlertById(ctx context.Context, notificationId int64) (*InternalAlertRow, bool, error) {
	row := &InternalAlertRow{}
	var fields, payload []byte
	var resolvedAt *time.Time
	err := a.db.QueryRow(ctx, `
		SELECT id, organization_id, notification_set, alert_key, kind, title, body,
			fields, payload, urgent, created_at, updated_at, resolved_at
		FROM internal_notifications
		WHERE id = $1;
	`, notificationId).Scan(
		&row.Id, &row.OrganizationId, &row.Set, &row.AlertKey, &row.Kind, &row.Title, &row.Body,
		&fields, &payload, &row.Urgent, &row.CreatedAt, &row.UpdatedAt, &resolvedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("error loading alert %d: %w", notificationId, err)
	}
	_ = json.Unmarshal(fields, &row.Fields)
	_ = json.Unmarshal(payload, &row.Payload)
	return row, resolvedAt == nil, nil
}

// ── Subscriptions ────────────────────────────────────────────────────────────

// GetOrganizationNotificationSubscriptions returns which sets each member of an
// organization receives.
func (a *AppDB) GetOrganizationNotificationSubscriptions(ctx context.Context, organizationId int64) (map[string][]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT user_id, notification_set
		FROM organization_notification_subscriptions
		WHERE organization_id = $1
		ORDER BY user_id, notification_set;
	`, organizationId)
	if err != nil {
		return nil, fmt.Errorf("error loading notification subscriptions: %w", err)
	}
	defer rows.Close()

	out := map[string][]string{}
	for rows.Next() {
		var userId, set string
		if err := rows.Scan(&userId, &set); err != nil {
			return nil, err
		}
		out[userId] = append(out[userId], set)
	}
	return out, rows.Err()
}

// GetUserNotificationSets returns the sets one user receives.
func (a *AppDB) GetUserNotificationSets(ctx context.Context, userId string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT notification_set FROM organization_notification_subscriptions
		WHERE user_id = $1 ORDER BY notification_set;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error loading notification sets for %s: %w", userId, err)
	}
	defer rows.Close()

	sets := []string{}
	for rows.Next() {
		var set string
		if err := rows.Scan(&set); err != nil {
			return nil, err
		}
		sets = append(sets, set)
	}
	return sets, rows.Err()
}

// UserReceivesSet reports whether a user is designated for one set, which is
// the authorization check for running an alert's action button.
func (a *AppDB) UserReceivesSet(ctx context.Context, organizationId int64, userId string, set string) (bool, error) {
	var exists bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organization_notification_subscriptions
			WHERE organization_id = $1 AND user_id = $2 AND notification_set = $3
		);
	`, organizationId, userId, set).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking notification subscription: %w", err)
	}
	return exists, nil
}

// SetOrganizationNotificationSets replaces one member's subscriptions.
//
// Delete-then-insert inside a transaction, rather than a diff: the request
// carries the complete intended set, so reconstructing which rows changed would
// be work in service of the same result.
//
// Newly added sets do not back-deliver what is already live. That is a
// deliberate choice against the "an alert is a condition" grain, and it is
// about the first hour rather than the steady state: being handed every
// outstanding alert the moment you are added to a set is how people turn a
// notification off. Anything still true will be re-reported by its own sweep
// within the minute, which brings them in on their own.
func (a *AppDB) SetOrganizationNotificationSets(
	ctx context.Context, organizationId int64, userId string, sets []string, actor string,
) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting notification subscription update: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM organization_notification_subscriptions
		WHERE organization_id = $1 AND user_id = $2;
	`, organizationId, userId); err != nil {
		return fmt.Errorf("error clearing notification subscriptions: %w", err)
	}

	cleaned := make([]string, 0, len(sets))
	seen := map[string]bool{}
	for _, set := range sets {
		set = strings.TrimSpace(set)
		if set == "" || seen[set] || !structs.IsNotificationSet(set) {
			continue
		}
		seen[set] = true
		cleaned = append(cleaned, set)
	}

	if len(cleaned) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_notification_subscriptions
				(organization_id, user_id, notification_set, created_by)
			SELECT $1, $2, UNNEST($3::text[]), $4
			ON CONFLICT DO NOTHING;
		`, organizationId, userId, cleaned, actor); err != nil {
			return fmt.Errorf("error saving notification subscriptions: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// LiveInternalAlertCounts is how many unresolved alerts sit in each set.
func (a *AppDB) LiveInternalAlertCounts(ctx context.Context, organizationId int64) (map[string]int, error) {
	rows, err := a.db.Query(ctx, `
		SELECT notification_set, COUNT(*)
		FROM internal_notifications
		WHERE organization_id = $1 AND resolved_at IS NULL
		GROUP BY notification_set;
	`, organizationId)
	if err != nil {
		return nil, fmt.Errorf("error counting live alerts: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var set string
		var count int
		if err := rows.Scan(&set, &count); err != nil {
			return nil, err
		}
		out[set] = count
	}
	return out, rows.Err()
}

// ── The per-user ledger ──────────────────────────────────────────────────────

// RecordUserNotification writes one ordinary notification to a user's history.
//
// Called beside a push rather than instead of it: the push is the thing that
// buzzes, this is the thing that is still there tomorrow. A failure here is
// returned but every caller logs and carries on, because losing a history row
// is not a reason to not send the notification.
func (a *AppDB) RecordUserNotification(
	ctx context.Context, userId string, kind string, title string, body string, data map[string]string,
) (int64, error) {
	encoded, err := json.Marshal(normalizePayload(data))
	if err != nil {
		return 0, fmt.Errorf("error encoding notification data: %w", err)
	}
	var id int64
	if err := a.db.QueryRow(ctx, `
		INSERT INTO user_notifications (user_id, kind, title, body, data)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		RETURNING id;
	`, userId, kind, title, body, string(encoded)).Scan(&id); err != nil {
		return 0, fmt.Errorf("error recording notification for %s: %w", userId, err)
	}
	return id, nil
}

// notificationPageSize bounds a history page. The client asks for a size and
// this caps it, so a client cannot ask for the whole table.
const notificationPageSize = 30

// maxNotificationPageSize is the ceiling on a client-requested page.
const maxNotificationPageSize = 100

// GetNotificationFeed builds the notification screen for one user.
//
// Two queries rather than one, matching the two halves of the screen. Active is
// unbounded-but-small: unseen ordinary notifications plus every live alert
// addressed to them. History is a keyset page, walked with a cursor.
//
// Keyset rather than OFFSET because the feed is written to while it is being
// read. With OFFSET, an alert arriving between page one and page two shifts
// every later row down and the client sees one notification twice and misses
// another; with (created_at, id) as the cursor, a new row simply is not in the
// pages already fetched.
func (a *AppDB) GetNotificationFeed(
	ctx context.Context, userId string, cursor string, limit int,
) (*structs.NotificationFeed, error) {
	if limit <= 0 {
		limit = notificationPageSize
	}
	if limit > maxNotificationPageSize {
		limit = maxNotificationPageSize
	}

	feed := &structs.NotificationFeed{
		Active:  []structs.UserNotification{},
		History: []structs.UserNotification{},
	}

	sets, err := a.GetUserNotificationSets(ctx, userId)
	if err != nil {
		return nil, err
	}
	feed.Sets = sets

	// Active: an ordinary notification is active until it has been seen; an
	// internal alert is active until it is resolved, whether or not anybody
	// has looked at it. That difference is the whole point — an unhandled
	// approval request should not stop asking because somebody scrolled past
	// it.
	activeRows, err := a.db.Query(ctx, `
		SELECT
			un.id, un.kind, un.title, un.body, un.data, un.created_at, un.seen_at,
			n.id, n.notification_set, n.alert_key, n.fields, n.payload, n.urgent, n.updated_at,
			n.resolved_at, n.outcome
		FROM user_notifications un
		LEFT JOIN internal_notifications n ON n.id = un.internal_notification_id
		WHERE un.user_id = $1
		AND (
			(un.internal_notification_id IS NULL AND un.seen_at IS NULL)
			OR (un.internal_notification_id IS NOT NULL AND n.resolved_at IS NULL)
		)
		-- COALESCE rather than NULLS LAST: an ordinary notification has no
		-- urgent flag at all, and sorting NULL last would push every
		-- transaction below every alert however old the alert was. Urgent
		-- first, then strict recency.
		ORDER BY COALESCE(n.urgent, FALSE) DESC, un.created_at DESC, un.id DESC
		LIMIT $2;
	`, userId, maxNotificationPageSize)
	if err != nil {
		return nil, fmt.Errorf("error loading active notifications: %w", err)
	}
	active, err := scanNotifications(activeRows)
	if err != nil {
		return nil, err
	}
	feed.Active = active

	// Counted in SQL rather than taken from len(active), because the list
	// above is capped. A badge reading 100 while 140 things are outstanding is
	// wrong in the direction that matters, and the cap exists to bound the
	// payload rather than to bound the truth.
	if err := a.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM user_notifications un
		LEFT JOIN internal_notifications n ON n.id = un.internal_notification_id
		WHERE un.user_id = $1
		AND (
			(un.internal_notification_id IS NULL AND un.seen_at IS NULL)
			OR (un.internal_notification_id IS NOT NULL AND n.resolved_at IS NULL)
		);
	`, userId).Scan(&feed.UnseenCount); err != nil {
		return nil, fmt.Errorf("error counting outstanding notifications: %w", err)
	}

	// History: everything no longer asking for anything.
	where := `
		un.user_id = $1
		AND (
			(un.internal_notification_id IS NULL AND un.seen_at IS NOT NULL)
			OR (un.internal_notification_id IS NOT NULL AND n.resolved_at IS NOT NULL)
		)`
	args := []any{userId}
	if cursor != "" {
		at, id, err := parseNotificationCursor(cursor)
		if err != nil {
			return nil, err
		}
		args = append(args, at, id)
		where += ` AND (un.created_at, un.id) < ($2, $3)`
	}
	args = append(args, limit+1)

	historyRows, err := a.db.Query(ctx, fmt.Sprintf(`
		SELECT
			un.id, un.kind, un.title, un.body, un.data, un.created_at, un.seen_at,
			n.id, n.notification_set, n.alert_key, n.fields, n.payload, n.urgent, n.updated_at,
			n.resolved_at, n.outcome
		FROM user_notifications un
		LEFT JOIN internal_notifications n ON n.id = un.internal_notification_id
		WHERE %s
		ORDER BY un.created_at DESC, un.id DESC
		LIMIT $%d;
	`, where, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("error loading notification history: %w", err)
	}
	history, err := scanNotifications(historyRows)
	if err != nil {
		return nil, err
	}

	// One row more than asked for was fetched, purely to learn whether there is
	// another page. Asking for a page and getting one back is not evidence of
	// the end of the list, and a client that stops only on an empty page always
	// makes one wasted request at the bottom.
	if len(history) > limit {
		last := history[limit-1]
		feed.NextCursor = formatNotificationCursor(last.CreatedAt, last.Id)
		history = history[:limit]
	}
	feed.History = history

	return feed, nil
}

func scanNotifications(rows pgx.Rows) ([]structs.UserNotification, error) {
	defer rows.Close()

	out := []structs.UserNotification{}
	for rows.Next() {
		var item structs.UserNotification
		var data []byte
		var seenAt *time.Time

		var internalId *int64
		var set, alertKey, outcome *string
		var fields, payload []byte
		var urgent *bool
		var updatedAt, resolvedAt *time.Time

		if err := rows.Scan(
			&item.Id, &item.Kind, &item.Title, &item.Body, &data, &item.CreatedAt, &seenAt,
			&internalId, &set, &alertKey, &fields, &payload, &urgent, &updatedAt,
			&resolvedAt, &outcome,
		); err != nil {
			return nil, fmt.Errorf("error scanning notification: %w", err)
		}

		item.Seen = seenAt != nil
		if len(data) > 0 {
			_ = json.Unmarshal(data, &item.Data)
		}

		if internalId != nil {
			detail := &structs.InternalNotificationDetail{
				Id:       *internalId,
				AlertKey: derefString(alertKey),
				Set:      derefString(set),
				Urgent:   urgent != nil && *urgent,
				Resolved: resolvedAt != nil,
				Outcome:  derefString(outcome),
			}
			if updatedAt != nil {
				detail.UpdatedAt = *updatedAt
			}
			detail.ResolvedAt = resolvedAt
			if len(fields) > 0 {
				_ = json.Unmarshal(fields, &detail.Fields)
			}
			// The payload is carried on the row for the action handler, not for
			// the client: it holds internal ids and is not part of the API.
			_ = payload
			item.Internal = detail
		}

		out = append(out, item)
	}
	return out, rows.Err()
}

// MarkNotificationsSeen stamps ordinary notifications as seen.
//
// Internal alerts are excluded in SQL rather than filtered by the caller, so
// there is no route by which viewing the screen can retire an alert that still
// needs somebody to do something. Those end by having their action run or
// their condition clear, and nothing else.
func (a *AppDB) MarkNotificationsSeen(ctx context.Context, userId string, ids []int64, all bool) (int64, error) {
	if all {
		tag, err := a.db.Exec(ctx, `
			UPDATE user_notifications
			SET seen_at = NOW()
			WHERE user_id = $1 AND seen_at IS NULL AND internal_notification_id IS NULL;
		`, userId)
		if err != nil {
			return 0, fmt.Errorf("error marking notifications seen: %w", err)
		}
		return tag.RowsAffected(), nil
	}
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := a.db.Exec(ctx, `
		UPDATE user_notifications
		SET seen_at = NOW()
		WHERE user_id = $1 AND seen_at IS NULL AND internal_notification_id IS NULL
		AND id = ANY($2::bigint[]);
	`, userId, ids)
	if err != nil {
		return 0, fmt.Errorf("error marking notifications seen: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PruneNotificationHistory bounds the ledger.
//
// Two rules, because the two kinds age differently. An ordinary notification is
// a past event and is worth a few months. An internal alert is a record of
// something operational being wrong and who dealt with it, so it is kept
// longer — that is the question this table gets asked.
func (a *AppDB) PruneNotificationHistory(ctx context.Context) (int64, error) {
	tag, err := a.db.Exec(ctx, `
		DELETE FROM user_notifications
		WHERE internal_notification_id IS NULL
		AND seen_at IS NOT NULL
		AND created_at < NOW() - INTERVAL '120 days';
	`)
	if err != nil {
		return 0, fmt.Errorf("error pruning notification history: %w", err)
	}
	removed := tag.RowsAffected()

	alertTag, err := a.db.Exec(ctx, `
		DELETE FROM internal_notifications
		WHERE resolved_at IS NOT NULL
		AND resolved_at < NOW() - INTERVAL '400 days';
	`)
	if err != nil {
		return removed, fmt.Errorf("error pruning resolved alerts: %w", err)
	}
	return removed + alertTag.RowsAffected(), nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func normalizeFields(fields []structs.NotificationField) []structs.NotificationField {
	out := make([]structs.NotificationField, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Label) == "" && strings.TrimSpace(field.Value) == "" {
			continue
		}
		out = append(out, field)
	}
	return out
}

func normalizePayload(payload map[string]string) map[string]string {
	if payload == nil {
		return map[string]string{}
	}
	return payload
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// The cursor is the sort key, not an opaque token: "<unix micros>:<id>". It is
// readable on purpose — a paginated feed that misbehaves is much easier to
// reason about when the cursor in the request says where it thinks it is.
func formatNotificationCursor(at time.Time, id int64) string {
	return fmt.Sprintf("%d:%d", at.UTC().UnixMicro(), id)
}

func parseNotificationCursor(cursor string) (time.Time, int64, error) {
	parts := strings.SplitN(cursor, ":", 2)
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("malformed notification cursor")
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("malformed notification cursor")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("malformed notification cursor")
	}
	return time.UnixMicro(micros).UTC(), id, nil
}

// ReopenInternalAlert undoes a claim whose action then failed.
//
// Claim-then-run means a failure has to be put back, or a transient error
// would retire an alert whose condition is still true — the worst outcome
// available, since it looks exactly like the problem having been fixed.
//
// Guarded on resolution = 'acted' so this can only ever reopen a row that an
// action claimed. A condition that cleared on its own, or an acknowledgement,
// is not something a failed button press may resurrect.
func (a *AppDB) ReopenInternalAlert(ctx context.Context, notificationId int64) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE internal_notifications
		SET resolved_at = NULL, resolved_by = '', resolution = '', outcome = ''
		WHERE id = $1 AND resolved_at IS NOT NULL AND resolution = $2;
	`, notificationId, structs.ResolutionActed)
	if err != nil {
		return false, fmt.Errorf("error reopening alert %d: %w", notificationId, err)
	}
	return tag.RowsAffected() > 0, nil
}

// SetInternalAlertOutcome records the sentence the history row shows.
//
// Written after the action has run rather than when it was claimed, because
// until then there is nothing true to say: "Approved by Dana" is a claim about
// something having happened.
func (a *AppDB) SetInternalAlertOutcome(ctx context.Context, notificationId int64, outcome string) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE internal_notifications SET outcome = $2 WHERE id = $1;
	`, notificationId, outcome); err != nil {
		return fmt.Errorf("error recording outcome for alert %d: %w", notificationId, err)
	}
	return nil
}

// AcknowledgeInternalAlert resolves an alert as acknowledged rather than acted
// on, for a button whose whole job is to say "seen, and nothing else".
func (a *AppDB) AcknowledgeInternalAlert(
	ctx context.Context, notificationId int64, userId string, outcome string,
) (bool, error) {
	return a.ResolveInternalAlertById(ctx, notificationId, structs.ResolutionAcked, userId, outcome)
}
