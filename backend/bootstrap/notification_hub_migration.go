package bootstrap

import (
	"context"
	"fmt"
)

// migrateNotificationHub is the 1.87 migration body: notification teams, the
// routes from topics to teams, and a record on each notification of where it
// went.
//
// Teams replace the per-member subscriptions to three fixed sets
// (organization_notification_subscriptions) and the per-member support
// category narrowing (support_category_subscriptions). Those tables are left in
// place and no longer read. Nothing is copied from them: PJ configures the
// teams by hand, and a guessed mapping from three sets onto forty topics would
// be a default nobody chose.
//
// Members are not foreign-keyed to organization_members. A team is bounded by
// the house organization at delivery time instead, by a join, so somebody who
// leaves the organization stops receiving at once without the team losing the
// record of who was on it.
func migrateNotificationHub(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		-- What a notification is about, the unit routing works on. The group it
		-- belongs to stays in notification_set, which the apps already show.
		ALTER TABLE internal_notifications
			ADD COLUMN IF NOT EXISTS topic TEXT NOT NULL DEFAULT '';
		-- Where it went: teams, recipients, pushes, addresses, chats, failures.
		-- Written once when it is raised. NULL on notifications from before
		-- this existed.
		ALTER TABLE internal_notifications
			ADD COLUMN IF NOT EXISTS delivery JSONB;
		CREATE INDEX IF NOT EXISTS internal_notifications_recent_idx
			ON internal_notifications(organization_id, created_at DESC);

		CREATE TABLE IF NOT EXISTS notification_teams(
			id BIGSERIAL PRIMARY KEY,
			organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			-- Members always get the notification in the app. These are the
			-- extra ways a team asks to be told.
			push_members BOOLEAN NOT NULL DEFAULT TRUE,
			email_members BOOLEAN NOT NULL DEFAULT FALSE,
			email_address TEXT NOT NULL DEFAULT '',
			chat_conversation_id BIGINT REFERENCES conversations(id) ON DELETE SET NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS notification_teams_name_idx
			ON notification_teams(organization_id, LOWER(name));

		CREATE TABLE IF NOT EXISTS notification_team_members(
			team_id BIGINT NOT NULL REFERENCES notification_teams(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (team_id, user_id)
		);
		CREATE INDEX IF NOT EXISTS notification_team_members_user_idx
			ON notification_team_members(user_id);

		-- topic is not a foreign key: topics are a code catalog, like the sets
		-- were, so adding one is a deploy. 'unrouted' is the catch-all.
		CREATE TABLE IF NOT EXISTS notification_routes(
			team_id BIGINT NOT NULL REFERENCES notification_teams(id) ON DELETE CASCADE,
			topic TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (topic, team_id)
		);
		CREATE INDEX IF NOT EXISTS notification_routes_team_idx
			ON notification_routes(team_id);

		-- Mail received through the Mailgun route is stored as a support
		-- request. Mailgun retries a forward it thinks failed, so the message's
		-- own id is kept and unique, and a retry finds the request it already
		-- made instead of making a second.
		ALTER TABLE support_requests
			ADD COLUMN IF NOT EXISTS email_message_id TEXT;
		CREATE UNIQUE INDEX IF NOT EXISTS support_requests_email_message_idx
			ON support_requests(email_message_id) WHERE email_message_id IS NOT NULL;
	`); err != nil {
		return fmt.Errorf("error creating the notification hub tables: %w", err)
	}
	return nil
}
