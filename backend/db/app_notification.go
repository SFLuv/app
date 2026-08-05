package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GetImproverNotifications builds the improver notification feed from live
// workflow state.
//
// Nothing is materialized: a pending-payout notification exists exactly as long
// as the payout is actually pending, so it cannot linger after the money lands
// and cannot be missed if a payout regresses. The only stored state is the
// per-user seen marker, which is what the unread bubble counts.
func (a *AppDB) GetImproverNotifications(ctx context.Context, improverId string) (*structs.ImproverNotificationFeed, error) {
	feed := &structs.ImproverNotificationFeed{Notifications: []structs.ImproverNotification{}}

	rows, err := a.db.Query(ctx, `
		SELECT
			'workflow_payout_pending:' || ws.workflow_id || ':' || ws.id AS key,
			ws.workflow_id,
			COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')) AS workflow_title,
			ws.id AS step_id,
			ws.title AS step_title,
			FALSE AS is_manager,
			ws.bounty,
			COALESCE(ws.payout_error, '') AS payout_error,
			COALESCE(ws.completed_at, ws.updated_at) AS created_at,
			r.seen_at
		FROM
			workflow_steps ws
		INNER JOIN
			workflows w
		ON
			w.id = ws.workflow_id
		LEFT JOIN
			workflow_states st
		ON
			st.id = w.workflow_state_id
		LEFT JOIN
			workflow_series s
		ON
			s.id = w.series_id
		LEFT JOIN
			improver_notification_reads r
		ON
			r.user_id = $1
		AND
			r.notification_key = 'workflow_payout_pending:' || ws.workflow_id || ':' || ws.id
		WHERE
			ws.assigned_improver_id = $1
		AND
			ws.status = 'completed'
		AND
			ws.bounty > 0

		UNION ALL

		SELECT
			'workflow_payout_pending:' || w.id || ':manager' AS key,
			w.id AS workflow_id,
			COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')) AS workflow_title,
			'' AS step_id,
			'' AS step_title,
			TRUE AS is_manager,
			w.manager_bounty AS bounty,
			COALESCE(w.manager_payout_error, '') AS payout_error,
			w.updated_at AS created_at,
			r.seen_at
		FROM
			workflows w
		LEFT JOIN
			workflow_states st
		ON
			st.id = w.workflow_state_id
		LEFT JOIN
			workflow_series s
		ON
			s.id = w.series_id
		LEFT JOIN
			improver_notification_reads r
		ON
			r.user_id = $1
		AND
			r.notification_key = 'workflow_payout_pending:' || w.id || ':manager'
		WHERE
			w.manager_improver_id = $1
		AND
			w.status = 'completed'
		AND
			w.manager_bounty > 0
		AND
			w.manager_paid_out_at IS NULL

		ORDER BY
			created_at DESC;
	`, improverId)
	if err != nil {
		return nil, fmt.Errorf("error querying improver notifications: %s", err)
	}
	defer rows.Close()

	for rows.Next() {
		notification := structs.ImproverNotification{Type: structs.NotificationTypeWorkflowPayoutPending}
		var seenAt *int64
		if err := rows.Scan(
			&notification.Key,
			&notification.WorkflowId,
			&notification.WorkflowTitle,
			&notification.StepId,
			&notification.StepTitle,
			&notification.IsManager,
			&notification.AmountSfluv,
			&notification.PayoutError,
			&notification.CreatedAt,
			&seenAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning improver notification: %s", err)
		}

		notification.Seen = seenAt != nil
		notification.SeenAt = seenAt
		notification.Title = "Payout pending"
		if notification.IsManager {
			notification.Body = fmt.Sprintf("Your manager payout for %s hasn't landed yet.", notification.WorkflowTitle)
		} else {
			notification.Body = fmt.Sprintf("Your payout for %s hasn't landed yet.", notification.StepTitle)
		}
		if strings.TrimSpace(notification.PayoutError) != "" {
			notification.Body = fmt.Sprintf("%s We hit an issue: %s", notification.Body, notification.PayoutError)
		}

		if !notification.Seen {
			feed.UnseenCount++
		}
		feed.Notifications = append(feed.Notifications, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	feed.Total = len(feed.Notifications)
	feed.HasUnseen = feed.UnseenCount > 0

	return feed, nil
}

// MarkImproverNotificationsSeen records seen markers. Passing no keys marks
// every currently-visible notification seen, which is what opening the bell
// does. Markers are idempotent so a repeated call cannot double-count.
func (a *AppDB) MarkImproverNotificationsSeen(ctx context.Context, improverId string, keys []string) (int, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	cleaned := make([]string, 0, len(keys))
	for _, key := range keys {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return 0, nil
	}

	tag, err := a.db.Exec(ctx, `
		INSERT INTO improver_notification_reads
			(user_id, notification_key)
		SELECT
			$1, UNNEST($2::text[])
		ON CONFLICT (user_id, notification_key) DO NOTHING;
	`, improverId, cleaned)
	if err != nil {
		return 0, fmt.Errorf("error marking improver notifications seen: %s", err)
	}

	return int(tag.RowsAffected()), nil
}

// PruneResolvedImproverNotificationReads deletes seen markers whose underlying
// condition has cleared, so the table does not grow without bound as payouts
// settle. Safe to run at any time: a marker for a live notification is never
// removed.
func (a *AppDB) PruneResolvedImproverNotificationReads(ctx context.Context) (int64, error) {
	tag, err := a.db.Exec(ctx, `
		DELETE FROM improver_notification_reads r
		WHERE r.notification_key LIKE 'workflow_payout_pending:%'
		AND NOT EXISTS (
			SELECT 1
			FROM workflow_steps ws
			WHERE ws.assigned_improver_id = r.user_id
			AND ws.status = 'completed'
			AND ws.bounty > 0
			AND r.notification_key = 'workflow_payout_pending:' || ws.workflow_id || ':' || ws.id
		)
		AND NOT EXISTS (
			SELECT 1
			FROM workflows w
			WHERE w.manager_improver_id = r.user_id
			AND w.status = 'completed'
			AND w.manager_bounty > 0
			AND w.manager_paid_out_at IS NULL
			AND r.notification_key = 'workflow_payout_pending:' || w.id || ':manager'
		);
	`)
	if err != nil {
		return 0, fmt.Errorf("error pruning resolved improver notification reads: %s", err)
	}

	return tag.RowsAffected(), nil
}

// Volunteer email list subscription states.
const (
	VolunteerListPending      = "pending"
	VolunteerListActive       = "active"
	VolunteerListUnsubscribed = "unsubscribed"
)

// UpsertVolunteerListSubscription records an opt-in.
//
// active=true writes the row straight to 'active' (PJ's ruling: an
// authenticated user is auto-confirmed, since a logged-in account with a known
// address is not the spam vector an anonymous form is). active=false leaves it
// 'pending' until the double opt-in link is followed. An existing 'active' row
// is never downgraded by a later pending opt-in.
func (a *AppDB) UpsertVolunteerListSubscription(
	ctx context.Context, email string, firstName string, lastName string, sourceEventId string, active bool,
) (string, string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", "", fmt.Errorf("email is required")
	}

	status := VolunteerListPending
	if active {
		status = VolunteerListActive
	}

	id := uuid.NewString()
	confirmToken := uuid.NewString()
	unsubscribeToken := uuid.NewString()

	var resultStatus, resultConfirmToken string
	err := a.db.QueryRow(ctx, `
		INSERT INTO volunteer_email_list
			(id, email, first_name, last_name, status, confirm_token, unsubscribe_token, source_event_id, confirmed_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, CASE WHEN $5 = 'active' THEN unix_now() ELSE NULL END)
		ON CONFLICT (LOWER(email)) DO UPDATE SET
			first_name = COALESCE(NULLIF(EXCLUDED.first_name, ''), volunteer_email_list.first_name),
			last_name = COALESCE(NULLIF(EXCLUDED.last_name, ''), volunteer_email_list.last_name),
			-- Never downgrade an existing active subscription, and treat a fresh
			-- opt-in as re-subscribing if they had previously unsubscribed.
			status = CASE
				WHEN volunteer_email_list.status = 'active' THEN 'active'
				ELSE EXCLUDED.status
			END,
			confirmed_at = CASE
				WHEN volunteer_email_list.status = 'active' THEN volunteer_email_list.confirmed_at
				WHEN EXCLUDED.status = 'active' THEN unix_now()
				ELSE volunteer_email_list.confirmed_at
			END,
			unsubscribed_at = NULL
		RETURNING status, confirm_token;
	`, id, email, firstName, lastName, status, confirmToken, unsubscribeToken, sourceEventId).Scan(&resultStatus, &resultConfirmToken)
	if err != nil {
		return "", "", fmt.Errorf("error recording volunteer list opt-in: %s", err)
	}

	return resultStatus, resultConfirmToken, nil
}

// GetVolunteerListSubscriptionByEmail reports the caller's current membership so
// a client can render an opt-in toggle that reflects reality rather than always
// defaulting to on.
func (a *AppDB) GetVolunteerListSubscriptionByEmail(ctx context.Context, email string) (string, error) {
	var status string
	err := a.db.QueryRow(ctx, `
		SELECT status FROM volunteer_email_list WHERE LOWER(email) = LOWER($1);
	`, strings.TrimSpace(email)).Scan(&status)
	if err != nil {
		return "", err
	}
	return status, nil
}

// SetVolunteerListStateByToken confirms or unsubscribes via a single-use token.
// Both are POST-only at the handler layer: mail scanners prefetch links, and a
// GET that mutates would confirm or unsubscribe people who never clicked.
func (a *AppDB) SetVolunteerListStateByToken(ctx context.Context, token string, confirm bool) (string, string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", fmt.Errorf("token is required")
	}

	var email, status string
	var err error
	if confirm {
		err = a.db.QueryRow(ctx, `
			UPDATE volunteer_email_list
			SET status = 'active', confirmed_at = unix_now(), confirm_token = ''
			WHERE confirm_token = $1 AND confirm_token <> ''
			RETURNING email, status;
		`, token).Scan(&email, &status)
	} else {
		err = a.db.QueryRow(ctx, `
			UPDATE volunteer_email_list
			SET status = 'unsubscribed', unsubscribed_at = unix_now()
			WHERE unsubscribe_token = $1 AND unsubscribe_token <> ''
			RETURNING email, status;
		`, token).Scan(&email, &status)
	}
	if err != nil {
		return "", "", err
	}
	return email, status, nil
}

// PeekVolunteerListByToken is the read half of the token flow: it renders the
// landing page without mutating anything, so a prefetching mail scanner cannot
// complete the action on the user's behalf.
func (a *AppDB) PeekVolunteerListByToken(ctx context.Context, token string, confirm bool) (string, string, error) {
	column := "unsubscribe_token"
	if confirm {
		column = "confirm_token"
	}

	var email, status string
	err := a.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT email, status FROM volunteer_email_list WHERE %s = $1 AND %s <> '';
	`, column, column), strings.TrimSpace(token)).Scan(&email, &status)
	if err != nil {
		return "", "", err
	}
	return email, status, nil
}

// Reminder preference bounds. Validated server-side rather than trusting a
// client, since these drive scheduled sends.
const (
	VolunteerReminderMinHours     = 1
	VolunteerReminderMaxHours     = 168
	VolunteerReminderDefaultHours = 24
)

// GetVolunteerReminderPreference returns the caller's preference, defaulting to
// enabled at 24 hours when they have never set one. Defaulting here rather than
// in a client means the value the client shows is the value the sender uses.
func (a *AppDB) GetVolunteerReminderPreference(ctx context.Context, userId string) (bool, int, error) {
	enabled := true
	hoursBefore := VolunteerReminderDefaultHours

	err := a.db.QueryRow(ctx, `
		SELECT enabled, hours_before FROM volunteer_reminder_preferences WHERE user_id = $1;
	`, userId).Scan(&enabled, &hoursBefore)
	if err != nil {
		if err == pgx.ErrNoRows {
			return true, VolunteerReminderDefaultHours, nil
		}
		return true, VolunteerReminderDefaultHours, fmt.Errorf("error loading reminder preference: %s", err)
	}

	return enabled, hoursBefore, nil
}

func (a *AppDB) SetVolunteerReminderPreference(ctx context.Context, userId string, enabled bool, hoursBefore int) error {
	if hoursBefore < VolunteerReminderMinHours {
		hoursBefore = VolunteerReminderMinHours
	}
	if hoursBefore > VolunteerReminderMaxHours {
		hoursBefore = VolunteerReminderMaxHours
	}

	_, err := a.db.Exec(ctx, `
		INSERT INTO volunteer_reminder_preferences (user_id, enabled, hours_before)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			hours_before = EXCLUDED.hours_before,
			updated_at = unix_now();
	`, userId, enabled, hoursBefore)
	if err != nil {
		return fmt.Errorf("error saving reminder preference: %s", err)
	}
	return nil
}

// VolunteerReminderTarget is one owed reminder.
type VolunteerReminderTarget struct {
	UserId  string
	EventId string
}

// GetUsersMatchingSignupEmails resolves signups to accounts for reminders.
//
// Matching is on user_id (an in-app signup) or a VERIFIED email — deliberately
// not the account's unverified contact email. @WEB caught that matching an
// unverified address would let anyone type a stranger's email into a public
// form and make that stranger's phone buzz with a genuine-looking SFLuv push;
// @MOBILE withdrew their own broader proposal in favour of this. The cost is
// that users who never verified an email get no reminder, which is both fixable
// in-app and a far better failure mode.
func (a *AppDB) GetUsersMatchingSignupEmails(ctx context.Context, emails []string, userIds []string) (map[string][]string, error) {
	result := map[string][]string{}
	if len(emails) == 0 && len(userIds) == 0 {
		return result, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT LOWER(TRIM(ve.email_normalized)), ve.user_id
		FROM user_verified_emails ve
		JOIN users u ON u.id = ve.user_id
		WHERE ve.active = TRUE
			AND u.active = TRUE
			AND LOWER(TRIM(ve.email_normalized)) = ANY($1);
	`, emails)
	if err != nil {
		return nil, fmt.Errorf("error matching signup emails to accounts: %s", err)
	}
	defer rows.Close()

	for rows.Next() {
		var email, userId string
		if err := rows.Scan(&email, &userId); err != nil {
			return nil, err
		}
		result[email] = append(result[email], userId)
	}

	return result, rows.Err()
}

// ClaimVolunteerReminderSend records that a reminder was sent, returning false
// when one already existed. The primary key makes this the dedup point: the
// sender can run repeatedly and a user still gets at most one push per event.
func (a *AppDB) ClaimVolunteerReminderSend(ctx context.Context, userId string, eventId string) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		INSERT INTO volunteer_reminder_sends (user_id, event_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, event_id) DO NOTHING;
	`, userId, eventId)
	if err != nil {
		return false, fmt.Errorf("error claiming reminder send: %s", err)
	}
	return tag.RowsAffected() > 0, nil
}
