package bootstrap

import (
	"context"
	"fmt"
)

// migrateBetaFeatures is the 1.69 migration body: who may use a feature that
// is still in beta.
//
// One table, and the shape of it carries the only requirement that is not
// obvious. Revoking access does not delete the row — it stamps revoked_at —
// and that is what makes re-granting put somebody back exactly where they
// were. The alternative, deleting on revoke, would read the same from the
// admin panel and then lose the thing that matters: a person taken out of the
// chat beta and let back in has to find their conversations still there, not
// start again. Nothing is removed from conversation_members at any point, so
// "re-added to the chats they were in" is handled by never having removed
// them; the gates answer from this table instead.
//
// The partial unique index is on (feature, user_id) via the primary key, so a
// person has at most one row per feature and re-granting updates it rather
// than stacking grants. The live index exists because every gated request asks
// the same question — is this person active on this feature — and that lookup
// is on the request path for the whole Bulletin and Messages surface.
//
// It starts empty, which means that on the deploy that runs this, nobody has
// either feature. That is the intended state: the features were previously
// behind one env switch that was off, and this replaces it with two switches
// plus a list.
func migrateBetaFeatures(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS beta_feature_access(
			feature TEXT NOT NULL,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			granted_by TEXT REFERENCES users(id),
			revoked_at TIMESTAMPTZ,
			revoked_by TEXT REFERENCES users(id),
			PRIMARY KEY (feature, user_id)
		);

		-- Every gated request asks "is this person active on this feature", so
		-- that lookup gets its own index rather than scanning a feature's whole
		-- membership including everyone ever removed from it.
		CREATE INDEX IF NOT EXISTS beta_feature_access_live_idx
			ON beta_feature_access(feature, user_id)
			WHERE revoked_at IS NULL;

		-- The panel lists one feature at a time, newest grant first.
		CREATE INDEX IF NOT EXISTS beta_feature_access_feature_idx
			ON beta_feature_access(feature, granted_at DESC);
	`); err != nil {
		return fmt.Errorf("error creating beta_feature_access: %w", err)
	}

	return nil
}
