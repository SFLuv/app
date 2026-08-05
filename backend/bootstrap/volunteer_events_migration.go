package bootstrap

import (
	"context"
)

// migrateVolunteerEvents is the 1.24 migration body. It upgrades the existing
// faucet event system into the volunteer event system rather than building a
// parallel one, so QR minting and redemption keep working untouched:
//
//  1. widens bot.events with volunteer, recurrence, signup, review, and funding
//     columns — every one defaulted so pre-existing faucet events keep their
//     current behaviour and stay invisible to the public portal
//     (is_volunteer = FALSE),
//  2. adds event_photos (cover photos, stored as BYTEA and served over a public
//     URL, mirroring workflow_submission_photos),
//  3. adds event_signups for the internal signup mode,
//  4. adds event_allocations, the per-event faucet reservation that replaces
//     standing per-cycle organization budgets,
//  5. adds volunteer_email_list (app db) for the opt-in marketing list, with
//     double opt-in and unsubscribe tokens.
//
// The qr_live_at column is the one behavioural change to existing rows' code
// path: redemption gates on COALESCE(qr_live_at, start_at), so legacy events
// (qr_live_at NULL) behave exactly as before while volunteer events open their
// QR codes 24h ahead of start.
func migrateVolunteerEvents(ctx context.Context, pools *MigrationPools) error {
	// Events table upgrade. Defaults are chosen so existing rows are already
	// correct: not volunteer events, already approved, already funded, codes
	// already generated.
	if _, err := pools.Bot.Exec(ctx, `
		ALTER TABLE events
			ADD COLUMN IF NOT EXISTS is_volunteer BOOLEAN NOT NULL DEFAULT FALSE,
			ADD COLUMN IF NOT EXISTS slug TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'America/Los_Angeles',
			ADD COLUMN IF NOT EXISTS max_participants INTEGER NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS signup_mode TEXT NOT NULL DEFAULT 'none',
			ADD COLUMN IF NOT EXISTS signup_url TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS review_status TEXT NOT NULL DEFAULT 'approved',
			ADD COLUMN IF NOT EXISTS rejected_reason TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS cancelled_at BIGINT,
			ADD COLUMN IF NOT EXISTS qr_live_at BIGINT,
			ADD COLUMN IF NOT EXISTS codes_generated BOOLEAN NOT NULL DEFAULT TRUE,
			ADD COLUMN IF NOT EXISTS funding_status TEXT NOT NULL DEFAULT 'funded',
			ADD COLUMN IF NOT EXISTS location_id BIGINT,
			ADD COLUMN IF NOT EXISTS recurrence_frequency TEXT NOT NULL DEFAULT 'none',
			ADD COLUMN IF NOT EXISTS recurrence_interval INTEGER NOT NULL DEFAULT 1,
			ADD COLUMN IF NOT EXISTS recurrence_monthly_mode TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS recurrence_day_of_month INTEGER,
			ADD COLUMN IF NOT EXISTS recurrence_week_of_month INTEGER,
			ADD COLUMN IF NOT EXISTS recurrence_weekday INTEGER,
			ADD COLUMN IF NOT EXISTS recurrence_until BIGINT,
			ADD COLUMN IF NOT EXISTS series_id TEXT,
			ADD COLUMN IF NOT EXISTS series_index INTEGER NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS requested_by TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS approved_at BIGINT,
			ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT,
			ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT;
	`); err != nil {
		return err
	}

	// Value constraints. Added separately (and idempotently) so a re-run on a
	// partially migrated database does not fail on an existing constraint.
	if _, err := pools.Bot.Exec(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'events_signup_mode_check') THEN
				ALTER TABLE events ADD CONSTRAINT events_signup_mode_check
					CHECK (signup_mode IN ('none', 'external', 'internal'));
			END IF;

			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'events_review_status_check') THEN
				ALTER TABLE events ADD CONSTRAINT events_review_status_check
					CHECK (review_status IN ('pending', 'approved', 'rejected', 'cancelled'));
			END IF;

			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'events_funding_status_check') THEN
				ALTER TABLE events ADD CONSTRAINT events_funding_status_check
					CHECK (funding_status IN ('funded', 'awaiting_funding'));
			END IF;

			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'events_recurrence_frequency_check') THEN
				ALTER TABLE events ADD CONSTRAINT events_recurrence_frequency_check
					CHECK (recurrence_frequency IN ('none', 'daily', 'weekly', 'monthly'));
			END IF;
		END
		$$;
	`); err != nil {
		return err
	}

	if _, err := pools.Bot.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS events_volunteer_public_idx
			ON events(is_volunteer, review_status, start_at)
			WHERE is_volunteer = TRUE;
		CREATE INDEX IF NOT EXISTS events_series_idx ON events(series_id) WHERE series_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS events_funding_status_idx
			ON events(funding_status)
			WHERE funding_status = 'awaiting_funding';
	`); err != nil {
		return err
	}

	// Cover photos. Mirrors workflow_submission_photos: bytes in Postgres,
	// served over a cacheable public URL so clients never handle base64.
	if _, err := pools.Bot.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS event_photos(
			id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			position INTEGER NOT NULL DEFAULT 0,
			file_name TEXT NOT NULL DEFAULT '',
			content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
			photo_data BYTEA NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT
		);

		CREATE INDEX IF NOT EXISTS event_photos_event_idx ON event_photos(event_id, position);
	`); err != nil {
		return err
	}

	// Internal-mode signups. The partial unique indexes let a cancelled signup
	// be remade while preventing duplicate live signups per email and per user.
	if _, err := pools.Bot.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS event_signups(
			id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			user_id TEXT,
			email TEXT NOT NULL,
			first_name TEXT NOT NULL DEFAULT '',
			last_name TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'web',
			cancel_token TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT,
			cancelled_at BIGINT
		);

		CREATE INDEX IF NOT EXISTS event_signups_event_idx ON event_signups(event_id);
		CREATE INDEX IF NOT EXISTS event_signups_user_idx ON event_signups(user_id) WHERE user_id IS NOT NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS event_signups_event_email_live_idx
			ON event_signups(event_id, LOWER(email))
			WHERE cancelled_at IS NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS event_signups_event_user_live_idx
			ON event_signups(event_id, user_id)
			WHERE user_id IS NOT NULL AND cancelled_at IS NULL;
	`); err != nil {
		return err
	}

	// Per-event faucet allocations. This is what replaces standing per-cycle
	// organization budgets: an approved one-off reserves once, an approved
	// recurring event reserves one cycle's worth for as long as it is active.
	if _, err := pools.Bot.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS event_allocations(
			id TEXT PRIMARY KEY,
			event_id TEXT REFERENCES events(id) ON DELETE CASCADE,
			series_id TEXT,
			organization_id BIGINT,
			cycle TEXT NOT NULL DEFAULT 'one_time',
			amount BIGINT NOT NULL DEFAULT 0,
			active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT,
			released_at BIGINT
		);

		CREATE INDEX IF NOT EXISTS event_allocations_active_idx ON event_allocations(active) WHERE active = TRUE;
		CREATE INDEX IF NOT EXISTS event_allocations_event_idx ON event_allocations(event_id);
		CREATE INDEX IF NOT EXISTS event_allocations_series_idx ON event_allocations(series_id) WHERE series_id IS NOT NULL;
	`); err != nil {
		return err
	}

	// Rate-limit ledger for the unauthenticated signup endpoint. Kept in the
	// database rather than in process memory so the limit holds across every
	// backend instance.
	if _, err := pools.Bot.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS event_signup_attempts(
			id BIGSERIAL PRIMARY KEY,
			ip TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT
		);

		CREATE INDEX IF NOT EXISTS event_signup_attempts_ip_idx ON event_signup_attempts(ip, created_at);
		CREATE INDEX IF NOT EXISTS event_signup_attempts_email_idx ON event_signup_attempts(LOWER(email), created_at);
	`); err != nil {
		return err
	}

	// Volunteer event locations are real rows in the shared locations table (PJ's
	// answer to Q1: full integration with the existing locations system), but
	// they must never render on the merchant map. location_kind is the
	// discriminator every merchant-facing query filters on; existing rows are
	// all merchants, which the DEFAULT already encodes.
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE locations
			ADD COLUMN IF NOT EXISTS location_kind TEXT NOT NULL DEFAULT 'merchant';

		CREATE INDEX IF NOT EXISTS locations_kind_idx ON locations(location_kind);
	`); err != nil {
		return err
	}

	if _, err := pools.App.Exec(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'locations_kind_check') THEN
				ALTER TABLE locations ADD CONSTRAINT locations_kind_check
					CHECK (location_kind IN ('merchant', 'volunteer'));
			END IF;
		END
		$$;
	`); err != nil {
		return err
	}

	// Volunteer marketing list. Deliberately separate from the account-level
	// mailing_list opt-in: consent given on an event signup form is consent for
	// volunteer mail only. Rows start 'pending' and only become 'active' when
	// the double opt-in link is followed.
	if _, err := pools.App.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS volunteer_email_list(
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			first_name TEXT NOT NULL DEFAULT '',
			last_name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			confirm_token TEXT NOT NULL DEFAULT '',
			unsubscribe_token TEXT NOT NULL DEFAULT '',
			source_event_id TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			confirmed_at BIGINT,
			unsubscribed_at BIGINT
		);

		CREATE UNIQUE INDEX IF NOT EXISTS volunteer_email_list_email_idx ON volunteer_email_list(LOWER(email));
		CREATE INDEX IF NOT EXISTS volunteer_email_list_status_idx ON volunteer_email_list(status);
		CREATE UNIQUE INDEX IF NOT EXISTS volunteer_email_list_confirm_token_idx
			ON volunteer_email_list(confirm_token) WHERE confirm_token <> '';
		CREATE UNIQUE INDEX IF NOT EXISTS volunteer_email_list_unsubscribe_token_idx
			ON volunteer_email_list(unsubscribe_token) WHERE unsubscribe_token <> '';
	`); err != nil {
		return err
	}

	return nil
}
