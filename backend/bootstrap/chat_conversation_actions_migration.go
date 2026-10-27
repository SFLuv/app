package bootstrap

import (
	"context"
	"fmt"
)

// migrateChatConversationActions is the 1.76 migration body: muting a
// conversation, deleting one for everybody, and the read receipt that "Delivered"
// and "Read" are derived from.
//
// ── "Delete for me" needs no column ─────────────────────────────────────────
//
// It is archiving, which 1.67 already has. The requirement that a conversation
// deleted for yourself "repopulates history when another message is sent" is
// satisfied for free, because archiving never removed the history in the first
// place — InsertMessage clears archived_at for every member, so the thread
// returns intact. Adding a history-clearing column would have been a second
// mechanism that did the same job worse.
//
// ── "Delete for both" is a soft delete, deliberately ────────────────────────
//
// One person removing a conversation from somebody else's phone is a real
// capability with a real cost: the most likely reason to want somebody else's
// copy gone is that it contains something you said. So the rows stay — the
// conversation is hidden from both members and excluded from every read, and
// the moderation queue can still resolve a report filed against a message in
// it. What is given up is the stronger promise that the data is destroyed; what
// is kept is that a report does not become unanswerable because the reported
// party pressed a button.
func migrateChatConversationActions(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE conversation_members
			-- When the mute expires. NULL means not muted; a far-future value is
			-- "forever", so one nullable timestamp covers all three options the
			-- client offers rather than a mode column beside a deadline.
			ADD COLUMN IF NOT EXISTS muted_until TIMESTAMPTZ;

		ALTER TABLE conversations
			-- Hidden from everybody. See the note above on why the rows stay.
			ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
			ADD COLUMN IF NOT EXISTS deleted_by TEXT REFERENCES users(id);

		-- The inbox reads live conversations only, and most are live, so the
		-- index is partial.
		CREATE INDEX IF NOT EXISTS conversations_live_idx
			ON conversations(last_message_at DESC) WHERE deleted_at IS NULL;
	`); err != nil {
		return fmt.Errorf("error adding conversation actions: %w", err)
	}

	return nil
}
