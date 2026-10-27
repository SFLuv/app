package bootstrap

import (
	"context"
	"fmt"
)

// migrateChatMessageManagement is the 1.75 migration body: editing a message
// shortly after sending it, and deleting one for everybody.
//
// ── Why `deleted_at` was not already enough ─────────────────────────────────
//
// 1.67 gave messages a `deleted_at`, and it has been used for moderation: an
// admin removes a reported message and every read path filters it out. That is
// a REMOVAL — the message stops existing as far as members are concerned.
//
// What this adds is a DELETION somebody performs on their own message, and the
// difference matters to the people reading the thread. A reply that answers a
// message which then vanishes entirely becomes a non-sequitur; an iMessage-style
// "Message was deleted" keeps the shape of the conversation. So deleted messages
// now come back from the read paths as tombstones rather than being filtered
// away, and `deleted_by` is what distinguishes the two cases.
//
// ── On keeping the body ─────────────────────────────────────────────────────
//
// `body` is deliberately NOT cleared on delete. Blanking it would be the
// stronger guarantee against ever serving it again, and it is tempting for
// exactly that reason — but a message is most likely to be deleted immediately
// after somebody reports it, and clearing the text would destroy the thing the
// report is about. So the row keeps its content for the moderation queue, and
// every member-facing read blanks it on the way out instead. That puts the
// burden on the serialization layer, which is why it is stated here and at each
// of those reads.
func migrateChatMessageManagement(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE messages
			-- When the sender last edited it. NULL means never edited, which is
			-- what lets the client show "edited" without a second column.
			ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ,
			-- Who deleted it. Distinguishes a sender deleting their own message
			-- from an admin removing a reported one: the first is shown as a
			-- tombstone, the second is filtered out entirely, because an admin
			-- removal should not advertise that something was there.
			ADD COLUMN IF NOT EXISTS deleted_by TEXT REFERENCES users(id);

		-- The thread read walks messages by conversation and id, and now has to
		-- include deleted ones; this keeps that ordering covered without the
		-- partial predicate the old index had.
		CREATE INDEX IF NOT EXISTS messages_conversation_all_idx
			ON messages(conversation_id, id DESC);
	`); err != nil {
		return fmt.Errorf("error adding message management columns: %w", err)
	}

	return nil
}
