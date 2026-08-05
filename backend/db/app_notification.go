package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
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
