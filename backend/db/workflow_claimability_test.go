package db

import (
	"strings"
	"testing"
)

// Regression test for the "claim button does nothing" bug.
//
// The improver board and the claim endpoint each hard-coded their own list of
// workflow statuses, and they disagreed: the board listed steps belonging to
// completed / paid_out / blocked workflows, while ClaimWorkflowStep rejected
// every one of them. Improvers saw a claimable step whose claim button silently
// failed. Both sides now read these sets, and these tests pin the agreement.
func TestWorkflowStepClaimableStatuses(t *testing.T) {
	claimable := map[string]bool{
		"approved":    true,
		"in_progress": true,
	}
	notClaimable := []string{"completed", "paid_out", "blocked", "pending", "rejected", "expired", "deleted", "skipped", "failed"}

	for status := range claimable {
		if !isWorkflowStepClaimableStatus(status) {
			t.Errorf("expected step claim to be allowed for workflow status %q", status)
		}
	}
	for _, status := range notClaimable {
		if isWorkflowStepClaimableStatus(status) {
			t.Errorf("step claim must be rejected for workflow status %q", status)
		}
	}
}

// A manager may be lined up on a 'blocked' series instance — it is waiting on
// its predecessor's payout, not on work — so its set is deliberately wider than
// the step set. This asserts the difference is intentional and bounded.
func TestWorkflowManagerClaimableStatuses(t *testing.T) {
	for _, status := range []string{"approved", "in_progress", "blocked"} {
		if !isWorkflowManagerClaimableStatus(status) {
			t.Errorf("expected manager claim to be allowed for workflow status %q", status)
		}
	}
	for _, status := range []string{"completed", "paid_out", "pending", "rejected", "deleted"} {
		if isWorkflowManagerClaimableStatus(status) {
			t.Errorf("manager claim must be rejected for workflow status %q", status)
		}
	}

	// The manager set must be a strict superset of the step set: anything a
	// step can be claimed on, a manager can too.
	for _, status := range workflowStepClaimableStatuses {
		if !isWorkflowManagerClaimableStatus(status) {
			t.Errorf("manager claimable statuses must include step status %q", status)
		}
	}
}

// The board SQL builds its IN-list from the same slice the claim validation
// uses. This checks the rendering, since a malformed list would silently change
// which workflows are listed.
func TestSQLStatusListRendering(t *testing.T) {
	got := sqlStatusList(workflowStepClaimableStatuses)
	if got != "'approved', 'in_progress'" {
		t.Fatalf("sqlStatusList = %q, want %q", got, "'approved', 'in_progress'")
	}

	for _, status := range workflowManagerClaimableStatuses {
		if !strings.Contains(sqlStatusList(workflowManagerClaimableStatuses), "'"+status+"'") {
			t.Errorf("rendered manager status list is missing %q", status)
		}
	}
}

// The improver board query must not list steps from workflows the claim
// endpoint would reject. This asserts the statuses that caused the original bug
// are absent from the claimable CTE.
func TestImproverBoardQueryExcludesUnclaimableStatuses(t *testing.T) {
	claimableCTE := improverClaimableWorkflowIDsCTE()

	for _, status := range []string{"completed", "paid_out", "blocked"} {
		if strings.Contains(claimableCTE, "'"+status+"'") {
			t.Errorf("claimable_workflow_ids CTE must not include workflow status %q — the claim endpoint rejects it", status)
		}
	}
	for _, status := range workflowStepClaimableStatuses {
		if !strings.Contains(claimableCTE, "'"+status+"'") {
			t.Errorf("claimable_workflow_ids CTE is missing claimable status %q", status)
		}
	}
}
