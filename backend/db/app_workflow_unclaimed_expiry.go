package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
)

// UnclaimedWorkflowExpiryDays is how long after its start date an approved
// one-time workflow may sit with nobody claiming it before it expires.
const UnclaimedWorkflowExpiryDays = 30

// ExpireUnclaimedOpenEndedWorkflows expires approved one-time workflows that
// have no end date and that nobody has claimed within
// UnclaimedWorkflowExpiryDays of their start.
//
// Every approved workflow reserves its unpaid bounties out of the faucet's
// unallocated balance until it finishes. The other sweeps already release the
// rest: a recurring occurrence is skipped when the next one starts, and a
// one-time workflow with an end date is closed out once that date passes
// (FinalizeWorkflowPartiallyIfElapsed). An open-ended one-time workflow had no
// such exit, so one nobody wanted held its money forever.
//
// "Claimed" means any improver activity on a step: an assignment, a start, a
// step past 'available', or a submission. A supervisor (manager) being named
// does not count; the proposer picks one at creation, so it says nothing about
// whether anyone took the work.
//
// 'expired' is already terminal everywhere and is outside every allocation
// query, so the status change alone releases the reservation.
func (a *AppDB) ExpireUnclaimedOpenEndedWorkflows(ctx context.Context) ([]structs.WorkflowUnclaimedExpiryNotice, error) {
	rows, err := a.db.Query(ctx, `
		WITH candidates AS (
			SELECT
				w.id
			FROM
				workflows w
			JOIN
				workflow_series s ON s.id = w.series_id
			LEFT JOIN
				workflow_states st ON st.id = COALESCE(s.current_state_id, w.workflow_state_id)
			WHERE
				w.status = 'approved'
			AND
				COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')) = 'one_time'
			AND
				(w.end_at IS NULL OR w.end_at <= 0)
			AND
				w.start_at > 0
			AND
				w.start_at <= (unix_now() - ($1::bigint * 24 * 60 * 60))
			AND NOT EXISTS (
				SELECT 1 FROM workflow_steps ws
				WHERE ws.workflow_id = w.id
				AND (
					ws.assigned_improver_id IS NOT NULL
					OR ws.started_at IS NOT NULL
					OR ws.status IN ('in_progress', 'completed', 'paid_out')
				)
			)
			AND NOT EXISTS (
				SELECT 1 FROM workflow_step_submissions sub
				WHERE sub.workflow_id = w.id
			)
			FOR UPDATE OF w SKIP LOCKED
		),
		expired AS (
			UPDATE
				workflows w
			SET
				status = 'expired',
				is_start_blocked = false,
				blocked_by_workflow_id = NULL,
				updated_at = unix_now()
			FROM
				candidates c
			WHERE
				w.id = c.id
			AND
				w.status = 'approved'
			RETURNING
				w.id,
				w.series_id,
				w.workflow_state_id,
				w.proposer_id,
				w.start_at,
				w.manager_required,
				w.manager_bounty,
				w.manager_paid_out_at
		)
		SELECT
			e.id,
			COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')),
			e.proposer_id,
			COALESCE(NULLIF(TRIM(p.email), ''), COALESCE(u.contact_email, '')),
			e.start_at,
			(COALESCE((
				SELECT SUM(ws.bounty) FROM workflow_steps ws
				WHERE ws.workflow_id = e.id AND ws.status <> 'paid_out'
			), 0)
			+ CASE
				WHEN e.manager_required AND e.manager_bounty > 0 AND e.manager_paid_out_at IS NULL
				THEN e.manager_bounty
				ELSE 0
			END)::bigint
		FROM
			expired e
		LEFT JOIN
			workflow_states st ON st.id = e.workflow_state_id
		LEFT JOIN
			workflow_series s ON s.id = e.series_id
		LEFT JOIN
			proposers p ON p.user_id = e.proposer_id
		LEFT JOIN
			users u ON u.id = e.proposer_id;
	`, UnclaimedWorkflowExpiryDays)
	if err != nil {
		return nil, fmt.Errorf("error expiring unclaimed workflows: %s", err)
	}
	defer rows.Close()

	notices := []structs.WorkflowUnclaimedExpiryNotice{}
	for rows.Next() {
		n := structs.WorkflowUnclaimedExpiryNotice{}
		var released int64
		if err := rows.Scan(&n.WorkflowId, &n.WorkflowTitle, &n.ProposerUserId, &n.ProposerEmail, &n.StartAt, &released); err != nil {
			return nil, fmt.Errorf("error scanning unclaimed workflow expiry: %s", err)
		}
		if released > 0 {
			n.ReleasedBounty = uint64(released)
		}
		n.ProposerEmail = strings.TrimSpace(n.ProposerEmail)
		notices = append(notices, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating unclaimed workflow expiries: %s", err)
	}
	return notices, nil
}
