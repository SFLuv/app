package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// legacyBerachainChainID is the chain id assigned to Ponder rows that predate
// chain tagging. Before the Celo cutover, Berachain (80094) is the only indexed
// chain, so all untagged Ponder transaction rows belong to it.
const legacyBerachainChainID = 80094

const baselineDBVersion = "1.0"

// MigrationDB is the database surface migrations run against. It is satisfied
// by both *pgxpool.Pool and pgx.Tx; RunPendingMigrations passes per-database
// TRANSACTIONS, so every statement in a migration either commits together with
// the version bump or rolls back together — a failed migration can never leave
// a half-applied schema behind.
type MigrationDB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// MigrationPools carries the per-migration transaction handles for the app and
// bot databases (field names mirror DBPools so migration bodies read the same).
type MigrationPools struct {
	App MigrationDB
	Bot MigrationDB
}

type SchemaMigration struct {
	Version     string
	Description string
	Apply       func(context.Context, *MigrationPools, *logger.LogCloser) error
}

// Everything currently in CreateTables() is treated as baseline schema v1.0.
// Future schema changes should be added here in ascending version order.
var schemaMigrations = []SchemaMigration{
	{
		Version:     "1.1",
		Description: "add support indexes for list, location, wallet, and event queries",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				CREATE INDEX IF NOT EXISTS affiliates_created_idx
					ON affiliates(created_at DESC);
				CREATE INDEX IF NOT EXISTS proposers_created_idx
					ON proposers(created_at DESC);
				CREATE INDEX IF NOT EXISTS improvers_created_idx
					ON improvers(created_at DESC);
				CREATE INDEX IF NOT EXISTS supervisors_created_idx
					ON supervisors(created_at DESC);
				CREATE INDEX IF NOT EXISTS issuers_created_idx
					ON issuers(created_at DESC);
				CREATE INDEX IF NOT EXISTS wallets_owner_id_idx
					ON wallets(owner, id);
				CREATE INDEX IF NOT EXISTS wallets_owner_smart_default_idx
					ON wallets(owner, smart_index, id)
					WHERE is_eoa = FALSE;
				CREATE INDEX IF NOT EXISTS locations_approval_id_idx
					ON locations(approval, id);
				CREATE INDEX IF NOT EXISTS locations_owner_approval_id_idx
					ON locations(owner_id, approval, id);
				CREATE INDEX IF NOT EXISTS location_hours_location_weekday_idx
					ON location_hours(location_id, weekday);
				CREATE INDEX IF NOT EXISTS contacts_owner_favorite_id_idx
					ON contacts(owner, is_favorite DESC, id ASC);
				CREATE INDEX IF NOT EXISTS issuer_credential_scopes_credential_issuer_idx
					ON issuer_credential_scopes(credential_type, issuer_id);
				CREATE INDEX IF NOT EXISTS workflow_templates_owner_created_idx
					ON workflow_templates(owner_user_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS workflow_templates_default_created_idx
					ON workflow_templates(is_default, created_at DESC);
			`); err != nil {
				return err
			}

			if _, err := pools.Bot.Exec(ctx, `
				CREATE INDEX IF NOT EXISTS events_expiration_id_idx
					ON events(expiration, id);
				CREATE INDEX IF NOT EXISTS events_owner_expiration_id_idx
					ON events(owner, expiration, id);
				CREATE INDEX IF NOT EXISTS codes_event_id_idx
					ON codes(event, id);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.2",
		Description: "add location-owned payment wallets and tipping wallets for merchant payouts",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE locations
				ADD COLUMN IF NOT EXISTS tipping_wallet_address TEXT NOT NULL DEFAULT '';

				CREATE TABLE IF NOT EXISTS location_payment_wallets(
					id SERIAL PRIMARY KEY,
					location_id INTEGER NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
					wallet_address TEXT NOT NULL,
					is_default BOOLEAN NOT NULL DEFAULT false,
					UNIQUE (location_id, wallet_address)
				);

				CREATE INDEX IF NOT EXISTS location_payment_wallets_location_idx
					ON location_payment_wallets(location_id);

				CREATE UNIQUE INDEX IF NOT EXISTS location_payment_wallets_default_idx
					ON location_payment_wallets(location_id)
					WHERE is_default = TRUE;
			`); err != nil {
				return err
			}

			if _, err := pools.App.Exec(ctx, `
				DO $$
				BEGIN
					IF EXISTS (
						SELECT 1
						FROM information_schema.columns
						WHERE table_name = 'users'
						AND column_name = 'tipping_wallet_address'
					) THEN
						EXECUTE $sql$
							UPDATE locations l
							SET tipping_wallet_address = TRIM(COALESCE(u.tipping_wallet_address, ''))
							FROM users u
							WHERE l.owner_id = u.id
							AND TRIM(COALESCE(l.tipping_wallet_address, '')) = ''
							AND TRIM(COALESCE(u.tipping_wallet_address, '')) <> ''
						$sql$;
					END IF;
				END
				$$;
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.3",
		Description: "track workflow payout transaction hashes for payout reconciliation",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE workflows
				ADD COLUMN IF NOT EXISTS manager_payout_tx_hash TEXT;

				ALTER TABLE workflow_steps
				ADD COLUMN IF NOT EXISTS payout_tx_hash TEXT;
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.4",
		Description: "add soft-delete support, account deletion metadata, and merged wallet history",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS deletion_requested_at TIMESTAMPTZ;
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS deletion_canceled_at TIMESTAMPTZ;
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS deletion_completed_at TIMESTAMPTZ;

				ALTER TABLE user_verified_emails
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE user_verified_emails
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE user_verified_emails
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;
				ALTER TABLE user_verified_emails
				DROP CONSTRAINT IF EXISTS user_verified_emails_user_id_email_normalized_key;
				DROP INDEX IF EXISTS user_verified_emails_user_email_unique_idx;
				DROP INDEX IF EXISTS user_verified_emails_token_unique_idx;
				CREATE UNIQUE INDEX IF NOT EXISTS user_verified_emails_user_email_active_idx
					ON user_verified_emails(user_id, email_normalized)
					WHERE active = TRUE;
				CREATE UNIQUE INDEX IF NOT EXISTS user_verified_emails_token_active_idx
					ON user_verified_emails(verification_token)
					WHERE verification_token IS NOT NULL AND active = TRUE;

				ALTER TABLE wallets
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE wallets
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE wallets
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;
				ALTER TABLE wallets
				ADD COLUMN IF NOT EXISTS merged_wallets TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[];
				ALTER TABLE wallets
				DROP CONSTRAINT IF EXISTS wallets_owner_is_eoa_eoa_address_smart_index_key;
				CREATE UNIQUE INDEX IF NOT EXISTS wallets_identity_active_idx
					ON wallets(owner, is_eoa, eoa_address, smart_index)
					WHERE active = TRUE;

				ALTER TABLE memos
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE memos
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE memos
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;

				ALTER TABLE affiliates
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE affiliates
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE affiliates
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;

				ALTER TABLE proposers
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE proposers
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE proposers
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;

				ALTER TABLE improvers
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE improvers
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE improvers
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;

				ALTER TABLE supervisors
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE supervisors
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE supervisors
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;

				ALTER TABLE issuers
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE issuers
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE issuers
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;

				ALTER TABLE locations
				ADD COLUMN IF NOT EXISTS tipping_wallet_address TEXT NOT NULL DEFAULT '';
				ALTER TABLE locations
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE locations
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE locations
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;
				ALTER TABLE locations
				DROP CONSTRAINT IF EXISTS locations_google_id_key;
				CREATE UNIQUE INDEX IF NOT EXISTS locations_google_id_active_idx
					ON locations(google_id)
					WHERE active = TRUE AND google_id IS NOT NULL;

				ALTER TABLE location_hours
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE location_hours
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE location_hours
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;

				CREATE TABLE IF NOT EXISTS location_payment_wallets(
					id SERIAL PRIMARY KEY,
					location_id INTEGER NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
					wallet_address TEXT NOT NULL,
					is_default BOOLEAN NOT NULL DEFAULT false
				);
				ALTER TABLE location_payment_wallets
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE location_payment_wallets
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE location_payment_wallets
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;
				ALTER TABLE location_payment_wallets
				DROP CONSTRAINT IF EXISTS location_payment_wallets_location_id_wallet_address_key;
				CREATE INDEX IF NOT EXISTS location_payment_wallets_location_idx
					ON location_payment_wallets(location_id);
				CREATE UNIQUE INDEX IF NOT EXISTS location_payment_wallets_location_wallet_active_idx
					ON location_payment_wallets(location_id, wallet_address)
					WHERE active = TRUE;
				DROP INDEX IF EXISTS location_payment_wallets_default_idx;
				CREATE UNIQUE INDEX IF NOT EXISTS location_payment_wallets_default_active_idx
					ON location_payment_wallets(location_id)
					WHERE active = TRUE AND is_default = TRUE;

				ALTER TABLE contacts
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE contacts
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE contacts
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;
				ALTER TABLE contacts
				DROP CONSTRAINT IF EXISTS contacts_owner_address_key;
				ALTER TABLE contacts
				DROP CONSTRAINT IF EXISTS contacts_owner_name_key;
				CREATE UNIQUE INDEX IF NOT EXISTS contacts_owner_address_active_idx
					ON contacts(owner, address)
					WHERE active = TRUE;
				CREATE UNIQUE INDEX IF NOT EXISTS contacts_owner_name_active_idx
					ON contacts(owner, name)
					WHERE active = TRUE;

				ALTER TABLE ponder_subscriptions
				ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
				ALTER TABLE ponder_subscriptions
				ADD COLUMN IF NOT EXISTS delete_date TIMESTAMPTZ;
				ALTER TABLE ponder_subscriptions
				ADD COLUMN IF NOT EXISTS delete_reason TEXT;

				CREATE INDEX IF NOT EXISTS users_active_delete_idx
					ON users(active, delete_date);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.5",
		Description: "store revocable oauth credentials for account deletion and apple recovery",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS user_oauth_credentials(
					user_id TEXT NOT NULL,
					provider TEXT NOT NULL,
					provider_subject TEXT NOT NULL DEFAULT '',
					provider_email TEXT NOT NULL DEFAULT '',
					is_private_relay BOOLEAN NOT NULL DEFAULT false,
					access_token_encrypted TEXT NOT NULL DEFAULT '',
					refresh_token_encrypted TEXT NOT NULL DEFAULT '',
					access_token_expires_at TIMESTAMPTZ,
					refresh_token_expires_at TIMESTAMPTZ,
					scopes TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					revoked_at TIMESTAMPTZ,
					PRIMARY KEY (user_id, provider)
				);

				CREATE INDEX IF NOT EXISTS user_oauth_credentials_provider_subject_idx
					ON user_oauth_credentials(provider, provider_subject);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.6",
		Description: "store mobile push subscriptions for Expo push delivery",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS mobile_push_subscriptions(
					id SERIAL PRIMARY KEY,
					owner TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					token TEXT NOT NULL,
					address TEXT NOT NULL,
					ponder_hook_id INTEGER,
					installation_id_hash TEXT NOT NULL DEFAULT '',
					preference_enabled BOOLEAN NOT NULL DEFAULT true,
					device_registered BOOLEAN NOT NULL DEFAULT true,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					active BOOLEAN NOT NULL DEFAULT true,
					delete_date TIMESTAMPTZ,
					delete_reason TEXT
				);

				CREATE INDEX IF NOT EXISTS mobile_push_subscriptions_owner_idx
					ON mobile_push_subscriptions(owner);
				CREATE INDEX IF NOT EXISTS mobile_push_subscriptions_owner_token_idx
					ON mobile_push_subscriptions(owner, token);
				CREATE INDEX IF NOT EXISTS mobile_push_subscriptions_owner_installation_idx
					ON mobile_push_subscriptions(owner, installation_id_hash)
					WHERE installation_id_hash <> '';
				CREATE INDEX IF NOT EXISTS mobile_push_subscriptions_address_idx
					ON mobile_push_subscriptions(address);
				CREATE INDEX IF NOT EXISTS mobile_push_subscriptions_token_idx
					ON mobile_push_subscriptions(token);
				CREATE INDEX IF NOT EXISTS mobile_push_subscriptions_ponder_hook_idx
					ON mobile_push_subscriptions(ponder_hook_id)
					WHERE ponder_hook_id IS NOT NULL;
				CREATE UNIQUE INDEX IF NOT EXISTS mobile_push_subscriptions_token_address_idx
					ON mobile_push_subscriptions(token, address);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.7",
		Description: "require privacy-policy acceptance and record mailing-list opt-in preferences",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS accepted_privacy_policy BOOLEAN NOT NULL DEFAULT false;
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS accepted_privacy_policy_at TIMESTAMPTZ;
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS privacy_policy_version TEXT NOT NULL DEFAULT '';
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS mailing_list_opt_in BOOLEAN NOT NULL DEFAULT false;
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS mailing_list_opt_in_at TIMESTAMPTZ;
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS mailing_list_policy_version TEXT NOT NULL DEFAULT '';
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.8",
		Description: "track ponder hook ids for mobile push subscriptions",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE mobile_push_subscriptions
				ADD COLUMN IF NOT EXISTS ponder_hook_id INTEGER;

				CREATE INDEX IF NOT EXISTS mobile_push_subscriptions_ponder_hook_idx
					ON mobile_push_subscriptions(ponder_hook_id)
					WHERE ponder_hook_id IS NOT NULL;
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.9",
		Description: "store Expo push tickets and receipt outcomes",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS mobile_push_notification_tickets(
					id SERIAL PRIMARY KEY,
					ticket_id TEXT NOT NULL UNIQUE,
					owner TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					token TEXT NOT NULL,
					address TEXT NOT NULL,
					status TEXT NOT NULL DEFAULT 'pending',
					receipt_status TEXT,
					receipt_message TEXT,
					receipt_error_code TEXT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					checked_at TIMESTAMPTZ
				);

				CREATE INDEX IF NOT EXISTS mobile_push_notification_tickets_owner_idx
					ON mobile_push_notification_tickets(owner, created_at DESC);
				CREATE INDEX IF NOT EXISTS mobile_push_notification_tickets_token_idx
					ON mobile_push_notification_tickets(token, created_at DESC);
				CREATE INDEX IF NOT EXISTS mobile_push_notification_tickets_status_idx
					ON mobile_push_notification_tickets(status, created_at DESC);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.10",
		Description: "separate mobile push preference and device registration state",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE mobile_push_subscriptions
				ADD COLUMN IF NOT EXISTS preference_enabled BOOLEAN NOT NULL DEFAULT true;

				ALTER TABLE mobile_push_subscriptions
				ADD COLUMN IF NOT EXISTS device_registered BOOLEAN NOT NULL DEFAULT true;

				ALTER TABLE mobile_push_subscriptions
				ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

				CREATE INDEX IF NOT EXISTS mobile_push_subscriptions_owner_token_idx
					ON mobile_push_subscriptions(owner, token);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.11",
		Description: "index mobile push subscriptions by owner and token",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				CREATE INDEX IF NOT EXISTS mobile_push_subscriptions_owner_token_idx
					ON mobile_push_subscriptions(owner, token);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.12",
		Description: "add merchant mode settings and device registrations",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS merchant_mode_settings (
					owner_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
					pin_hash TEXT NOT NULL DEFAULT '',
					pin_hash_version TEXT NOT NULL DEFAULT 'bcrypt:v1',
					failed_attempt_count INTEGER NOT NULL DEFAULT 0,
					locked_until TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE TABLE IF NOT EXISTS merchant_mode_devices (
					id TEXT PRIMARY KEY,
					owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					location_id INTEGER NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
					installation_id_hash TEXT NOT NULL,
					display_name TEXT NOT NULL DEFAULT '',
					platform TEXT NOT NULL DEFAULT '',
					app_version TEXT NOT NULL DEFAULT '',
					wallet_address TEXT NOT NULL DEFAULT '',
					merchant_mode_enabled BOOLEAN NOT NULL DEFAULT false,
					enabled_at TIMESTAMPTZ,
					enabled_by TEXT NOT NULL DEFAULT '',
					disabled_at TIMESTAMPTZ,
					disabled_by TEXT NOT NULL DEFAULT '',
					last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					active BOOLEAN NOT NULL DEFAULT true
				);

				CREATE INDEX IF NOT EXISTS merchant_mode_devices_owner_location_idx
					ON merchant_mode_devices(owner_id, location_id)
					WHERE active = TRUE;

				CREATE UNIQUE INDEX IF NOT EXISTS merchant_mode_devices_owner_installation_active_idx
					ON merchant_mode_devices(owner_id, installation_id_hash)
					WHERE active = TRUE;

				CREATE INDEX IF NOT EXISTS merchant_mode_devices_location_enabled_idx
					ON merchant_mode_devices(location_id, merchant_mode_enabled)
					WHERE active = TRUE;
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.13",
		Description: "add analytics wallet role history and user activity snapshots",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS analytics_wallet_role_history(
					id BIGSERIAL PRIMARY KEY,
					address TEXT NOT NULL,
					role TEXT NOT NULL CHECK (role IN ('admin', 'merchant', 'faucet', 'zapper')),
					chain_id BIGINT NOT NULL,
					user_id TEXT,
					location_id INTEGER,
					source TEXT NOT NULL DEFAULT '',
					started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					ended_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS analytics_wallet_role_history_lookup_idx
					ON analytics_wallet_role_history(LOWER(address), role, chain_id, started_at, ended_at);
				CREATE UNIQUE INDEX IF NOT EXISTS analytics_wallet_role_history_active_idx
					ON analytics_wallet_role_history(LOWER(address), role, chain_id, COALESCE(user_id, ''), COALESCE(location_id, 0), source)
					WHERE ended_at IS NULL;

				CREATE TABLE IF NOT EXISTS analytics_user_activity(
					id BIGSERIAL PRIMARY KEY,
					user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					activity_date DATE NOT NULL,
					platform TEXT NOT NULL DEFAULT 'web',
					first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(user_id, activity_date, platform)
				);

				CREATE INDEX IF NOT EXISTS analytics_user_activity_date_idx
					ON analytics_user_activity(activity_date, user_id);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.14",
		Description: "seed historical analytics faucet wallet for fiscal-year backfill",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
					INSERT INTO analytics_wallet_role_history(address, role, chain_id, source, started_at, ended_at)
					SELECT
						'0x6c28631f17f2b6b6e9349ec8e7eba3827318bce3',
						'faucet',
						80094,
						'historical.manual.old_faucet_2025_08',
						TIMESTAMPTZ '2025-07-01 00:00:00+00',
						TIMESTAMPTZ '2026-02-01 00:00:00+00'
					WHERE NOT EXISTS (
						SELECT 1
						FROM analytics_wallet_role_history
						WHERE LOWER(address) = '0x6c28631f17f2b6b6e9349ec8e7eba3827318bce3'
						AND role = 'faucet'
						AND chain_id = 80094
						AND source = 'historical.manual.old_faucet_2025_08'
					);
				`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.15",
		Description: "track mobile client versions for migration readiness",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS user_client_versions(
					id BIGSERIAL PRIMARY KEY,
					user_id TEXT,
					client_key TEXT NOT NULL,
					platform TEXT NOT NULL DEFAULT '',
					version TEXT NOT NULL DEFAULT '',
					build TEXT NOT NULL DEFAULT '',
					build_number INTEGER NOT NULL DEFAULT 0,
					user_agent TEXT NOT NULL DEFAULT '',
					source TEXT NOT NULL DEFAULT '',
					legacy_inferred BOOLEAN NOT NULL DEFAULT FALSE,
					first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE UNIQUE INDEX IF NOT EXISTS user_client_versions_client_key_idx
					ON user_client_versions(client_key);
				CREATE INDEX IF NOT EXISTS user_client_versions_user_last_seen_idx
					ON user_client_versions(user_id, last_seen_at DESC)
					WHERE user_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS user_client_versions_version_build_idx
					ON user_client_versions(LOWER(version), LOWER(build));
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.16",
		Description: "tag transaction-bearing records with chain ids",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
					ALTER TABLE memos
					ADD COLUMN IF NOT EXISTS chain_id BIGINT;

					ALTER TABLE memos
					DROP CONSTRAINT IF EXISTS memos_pkey;

					CREATE UNIQUE INDEX IF NOT EXISTS memos_chain_tx_hash_unique_idx
						ON memos(chain_id, tx_hash);
					CREATE INDEX IF NOT EXISTS memos_chain_tx_hash_idx
						ON memos(chain_id, LOWER(tx_hash));

					ALTER TABLE workflows
					ADD COLUMN IF NOT EXISTS manager_payout_chain_id BIGINT;

					ALTER TABLE workflow_steps
					ADD COLUMN IF NOT EXISTS payout_chain_id BIGINT;

					ALTER TABLE w9_wallet_earnings
					ADD COLUMN IF NOT EXISTS chain_id BIGINT;

					ALTER TABLE w9_wallet_earnings
					ADD COLUMN IF NOT EXISTS last_tx_chain_id BIGINT;

					ALTER TABLE w9_wallet_earnings
					DROP CONSTRAINT IF EXISTS w9_wallet_earnings_pkey;

					CREATE UNIQUE INDEX IF NOT EXISTS w9_wallet_earnings_chain_wallet_year_unique_idx
						ON w9_wallet_earnings(wallet_address, year, chain_id);
					CREATE INDEX IF NOT EXISTS w9_wallet_earnings_chain_idx
						ON w9_wallet_earnings(chain_id, year);
				`); err != nil {
				return err
			}

			if _, err := pools.Bot.Exec(ctx, `
					ALTER TABLE redemptions
					ADD COLUMN IF NOT EXISTS chain_id BIGINT;

					CREATE INDEX IF NOT EXISTS redemptions_chain_idx
						ON redemptions(chain_id);
				`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.17",
		Description: "track anonymous client phone-home hits for app usage metrics",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
					CREATE TABLE IF NOT EXISTS client_phone_home_metrics(
						day DATE NOT NULL,
						endpoint TEXT NOT NULL,
						platform TEXT NOT NULL DEFAULT '',
						version TEXT NOT NULL DEFAULT '',
						build TEXT NOT NULL DEFAULT '',
						hits BIGINT NOT NULL DEFAULT 0,
						first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
						last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
						PRIMARY KEY (day, endpoint, platform, version, build)
					);

					CREATE INDEX IF NOT EXISTS client_phone_home_metrics_day_idx
						ON client_phone_home_metrics(day DESC);
					CREATE INDEX IF NOT EXISTS client_phone_home_metrics_platform_day_idx
						ON client_phone_home_metrics(platform, day DESC);
				`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.18",
		Description: "backfill legacy Berachain chain ids on Ponder transaction tables (DISABLED)",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Disabled: the Ponder database is owned by the Ponder indexer and must
			// not be altered or written to from the backend. Doing so (ALTER ADD
			// chain_id / SET DEFAULT / indexes / UPDATE) changed Ponder's schema out
			// from under the running indexer and tripped its live-query triggers,
			// which halted indexing in production. Legacy chain-id tagging is handled
			// by the cross-chain migration on a clone only — never the live Ponder DB.
			if appLogger != nil {
				appLogger.Logf("migration 1.18: ponder chain-id backfill is disabled (Ponder DB must not be modified by the backend)")
			}
			return nil
		},
	},
	{
		Version:     "1.19",
		Description: "scope mobile push subscriptions by app installation",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE mobile_push_subscriptions
				ADD COLUMN IF NOT EXISTS installation_id_hash TEXT NOT NULL DEFAULT '';

				CREATE INDEX IF NOT EXISTS mobile_push_subscriptions_owner_installation_idx
					ON mobile_push_subscriptions(owner, installation_id_hash)
					WHERE installation_id_hash <> '';
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.20",
		Description: "record non-migrated holder balances for post-migration recovery claims",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.Bot.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS recovery_balances(
					address TEXT PRIMARY KEY,
					chain_id BIGINT NOT NULL,
					amount NUMERIC(78, 0) NOT NULL,
					claim_status TEXT NOT NULL DEFAULT 'unclaimed',
					claimed_by TEXT,
					claimed_by_user_id TEXT,
					claim_tx_hash TEXT,
					claim_tx_chain_id BIGINT,
					claimed_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS recovery_balances_status_idx
					ON recovery_balances(claim_status);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.21",
		Description: "add OAuth state for the read-only admin MCP endpoint (identity = SFLUV user DID)",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			if _, err := pools.App.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS admin_mcp_oauth_clients(
					client_id TEXT PRIMARY KEY,
					client_name TEXT NOT NULL DEFAULT '',
					redirect_uris TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE TABLE IF NOT EXISTS admin_mcp_oauth_login_states(
					state_hash TEXT PRIMARY KEY,
					client_id TEXT NOT NULL REFERENCES admin_mcp_oauth_clients(client_id) ON DELETE CASCADE,
					redirect_uri TEXT NOT NULL,
					client_state TEXT NOT NULL DEFAULT '',
					code_challenge TEXT NOT NULL,
					code_challenge_method TEXT NOT NULL DEFAULT 'S256',
					scope TEXT NOT NULL DEFAULT '',
					resource TEXT NOT NULL DEFAULT '',
					expires_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE TABLE IF NOT EXISTS admin_mcp_oauth_auth_codes(
					code_hash TEXT PRIMARY KEY,
					client_id TEXT NOT NULL REFERENCES admin_mcp_oauth_clients(client_id) ON DELETE CASCADE,
					user_did TEXT NOT NULL,
					redirect_uri TEXT NOT NULL,
					code_challenge TEXT NOT NULL,
					code_challenge_method TEXT NOT NULL DEFAULT 'S256',
					scope TEXT NOT NULL DEFAULT '',
					resource TEXT NOT NULL DEFAULT '',
					expires_at TIMESTAMPTZ NOT NULL,
					used_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE TABLE IF NOT EXISTS admin_mcp_oauth_tokens(
					token_hash TEXT PRIMARY KEY,
					client_id TEXT NOT NULL REFERENCES admin_mcp_oauth_clients(client_id) ON DELETE CASCADE,
					user_did TEXT NOT NULL,
					refresh_token_hash TEXT,
					scope TEXT NOT NULL DEFAULT '',
					resource TEXT NOT NULL DEFAULT '',
					issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					expires_at TIMESTAMPTZ NOT NULL,
					last_used_at TIMESTAMPTZ,
					revoked_at TIMESTAMPTZ
				);

				CREATE INDEX IF NOT EXISTS admin_mcp_oauth_tokens_did_idx
					ON admin_mcp_oauth_tokens(user_did, expires_at DESC);
				CREATE INDEX IF NOT EXISTS admin_mcp_oauth_tokens_client_idx
					ON admin_mcp_oauth_tokens(client_id, expires_at DESC);

				CREATE TABLE IF NOT EXISTS admin_mcp_oauth_refresh_tokens(
					token_hash TEXT PRIMARY KEY,
					client_id TEXT NOT NULL REFERENCES admin_mcp_oauth_clients(client_id) ON DELETE CASCADE,
					user_did TEXT NOT NULL,
					scope TEXT NOT NULL DEFAULT '',
					resource TEXT NOT NULL DEFAULT '',
					issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					expires_at TIMESTAMPTZ NOT NULL,
					last_used_at TIMESTAMPTZ,
					revoked_at TIMESTAMPTZ
				);

				CREATE INDEX IF NOT EXISTS admin_mcp_oauth_refresh_tokens_did_idx
					ON admin_mcp_oauth_refresh_tokens(user_did, expires_at DESC);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.22",
		Description: "introduce organizations: merge duplicate role orgs, org membership/roles, invites, and cycle-based allocations",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			return migrateOrganizations(ctx, pools)
		},
	},
	{
		Version:     "1.23",
		Description: "org-level issuer scopes seeded from member credentials, synced to members, and inherited org settings (affiliate logo)",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			return migrateOrganizationIssuerScopes(ctx, pools)
		},
	},
	{
		Version:     "1.24",
		Description: "one location_hours row per weekday",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			return migrateLocationHoursUniqueness(ctx, pools, appLogger)
		},
	},
	{
		Version:     "1.25",
		Description: "volunteer events: upgrade events with volunteer/recurrence/signup fields, cover photos, signups, per-event faucet allocations, and the volunteer email list",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			return migrateVolunteerEvents(ctx, pools)
		},
	},
	{
		Version:     "1.26",
		Description: "improver notification read markers for the workflow notifications feed",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Notifications themselves are derived from live workflow state
			// rather than materialized, so a notification cannot drift out of
			// sync with the thing it describes. Only the per-user "I have seen
			// this" marker needs storing.
			if _, err := pools.App.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS improver_notification_reads(
					user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					notification_key TEXT NOT NULL,
					seen_at BIGINT NOT NULL DEFAULT unix_now(),
					PRIMARY KEY (user_id, notification_key)
				);

				CREATE INDEX IF NOT EXISTS improver_notification_reads_user_idx
					ON improver_notification_reads(user_id);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.27",
		Description: "partner organizations shown in the public site's partner carousel",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Logos are stored as bytes and served over a public URL, the same
			// pattern as workflow photos and volunteer event covers, so the
			// public site consumes a plain URL rather than inline base64.
			// Dimensions are captured at upload because the carousel needs them
			// to reserve layout space before the image loads.
			if _, err := pools.App.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS partners(
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					link_url TEXT NOT NULL DEFAULT '',
					logo_data BYTEA,
					logo_content_type TEXT NOT NULL DEFAULT '',
					logo_width INTEGER NOT NULL DEFAULT 0,
					logo_height INTEGER NOT NULL DEFAULT 0,
					logo_updated_at BIGINT,
					position INTEGER NOT NULL DEFAULT 0,
					active BOOLEAN NOT NULL DEFAULT TRUE,
					created_at BIGINT NOT NULL DEFAULT unix_now(),
					updated_at BIGINT NOT NULL DEFAULT unix_now()
				);

				CREATE INDEX IF NOT EXISTS partners_active_position_idx
					ON partners(active, position, created_at);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.28",
		Description: "volunteer event reminder preferences and sent-reminder ledger",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Preferences are server-side, not device-local: the backend sends
			// the push at a time the phone may not be running, so it needs the
			// value. A missing row means the defaults (on, 24h), so a user who
			// never touches the setting still gets reminders.
			if _, err := pools.App.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS volunteer_reminder_preferences(
					user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
					enabled BOOLEAN NOT NULL DEFAULT TRUE,
					hours_before INTEGER NOT NULL DEFAULT 24,
					updated_at BIGINT NOT NULL DEFAULT unix_now()
				);

				-- One reminder per (user, event) ever, which is what makes the
				-- sender idempotent: several matching emails, a retry, or a
				-- second pass cannot produce a second buzz.
				CREATE TABLE IF NOT EXISTS volunteer_reminder_sends(
					user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					event_id TEXT NOT NULL,
					sent_at BIGINT NOT NULL DEFAULT unix_now(),
					PRIMARY KEY (user_id, event_id)
				);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.29",
		Description: "confirmed-by-email volunteer signups and the organizer event blast log",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Portal (anonymous) signups must confirm by email before they count.
			// Existing rows are backfilled as confirmed: they were made under the
			// old rules and retroactively invalidating someone's spot would be
			// worse than the inconsistency.
			if _, err := pools.Bot.Exec(ctx, `
				ALTER TABLE event_signups
					ADD COLUMN IF NOT EXISTS confirm_token TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS confirmed_at BIGINT;

				UPDATE event_signups SET confirmed_at = created_at WHERE confirmed_at IS NULL;

				CREATE UNIQUE INDEX IF NOT EXISTS event_signups_confirm_token_idx
					ON event_signups(confirm_token) WHERE confirm_token <> '';

				CREATE TABLE IF NOT EXISTS event_blasts(
					id TEXT PRIMARY KEY,
					event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
					sent_by TEXT NOT NULL DEFAULT '',
					subject TEXT NOT NULL DEFAULT '',
					message TEXT NOT NULL DEFAULT '',
					push_count INTEGER NOT NULL DEFAULT 0,
					email_count INTEGER NOT NULL DEFAULT 0,
					created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT
				);

				CREATE INDEX IF NOT EXISTS event_blasts_event_idx ON event_blasts(event_id, created_at DESC);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.30",
		Description: "explicit QR redemption cutoff for volunteer events",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Codes previously expired the moment the event ended, which left no
			// room for someone still in the queue. The redemption window now runs
			// to a separate cutoff, defaulting to 24h after the end. NULL means
			// "fall back to the event end", so legacy events are unchanged.
			if _, err := pools.Bot.Exec(ctx, `
				ALTER TABLE events
					ADD COLUMN IF NOT EXISTS qr_expires_at BIGINT;

				UPDATE events
				SET qr_expires_at = expiration + 86400
				WHERE is_volunteer = TRUE AND qr_expires_at IS NULL AND expiration > 0;
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.31",
		Description: "affiliate event edit requests awaiting admin approval",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// An affiliate editing a LIVE event cannot apply it directly: the
			// change may raise the reward cost, and committing faucet funds is an
			// admin decision. The proposed payload is parked here and applied on
			// approval, so the published event is never disturbed by a request
			// that may be refused.
			if _, err := pools.Bot.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS event_edit_requests(
					id TEXT PRIMARY KEY,
					event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
					requested_by TEXT NOT NULL DEFAULT '',
					payload TEXT NOT NULL,
					status TEXT NOT NULL DEFAULT 'pending',
					reject_reason TEXT NOT NULL DEFAULT '',
					created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT,
					decided_at BIGINT,
					decided_by TEXT NOT NULL DEFAULT ''
				);

				CREATE INDEX IF NOT EXISTS event_edit_requests_event_idx
					ON event_edit_requests(event_id, created_at DESC);
				-- At most one open request per event, so approving one cannot be
				-- racing another written against different values.
				CREATE UNIQUE INDEX IF NOT EXISTS event_edit_requests_open_idx
					ON event_edit_requests(event_id) WHERE status = 'pending';
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.32",
		Description: "inline images for organizer event blasts",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Email clients cannot read an authenticated URL, so blast images are
			// served from a public, unguessable id — the same pattern as event
			// cover photos.
			if _, err := pools.Bot.Exec(ctx, `
				CREATE TABLE IF NOT EXISTS event_blast_images(
					id TEXT PRIMARY KEY,
					event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
					content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
					image_data BYTEA NOT NULL,
					size_bytes INTEGER NOT NULL DEFAULT 0,
					created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT
				);

				CREATE INDEX IF NOT EXISTS event_blast_images_event_idx ON event_blast_images(event_id);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.33",
		Description: "stable per-event code numbers for printed QR labels",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Printed QR sheets are labelled "{title} #{n}". The number must be
			// STABLE: reprinting a batch has to put the same number on the same
			// QR, or a reprint cannot be reconciled against the originals.
			//
			// Deriving it at print time from row order would not survive minting
			// extra codes — a new UUID sorts into the middle and renumbers
			// everything after it. So the number is assigned once and stored.
			if _, err := pools.Bot.Exec(ctx, `
				ALTER TABLE codes ADD COLUMN IF NOT EXISTS code_number INTEGER;

				-- Backfill deterministically by id, so existing sheets keep a
				-- consistent ordering rather than an arbitrary one.
				WITH numbered AS (
					SELECT id, ROW_NUMBER() OVER (PARTITION BY event ORDER BY id) AS n
					FROM codes
					WHERE code_number IS NULL AND event IS NOT NULL
				)
				UPDATE codes c
				SET code_number = numbered.n
				FROM numbered
				WHERE c.id = numbered.id;

				CREATE INDEX IF NOT EXISTS codes_event_number_idx ON codes(event, code_number);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.34",
		Description: "structured location hours and per-location manual hours mode",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Hours were stored only as Google's display text ("Monday: 7:00 AM
			// – 8:00 PM"). That cannot back a time picker, cannot be compared,
			// and cannot distinguish "closed" from "we never learned this day".
			// The text column stays as the rendered form so every existing
			// reader keeps working; these columns become the source of truth.
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE location_hours ADD COLUMN IF NOT EXISTS open_minute INTEGER;
				ALTER TABLE location_hours ADD COLUMN IF NOT EXISTS close_minute INTEGER;
				ALTER TABLE location_hours ADD COLUMN IF NOT EXISTS is_closed BOOLEAN NOT NULL DEFAULT FALSE;

				-- Manual mode opts a listing out of the nightly Google sync, so a
				-- hand-corrected set of hours is never silently overwritten.
				ALTER TABLE locations ADD COLUMN IF NOT EXISTS hours_manual BOOLEAN NOT NULL DEFAULT FALSE;
				ALTER TABLE locations ADD COLUMN IF NOT EXISTS hours_synced_at TIMESTAMPTZ;
			`); err != nil {
				return err
			}

			// Backfill through the same parser the app uses, so historical rows
			// and new ones agree on what the text meant. Anything unparseable is
			// left as NULL rather than guessed: a wrong opening time published to
			// customers is worse than a missing one.
			rows, err := pools.App.Query(ctx, `
				SELECT location_id, weekday, COALESCE(hours, '')
				FROM location_hours
				WHERE open_minute IS NULL AND close_minute IS NULL AND is_closed = FALSE;
			`)
			if err != nil {
				return err
			}
			type parsedRow struct {
				locationID int
				day        structs.LocationDayHours
			}
			parsed := []parsedRow{}
			for rows.Next() {
				var locationID, weekday int
				var text string
				if err := rows.Scan(&locationID, &weekday, &text); err != nil {
					rows.Close()
					return err
				}
				day := structs.ParseDisplayHours(weekday, text)
				if day.IsClosed || day.HasTimes() {
					parsed = append(parsed, parsedRow{locationID: locationID, day: day})
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}

			for _, entry := range parsed {
				if _, err := pools.App.Exec(ctx, `
					UPDATE location_hours
					SET open_minute = $1, close_minute = $2, is_closed = $3
					WHERE location_id = $4 AND weekday = $5;
				`, firstMinutes(entry.day, true), firstMinutes(entry.day, false), entry.day.IsClosed, entry.locationID, entry.day.Weekday); err != nil {
					return err
				}
			}
			appLogger.Logf("migration 1.34: lifted %d location hour rows into structured times", len(parsed))

			return nil
		},
	},
	{
		Version:     "1.35",
		Description: "split opening hours per day",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// A single open/close pair cannot describe a kitchen that shuts
			// between lunch and dinner, and flattening one into 11:00–21:00 tells
			// customers a shop is open while it is shut.
			//
			// Stored as JSON on the existing row rather than as extra rows: every
			// reader of location_hours assumes one row per weekday and seven
			// strings per location, and adding rows would silently break all of
			// them. open_minute/close_minute stay populated with the first stretch
			// so anything still reading those keeps working.
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE location_hours
				ADD COLUMN IF NOT EXISTS intervals JSONB NOT NULL DEFAULT '[]'::jsonb;

				UPDATE location_hours
				SET intervals = jsonb_build_array(
					jsonb_build_object('open_minute', open_minute, 'close_minute', close_minute)
				)
				WHERE open_minute IS NOT NULL
				AND close_minute IS NOT NULL
				AND intervals = '[]'::jsonb;
			`); err != nil {
				return err
			}

			// Days whose text held a split that 1.34 could not represent are now
			// readable, so lift them rather than leaving them blank.
			rows, err := pools.App.Query(ctx, `
				SELECT location_id, weekday, COALESCE(hours, '')
				FROM location_hours
				WHERE intervals = '[]'::jsonb AND is_closed = FALSE;
			`)
			if err != nil {
				return err
			}
			type parsedRow struct {
				locationID int
				day        structs.LocationDayHours
			}
			parsed := []parsedRow{}
			for rows.Next() {
				var locationID, weekday int
				var text string
				if err := rows.Scan(&locationID, &weekday, &text); err != nil {
					rows.Close()
					return err
				}
				day := structs.ParseDisplayHours(weekday, text)
				if len(day.Intervals) > 0 {
					parsed = append(parsed, parsedRow{locationID: locationID, day: day})
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}

			for _, entry := range parsed {
				encoded, err := json.Marshal(entry.day.Intervals)
				if err != nil {
					return err
				}
				if _, err := pools.App.Exec(ctx, `
					UPDATE location_hours
					SET intervals = $1::jsonb, open_minute = $2, close_minute = $3
					WHERE location_id = $4 AND weekday = $5;
				`, encoded, entry.day.Intervals[0].OpenMinute, entry.day.Intervals[0].CloseMinute,
					entry.locationID, entry.day.Weekday); err != nil {
					return err
				}
			}
			appLogger.Logf("migration 1.35: recovered %d split-hour location days", len(parsed))

			return nil
		},
	},
	{
		Version:     "1.36",
		Description: "manual merchant listings for businesses with no Google place",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Not every merchant has a Google Business Profile, and requiring one
			// blocked onboarding outright. listing_source records which path a
			// location came in through so an admin reviewing the queue knows
			// whether the name and address were verified against Google or typed
			// by hand. Existing rows all came from a place id, hence the default.
			//
			// google_id also stops being an empty string for manual rows: the
			// partial unique index covers google_id IS NOT NULL, so a second
			// manual listing would otherwise collide with the first on ''.
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE locations
				ADD COLUMN IF NOT EXISTS listing_source TEXT NOT NULL DEFAULT 'google_place';

				UPDATE locations
				SET google_id = NULL
				WHERE google_id IS NOT NULL AND TRIM(google_id) = '';

				UPDATE locations
				SET listing_source = 'manual'
				WHERE google_id IS NULL AND listing_source = 'google_place';
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.37",
		Description: "merchant map icons",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Bytes live in their own table because every map read selects the
			// whole listing row and none of them want the image.
			//
			// icon_updated_at mirrors the upload time onto `locations` so a
			// listing can advertise "there is an icon, at this version" without
			// joining the blob. The version is what lets the image itself be
			// served with a long cache lifetime and still change when a
			// merchant replaces it.
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE locations
				ADD COLUMN IF NOT EXISTS icon_updated_at TIMESTAMPTZ;

				CREATE TABLE IF NOT EXISTS location_icons(
					location_id INTEGER PRIMARY KEY REFERENCES locations(id) ON DELETE CASCADE,
					content_type TEXT NOT NULL,
					image_data BYTEA NOT NULL,
					size_bytes INTEGER NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.38",
		Description: "staged event cover photos, uploaded before their event exists",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Cover photos used to be uploadable only after their event had been
			// created, which forced the client to create the event first and
			// then attach — so a failed photo left a published event with
			// missing artwork and no way to undo it.
			//
			// A staged photo is one with no event yet: uploaded the moment it is
			// chosen, owned by whoever uploaded it, and attached inside the same
			// transaction that creates the event. Either the event exists with
			// all of its photos or it does not exist at all.
			if _, err := pools.Bot.Exec(ctx, `
				ALTER TABLE event_photos ALTER COLUMN event_id DROP NOT NULL;

				ALTER TABLE event_photos
				ADD COLUMN IF NOT EXISTS staged_by TEXT NOT NULL DEFAULT '';

				-- Only staged rows are ever looked up this way, so the index
				-- carries none of the attached ones.
				CREATE INDEX IF NOT EXISTS event_photos_staged_idx
					ON event_photos(staged_by, created_at)
					WHERE event_id IS NULL;
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.39",
		Description: "merchant location photos",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// A picture of the place itself, distinct from the map icon added in
			// 1.37: the icon is a mark drawn a few pixels wide inside a pin, this
			// is a photograph shown at card width on the listing.
			//
			// `locations.image_url` was not reused for this. It is written once at
			// creation from the Google place and holds a link to a Maps *page*
			// rather than to an image, so anything rendering it as a picture gets
			// a broken one. Storing bytes here keeps a merchant's own photo out of
			// that ambiguity, and out of the third-party lifetime that comes with
			// hotlinking Google's.
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE locations
				ADD COLUMN IF NOT EXISTS photo_updated_at TIMESTAMPTZ;

				CREATE TABLE IF NOT EXISTS location_photos(
					location_id INTEGER PRIMARY KEY REFERENCES locations(id) ON DELETE CASCADE,
					content_type TEXT NOT NULL,
					image_data BYTEA NOT NULL,
					size_bytes INTEGER NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.40",
		Description: "per-location payment wallets, so two shops can be told apart",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Until now location_payment_wallets was empty and every location fell
			// back to its owner's users.primary_wallet_address. That works while a
			// merchant has one shop and silently breaks the moment they have two:
			// both resolve to the same address, and an incoming transfer carries
			// nothing that says which shop it belongs to. Takings, tips and the
			// merchant-mode day view all become unsplittable, and no later fix can
			// separate history that was already commingled.
			//
			// So: write down what each location resolves to today. Behaviour is
			// unchanged the moment this lands — the link merely stops being implied
			// by a COALESCE and starts being a row someone can see and change.
			tag, err := pools.App.Exec(ctx, `
				INSERT INTO location_payment_wallets (location_id, wallet_address, is_default)
				SELECT
					l.id,
					COALESCE(
						NULLIF(TRIM(u.primary_wallet_address), ''),
						NULLIF(TRIM(legacy.smart_address), '')
					),
					TRUE
				FROM locations l
				LEFT JOIN users u
					ON u.id = l.owner_id
					AND u.active = TRUE
				LEFT JOIN LATERAL (
					SELECT w.smart_address
					FROM wallets w
					WHERE w.owner = l.owner_id
					AND w.active = TRUE
					AND w.is_eoa = FALSE
					AND NULLIF(TRIM(w.smart_address), '') IS NOT NULL
					ORDER BY w.smart_index ASC NULLS LAST, w.id ASC
					LIMIT 1
				) legacy ON TRUE
				WHERE l.active = TRUE
				AND COALESCE(
					NULLIF(TRIM(u.primary_wallet_address), ''),
					NULLIF(TRIM(legacy.smart_address), '')
				) IS NOT NULL
				AND NOT EXISTS (
					SELECT 1 FROM location_payment_wallets existing
					WHERE existing.location_id = l.id
					AND existing.active = TRUE
				);
			`)
			if err != nil {
				return err
			}
			if appLogger != nil {
				appLogger.Logf("migration 1.40: recorded %d location payment wallets", tag.RowsAffected())
			}

			// A location left without a wallet would have no payable address at
			// all once the fallback goes, so say so loudly rather than letting it
			// surface later as a merchant who cannot be paid.
			var unresolved int
			if err := pools.App.QueryRow(ctx, `
				SELECT COUNT(*)
				FROM locations l
				WHERE l.active = TRUE
				AND NOT EXISTS (
					SELECT 1 FROM location_payment_wallets p
					WHERE p.location_id = l.id AND p.active = TRUE
				);
			`).Scan(&unresolved); err != nil {
				return err
			}
			if unresolved > 0 && appLogger != nil {
				appLogger.Logf("migration 1.40: WARNING %d active locations still have no payment wallet", unresolved)
			}

			// One address, one role, one location. Enforced in the database as well
			// as the handler because the whole point is that two shops can never
			// share a till — a bug in a write path should not be able to undo that.
			//
			// Global rather than per-owner: a wallet belongs to exactly one user, so
			// two locations sharing one are necessarily the same merchant's anyway,
			// and a global index is both simpler and stricter.
			if _, err := pools.App.Exec(ctx, `
				CREATE UNIQUE INDEX IF NOT EXISTS location_payment_wallets_address_unique_idx
					ON location_payment_wallets (LOWER(TRIM(wallet_address)))
					WHERE active = TRUE;

				CREATE UNIQUE INDEX IF NOT EXISTS locations_tipping_wallet_unique_idx
					ON locations (LOWER(TRIM(tipping_wallet_address)))
					WHERE active = TRUE AND NULLIF(TRIM(tipping_wallet_address), '') IS NOT NULL;
			`); err != nil {
				return err
			}

			return nil
		},
	},
	{
		Version:     "1.41",
		Description: "w9 rebuild: tax payees, filings, payout ledger, escrow",
		Apply:       migrateW9Rebuild,
	},
	{
		Version:     "1.42",
		Description: "payout ledger: a failed payout must not block its source forever",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// The uniqueness rule is "one payout per source record", which is
			// right — a redemption code must never pay out twice. But as first
			// written it counted failed rows too, so a single failed attempt
			// held that code's slot permanently and made it unredeemable with
			// no route back short of editing the ledger by hand.
			if _, err := pools.App.Exec(ctx, `
				DROP INDEX IF EXISTS payout_ledger_source_ref_idx;
				CREATE UNIQUE INDEX IF NOT EXISTS payout_ledger_source_ref_idx
					ON payout_ledger (source, source_ref)
					WHERE source_ref <> '' AND state NOT IN ('failed', 'cancelled');
			`); err != nil {
				return fmt.Errorf("error rebuilding the payout ledger source index: %w", err)
			}
			return nil
		},
	},
	{
		Version:     "1.43",
		Description: "w9 filings: record the asynchronous TIN match separately from signing",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Signing and TIN matching are two events, not one. The vendor's
			// match is asynchronous and can resolve about a day after the form
			// is signed, so escrow releases on the signature alone — holding
			// somebody's money for a day after they did everything asked of
			// them would be the wrong trade.
			//
			// The match still has to be recorded when it lands, so the sweeper
			// keeps polling past release and needs somewhere to put the answer.
			// A rejected match never claws back a released payout; it blocks the
			// next one.
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE w9_filings ADD COLUMN IF NOT EXISTS tin_match TEXT NOT NULL DEFAULT '';
				ALTER TABLE w9_filings ADD COLUMN IF NOT EXISTS tin_match_at TIMESTAMPTZ;
				CREATE INDEX IF NOT EXISTS w9_filings_unresolved_match_idx
					ON w9_filings (status) WHERE tin_match IN ('', 'pending');
			`); err != nil {
				return fmt.Errorf("error adding tin match columns: %w", err)
			}
			return nil
		},
	},
	{
		Version:     "1.44",
		Description: "w9: escalating warning tiers, and escrow that cannot accumulate",
		Apply:       migrateW9WarningTiers,
	},
	{
		Version:     "1.45",
		Description: "users: the account type picked at signup, separate from the derived merchant flag",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// is_merchant cannot hold this. It is recomputed as EXISTS(approved
			// location) every time an approval changes (see AppDB.UpdateLocationApproval),
			// so a signup answer written there is overwritten by the next
			// approval that touches the owner, with nothing left to say what
			// the person originally chose.
			//
			// Nor is it backfilled from is_merchant: that flag records that a
			// listing was approved, not what somebody signed up as, and a
			// merchant who also spends in the app as a customer would be
			// reclassified by the copy. Everyone existing becomes 'regular';
			// the signup question is the only thing that ever sets this.
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS account_type TEXT NOT NULL DEFAULT 'regular';

				ALTER TABLE users
				ADD COLUMN IF NOT EXISTS merchant_onboarding_completed_at TIMESTAMPTZ;

				ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_type_check;
				ALTER TABLE users ADD CONSTRAINT users_account_type_check
					CHECK (account_type IN ('regular', 'merchant'));
			`); err != nil {
				return fmt.Errorf("error adding user account type columns: %w", err)
			}

			return nil
		},
	},
	{
		Version:     "1.46",
		Description: "locations: the payment wallet address, readable from the location row",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Every reader of a location's payable address currently rebuilds it
			// with the same lateral join into location_payment_wallets. Writing
			// it onto the row makes it one column instead of a join each caller
			// has to remember to get right.
			//
			// Nothing reads it yet, on purpose. Representation lands first so
			// the two can be diffed against each other on real data before any
			// read path moves; a migration that also switched the readers could
			// only be checked after the fact.
			if _, err := pools.App.Exec(ctx, `
				ALTER TABLE locations
				ADD COLUMN IF NOT EXISTS payment_wallet_address TEXT NOT NULL DEFAULT '';
			`); err != nil {
				return fmt.Errorf("error adding the location payment wallet column: %w", err)
			}

			// location_payment_wallets is the only source. users.primary_wallet_address
			// is not consulted: migration 1.40 already resolved that fallback
			// with the owner's full wallet history to hand, and reading it again
			// now would hand the same owner address to every shop they run —
			// exactly the collision 1.40 existed to end.
			//
			// Volunteer event rows live in this table too and have no till, so
			// only merchant listings are considered. Rows that already carry an
			// address are left alone, which also makes a re-run a no-op.
			tag, err := pools.App.Exec(ctx, `
				UPDATE locations l
				SET payment_wallet_address = COALESCE((
					SELECT NULLIF(TRIM(w.wallet_address), '')
					FROM location_payment_wallets w
					WHERE w.location_id = l.id
					AND w.active = TRUE
					AND NULLIF(TRIM(w.wallet_address), '') IS NOT NULL
					ORDER BY w.is_default DESC, w.id ASC
					LIMIT 1
				), '')
				WHERE l.location_kind = 'merchant'
				AND l.payment_wallet_address = '';
			`)
			if err != nil {
				return fmt.Errorf("error backfilling location payment wallet addresses: %w", err)
			}

			// A merchant listing with no wallet row gets '' rather than failing
			// the migration: an empty string is the honest answer for a location
			// that genuinely has nowhere to be paid, and refusing to migrate over
			// it would block a schema change on unrelated data. 1.40 warned in
			// the same situation for the same reason.
			var unresolved int
			if err := pools.App.QueryRow(ctx, `
				SELECT COUNT(*)
				FROM locations l
				WHERE l.location_kind = 'merchant'
				AND l.active = TRUE
				AND l.payment_wallet_address = '';
			`).Scan(&unresolved); err != nil {
				return fmt.Errorf("error counting locations without a payment wallet: %w", err)
			}
			if appLogger != nil {
				appLogger.Logf("migration 1.46: wrote %d location payment wallet addresses", tag.RowsAffected())
				if unresolved > 0 {
					appLogger.Logf("migration 1.46: WARNING %d active merchant locations have no payment wallet and were left empty", unresolved)
				}
			}

			// Same rule the tipping address already lives under, and for the same
			// reason: one address serves one location, so a write path that
			// duplicated one would be stopped by the database rather than
			// discovered later as two shops sharing a till.
			if _, err := pools.App.Exec(ctx, `
				CREATE UNIQUE INDEX IF NOT EXISTS locations_payment_wallet_unique_idx
					ON locations (LOWER(TRIM(payment_wallet_address)))
					WHERE active = TRUE AND NULLIF(TRIM(payment_wallet_address), '') IS NOT NULL;
			`); err != nil {
				return fmt.Errorf("error creating the location payment wallet unique index: %w", err)
			}

			return nil
		},
	},
	{
		Version:     "1.47",
		Description: "classify existing merchants, and re-derive the location payment address the way the readers do",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Existing merchants never pass through the new signup choice — they
			// accepted the privacy policy long ago, and account_type is written
			// once, at first acceptance. Left alone they would all read as
			// 'regular' forever: the Locations tab would not appear, and the
			// merchant surfaces would be unreachable for the only people who
			// need them.
			//
			// Owning an approved location is the honest definition. It is also
			// exactly the set is_merchant already tracks (10 of each on the
			// development clone, with no user flagged as a merchant who owns no
			// approved location), so this reclassifies nobody by surprise.
			classified, err := pools.App.Exec(ctx, `
				UPDATE users u
				SET account_type = 'merchant'
				WHERE u.account_type = 'regular'
				AND EXISTS (
					SELECT 1 FROM locations l
					WHERE l.owner_id = u.id
					AND l.approval = TRUE
					AND l.location_kind = 'merchant'
				);
			`)
			if err != nil {
				return fmt.Errorf("error classifying existing merchants: %w", err)
			}

			// And stamp them as onboarded, in the same migration and for the same
			// reason. The forced-onboarding gate refuses anyone who is
			// merchant-typed with a NULL timestamp, and the stamp is only written
			// when a location is created — which for these people already
			// happened, months ago, under rules that did not record it.
			//
			// Classifying without stamping would lock every existing merchant out
			// of the app behind a form asking them to do a thing they have
			// already done. The two statements belong together and must never be
			// split.
			stamped, err := pools.App.Exec(ctx, `
				UPDATE users u
				SET merchant_onboarding_completed_at = COALESCE(
					(
						-- Their first approval is the truest record of when they
						-- finished becoming a merchant. It is a naive timestamp,
						-- so it is read as UTC rather than as the server's local
						-- zone, which would shift every one of these by hours.
						SELECT MIN(l.approved_at AT TIME ZONE 'UTC') FROM locations l
						WHERE l.owner_id = u.id
						AND l.approval = TRUE
						AND l.location_kind = 'merchant'
						AND l.approved_at IS NOT NULL
					),
					NOW()
				)
				WHERE u.account_type = 'merchant'
				AND u.merchant_onboarding_completed_at IS NULL
				AND EXISTS (
					SELECT 1 FROM locations l
					WHERE l.owner_id = u.id
					AND l.approval = TRUE
					AND l.location_kind = 'merchant'
				);
			`)
			if err != nil {
				return fmt.Errorf("error stamping merchant onboarding for existing merchants: %w", err)
			}

			// Migration 1.46 derived this column with a predicate the read paths
			// do not have: it skipped a blank address and took the next row. So a
			// location whose default wallet row holds a blank could end up named
			// after a wallet the map never shows — and nothing repairs it, because
			// the runtime sync only fires when a wallet is written.
			//
			// Re-derive every row with the expression the readers actually use.
			// It is idempotent by construction: where 1.46 already agreed, this
			// writes the same value.
			resynced, err := pools.App.Exec(ctx, `
				UPDATE locations l
				SET payment_wallet_address = COALESCE((
					SELECT NULLIF(TRIM(lpw.wallet_address), '')
					FROM location_payment_wallets lpw
					WHERE lpw.location_id = l.id
					AND lpw.active = TRUE
					ORDER BY
						CASE WHEN lpw.is_default = TRUE THEN 0 ELSE 1 END,
						lpw.id ASC
					LIMIT 1
				), '')
				WHERE l.payment_wallet_address IS DISTINCT FROM COALESCE((
					SELECT NULLIF(TRIM(lpw.wallet_address), '')
					FROM location_payment_wallets lpw
					WHERE lpw.location_id = l.id
					AND lpw.active = TRUE
					ORDER BY
						CASE WHEN lpw.is_default = TRUE THEN 0 ELSE 1 END,
						lpw.id ASC
					LIMIT 1
				), '');
			`)
			if err != nil {
				return fmt.Errorf("error re-deriving location payment addresses: %w", err)
			}

			if appLogger != nil {
				appLogger.Logf("migration 1.47: classified %d existing merchants, stamped %d as onboarded, re-derived %d location payment addresses",
					classified.RowsAffected(), stamped.RowsAffected(), resynced.RowsAffected())
			}

			return nil
		},
	},
	{
		Version:     "1.48",
		Description: "make the location text and number columns NOT NULL, because the readers already assume it",
		Apply: func(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
			// Nearly every column on locations is nullable while the Go structs
			// scan them into plain strings and numbers. A single NULL therefore
			// fails the scan, and because GET /locations reads every row to draw
			// the public merchant map, one bad row takes the map down for
			// everyone rather than just for that shop:
			//
			//   can't scan into dest[15] (col: website): cannot scan NULL into *string
			//
			// Nothing in the API insert path writes NULL — it always sends a
			// string — so this has stayed latent. But any path that does not go
			// through that handler (a migration, an admin insert, a seed script)
			// can introduce one, and the integration test selects
			// COALESCE(website, ''), so the suite would stay green through it.
			//
			// COALESCE in the queries would work too, but it has to be repeated
			// in every SELECT forever and is one omission away from the same
			// outage. Making the invariant real in the schema is the version the
			// code already believes.
			//
			// Deliberately NOT included:
			//   google_id     - the partial unique index covers google_id IS NOT
			//                   NULL, and rows created through the API leave it
			//                   NULL; '' would collide on the second such row.
			//   owner_id      - a FK where NULL means genuinely unowned.
			//   delete_reason - NULL means "not deleted", which '' cannot say.
			//   approval      - tri-state; NULL is "not yet decided" and the
			//                   readers filter on it rather than scanning it.
			//   timestamps    - already scanned through sql.NullTime.
			textColumns := []string{
				"admin_email", "admin_phone", "city", "contact_firstname",
				"contact_lastname", "contact_phone", "description", "email",
				"image_url", "maps_page", "messaging_service", "name", "phone",
				"pos_system", "reference", "sole_proprietorship", "state",
				"street", "table_coverage", "tablet_model", "tipping_division",
				"tipping_policy", "type", "website", "zip",
			}
			numberColumns := []string{"lat", "lng", "rating", "service_stations"}

			for _, column := range textColumns {
				// Backfilled first so SET NOT NULL cannot fail on existing rows.
				// The development clone has none, but production is not this
				// clone and a migration that aborts halfway is worse than one
				// that does redundant work.
				if _, err := pools.App.Exec(ctx, fmt.Sprintf(
					`UPDATE locations SET %s = '' WHERE %s IS NULL;`, column, column),
				); err != nil {
					return fmt.Errorf("error backfilling locations.%s: %w", column, err)
				}
				if _, err := pools.App.Exec(ctx, fmt.Sprintf(
					`ALTER TABLE locations ALTER COLUMN %s SET DEFAULT '', ALTER COLUMN %s SET NOT NULL;`,
					column, column),
				); err != nil {
					return fmt.Errorf("error constraining locations.%s: %w", column, err)
				}
			}

			for _, column := range numberColumns {
				if _, err := pools.App.Exec(ctx, fmt.Sprintf(
					`UPDATE locations SET %s = 0 WHERE %s IS NULL;`, column, column),
				); err != nil {
					return fmt.Errorf("error backfilling locations.%s: %w", column, err)
				}
				if _, err := pools.App.Exec(ctx, fmt.Sprintf(
					`ALTER TABLE locations ALTER COLUMN %s SET DEFAULT 0, ALTER COLUMN %s SET NOT NULL;`,
					column, column),
				); err != nil {
					return fmt.Errorf("error constraining locations.%s: %w", column, err)
				}
			}

			appLogger.Logf("migration 1.48: constrained %d text and %d number columns on locations",
				len(textColumns), len(numberColumns))
			return nil
		},
	},
}

// migrateW9WarningTiers replaces one hard gate with an escalating sequence.
//
// Before this, a volunteer's first indication that a tax form existed was their
// reward not arriving. Now they get a polite notice partway to the limit, a
// firmer one closer to it, and only then does anything stop — so by the time
// money is withheld they have been warned twice while still being paid.
//
// The second change is quieter but removes more code than it adds. A person who
// already has money held is now refused rather than held again, which bounds
// escrow to exactly one payment. Nothing accumulates, so nothing has to expire,
// so there is no owed-but-unreserved money and no admin queue to chase it. The
// 'expired' and 'back_pay_requested' states go with it.
func migrateW9WarningTiers(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
	if _, err := pools.App.Exec(ctx, db.TaxSchemaDDL); err != nil {
		return fmt.Errorf("error creating the w9 tier tables: %w", err)
	}

	// Anything still sitting in a retired state has to land somewhere before the
	// constraint stops describing it. Both meant "owed but not reserved", and
	// the honest resting place is back in escrow — reserved again, and released
	// the moment the filing clears.
	tag, err := pools.App.Exec(ctx, `
		UPDATE payout_ledger
		SET state = 'escrowed',
			escrowed_at = COALESCE(escrowed_at, NOW()),
			expired_at = NULL,
			back_pay_requested_at = NULL,
			updated_at = NOW()
		WHERE state IN ('expired', 'back_pay_requested');
	`)
	if err != nil {
		return fmt.Errorf("error returning retired payout states to escrow: %w", err)
	}
	if appLogger != nil && tag.RowsAffected() > 0 {
		appLogger.Logf(
			"w9 tiers: returned %d payouts from expired/back-pay to escrow; they release when the filing clears",
			tag.RowsAffected(),
		)
	}

	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE payout_ledger DROP CONSTRAINT IF EXISTS payout_ledger_state_check;
		ALTER TABLE payout_ledger ADD CONSTRAINT payout_ledger_state_check CHECK (state IN (
			'pending','escrowed','releasing','paid','failed','cancelled'
		));
	`); err != nil {
		return fmt.Errorf("error narrowing the payout state constraint: %w", err)
	}

	return nil
}

// migrateW9Rebuild replaces a W9 system that never held a W9.
//
// The old one recorded a wallet, an email and a year — no name, no TIN, no
// signature, no document — and pointed people at a form on another website. It
// gated exactly one payout path, and it measured the annual threshold per
// wallet per chain, so a person with several wallets, or one wallet spanning
// the Celo cutover, could earn well past the limit without anything noticing.
//
// What replaces it keys on the person, records every platform-originated payout
// in one ledger, and holds money that cannot lawfully be paid yet instead of
// refusing it.
//
// This migration is additive. Nothing is dropped: w9_submissions is renamed
// rather than deleted so the backfill can be re-run, and w9_wallet_earnings is
// left alone because backend/mcp/reports.go still reads it. A later migration
// removes both, once the reports are repointed.
func migrateW9Rebuild(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
	if _, err := pools.App.Exec(ctx, db.TaxSchemaDDL); err != nil {
		return fmt.Errorf("error creating w9 rebuild tables: %w", err)
	}

	if err := migrateW9LegacyApprovals(ctx, pools, appLogger); err != nil {
		return err
	}

	// Renamed, not dropped. A rename is reversible and keeps the source data
	// available if the backfill above has to be re-run; a DROP is neither.
	if _, err := pools.App.Exec(ctx, `
		ALTER TABLE IF EXISTS w9_submissions RENAME TO w9_submissions_legacy_v1;
	`); err != nil {
		return fmt.Errorf("error retiring the legacy w9 submissions table: %w", err)
	}

	return nil
}

// migrateW9LegacyApprovals carries the old system's approvals forward.
//
// Those people completed what was asked of them at the time. Making them repeat
// it because we changed our storage is the fastest way to lose them, so they
// arrive as legacy_approved: unblocked, and never prompted again for that year.
//
// Approvals were keyed by wallet, and one person may hold several, so several
// old rows can collapse into one filing. Rows whose wallet matches no account
// are counted and logged rather than dropped silently — that number is the
// measure of how much of the old data was unattributable.
func migrateW9LegacyApprovals(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
	var exists bool
	if err := pools.App.QueryRow(ctx, `
		SELECT to_regclass('public.w9_submissions') IS NOT NULL;
	`).Scan(&exists); err != nil {
		return fmt.Errorf("error checking for the legacy w9 submissions table: %w", err)
	}
	if !exists {
		return nil
	}

	tag, err := pools.App.Exec(ctx, `
		INSERT INTO w9_filings (user_id, tax_year, status, completed_at, created_at, updated_at)
		SELECT
			owner.user_id,
			s.year,
			'legacy_approved',
			MIN(s.approved_at),
			NOW(),
			NOW()
		FROM w9_submissions s
		JOIN LATERAL (
			SELECT w.owner AS user_id
			FROM wallets w
			WHERE w.active = TRUE
			AND (
				LOWER(TRIM(COALESCE(w.smart_address, ''))) = LOWER(TRIM(s.wallet_address))
				OR LOWER(TRIM(COALESCE(w.eoa_address, ''))) = LOWER(TRIM(s.wallet_address))
			)
			ORDER BY w.id ASC
			LIMIT 1
		) owner ON TRUE
		WHERE s.approved_at IS NOT NULL
		AND s.rejected_at IS NULL
		GROUP BY owner.user_id, s.year
		ON CONFLICT (user_id, tax_year) DO NOTHING;
	`)
	if err != nil {
		return fmt.Errorf("error carrying legacy w9 approvals forward: %w", err)
	}

	var unresolved int
	if err := pools.App.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM w9_submissions s
		WHERE s.approved_at IS NOT NULL
		AND s.rejected_at IS NULL
		AND NOT EXISTS (
			SELECT 1 FROM wallets w
			WHERE w.active = TRUE
			AND (
				LOWER(TRIM(COALESCE(w.smart_address, ''))) = LOWER(TRIM(s.wallet_address))
				OR LOWER(TRIM(COALESCE(w.eoa_address, ''))) = LOWER(TRIM(s.wallet_address))
			)
		);
	`).Scan(&unresolved); err != nil {
		return fmt.Errorf("error counting unresolved legacy w9 approvals: %w", err)
	}

	if appLogger != nil {
		appLogger.Logf("w9 migration: carried %d approved submissions forward as legacy filings", tag.RowsAffected())
		if unresolved > 0 {
			appLogger.Logf(
				"w9 migration: %d approved submissions could not be matched to an account and were not carried forward; "+
					"those wallets will be asked to file if they earn past the threshold",
				unresolved,
			)
		}
	}

	return nil
}

// migrateLocationHoursUniqueness enforces at most one hours row per weekday per
// location. Before this, an hours "update" that matched only on location_id
// could leave a location with several rows claiming the same weekday.
//
// Uniqueness on locations.google_id is not handled here: CreateTables already
// maintains locations_google_id_active_idx for that.
//
// The index is created only when the existing rows already satisfy it. A
// duplicate is real data, and silently deleting merchant rows during a
// migration is worse than shipping without the constraint —
// db.replaceLocationHours enforces the same rule for every new write either
// way. When duplicates are present the migration logs what to clean up and
// moves on.
func migrateLocationHoursUniqueness(ctx context.Context, pools *MigrationPools, appLogger *logger.LogCloser) error {
	var duplicateWeekdays int
	if err := pools.App.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT location_id, weekday
			FROM location_hours
			GROUP BY location_id, weekday
			HAVING COUNT(*) > 1
		) duplicates;
	`).Scan(&duplicateWeekdays); err != nil {
		return fmt.Errorf("error checking for duplicate location hours: %w", err)
	}

	if duplicateWeekdays > 0 {
		appLogger.Logf(
			"skipping location_hours_location_weekday_key: %d (location, weekday) pairs have more than one row; "+
				"resolve the duplicates and re-run this migration to add the constraint",
			duplicateWeekdays,
		)
		return nil
	}

	if _, err := pools.App.Exec(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS location_hours_location_weekday_key
			ON location_hours(location_id, weekday);
	`); err != nil {
		return fmt.Errorf("error creating unique location hours index: %w", err)
	}

	return nil
}

type versionTarget struct {
	name          string
	pool          *pgxpool.Pool
	requiredTable string
}

func latestDBVersion() string {
	if len(schemaMigrations) == 0 {
		return baselineDBVersion
	}
	return schemaMigrations[len(schemaMigrations)-1].Version
}

func RunPendingMigrations(ctx context.Context, pools *DBPools, appLogger *logger.LogCloser) error {
	if pools == nil || pools.App == nil || pools.Bot == nil {
		return fmt.Errorf("app and bot db pools are required")
	}

	if err := validateMigrationSequence(); err != nil {
		return err
	}

	targets := []versionTarget{
		{name: "app", pool: pools.App, requiredTable: "users"},
		{name: "bot", pool: pools.Bot, requiredTable: "events"},
	}

	currentVersion, err := ensureConsistentDBVersion(ctx, targets, appLogger)
	if err != nil {
		return err
	}

	if err := ensureVersionIsKnown(currentVersion); err != nil {
		return err
	}

	latestVersion := latestDBVersion()
	cmp, err := compareVersions(currentVersion, latestVersion)
	if err != nil {
		return err
	}
	if cmp > 0 {
		return fmt.Errorf("database version %s is newer than server schema version %s", currentVersion, latestVersion)
	}
	if cmp == 0 {
		if appLogger != nil {
			appLogger.Logf("database schema already at version %s", currentVersion)
		}
		return nil
	}

	for _, migration := range schemaMigrations {
		isAfterCurrent, err := isVersionGreater(migration.Version, currentVersion)
		if err != nil {
			return err
		}
		if !isAfterCurrent {
			continue
		}

		if appLogger != nil {
			appLogger.Logf("applying schema migration %s: %s", migration.Version, migration.Description)
		}

		// Each migration runs inside one transaction per database, and the
		// version bump is written INSIDE those same transactions: the schema
		// change and the recorded version commit atomically, so a failure at
		// any point rolls the whole migration back and a re-run starts clean.
		appTx, err := pools.App.Begin(ctx)
		if err != nil {
			return fmt.Errorf("error beginning app transaction for migration %s: %w", migration.Version, err)
		}
		botTx, err := pools.Bot.Begin(ctx)
		if err != nil {
			_ = appTx.Rollback(ctx)
			return fmt.Errorf("error beginning bot transaction for migration %s: %w", migration.Version, err)
		}

		applyErr := migration.Apply(ctx, &MigrationPools{App: appTx, Bot: botTx}, appLogger)
		if applyErr == nil {
			applyErr = setCurrentVersion(ctx, appTx, migration.Version)
		}
		if applyErr == nil {
			applyErr = setCurrentVersion(ctx, botTx, migration.Version)
		}
		if applyErr != nil {
			_ = botTx.Rollback(ctx)
			_ = appTx.Rollback(ctx)
			return fmt.Errorf("error applying schema migration %s (%s): %w", migration.Version, migration.Description, applyErr)
		}

		if err := appTx.Commit(ctx); err != nil {
			_ = botTx.Rollback(ctx)
			return fmt.Errorf("error committing app transaction for migration %s: %w", migration.Version, err)
		}
		if err := botTx.Commit(ctx); err != nil {
			// The app db committed but the bot db did not: the version rows now
			// disagree, which ensureConsistentDBVersion reports loudly on the
			// next start instead of silently proceeding half-migrated.
			return fmt.Errorf("error committing bot transaction for migration %s (app db already committed; version mismatch will be reported on next start): %w", migration.Version, err)
		}
		currentVersion = migration.Version
	}

	if appLogger != nil {
		appLogger.Logf("database schema updated to version %s", currentVersion)
	}

	return nil
}

func validateMigrationSequence() error {
	previousVersion := baselineDBVersion
	seen := map[string]struct{}{}

	for _, migration := range schemaMigrations {
		if migration.Version == "" {
			return fmt.Errorf("schema migration is missing a version")
		}
		if migration.Description == "" {
			return fmt.Errorf("schema migration %s is missing a description", migration.Version)
		}
		if migration.Apply == nil {
			return fmt.Errorf("schema migration %s is missing an apply function", migration.Version)
		}
		if _, exists := seen[migration.Version]; exists {
			return fmt.Errorf("duplicate schema migration version %s", migration.Version)
		}
		seen[migration.Version] = struct{}{}

		cmp, err := compareVersions(migration.Version, previousVersion)
		if err != nil {
			return err
		}
		if cmp <= 0 {
			return fmt.Errorf("schema migration versions must be strictly increasing: %s came after %s", migration.Version, previousVersion)
		}
		previousVersion = migration.Version
	}

	return nil
}

func ensureVersionIsKnown(version string) error {
	if version == baselineDBVersion {
		return nil
	}
	for _, migration := range schemaMigrations {
		if migration.Version == version {
			return nil
		}
	}
	return fmt.Errorf("unsupported database version %s; run backend/cmd/init or add the missing migration batch", version)
}

func ensureConsistentDBVersion(ctx context.Context, targets []versionTarget, appLogger *logger.LogCloser) (string, error) {
	var currentVersion string

	for _, target := range targets {
		version, err := ensureTargetVersion(ctx, target)
		if err != nil {
			return "", err
		}
		if currentVersion == "" {
			currentVersion = version
			continue
		}
		if version != currentVersion {
			return "", fmt.Errorf("database version mismatch: %s=%s, expected %s", target.name, version, currentVersion)
		}
	}

	if currentVersion == "" {
		currentVersion = baselineDBVersion
	}

	if appLogger != nil {
		appLogger.Logf("database schema version is %s", currentVersion)
	}

	return currentVersion, nil
}

func ensureTargetVersion(ctx context.Context, target versionTarget) (string, error) {
	if err := ensureVersionTable(ctx, target.pool); err != nil {
		return "", fmt.Errorf("error ensuring db_version table in %s database: %w", target.name, err)
	}

	version, exists, err := getCurrentVersion(ctx, target.pool)
	if err != nil {
		return "", fmt.Errorf("error reading %s database version: %w", target.name, err)
	}
	if exists {
		return version, nil
	}

	hasBaselineSchema, err := tableExists(ctx, target.pool, target.requiredTable)
	if err != nil {
		return "", fmt.Errorf("error checking %s baseline schema table %s: %w", target.name, target.requiredTable, err)
	}
	if !hasBaselineSchema {
		return "", fmt.Errorf("%s database is missing baseline schema table %s and has no db_version row; run backend/cmd/init", target.name, target.requiredTable)
	}

	if err := setCurrentVersion(ctx, target.pool, baselineDBVersion); err != nil {
		return "", fmt.Errorf("error seeding %s database version to %s: %w", target.name, baselineDBVersion, err)
	}

	return baselineDBVersion, nil
}

func setVersionForTargets(ctx context.Context, targets []versionTarget, version string) error {
	for _, target := range targets {
		if err := setCurrentVersion(ctx, target.pool, version); err != nil {
			return fmt.Errorf("error updating %s database version: %w", target.name, err)
		}
	}
	return nil
}

func ensureVersionTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS db_version(
			id SMALLINT PRIMARY KEY CHECK (id = 1),
			version TEXT NOT NULL,
			updated_at BIGINT NOT NULL
		);
	`)
	return err
}

func getCurrentVersion(ctx context.Context, pool *pgxpool.Pool) (string, bool, error) {
	row := pool.QueryRow(ctx, `
		SELECT
			version
		FROM
			db_version
		WHERE
			id = 1;
	`)

	var version string
	if err := row.Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return version, true, nil
}

func setCurrentVersion(ctx context.Context, db MigrationDB, version string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO db_version(id, version, updated_at)
		VALUES (1, $1, $2)
		ON CONFLICT (id)
		DO UPDATE SET
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at;
	`, version, time.Now().Unix())
	return err
}

func tableExists(ctx context.Context, pool *pgxpool.Pool, tableName string) (bool, error) {
	row := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT
				1
			FROM
				information_schema.tables
			WHERE
				table_schema = 'public'
			AND
				table_name = $1
		);
	`, tableName)

	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func isVersionGreater(left, right string) (bool, error) {
	cmp, err := compareVersions(left, right)
	if err != nil {
		return false, err
	}
	return cmp > 0, nil
}

func compareVersions(left, right string) (int, error) {
	leftParts, err := parseVersion(left)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", left, err)
	}
	rightParts, err := parseVersion(right)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", right, err)
	}

	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}

	for index := 0; index < maxLen; index += 1 {
		leftValue := 0
		if index < len(leftParts) {
			leftValue = leftParts[index]
		}
		rightValue := 0
		if index < len(rightParts) {
			rightValue = rightParts[index]
		}
		if leftValue < rightValue {
			return -1, nil
		}
		if leftValue > rightValue {
			return 1, nil
		}
	}

	return 0, nil
}

func parseVersion(version string) ([]int, error) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return nil, fmt.Errorf("empty version")
	}

	parts := strings.Split(trimmed, ".")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("invalid empty version segment")
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid numeric segment %q", part)
		}
		if value < 0 {
			return nil, fmt.Errorf("negative version segment %q", part)
		}
		values = append(values, value)
	}
	return values, nil
}

// firstMinutes returns the opening (or closing) minute of a day's first stretch,
// or nil when it has none. Migration 1.34 predates split hours and only has the
// flat columns to write; 1.35 backfills the full set.
func firstMinutes(day structs.LocationDayHours, open bool) *int {
	if len(day.Intervals) == 0 {
		return nil
	}
	if open {
		return &day.Intervals[0].OpenMinute
	}
	return &day.Intervals[0].CloseMinute
}
