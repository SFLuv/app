package bootstrap

import (
	"context"
	"fmt"
)

// migrateChatMentions is the 1.86 migration body: who a message tags.
//
// ── Why a column of entities rather than markup in the body ─────────────────
//
// The body stays the plain "@Maria are you coming?" a person typed. Every
// reader that predates mentions — older app builds, the web, a push preview,
// the moderation queue — keeps showing exactly what it shows today. What is
// stored beside it is who each "@…" refers to, as UTF-16 offsets into the body,
// which is what lets each reader's app draw the person under the name THAT
// reader saved them as, and lets the server reach a tagged member who has the
// thread muted. Markup in the body ("<@user_id>") would have broken every one
// of those older readers at once.
//
// ── Why JSONB and not a join table ──────────────────────────────────────────
//
// Mentions are read only ever with their message and never queried on their
// own ("everything that tags me" is not a feature), so a side table would be a
// second query or a join on the hottest read in chat for no gain. Nullable, so
// every existing row and every message that tags nobody is simply NULL.
func migrateChatMentions(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE messages
			ADD COLUMN IF NOT EXISTS mentions JSONB;
	`); err != nil {
		return fmt.Errorf("error adding message mentions column: %w", err)
	}
	return nil
}
