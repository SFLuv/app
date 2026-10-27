package bootstrap

import (
	"context"
	"fmt"
	"strings"
)

// migrateChatV2 is the 1.74 migration body: what Messages needed to stop being
// a pair of people swapping lines of text.
//
// Five things arrive together because they are one feature as far as the client
// is concerned — a chat screen you can actually use:
//
//  1. Group conversations. The 1.67 schema keyed a conversation on
//     (user_low, user_high, post_id) with a CHECK that the pair was ordered, so
//     "a conversation" and "two people" were the same statement. Membership
//     already lived in its own table, which is what makes this a widening
//     rather than a rewrite: the pair columns stay, and stay authoritative, for
//     direct threads.
//  2. Replies and reactions.
//  3. Attachments — photos, files, links with their metadata, and references to
//     a transaction, a merchant location or a volunteer event.
//  4. Contact details: notes and a picture, so a saved contact is a card rather
//     than a row, and the user_id link 1.67 added is finally populated.
//  5. Profile pictures.
//
// ── On the file columns ──────────────────────────────────────────────────────
//
// message_attachments.file_data, contact_avatars.image_data and
// profile_avatars.image_data hold bytes in Postgres, the same way
// bulletin_post_photos.image_data has since 1.67. That is deliberate and
// temporary: it keeps one store to back up and no credentials to hold while the
// chat beta is small. It does not scale, and the move to object storage is
// expected.
//
// Every table and route that stores a file this way is listed in
// docs/DB_FILE_STORAGE.md. When the bucket arrives, that document is the
// migration checklist — add to it rather than discovering the set again.
func migrateChatV2(ctx context.Context, pools *MigrationPools) error {
	// 1.67 wrote CHECK (user_low < user_high) without a name, so Postgres chose
	// one. Dropping it by the name we expect it chose would leave a release in
	// which the old constraint survived and every group insert failed its
	// ordering test, so it is found by what it says rather than what it is
	// called. In Go rather than a DO block: nothing else in this package uses
	// PL/pgSQL, and dollar-quoting through a driver that also treats $ as a
	// parameter marker is not a thing to find out about during a deploy.
	if err := dropOrderedPairCheck(ctx, pools); err != nil {
		return err
	}

	// ── Group conversations ──────────────────────────────────────────────────
	//
	// The pair columns become nullable and their ordering CHECK is replaced by
	// one that holds for both shapes: a direct thread still has an ordered pair,
	// a group has neither. Dropping a CHECK by name is safe because 1.67 created
	// it unnamed — Postgres named it conversations_check — so the drop is
	// written IF EXISTS and the new one is added under a name we chose.
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE conversations
			ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'direct',
			ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS icon_attachment_id BIGINT,
			ADD COLUMN IF NOT EXISTS owner_id TEXT REFERENCES users(id);

		ALTER TABLE conversations ALTER COLUMN user_low DROP NOT NULL;
		ALTER TABLE conversations ALTER COLUMN user_high DROP NOT NULL;

		ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_kind_shape_check;
		ALTER TABLE conversations ADD CONSTRAINT conversations_kind_shape_check CHECK (
			(kind = 'direct' AND user_low IS NOT NULL AND user_high IS NOT NULL AND user_low < user_high)
			OR (kind = 'group' AND user_low IS NULL AND user_high IS NULL)
		);
		ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_kind_values_check;
		ALTER TABLE conversations ADD CONSTRAINT conversations_kind_values_check
			CHECK (kind IN ('direct', 'group'));

		-- A group has no pair to be unique on, so the pair index must stop
		-- applying to it. Without this, every group would collide on
		-- (NULL, NULL, 0) — which Postgres would in fact permit, since NULLs are
		-- distinct in a unique index, but the index would be meaningless and the
		-- next person to read it would be misled about what it enforces.
		DROP INDEX IF EXISTS conversations_pair_post_uidx;
		CREATE UNIQUE INDEX IF NOT EXISTS conversations_pair_post_uidx
			ON conversations(user_low, user_high, COALESCE(post_id, 0))
			WHERE kind = 'direct';

		-- Every thread that exists today is a direct one, and its creator is not
		-- its owner in any meaningful sense (there is nothing to administer), so
		-- owner_id is left NULL for them rather than backfilled from started_by.
		-- It is set only on groups, where it decides who may hand out admin.
	`); err != nil {
		return fmt.Errorf("error widening conversations for groups: %w", err)
	}

	// ── Membership roles ─────────────────────────────────────────────────────
	//
	// left_at rather than a delete: somebody who leaves a group keeps their
	// messages in it, and the rows in front of the people still there must not
	// change their sender. It also means rejoining is an update, so the history
	// they already had comes back with them.
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE conversation_members
			ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'member',
			ADD COLUMN IF NOT EXISTS left_at TIMESTAMPTZ,
			ADD COLUMN IF NOT EXISTS added_by TEXT REFERENCES users(id),
			ADD COLUMN IF NOT EXISTS joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

		ALTER TABLE conversation_members DROP CONSTRAINT IF EXISTS conversation_members_role_check;
		ALTER TABLE conversation_members ADD CONSTRAINT conversation_members_role_check
			CHECK (role IN ('owner', 'admin', 'member'));

		CREATE INDEX IF NOT EXISTS conversation_members_active_idx
			ON conversation_members(conversation_id) WHERE left_at IS NULL;
	`); err != nil {
		return fmt.Errorf("error adding membership roles: %w", err)
	}

	// ── Replies, and the message kinds attachments need ──────────────────────
	//
	// ON DELETE SET NULL on reply_to_id, not CASCADE: deleting a message must
	// not take the replies to it with it. The reply survives and renders
	// without its quoted line, which is what every other chat app does.
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE messages
			ADD COLUMN IF NOT EXISTS reply_to_id BIGINT REFERENCES messages(id) ON DELETE SET NULL;

		ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_kind_check;
		ALTER TABLE messages ADD CONSTRAINT messages_kind_check
			CHECK (kind IN ('text', 'payment', 'system', 'attachment'));

		CREATE INDEX IF NOT EXISTS messages_reply_to_idx
			ON messages(reply_to_id) WHERE reply_to_id IS NOT NULL;
	`); err != nil {
		return fmt.Errorf("error adding message replies: %w", err)
	}

	// ── Attachments ──────────────────────────────────────────────────────────
	//
	// One table for seven kinds, because they are one thing to the client: a
	// card under a message. Three carry bytes (image, file) or scraped metadata
	// (link); four are references resolved at read time against data that lives
	// elsewhere and may have changed or gone — a transaction, a merchant
	// location, a volunteer event, a support ticket.
	//
	// The reference kinds are soft where they have to be. location_id and
	// support_request_id have real foreign keys because both tables are in this
	// database; tx_hash and event_id do not, because the chain and the bot
	// database are not ours to constrain. A reference whose target has vanished
	// renders as "no longer available" rather than failing the message that
	// carried it.
	if _, err := pools.App.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS message_attachments(
			id BIGSERIAL PRIMARY KEY,
			message_id BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
			position SMALLINT NOT NULL DEFAULT 0 CHECK (position BETWEEN 0 AND 9),
			kind TEXT NOT NULL CHECK (kind IN ('image', 'file', 'link', 'transaction', 'merchant', 'event', 'support')),

			-- Bytes, for image and file. See docs/DB_FILE_STORAGE.md.
			content_type TEXT NOT NULL DEFAULT '',
			file_name TEXT NOT NULL DEFAULT '',
			file_data BYTEA,
			size_bytes INTEGER CHECK (size_bytes IS NULL OR (size_bytes > 0 AND size_bytes <= 26214400)),
			-- Pixel dimensions, so the client can reserve the right space before
			-- the image arrives rather than reflowing the thread under the reader.
			image_width INTEGER,
			image_height INTEGER,

			-- Link preview, scraped once at send time and then frozen: a card
			-- that silently changes under a message nobody edited would be a way
			-- to rewrite what was said.
			link_url TEXT NOT NULL DEFAULT '',
			link_title TEXT NOT NULL DEFAULT '',
			link_description TEXT NOT NULL DEFAULT '',
			link_image_url TEXT NOT NULL DEFAULT '',
			link_site_name TEXT NOT NULL DEFAULT '',

			-- References.
			tx_hash TEXT,
			location_id INTEGER REFERENCES locations(id) ON DELETE SET NULL,
			event_id TEXT,
			-- A support ticket, so a reply opened from the alert carries what it
			-- is about. ON DELETE SET NULL is unreachable in practice: a support
			-- request is archived, never deleted.
			support_request_id BIGINT REFERENCES support_requests(id) ON DELETE SET NULL,

			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

			CONSTRAINT message_attachments_payload_check CHECK (
				(kind IN ('image', 'file') AND file_data IS NOT NULL AND content_type <> '')
				OR (kind = 'link' AND link_url <> '')
				OR (kind = 'transaction' AND tx_hash IS NOT NULL)
				OR (kind = 'merchant' AND location_id IS NOT NULL)
				OR (kind = 'event' AND event_id IS NOT NULL)
				OR (kind = 'support' AND support_request_id IS NOT NULL)
			)
		);
		CREATE INDEX IF NOT EXISTS message_attachments_message_idx
			ON message_attachments(message_id, position);
		-- The "files in this chat" search the info screen offers. Partial,
		-- because it only ever lists the kinds that are actually files.
		CREATE INDEX IF NOT EXISTS message_attachments_file_search_idx
			ON message_attachments(message_id) WHERE kind IN ('image', 'file');

		-- The group icon is an attachment row with no message, which is why the
		-- reference is added here rather than in the ALTER above: the table has
		-- to exist first.
		ALTER TABLE conversations
			DROP CONSTRAINT IF EXISTS conversations_icon_attachment_id_fkey;
		ALTER TABLE conversations
			ADD CONSTRAINT conversations_icon_attachment_id_fkey
			FOREIGN KEY (icon_attachment_id) REFERENCES message_attachments(id) ON DELETE SET NULL;
	`); err != nil {
		return fmt.Errorf("error creating message attachments: %w", err)
	}

	// An icon and a sticker-less group avatar have no message to hang off, so
	// message_id has to be nullable for them. Done after the table is created so
	// the CHECK above is written against the simple case and relaxed once.
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE message_attachments ALTER COLUMN message_id DROP NOT NULL;
		CREATE INDEX IF NOT EXISTS message_attachments_orphan_idx
			ON message_attachments(created_at) WHERE message_id IS NULL;
	`); err != nil {
		return fmt.Errorf("error allowing standalone attachments: %w", err)
	}

	// ── Reactions ────────────────────────────────────────────────────────────
	//
	// The primary key is (message, user, emoji), so one person may react twice
	// with different emoji but not twice with the same one. The emoji itself is
	// stored rather than an id from a fixed list: the client offers six by
	// default and a picker for the rest, and a server-side list would have to be
	// released to add one.
	if _, err := pools.App.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS message_reactions(
			message_id BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			emoji TEXT NOT NULL CHECK (char_length(emoji) BETWEEN 1 AND 16),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (message_id, user_id, emoji)
		);
		CREATE INDEX IF NOT EXISTS message_reactions_message_idx
			ON message_reactions(message_id);
	`); err != nil {
		return fmt.Errorf("error creating message reactions: %w", err)
	}

	// ── Contact details ──────────────────────────────────────────────────────
	//
	// 1.67 added contacts.user_id and nothing ever wrote it. It is written from
	// now on when a contact's address belongs to an account, and backfilled here
	// for the ones already saved — which is what makes a saved contact
	// searchable as a person in the new-message picker.
	//
	// The backfill joins wallets rather than users because a contact is saved by
	// address, and one account has several. LOWER() on both sides: addresses are
	// stored however they were pasted.
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE contacts
			ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '' CHECK (char_length(notes) <= 2000);

		UPDATE contacts c
		SET user_id = w.owner
		FROM wallets w
		WHERE c.user_id IS NULL
			AND c.active = TRUE
			AND (LOWER(w.eoa_address) = LOWER(c.address) OR LOWER(w.smart_address) = LOWER(c.address));

		CREATE INDEX IF NOT EXISTS contacts_owner_user_idx
			ON contacts(owner, user_id) WHERE user_id IS NOT NULL AND active = TRUE;

		-- Separate table rather than columns on contacts, so the row every
		-- contact list query reads stays small and the picture is fetched only
		-- when a card is opened. See docs/DB_FILE_STORAGE.md.
		CREATE TABLE IF NOT EXISTS contact_avatars(
			contact_id INTEGER PRIMARY KEY REFERENCES contacts(id) ON DELETE CASCADE,
			content_type TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
			image_data BYTEA NOT NULL,
			size_bytes INTEGER NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 4194304),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`); err != nil {
		return fmt.Errorf("error adding contact details: %w", err)
	}

	// ── Profile pictures ─────────────────────────────────────────────────────
	if _, err := pools.App.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS profile_avatars(
			user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			content_type TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
			image_data BYTEA NOT NULL,
			size_bytes INTEGER NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 4194304),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`); err != nil {
		return fmt.Errorf("error creating profile avatars: %w", err)
	}

	// ── Support requests from inside the app ─────────────────────────────────
	//
	// A request submitted from the website has an email address and nothing
	// else. One submitted from the app is from a signed-in account, and knowing
	// which lets the alert offer to open a chat with them instead of only a
	// mailto. Nullable, because the website form is still the common case.
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE support_requests
			ADD COLUMN IF NOT EXISTS user_id TEXT REFERENCES users(id);
		CREATE INDEX IF NOT EXISTS support_requests_user_idx
			ON support_requests(user_id) WHERE user_id IS NOT NULL;
	`); err != nil {
		return fmt.Errorf("error linking support requests to accounts: %w", err)
	}

	return nil
}

// dropOrderedPairCheck removes the unnamed CHECK 1.67 put on conversations to
// require an ordered pair of users, whatever Postgres named it. Idempotent:
// finding none is the normal result on a database that has already run 1.74.
func dropOrderedPairCheck(ctx context.Context, pools *MigrationPools) error {
	rows, err := pools.App.Query(ctx, `
		SELECT conname FROM pg_constraint
		WHERE conrelid = 'conversations'::regclass
			AND contype = 'c'
			AND pg_get_constraintdef(oid) ILIKE '%user_low%<%user_high%';
	`)
	if err != nil {
		return fmt.Errorf("error finding the conversations pair check: %w", err)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return fmt.Errorf("error reading a conversations constraint name: %w", err)
		}
		names = append(names, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error listing conversations constraints: %w", err)
	}

	for _, name := range names {
		// The name comes from pg_constraint, not from input, but it is still an
		// identifier being pasted into DDL — quoted so a constraint named
		// oddly cannot change the statement's shape.
		if _, err := pools.App.Exec(ctx, `ALTER TABLE conversations DROP CONSTRAINT `+pgQuoteIdentifier(name)+`;`); err != nil {
			return fmt.Errorf("error dropping the conversations pair check %q: %w", name, err)
		}
	}
	return nil
}

// pgQuoteIdentifier double-quotes an identifier for use in DDL, doubling any
// embedded quote, the same way quote_ident does server-side.
func pgQuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
