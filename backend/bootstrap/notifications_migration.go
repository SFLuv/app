package bootstrap

import (
	"context"
	"fmt"
)

// migrateInternalNotifications is the 1.68 migration body: internal
// organization alerts, who receives them, and the per-user history the mobile
// app reads.
//
// Three tables, and the reason there are three rather than one is the
// distinction the whole feature turns on.
//
// An internal alert is a condition, not an event. "The faucet is low" is true
// until somebody tops it up; "this affiliate is waiting on a decision" is true
// until somebody decides. So internal_notifications holds one row per live
// condition, keyed by alert_key, and a partial unique index enforces that:
// re-posting the same condition updates the row it already has instead of
// adding a second one. An alert that nobody has to think about any more is
// resolved, not deleted, because the history is the point of the screen.
//
// Resolution is deliberately global rather than per-recipient. If one admin
// approves a merchant, the decision is made — the alert should stop asking
// everyone else, not sit in four other inboxes as a question that has already
// been answered.
//
// user_notifications is the other half: the per-person ledger behind the
// notification screen, holding every notification a user was sent, internal or
// not, so the history can be paginated server-side. Before this, the mobile
// inbox was the last fifty pushes in AsyncStorage on one device — history
// did not survive a reinstall and did not exist on a second phone. Rows for an
// internal alert link back to it, which is what lets one person's action clear
// it from everybody's active list at once.
//
// It starts empty. Nothing can backfill it, because the notifications it would
// be backfilled from were never recorded anywhere.
func migrateInternalNotifications(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		-- Which members receive which sets of internal alerts.
		--
		-- Scoped to an organization rather than global because that is where it
		-- is administered from, and because partner organizations will want
		-- their own sets later. Only the house organization has any today, and
		-- the handler enforces that.
		CREATE TABLE IF NOT EXISTS organization_notification_subscriptions(
			organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			-- Not a foreign key: the sets are a code catalog, like the
			-- capability catalog beside it, so adding one is a deploy rather
			-- than a migration and a removed one stops resolving on its own.
			notification_set TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (organization_id, user_id, notification_set)
		);
		CREATE INDEX IF NOT EXISTS organization_notification_subscriptions_set_idx
			ON organization_notification_subscriptions(organization_id, notification_set);

		-- One row per live operational condition.
		CREATE TABLE IF NOT EXISTS internal_notifications(
			id BIGSERIAL PRIMARY KEY,
			organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			notification_set TEXT NOT NULL,
			-- Stable for the condition, which is what makes posting idempotent.
			-- "funding:faucet:celo" is the same alert this minute as it was
			-- last minute, so the sweep can report it every run without
			-- generating a row every run.
			alert_key TEXT NOT NULL,
			-- Selects the action buttons from the registry in the handler
			-- layer. Buttons are not stored: they are a property of the kind,
			-- so a deploy that changes them changes them everywhere at once
			-- rather than leaving old rows carrying buttons that no longer
			-- work.
			kind TEXT NOT NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			-- Label/value detail rows, rendered under the body.
			fields JSONB NOT NULL DEFAULT '[]'::jsonb,
			-- What an action needs to do its job: the subject's id, the role
			-- being requested, the support reference.
			payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			urgent BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			resolved_at TIMESTAMPTZ,
			resolved_by TEXT NOT NULL DEFAULT '',
			-- How it ended, for the history row: 'acted', 'acked', 'cleared'.
			resolution TEXT NOT NULL DEFAULT '',
			-- A sentence describing the outcome, kept on the resolved row so
			-- the history says who approved a merchant rather than only that
			-- the alert went away.
			outcome TEXT NOT NULL DEFAULT ''
		);
		-- The invariant: at most one unresolved alert per condition. Resolved
		-- rows are excluded, so the same condition recurring after it was
		-- acknowledged posts a fresh alert rather than being swallowed as a
		-- duplicate of the one already dealt with.
		CREATE UNIQUE INDEX IF NOT EXISTS internal_notifications_live_key_idx
			ON internal_notifications(organization_id, alert_key)
			WHERE resolved_at IS NULL;
		CREATE INDEX IF NOT EXISTS internal_notifications_live_set_idx
			ON internal_notifications(organization_id, notification_set)
			WHERE resolved_at IS NULL;

		-- The per-user notification ledger, and the history the app paginates.
		CREATE TABLE IF NOT EXISTS user_notifications(
			id BIGSERIAL PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			kind TEXT NOT NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			-- The push data payload, so a history row can still route a tap.
			data JSONB NOT NULL DEFAULT '{}'::jsonb,
			-- Set for internal alerts. The join is what lets one member's
			-- decision clear the alert from every other member's active list.
			internal_notification_id BIGINT REFERENCES internal_notifications(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			-- Viewing the notification screen stamps this on everything an
			-- ordinary notification covers. It is not what retires an internal
			-- alert: those need their action run or their condition cleared.
			seen_at TIMESTAMPTZ
		);
		-- id breaks ties in the keyset pagination, so two notifications written
		-- in the same transaction cannot straddle a page boundary and cause one
		-- to be skipped or repeated as the client scrolls.
		CREATE INDEX IF NOT EXISTS user_notifications_feed_idx
			ON user_notifications(user_id, created_at DESC, id DESC);
		CREATE INDEX IF NOT EXISTS user_notifications_unseen_idx
			ON user_notifications(user_id) WHERE seen_at IS NULL;
		-- One copy of an alert per recipient, so re-posting a condition while
		-- it is live cannot deliver it twice to the same person.
		CREATE UNIQUE INDEX IF NOT EXISTS user_notifications_internal_recipient_idx
			ON user_notifications(internal_notification_id, user_id)
			WHERE internal_notification_id IS NOT NULL;
	`); err != nil {
		return fmt.Errorf("error creating the internal notification tables: %w", err)
	}

	return nil
}
