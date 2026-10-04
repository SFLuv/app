package db

import (
	"context"
	"fmt"
	"time"
)

// Finalizing a workflow nobody finished.
//
// A workflow only reaches 'completed' when every step is done, and the
// recurrence successor is created only at that transition. So a workflow with a
// step nobody ever started sat in 'in_progress' forever and its series stopped
// dead — no successor, no finalization, and an open bounty commitment on the
// books for work that is no longer possible.
//
// This closes those out once their window has passed: the parts nobody did are
// marked 'skipped', the workflow is finalized as paid_out and flagged
// partially_completed so it is never mistaken for work fully delivered, and the
// series is allowed to continue.
//
// The rule it will not break: a step sitting at 'completed' with a bounty still
// owed BLOCKS this. That is money an improver earned, and writing it off because
// a date passed would be the worst possible reading of "finalize".

// WorkflowWindowEnd returns when a workflow stopped being available, and whether
// it has an end at all.
//
// Recurring: the next occurrence's start, because that is when this one is
// superseded. One-time: the end date the proposer set, if they set one — an
// open-ended one-time workflow never elapses and is never swept, which is why
// every workflow that predates the end_at column is left alone.
func WorkflowWindowEnd(recurrence string, startAt int64, endAt *int64, anchorStartAt *int64) (int64, bool) {
	if recurrence == "" || recurrence == "one_time" {
		if endAt != nil && *endAt > 0 {
			return *endAt, true
		}
		return 0, false
	}
	next, err := nextRecurringStartAtWithAnchor(startAt, recurrence, anchorStartAt)
	if err != nil || next <= 0 {
		return 0, false
	}
	return next, true
}

// GetWorkflowIDsPossiblyElapsed lists workflows that are unfinished and might be
// past their window. The window itself is computed per row in Go, since it
// depends on the recurrence rules; this only narrows the candidates.
func (a *AppDB) GetWorkflowIDsPossiblyElapsed(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := a.db.Query(ctx, `
		SELECT
			w.id
		FROM
			workflows w
		WHERE
			w.status IN ('in_progress', 'approved')
		AND
			EXISTS (
				SELECT 1 FROM workflow_steps ws
				WHERE ws.workflow_id = w.id
				AND ws.status IN ('locked', 'available', 'in_progress')
			)
		ORDER BY
			w.start_at ASC
		LIMIT $1;
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("error querying workflows possibly past their window: %s", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FinalizeWorkflowPartiallyIfElapsed closes out one workflow whose window has
// passed. Returns whether it finalized, and why not when it did not — the reason
// is for the log, since "nothing happened" is indistinguishable from "blocked by
// money owed" otherwise.
func (a *AppDB) FinalizeWorkflowPartiallyIfElapsed(ctx context.Context, workflowID string) (bool, string, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, "", err
	}
	defer tx.Rollback(ctx)

	var status, recurrence string
	var startAt int64
	var endAt, anchorStartAt *int64
	err = tx.QueryRow(ctx, `
		SELECT
			w.status,
			COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')),
			w.start_at,
			w.end_at,
			st.start_at
		FROM
			workflows w
		JOIN
			workflow_series s ON s.id = w.series_id
		LEFT JOIN
			workflow_states st ON st.id = COALESCE(s.current_state_id, w.workflow_state_id)
		WHERE
			w.id = $1
		FOR UPDATE OF w;
	`, workflowID).Scan(&status, &recurrence, &startAt, &endAt, &anchorStartAt)
	if err != nil {
		return false, "", err
	}

	if status != "in_progress" && status != "approved" {
		return false, "not unfinished", tx.Commit(ctx)
	}

	windowEnd, bounded := WorkflowWindowEnd(recurrence, startAt, endAt, anchorStartAt)
	if !bounded {
		// Open-ended. Nothing to elapse, so nothing to close out.
		return false, "no end date", tx.Commit(ctx)
	}
	if time.Now().UTC().Unix() < windowEnd {
		return false, "still open", tx.Commit(ctx)
	}

	// Zero-bounty completed steps owe nothing, so they settle themselves —
	// the same rule the ordinary finalization uses.
	if _, err := tx.Exec(ctx, `
		UPDATE workflow_steps
		SET status = 'paid_out', payout_in_progress = false, updated_at = unix_now()
		WHERE workflow_id = $1 AND status = 'completed' AND bounty = 0;
	`, workflowID); err != nil {
		return false, "", fmt.Errorf("error settling zero-bounty steps: %s", err)
	}

	// Work that was done and not yet paid stops this dead.
	var owed int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM workflow_steps
		WHERE workflow_id = $1 AND status = 'completed' AND bounty > 0;
	`, workflowID).Scan(&owed); err != nil {
		return false, "", fmt.Errorf("error counting steps still owed: %s", err)
	}
	if owed > 0 {
		return false, fmt.Sprintf("%d completed step(s) still awaiting payout", owed), tx.Commit(ctx)
	}

	// Everything nobody started is abandoned, not failed: 'skipped' says the
	// window closed on it, which is what actually happened.
	//
	// Deliberately NOT adjusting total_bounty or the budget_*_deducted columns.
	// Those are the allocation side of the ledger, and a skipped step's reserved
	// funds are a separate decision from tagging the workflow — quietly editing
	// them here would trade one accounting discrepancy for a subtler one. The
	// skipped steps and the partially_completed mark are the record; releasing
	// the reservation is a call for whoever owns the faucet budget.
	skipTag, err := tx.Exec(ctx, `
		UPDATE workflow_steps
		SET status = 'skipped', payout_in_progress = false, updated_at = unix_now()
		WHERE workflow_id = $1 AND status IN ('locked', 'available', 'in_progress');
	`, workflowID)
	if err != nil {
		return false, "", fmt.Errorf("error skipping unstarted steps: %s", err)
	}
	skipped := skipTag.RowsAffected()
	if skipped == 0 {
		return false, "nothing left unstarted", tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE workflows
		SET
			status = 'paid_out',
			partially_completed = true,
			is_start_blocked = false,
			blocked_by_workflow_id = NULL,
			updated_at = unix_now()
		WHERE id = $1;
	`, workflowID); err != nil {
		return false, "", fmt.Errorf("error finalizing partially completed workflow: %s", err)
	}

	// Release anything this one was holding up, the way ordinary finalization
	// does — otherwise the series is still stuck, just one link further along.
	if _, err := tx.Exec(ctx, `
		UPDATE workflows
		SET
			is_start_blocked = false,
			blocked_by_workflow_id = NULL,
			status = CASE WHEN status = 'blocked' THEN 'approved' ELSE status END,
			updated_at = unix_now()
		WHERE status = 'blocked' AND blocked_by_workflow_id = $1;
	`, workflowID); err != nil {
		return false, "", fmt.Errorf("error releasing blocked workflows: %s", err)
	}

	// And generate the occurrence the stall swallowed.
	if _, err := ensureRecurringWorkflowSuccessorTx(ctx, tx, workflowID); err != nil {
		return false, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, "", err
	}
	return true, fmt.Sprintf("%d step(s) skipped", skipped), nil
}
