package bootstrap

import (
	"context"
	"fmt"
)

// migrateRemoveSupervisorRewards is the 1.72 migration body: existing
// workflows stop carrying a supervisor reward.
//
// Supervisors are not being removed — one is still assigned to a workflow and
// still fills in its data fields. What is removed is the payment for it, so a
// workflow's only bounties are its steps'.
//
// ── Why unpaid only ──────────────────────────────────────────────────────────
//
// Only rows with manager_paid_out_at IS NULL are zeroed. A supervisor reward
// that has already been paid is a record of money that actually left the
// faucet, and rewriting its amount to zero would make the ledger disagree with
// the chain — the payout rows, the analytics totals and the transfer on
// Berachain would all still say what was sent, while the workflow claimed it
// had never owed anything. Those rows keep their history and are already
// inert: the eligibility check refuses a workflow whose supervisor reward is
// marked paid.
//
// ── Why this is also the kill switch ─────────────────────────────────────────
//
// The payout path's first question is whether manager_bounty is zero, and it
// declines to pay when it is. Zeroing every unpaid reward therefore stops
// every future supervisor payout by itself, without reaching into the payout
// pipeline. The pipeline is left in place and unreachable rather than cut out
// from under a money path that cannot be tested here; removing the dead code
// is a separate change.
//
// manager_required is cleared alongside, because a workflow that requires a
// supervisor in order to pay one requires nothing now, and leaving it set
// makes the "waiting on a supervisor" index describe a wait that cannot end in
// a payment.
func migrateRemoveSupervisorRewards(ctx context.Context, pools *MigrationPools) error {
	_, err := pools.App.Exec(ctx, `
		UPDATE workflows
		SET manager_bounty = 0,
			manager_required = false
		WHERE manager_paid_out_at IS NULL
		AND (manager_bounty <> 0 OR manager_required);
	`)
	if err != nil {
		return fmt.Errorf("error clearing supervisor rewards: %w", err)
	}

	// The amount lives in three places, and all three have to be cleared or the
	// create path starts refusing work it is handed.
	//
	// workflow_states is a series' current definition, and the next occurrence
	// of a recurring workflow is built from it. workflow_templates is the
	// library a proposer starts from. Either one left holding an amount would
	// hand the create path a positive supervisor bounty, which it now rejects —
	// so the next occurrence of a recurring workflow would simply fail to be
	// created, which is a worse outcome than the reward it was carrying.
	//
	// IS DISTINCT FROM rather than <> because workflow_templates.supervisor_bounty
	// is nullable: <> 0 is NULL for those rows, so they would be skipped. They
	// mean "no reward" already, but matching them keeps the column uniform
	// rather than leaving two spellings of zero.
	if _, err := pools.App.Exec(ctx, `
		UPDATE workflow_states
		SET supervisor_bounty = 0
		WHERE supervisor_bounty IS DISTINCT FROM 0;
	`); err != nil {
		return fmt.Errorf("error clearing supervisor rewards on workflow states: %w", err)
	}

	if _, err := pools.App.Exec(ctx, `
		UPDATE workflow_templates
		SET supervisor_bounty = 0
		WHERE supervisor_bounty IS DISTINCT FROM 0;
	`); err != nil {
		return fmt.Errorf("error clearing supervisor rewards on workflow templates: %w", err)
	}

	// The "Change the rewards account" capability goes with the payouts it
	// pointed at. Its grants are deleted rather than left: the catalog no
	// longer declares the key, so a surviving row would come back from
	// GetMemberGrants as a capability the permissions screen cannot name, and
	// CapabilityRoleType would call it unknown.
	if _, err := pools.App.Exec(ctx, `
		DELETE FROM organization_member_permissions
		WHERE capability = 'supervisor.payout_account.manage';
	`); err != nil {
		return fmt.Errorf("error removing the supervisor payout account capability: %w", err)
	}

	return nil
}
