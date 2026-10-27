package bootstrap

import (
	"context"
	"fmt"
)

// migrateInvitePermissions is the 1.73 migration body: an invite carries the
// role and permissions the invitee will have when they accept.
//
// Before this, every invite produced the same member: role "member",
// permissions_mode "inherit" — which means every capability of every role the
// organization holds. So the only way to give somebody narrow access was to
// invite them, wait for them to accept, and then take things away. In between,
// they had everything.
//
// Now the decision is made when the invitation is written, and applied when it
// is accepted.
//
// ── Why the column defaults are the OLD behaviour ────────────────────────────
//
// permissions_mode defaults to 'inherit' and role to 'member', which is
// exactly what an invite did before this ran. That matters because invites
// already sent and not yet accepted are rows in this table: somebody was told
// they were being added to a team, and this migration must not quietly change
// what they are walking into. They get what they were promised; invites
// written from here carry an explicit answer.
//
// The new default for a NEW invite is the opposite — no permissions — but that
// is a decision the invite form makes and sends, not one the column makes.
func migrateInvitePermissions(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE organization_invites
			ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'member',
			ADD COLUMN IF NOT EXISTS permissions_mode TEXT NOT NULL DEFAULT 'inherit',
			-- Capability keys, as a JSON array of strings. Only read when
			-- permissions_mode is 'custom', and only meaningful for a member:
			-- an admin holds everything regardless, which is why the invite
			-- form only offers the fine-grained list for the member role.
			ADD COLUMN IF NOT EXISTS capabilities JSONB NOT NULL DEFAULT '[]'::jsonb;
	`); err != nil {
		return fmt.Errorf("error adding invite permissions: %w", err)
	}

	return nil
}
