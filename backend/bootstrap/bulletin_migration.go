package bootstrap

import (
	"context"
	"fmt"
)

// migrateBulletin is the 1.66 migration body: the neighborhood Bulletin board
// (posts and their photos), direct messages between users, and the safety
// tables both need (blocks, reports, moderation actions, trade feedback).
//
// Everything lives in the app database. A notice may link a volunteer event,
// but events live in the bot database, so bulletin_posts.linked_event_id is a
// soft reference resolved by the handler, the same way events.location_id
// points back the other way.
func migrateBulletin(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		-- A person's public face on the Bulletin and in Messages. Kept apart from
		-- users.contact_name, which can hold a full legal name and is never shown
		-- to other users.
		CREATE TABLE IF NOT EXISTS user_profiles(
			user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			public_name TEXT NOT NULL CHECK (char_length(public_name) BETWEEN 1 AND 40),
			handle TEXT UNIQUE CHECK (handle IS NULL OR handle ~ '^[a-z0-9_]{3,20}$'),
			discoverable BOOLEAN NOT NULL DEFAULT FALSE,
			bulletin_suspended_until TIMESTAMPTZ,
			bulletin_suspended_reason TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS user_profiles_discoverable_name_idx
			ON user_profiles(LOWER(public_name)) WHERE discoverable = TRUE;

		CREATE TABLE IF NOT EXISTS bulletin_posts(
			id BIGSERIAL PRIMARY KEY,
			author_id TEXT NOT NULL REFERENCES users(id),
			category TEXT NOT NULL CHECK (category IN ('free', 'for_sale', 'notice')),
			title TEXT NOT NULL CHECK (char_length(title) BETWEEN 3 AND 80),
			body TEXT NOT NULL DEFAULT '' CHECK (char_length(body) <= 2000),
			price_sfluv NUMERIC(12,2),
			accepts_sfluv BOOLEAN NOT NULL DEFAULT FALSE,
			accepts_cash BOOLEAN NOT NULL DEFAULT FALSE,
			-- Approximate location only: the server snaps coordinates to a grid
			-- before they are stored, so an exact position is never persisted.
			approx_lat NUMERIC(8,5) NOT NULL CHECK (approx_lat BETWEEN -90 AND 90),
			approx_lng NUMERIC(8,5) NOT NULL CHECK (approx_lng BETWEEN -180 AND 180),
			area_label TEXT NOT NULL DEFAULT '',
			meetup_location_id INTEGER REFERENCES locations(id) ON DELETE SET NULL,
			linked_event_id TEXT,
			status TEXT NOT NULL DEFAULT 'active'
				CHECK (status IN ('active', 'sold', 'claimed', 'removed', 'hidden')),
			expires_at TIMESTAMPTZ NOT NULL,
			removed_reason TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT bulletin_posts_for_sale_price_check CHECK (
				category <> 'for_sale'
				OR (price_sfluv > 0 AND (accepts_sfluv OR accepts_cash))
			),
			CONSTRAINT bulletin_posts_no_price_check CHECK (
				category = 'for_sale'
				OR (price_sfluv IS NULL AND NOT accepts_sfluv AND NOT accepts_cash)
			),
			CONSTRAINT bulletin_posts_event_notice_check CHECK (
				linked_event_id IS NULL OR category = 'notice'
			)
		);
		CREATE INDEX IF NOT EXISTS bulletin_posts_feed_idx
			ON bulletin_posts(approx_lat, approx_lng) WHERE status = 'active';
		CREATE INDEX IF NOT EXISTS bulletin_posts_author_idx
			ON bulletin_posts(author_id, created_at DESC);

		CREATE TABLE IF NOT EXISTS bulletin_post_photos(
			id BIGSERIAL PRIMARY KEY,
			post_id BIGINT NOT NULL REFERENCES bulletin_posts(id) ON DELETE CASCADE,
			position SMALLINT NOT NULL CHECK (position BETWEEN 0 AND 5),
			content_type TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
			image_data BYTEA NOT NULL,
			size_bytes INTEGER NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 4194304),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (post_id, position)
		);

		-- One thread per pair of people per post, plus one plain thread per pair
		-- (post_id NULL) for conversations started from Messages.
		CREATE TABLE IF NOT EXISTS conversations(
			id BIGSERIAL PRIMARY KEY,
			user_low TEXT NOT NULL REFERENCES users(id),
			user_high TEXT NOT NULL REFERENCES users(id),
			post_id BIGINT REFERENCES bulletin_posts(id) ON DELETE SET NULL,
			started_by TEXT NOT NULL REFERENCES users(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CHECK (user_low < user_high)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS conversations_pair_post_uidx
			ON conversations(user_low, user_high, COALESCE(post_id, 0));
		CREATE INDEX IF NOT EXISTS conversations_started_by_idx
			ON conversations(started_by, created_at);

		CREATE TABLE IF NOT EXISTS conversation_members(
			conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id),
			last_read_message_id BIGINT NOT NULL DEFAULT 0,
			archived_at TIMESTAMPTZ,
			PRIMARY KEY (conversation_id, user_id)
		);
		CREATE INDEX IF NOT EXISTS conversation_members_user_idx
			ON conversation_members(user_id);

		CREATE TABLE IF NOT EXISTS messages(
			id BIGSERIAL PRIMARY KEY,
			conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			sender_id TEXT NOT NULL REFERENCES users(id),
			kind TEXT NOT NULL DEFAULT 'text' CHECK (kind IN ('text', 'payment', 'system')),
			body TEXT NOT NULL DEFAULT '' CHECK (char_length(body) <= 2000),
			tx_hash TEXT,
			amount_sfluv_base NUMERIC(78,0),
			payment_verified BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			CHECK (kind <> 'payment' OR tx_hash IS NOT NULL)
		);
		CREATE INDEX IF NOT EXISTS messages_conversation_idx
			ON messages(conversation_id, id DESC);
		CREATE UNIQUE INDEX IF NOT EXISTS messages_payment_tx_uidx
			ON messages(LOWER(tx_hash)) WHERE kind = 'payment';

		CREATE TABLE IF NOT EXISTS user_blocks(
			blocker_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			blocked_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (blocker_id, blocked_id),
			CHECK (blocker_id <> blocked_id)
		);
		CREATE INDEX IF NOT EXISTS user_blocks_blocked_idx
			ON user_blocks(blocked_id);

		CREATE TABLE IF NOT EXISTS content_reports(
			id BIGSERIAL PRIMARY KEY,
			reporter_id TEXT NOT NULL REFERENCES users(id),
			target_type TEXT NOT NULL CHECK (target_type IN ('post', 'message', 'user')),
			target_id TEXT NOT NULL,
			reported_user_id TEXT REFERENCES users(id),
			reason TEXT NOT NULL CHECK (reason IN ('prohibited_item', 'scam', 'harassment', 'spam', 'unsafe', 'other')),
			details TEXT NOT NULL DEFAULT '' CHECK (char_length(details) <= 1000),
			status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'actioned', 'dismissed')),
			reviewed_by TEXT REFERENCES users(id),
			reviewed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (reporter_id, target_type, target_id)
		);
		CREATE INDEX IF NOT EXISTS content_reports_open_idx
			ON content_reports(created_at) WHERE status = 'open';
		CREATE INDEX IF NOT EXISTS content_reports_target_idx
			ON content_reports(target_type, target_id);

		-- What an admin did about a report, kept so a user's history is visible
		-- on the next report against them.
		CREATE TABLE IF NOT EXISTS bulletin_moderation_actions(
			id BIGSERIAL PRIMARY KEY,
			admin_id TEXT NOT NULL REFERENCES users(id),
			target_user_id TEXT REFERENCES users(id),
			report_id BIGINT REFERENCES content_reports(id) ON DELETE SET NULL,
			action TEXT NOT NULL CHECK (action IN ('dismiss', 'remove_post', 'remove_message', 'warn', 'suspend', 'unsuspend')),
			note TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS bulletin_moderation_actions_user_idx
			ON bulletin_moderation_actions(target_user_id, created_at DESC);

		CREATE TABLE IF NOT EXISTS bulletin_trade_feedback(
			post_id BIGINT NOT NULL REFERENCES bulletin_posts(id) ON DELETE CASCADE,
			from_user TEXT NOT NULL REFERENCES users(id),
			to_user TEXT NOT NULL REFERENCES users(id),
			positive BOOLEAN NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (post_id, from_user),
			CHECK (from_user <> to_user)
		);
		CREATE INDEX IF NOT EXISTS bulletin_trade_feedback_to_idx
			ON bulletin_trade_feedback(to_user) WHERE positive = TRUE;

		-- A saved contact can now point at an SFLuv user as well as a wallet
		-- address, which is what lets Messages open a thread from Contacts.
		ALTER TABLE contacts ADD COLUMN IF NOT EXISTS user_id TEXT REFERENCES users(id);
	`); err != nil {
		return fmt.Errorf("error creating bulletin tables: %w", err)
	}

	return nil
}
