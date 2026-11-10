package bootstrap

import (
	"context"
	"fmt"
)

// migrateStrandedSkippedWorkflows is the 1.81 migration body: recurring
// occurrences that were skipped after a step in them had been paid are
// finalized partially instead, as they would have been had partial finalization
// reached them.
//
// The recurrence catch-up used to skip a superseded occurrence outright, and it
// runs before partial finalization, so every recurring workflow with a paid
// step and an unfinished one ended 'skipped'. The supervisor export then
// reported the paid step as skipped, and Finalized never listed it.
//
// The repair is what partial finalization does: a completed zero-bounty step
// settles, the steps nobody did are skipped, and the workflow becomes paid_out
// and partially_completed. Two things it does not do. It leaves any workflow
// with a completed step still awaiting payout, since that is money owed and
// needs somebody to look at it. And it creates no successors, because the
// catch-up already created them when it skipped these.
func migrateStrandedSkippedWorkflows(ctx context.Context, pools *MigrationPools) error {
	rows, err := pools.App.Query(ctx, `
		SELECT
			w.id
		FROM
			workflows w
		WHERE
			w.status = 'skipped'
		AND
			EXISTS (
				SELECT 1 FROM workflow_steps ws
				WHERE ws.workflow_id = w.id AND ws.status = 'paid_out'
			)
		AND
			NOT EXISTS (
				SELECT 1 FROM workflow_steps ws
				WHERE ws.workflow_id = w.id AND ws.status = 'completed' AND ws.bounty > 0
			);
	`)
	if err != nil {
		return fmt.Errorf("error finding skipped workflows with paid steps: %w", err)
	}
	stranded := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("error scanning skipped workflow with paid steps: %w", err)
		}
		stranded = append(stranded, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error reading skipped workflows with paid steps: %w", err)
	}
	if len(stranded) == 0 {
		return nil
	}

	if _, err := pools.App.Exec(ctx, `
		UPDATE workflow_steps
		SET status = 'paid_out', payout_in_progress = false, updated_at = unix_now()
		WHERE workflow_id = ANY($1::text[]) AND status = 'completed' AND bounty = 0;
	`, stranded); err != nil {
		return fmt.Errorf("error settling zero-bounty steps of skipped workflows: %w", err)
	}

	if _, err := pools.App.Exec(ctx, `
		UPDATE workflow_steps
		SET status = 'skipped', payout_in_progress = false, updated_at = unix_now()
		WHERE workflow_id = ANY($1::text[]) AND status IN ('locked', 'available', 'in_progress');
	`, stranded); err != nil {
		return fmt.Errorf("error skipping unfinished steps of skipped workflows: %w", err)
	}

	if _, err := pools.App.Exec(ctx, `
		UPDATE workflows
		SET
			status = 'paid_out',
			partially_completed = true,
			is_start_blocked = false,
			blocked_by_workflow_id = NULL,
			updated_at = unix_now()
		WHERE id = ANY($1::text[]);
	`, stranded); err != nil {
		return fmt.Errorf("error finalizing skipped workflows with paid steps: %w", err)
	}

	// Release anything these were holding up, as finalization does.
	if _, err := pools.App.Exec(ctx, `
		UPDATE workflows
		SET
			is_start_blocked = false,
			blocked_by_workflow_id = NULL,
			status = CASE WHEN status = 'blocked' THEN 'approved' ELSE status END,
			updated_at = unix_now()
		WHERE status = 'blocked' AND blocked_by_workflow_id = ANY($1::text[]);
	`, stranded); err != nil {
		return fmt.Errorf("error releasing workflows blocked by skipped ones: %w", err)
	}

	return nil
}
