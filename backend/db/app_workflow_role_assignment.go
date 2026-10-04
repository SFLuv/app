package db

import (
	"context"
	"fmt"
)

// Keeping a claimed role's steps assigned to whoever claimed it.
//
// A role is claimed once per workflow and covers every step carrying it, so the
// claimant owns all of them. ClaimWorkflowStep does assign them all — but only
// the ones that exist, unassigned, at the moment of the claim. Anything that
// adds or reopens a step afterwards leaves it unassigned with the role already
// taken, and nothing was putting that right.
//
// The visible symptom: an improver holds a role, completes step 1, and step 2 of
// the same role sits 'available' with no assignee. It never appears as theirs,
// and the occurrence cannot finish.
//
// So assignment follows the role continuously rather than only at claim time.
// Idempotent and cheap: it only ever fills a NULL, and only where the role
// already has exactly one claimant in that workflow.
func (a *AppDB) BackfillClaimedRoleStepAssignments(ctx context.Context) (int64, error) {
	tag, err := a.db.Exec(ctx, `
		WITH role_claimant AS (
			SELECT
				ws.workflow_id,
				ws.role_id,
				MIN(ws.assigned_improver_id) AS assigned_improver_id
			FROM
				workflow_steps ws
			WHERE
				ws.role_id IS NOT NULL
			AND
				ws.assigned_improver_id IS NOT NULL
			GROUP BY
				ws.workflow_id, ws.role_id
			-- Exactly one claimant, or we do not guess. Two different improvers
			-- on one role should be impossible (the claim guard refuses it), and
			-- if it ever happens, picking one of them silently is the wrong
			-- answer to an inconsistency somebody needs to see.
			HAVING
				COUNT(DISTINCT ws.assigned_improver_id) = 1
		)
		UPDATE
			workflow_steps target
		SET
			assigned_improver_id = rc.assigned_improver_id,
			updated_at = unix_now()
		FROM
			role_claimant rc
		JOIN
			workflows w
		ON
			w.id = rc.workflow_id
		WHERE
			target.workflow_id = rc.workflow_id
		AND
			target.role_id = rc.role_id
		AND
			target.assigned_improver_id IS NULL
		-- Only work that can still be done. A completed or paid step is history
		-- and must not be retitled to somebody who did not do it.
		AND
			target.status IN ('locked', 'available')
		AND
			w.status IN ('approved', 'in_progress', 'blocked');
	`)
	if err != nil {
		return 0, fmt.Errorf("error backfilling claimed role step assignments: %w", err)
	}
	return tag.RowsAffected(), nil
}
