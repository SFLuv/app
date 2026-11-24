package bootstrap

import (
	"context"
	"fmt"
)

// migrateNotificationTeamRouting is the 1.88 migration body: a lead and admins
// for each notification team, any number of group chats per team, and the
// team's own routing of the topics it is given.
//
// The house organization's admins decide which teams get which topics. A team's
// lead and admins decide where each of those goes inside the team: everyone,
// particular people, the team's shared address, its chats. Until they say,
// everyone on the team gets it in the app and on their phone.
//
// A team set up before this, with channels other than that default, keeps doing
// what it did: its channels become its default rule. Its one chat moves into
// the chats table. The old channel columns are left in place and no longer
// read.
func migrateNotificationTeamRouting(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		-- Who or what a notification is about, the line the app's list shows
		-- under its topic.
		ALTER TABLE internal_notifications
			ADD COLUMN IF NOT EXISTS subtitle TEXT NOT NULL DEFAULT '';

		ALTER TABLE notification_teams
			ADD COLUMN IF NOT EXISTS lead_user_id TEXT REFERENCES users(id) ON DELETE SET NULL;
		ALTER TABLE notification_team_members
			ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;

		CREATE TABLE IF NOT EXISTS notification_team_chats(
			team_id BIGINT NOT NULL REFERENCES notification_teams(id) ON DELETE CASCADE,
			conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (team_id, conversation_id)
		);

		-- topic is a topic id, 'unrouted' for whatever reaches the team through
		-- the catch-all, or '*' for the team's default.
		CREATE TABLE IF NOT EXISTS notification_team_topic_routes(
			team_id BIGINT NOT NULL REFERENCES notification_teams(id) ON DELETE CASCADE,
			topic TEXT NOT NULL,
			destinations JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_by TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (team_id, topic)
		);

		INSERT INTO notification_team_chats (team_id, conversation_id, created_by)
		SELECT t.id, t.chat_conversation_id, 'migration'
		FROM notification_teams t
		JOIN conversations c ON c.id = t.chat_conversation_id
		WHERE t.chat_conversation_id IS NOT NULL
		ON CONFLICT DO NOTHING;

		INSERT INTO notification_team_topic_routes (team_id, topic, destinations, updated_by)
		SELECT t.id, '*', jsonb_build_object(
				'everyone', jsonb_build_object('push', t.push_members, 'email', t.email_members),
				'members', '[]'::jsonb,
				'email_address', t.email_address <> '',
				'chats', CASE WHEN t.chat_conversation_id IS NULL THEN '[]'::jsonb
					ELSE jsonb_build_array(t.chat_conversation_id) END
			), 'migration'
		FROM notification_teams t
		WHERE t.push_members = FALSE OR t.email_members = TRUE
			OR t.email_address <> '' OR t.chat_conversation_id IS NOT NULL
		ON CONFLICT DO NOTHING;

		UPDATE notification_teams t
		SET lead_user_id = (
			SELECT m.user_id FROM notification_team_members m
			WHERE m.team_id = t.id
			ORDER BY m.created_at, m.user_id
			LIMIT 1
		)
		WHERE t.lead_user_id IS NULL;
	`); err != nil {
		return fmt.Errorf("error adding notification team leads and routing: %w", err)
	}
	return nil
}
