package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/logger"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AppDB struct {
	db     *pgxpool.Pool
	logger *logger.LogCloser
}

func App(db *pgxpool.Pool, logger *logger.LogCloser) *AppDB {
	return &AppDB{db, logger}
}

func (s *AppDB) CreateTables() error {
	_, err := s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS users(
				id TEXT PRIMARY KEY,
				is_admin BOOLEAN NOT NULL DEFAULT false,
				is_merchant BOOLEAN NOT NULL DEFAULT false,
				is_organizer BOOLEAN NOT NULL DEFAULT false,
				is_improver BOOLEAN NOT NULL DEFAULT false,
				is_proposer BOOLEAN NOT NULL DEFAULT false,
				is_voter BOOLEAN NOT NULL DEFAULT false,
				is_issuer BOOLEAN NOT NULL DEFAULT false,
				is_supervisor BOOLEAN NOT NULL DEFAULT false,
				is_affiliate BOOLEAN NOT NULL DEFAULT false,
				contact_email TEXT,
				contact_phone TEXT,
				contact_name TEXT,
				paypal_eth TEXT NOT NULL DEFAULT '',
			last_redemption INTEGER NOT NULL DEFAULT 0
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating users table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE users
		ADD COLUMN IF NOT EXISTS is_affiliate BOOLEAN NOT NULL DEFAULT false;
	`)
	if err != nil {
		return fmt.Errorf("error adding is_affiliate column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE users
		ADD COLUMN IF NOT EXISTS is_proposer BOOLEAN NOT NULL DEFAULT false;
	`)
	if err != nil {
		return fmt.Errorf("error adding is_proposer column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE users
		ADD COLUMN IF NOT EXISTS is_voter BOOLEAN NOT NULL DEFAULT false;
	`)
	if err != nil {
		return fmt.Errorf("error adding is_voter column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		UPDATE users
		SET is_voter = true
		WHERE is_admin = true
		AND is_voter = false;
	`)
	if err != nil {
		return fmt.Errorf("error defaulting admins to voters: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			ALTER TABLE users
			ADD COLUMN IF NOT EXISTS is_issuer BOOLEAN NOT NULL DEFAULT false;
		`)
	if err != nil {
		return fmt.Errorf("error adding is_issuer column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			ALTER TABLE users
			ADD COLUMN IF NOT EXISTS is_supervisor BOOLEAN NOT NULL DEFAULT false;
		`)
	if err != nil {
		return fmt.Errorf("error adding is_supervisor column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE users
		ADD COLUMN IF NOT EXISTS paypal_eth TEXT NOT NULL DEFAULT '';
	`)
	if err != nil {
		return fmt.Errorf("error adding paypal_eth column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE users
		ADD COLUMN IF NOT EXISTS last_redemption INTEGER NOT NULL DEFAULT 0;
	`)
	if err != nil {
		return fmt.Errorf("error adding last_redemption column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS user_verified_emails(
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			email TEXT NOT NULL,
			email_normalized TEXT NOT NULL,
			verified_at TIMESTAMP,
			verification_token TEXT,
			verification_sent_at TIMESTAMP,
			verification_token_expires_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			UNIQUE (user_id, email_normalized)
		);

		CREATE INDEX IF NOT EXISTS user_verified_emails_user_idx ON user_verified_emails(user_id);
		CREATE UNIQUE INDEX IF NOT EXISTS user_verified_emails_user_email_unique_idx
			ON user_verified_emails(user_id, email_normalized);
		CREATE UNIQUE INDEX IF NOT EXISTS user_verified_emails_token_unique_idx
			ON user_verified_emails(verification_token)
			WHERE verification_token IS NOT NULL;
	`)
	if err != nil {
		return fmt.Errorf("error creating user_verified_emails table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE user_verified_emails
		ADD COLUMN IF NOT EXISTS verification_sent_at TIMESTAMP;

		ALTER TABLE user_verified_emails
		ADD COLUMN IF NOT EXISTS verification_token_expires_at TIMESTAMP;

		ALTER TABLE user_verified_emails
		ADD COLUMN IF NOT EXISTS verified_at TIMESTAMP;

		ALTER TABLE user_verified_emails
		ADD COLUMN IF NOT EXISTS email_normalized TEXT;
	`)
	if err != nil {
		return fmt.Errorf("error altering user_verified_emails columns: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		UPDATE user_verified_emails
		SET
			email_normalized = LOWER(TRIM(email))
		WHERE
			COALESCE(NULLIF(TRIM(email_normalized), ''), '') = ''
		AND
			COALESCE(NULLIF(TRIM(email), ''), '') <> '';
	`)
	if err != nil {
		return fmt.Errorf("error normalizing user_verified_emails rows: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS wallets(
			id SERIAL PRIMARY KEY NOT NULL,
			owner TEXT NOT NULL REFERENCES users(id),
			name TEXT NOT NULL,
			is_eoa BOOLEAN NOT NULL,
			is_redeemer BOOLEAN NOT NULL DEFAULT false,
			is_minter BOOLEAN NOT NULL DEFAULT false,
			eoa_address TEXT NOT NULL,
			smart_address TEXT,
			smart_index INTEGER,
			UNIQUE (owner, is_eoa, eoa_address, smart_index)
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating wallets table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE wallets
		ADD COLUMN IF NOT EXISTS is_redeemer BOOLEAN NOT NULL DEFAULT false;
	`)
	if err != nil {
		return fmt.Errorf("error adding is_redeemer column to wallets table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE wallets
		ADD COLUMN IF NOT EXISTS is_minter BOOLEAN NOT NULL DEFAULT false;
	`)
	if err != nil {
		return fmt.Errorf("error adding is_minter column to wallets table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE wallets
		ADD COLUMN IF NOT EXISTS last_unwrap_at TIMESTAMP NULL;
	`)
	if err != nil {
		return fmt.Errorf("error adding last_unwrap_at column to wallets table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE INDEX IF NOT EXISTS wallets_smart_address_lower_idx
			ON wallets (LOWER(smart_address))
			WHERE smart_address IS NOT NULL;

		CREATE INDEX IF NOT EXISTS wallets_eoa_address_lower_idx
			ON wallets (LOWER(eoa_address));
	`)
	if err != nil {
		return fmt.Errorf("error creating wallet address lookup indexes: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS memos(
			tx_hash TEXT PRIMARY KEY,
			memo TEXT NOT NULL,
			owner TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS memos_tx_hash_idx
			ON memos (LOWER(tx_hash));
	`)
	if err != nil {
		return fmt.Errorf("error creating memos table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS affiliates(
			user_id TEXT PRIMARY KEY REFERENCES users(id),
			organization TEXT NOT NULL,
			nickname TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			affiliate_logo TEXT,
			weekly_allocation BIGINT NOT NULL DEFAULT 0,
			weekly_balance BIGINT NOT NULL DEFAULT 0,
			one_time_balance BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS affiliates_status_idx ON affiliates(status);
	`)
	if err != nil {
		return fmt.Errorf("error creating affiliates table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS proposers(
			user_id TEXT PRIMARY KEY REFERENCES users(id),
			organization TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			nickname TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			weekly_allocation BIGINT NOT NULL DEFAULT 0,
			weekly_balance BIGINT NOT NULL DEFAULT 0,
			one_time_balance BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS proposers_status_idx ON proposers(status);
	`)
	if err != nil {
		return fmt.Errorf("error creating proposers table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE proposers
		ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT '';

		UPDATE proposers p
		SET email = COALESCE(NULLIF(TRIM(u.contact_email), ''), p.email)
		FROM users u
		WHERE
			u.id = p.user_id
		AND
			TRIM(COALESCE(p.email, '')) = '';
	`)
	if err != nil {
		return fmt.Errorf("error altering proposers email column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS improvers(
				user_id TEXT PRIMARY KEY REFERENCES users(id),
				first_name TEXT NOT NULL,
				last_name TEXT NOT NULL,
				email TEXT NOT NULL,
				primary_rewards_account TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL DEFAULT 'pending',
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW()
			);

		CREATE INDEX IF NOT EXISTS improvers_status_idx ON improvers(status);
	`)
	if err != nil {
		return fmt.Errorf("error creating improvers table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE improvers
		ADD COLUMN IF NOT EXISTS primary_rewards_account TEXT NOT NULL DEFAULT '';

			UPDATE improvers i
			SET primary_rewards_account = COALESCE(
				(
					SELECT
						NULLIF(TRIM(w.smart_address), '')
					FROM
						wallets w
					WHERE
						w.owner = i.user_id
				AND
					w.is_eoa = false
				AND
					w.smart_index = 0
				ORDER BY
					w.id ASC
				LIMIT 1
			),
			TRIM(COALESCE(i.primary_rewards_account, ''))
		)
		WHERE
			TRIM(COALESCE(i.primary_rewards_account, '')) = '';
	`)
	if err != nil {
		return fmt.Errorf("error altering improvers primary rewards account column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS supervisors(
				user_id TEXT PRIMARY KEY REFERENCES users(id),
				organization TEXT NOT NULL,
				email TEXT NOT NULL DEFAULT '',
				primary_rewards_account TEXT NOT NULL DEFAULT '',
				nickname TEXT,
				status TEXT NOT NULL DEFAULT 'pending',
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW()
			);

			CREATE INDEX IF NOT EXISTS supervisors_status_idx ON supervisors(status);
		`)
	if err != nil {
		return fmt.Errorf("error creating supervisors table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			ALTER TABLE supervisors
			ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT '';

			ALTER TABLE supervisors
			ADD COLUMN IF NOT EXISTS primary_rewards_account TEXT NOT NULL DEFAULT '';

			UPDATE supervisors s
			SET email = COALESCE(NULLIF(TRIM(u.contact_email), ''), s.email)
			FROM users u
			WHERE
				u.id = s.user_id
			AND
				TRIM(COALESCE(s.email, '')) = '';

				UPDATE supervisors s
				SET primary_rewards_account = COALESCE(
					(
						SELECT
							NULLIF(TRIM(w.smart_address), '')
						FROM
							wallets w
						WHERE
							w.owner = s.user_id
					AND
						w.is_eoa = false
					AND
						w.smart_index = 0
					ORDER BY
						w.id ASC
					LIMIT 1
				),
				TRIM(COALESCE(s.primary_rewards_account, ''))
			)
			WHERE
				TRIM(COALESCE(s.primary_rewards_account, '')) = '';
		`)
	if err != nil {
		return fmt.Errorf("error altering supervisors email column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS issuer_credential_scopes(
			issuer_id TEXT NOT NULL REFERENCES users(id),
			credential_type TEXT NOT NULL,
			created_by TEXT REFERENCES users(id),
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (issuer_id, credential_type),
			CHECK (credential_type IN ('dpw_certified', 'sfluv_verifier'))
		);

		CREATE TABLE IF NOT EXISTS user_credentials(
			id SERIAL PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id),
			credential_type TEXT NOT NULL,
			issued_by TEXT REFERENCES users(id),
			issued_at TIMESTAMP NOT NULL DEFAULT NOW(),
			is_revoked BOOLEAN NOT NULL DEFAULT false,
			revoked_at TIMESTAMP,
			CHECK (credential_type IN ('dpw_certified', 'sfluv_verifier'))
		);

		CREATE UNIQUE INDEX IF NOT EXISTS user_credentials_active_unique
			ON user_credentials(user_id, credential_type)
			WHERE is_revoked = false;

		CREATE INDEX IF NOT EXISTS user_credentials_user_idx ON user_credentials(user_id);
	`)
	if err != nil {
		return fmt.Errorf("error creating credential tables: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS credential_requests(
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			credential_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			requested_at TIMESTAMP NOT NULL DEFAULT NOW(),
			resolved_at TIMESTAMP,
			resolved_by TEXT REFERENCES users(id),
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			CHECK (status IN ('pending', 'approved', 'rejected'))
		);

		CREATE INDEX IF NOT EXISTS credential_requests_user_idx
			ON credential_requests(user_id, requested_at DESC);
		CREATE INDEX IF NOT EXISTS credential_requests_status_idx
			ON credential_requests(status, requested_at DESC);
		CREATE INDEX IF NOT EXISTS credential_requests_credential_idx
			ON credential_requests(credential_type, status, requested_at DESC);
		CREATE UNIQUE INDEX IF NOT EXISTS credential_requests_pending_unique
			ON credential_requests(user_id, credential_type)
			WHERE status = 'pending';
	`)
	if err != nil {
		return fmt.Errorf("error creating credential_requests table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE OR REPLACE FUNCTION unix_now()
			RETURNS BIGINT
			LANGUAGE SQL
			STABLE
			AS $$
				SELECT EXTRACT(EPOCH FROM NOW())::BIGINT;
			$$;

			CREATE TABLE IF NOT EXISTS workflows(
				id TEXT PRIMARY KEY,
				series_id TEXT NOT NULL,
				workflow_state_id TEXT,
				proposer_id TEXT NOT NULL REFERENCES users(id),
				start_at BIGINT NOT NULL,
				status TEXT NOT NULL DEFAULT 'pending',
				is_start_blocked BOOLEAN NOT NULL DEFAULT false,
				blocked_by_workflow_id TEXT,
			total_bounty BIGINT NOT NULL DEFAULT 0,
			weekly_bounty_requirement BIGINT NOT NULL DEFAULT 0,
			budget_weekly_deducted BIGINT NOT NULL DEFAULT 0,
			budget_one_time_deducted BIGINT NOT NULL DEFAULT 0,
				vote_quorum_reached_at BIGINT,
				vote_finalize_at BIGINT,
				vote_finalized_at BIGINT,
				vote_finalized_by_user_id TEXT REFERENCES users(id),
				vote_decision TEXT,
				manager_required BOOLEAN NOT NULL DEFAULT false,
				manager_role_id TEXT,
				manager_improver_id TEXT REFERENCES users(id),
				manager_bounty BIGINT NOT NULL DEFAULT 0,
				manager_paid_out_at BIGINT,
				manager_payout_error TEXT,
				manager_payout_last_try_at BIGINT,
				manager_payout_in_progress BOOLEAN NOT NULL DEFAULT false,
				manager_retry_requested_at BIGINT,
				manager_retry_requested_by TEXT REFERENCES users(id),
				approved_at BIGINT,
				approved_by_user_id TEXT,
				created_at BIGINT NOT NULL DEFAULT unix_now(),
				updated_at BIGINT NOT NULL DEFAULT unix_now(),
				CHECK (status IN ('pending', 'approved', 'rejected', 'in_progress', 'completed', 'paid_out', 'blocked', 'expired', 'failed', 'skipped', 'deleted')),
				CHECK (vote_decision IN ('approve', 'deny', 'admin_approve') OR vote_decision IS NULL)
			);

		CREATE INDEX IF NOT EXISTS workflows_series_idx ON workflows(series_id);
		CREATE INDEX IF NOT EXISTS workflows_status_idx ON workflows(status);
		CREATE INDEX IF NOT EXISTS workflows_proposer_idx ON workflows(proposer_id);
		CREATE INDEX IF NOT EXISTS workflows_start_idx ON workflows(start_at);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflows table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS workflow_series(
			id TEXT PRIMARY KEY,
			proposer_id TEXT NOT NULL REFERENCES users(id),
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			recurrence TEXT NOT NULL,
			recurrence_end_at BIGINT,
			current_state_id TEXT,
			supervisor_data_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			CHECK (recurrence IN ('one_time', 'daily', 'weekly', 'monthly'))
		);

		CREATE INDEX IF NOT EXISTS workflow_series_proposer_idx ON workflow_series(proposer_id);
		CREATE INDEX IF NOT EXISTS workflow_series_recurrence_idx ON workflow_series(recurrence);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_series table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			WITH missing_series AS (
				SELECT DISTINCT
					w.series_id,
					w.proposer_id,
					w.created_at,
					w.updated_at
				FROM
					workflows w
				LEFT JOIN
					workflow_series s
				ON
					s.id = w.series_id
				WHERE
					s.id IS NULL
			)
			INSERT INTO workflow_series(
				id,
				proposer_id,
				title,
				description,
				recurrence,
				created_at,
				updated_at
			)
			SELECT
				m.series_id,
				m.proposer_id,
				CONCAT('Workflow ', UPPER(SUBSTRING(m.series_id FROM 1 FOR 8))),
				'',
				'one_time',
				m.created_at,
				m.updated_at
			FROM
				missing_series m
			ON CONFLICT (id)
			DO NOTHING;
		`)
	if err != nil {
		return fmt.Errorf("error backfilling workflow_series table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS workflow_states(
			id TEXT PRIMARY KEY,
			series_id TEXT NOT NULL REFERENCES workflow_series(id) ON DELETE CASCADE,
			proposer_id TEXT NOT NULL REFERENCES users(id),
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			recurrence TEXT NOT NULL,
			recurrence_end_at BIGINT,
			supervisor_user_id TEXT REFERENCES users(id),
			supervisor_bounty BIGINT NOT NULL DEFAULT 0,
			supervisor_data_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			roles_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			steps_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			source_workflow_id TEXT,
			proposed_by_user_id TEXT REFERENCES users(id),
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			CHECK (recurrence IN ('one_time', 'daily', 'weekly', 'monthly'))
		);

		CREATE INDEX IF NOT EXISTS workflow_states_series_idx ON workflow_states(series_id);
		CREATE INDEX IF NOT EXISTS workflow_states_proposer_idx ON workflow_states(proposer_id);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_states table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE workflows
		ADD COLUMN IF NOT EXISTS workflow_state_id TEXT;

		ALTER TABLE workflow_series
		ADD COLUMN IF NOT EXISTS recurrence_end_at BIGINT;

		ALTER TABLE workflow_series
		ADD COLUMN IF NOT EXISTS current_state_id TEXT;
	`)
	if err != nil {
		return fmt.Errorf("error altering workflow state columns: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS workflow_roles(
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			title TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS workflow_roles_workflow_idx ON workflow_roles(workflow_id);

		ALTER TABLE workflow_roles
		ADD COLUMN IF NOT EXISTS is_manager BOOLEAN NOT NULL DEFAULT false;

		CREATE TABLE IF NOT EXISTS workflow_role_credentials(
			role_id TEXT NOT NULL REFERENCES workflow_roles(id) ON DELETE CASCADE,
			credential_type TEXT NOT NULL,
			PRIMARY KEY (role_id, credential_type),
			CHECK (credential_type IN ('dpw_certified', 'sfluv_verifier'))
		);

		CREATE TABLE IF NOT EXISTS workflow_steps(
			id TEXT PRIMARY KEY,
			series_id TEXT NOT NULL REFERENCES workflow_series(id) ON DELETE CASCADE,
			workflow_id TEXT NOT NULL,
			step_order INTEGER NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			bounty BIGINT NOT NULL DEFAULT 0,
			role_id TEXT REFERENCES workflow_roles(id),
			assigned_improver_id TEXT REFERENCES users(id),
			allow_step_not_possible BOOLEAN NOT NULL DEFAULT false,
			status TEXT NOT NULL DEFAULT 'locked',
			started_at BIGINT,
			completed_at BIGINT,
			payout_error TEXT,
			payout_last_try_at BIGINT,
			payout_in_progress BOOLEAN NOT NULL DEFAULT false,
			retry_requested_at BIGINT,
			retry_requested_by TEXT REFERENCES users(id),
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			UNIQUE (workflow_id, step_order),
			CHECK (status IN ('locked', 'available', 'in_progress', 'completed', 'paid_out'))
		);
		CREATE INDEX IF NOT EXISTS workflow_steps_workflow_idx ON workflow_steps(workflow_id);

		CREATE TABLE IF NOT EXISTS workflow_step_items(
			id TEXT PRIMARY KEY,
			step_id TEXT NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
			item_order INTEGER NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			is_optional BOOLEAN NOT NULL DEFAULT false,
			requires_photo BOOLEAN NOT NULL DEFAULT false,
			camera_capture_only BOOLEAN NOT NULL DEFAULT false,
			photo_required_count INTEGER NOT NULL DEFAULT 1,
			photo_allow_any_count BOOLEAN NOT NULL DEFAULT false,
			photo_aspect_ratio TEXT NOT NULL DEFAULT 'square',
			requires_written_response BOOLEAN NOT NULL DEFAULT false,
			requires_dropdown BOOLEAN NOT NULL DEFAULT false,
			dropdown_options JSONB NOT NULL DEFAULT '[]'::jsonb,
			dropdown_requires_written_response JSONB NOT NULL DEFAULT '{}'::jsonb,
			notify_emails JSONB NOT NULL DEFAULT '[]'::jsonb,
			notify_on_dropdown_values JSONB NOT NULL DEFAULT '[]'::jsonb,
			UNIQUE (step_id, item_order),
			CHECK (photo_required_count >= 1),
			CHECK (photo_aspect_ratio IN ('vertical', 'square', 'horizontal'))
		);
		CREATE INDEX IF NOT EXISTS workflow_step_items_step_idx ON workflow_step_items(step_id);
	`)
	if err != nil {
		return fmt.Errorf("error ensuring workflow definition tables before state backfill: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		WITH role_payload AS (
			SELECT
				r.workflow_id,
				COALESCE(
					JSONB_AGG(
						JSONB_BUILD_OBJECT(
							'client_id', r.id,
							'title', COALESCE(r.title, ''),
							'required_credentials',
							COALESCE(
								(
									SELECT
										JSONB_AGG(rc.credential_type ORDER BY rc.credential_type)
									FROM
										workflow_role_credentials rc
									WHERE
										rc.role_id = r.id
								),
								'[]'::jsonb
							)
						)
						ORDER BY
							r.id
					),
					'[]'::jsonb
				) AS roles_json
			FROM
				workflow_roles r
			WHERE
				COALESCE(r.is_manager, false) = false
			GROUP BY
				r.workflow_id
		),
		step_payload AS (
			SELECT
				ws.workflow_id,
				COALESCE(
					JSONB_AGG(
						JSONB_BUILD_OBJECT(
							'title', COALESCE(ws.title, ''),
							'description', COALESCE(ws.description, ''),
							'bounty', COALESCE(ws.bounty, 0),
							'role_client_id', COALESCE(ws.role_id, ''),
							'allow_step_not_possible', COALESCE(ws.allow_step_not_possible, false),
							'work_items',
							COALESCE(
								(
									SELECT
										JSONB_AGG(
											JSONB_BUILD_OBJECT(
												'title', COALESCE(wi.title, ''),
												'description', COALESCE(wi.description, ''),
												'optional', COALESCE(wi.is_optional, false),
												'requires_photo', COALESCE(wi.requires_photo, false),
												'camera_capture_only', COALESCE(wi.camera_capture_only, false),
												'photo_required_count', GREATEST(1, COALESCE(wi.photo_required_count, 1)),
												'photo_allow_any_count', COALESCE(wi.photo_allow_any_count, false),
												'photo_aspect_ratio', COALESCE(NULLIF(BTRIM(wi.photo_aspect_ratio), ''), 'square'),
												'requires_written_response', COALESCE(wi.requires_written_response, false),
												'requires_dropdown', COALESCE(wi.requires_dropdown, false),
												'dropdown_options',
												COALESCE(
													(
														SELECT
															JSONB_AGG(
																JSONB_BUILD_OBJECT(
																	'label',
																	COALESCE(
																		NULLIF(BTRIM(option_value->>'label'), ''),
																		NULLIF(BTRIM(option_value->>'value'), ''),
																		CONCAT('Option ', option_index::text)
																	),
																	'requires_written_response',
																	(COALESCE(LOWER(option_value->>'requires_written_response'), 'false') = 'true'),
																	'notify_emails',
																	CASE
																		WHEN JSONB_TYPEOF(option_value->'notify_emails') = 'array' THEN option_value->'notify_emails'
																		ELSE '[]'::jsonb
																	END
																)
																ORDER BY
																	option_index
															)
														FROM
															JSONB_ARRAY_ELEMENTS(COALESCE(wi.dropdown_options, '[]'::jsonb)) WITH ORDINALITY AS options(option_value, option_index)
													),
													'[]'::jsonb
												)
											)
											ORDER BY
												wi.item_order
										)
									FROM
										workflow_step_items wi
									WHERE
										wi.step_id = ws.id
								),
								'[]'::jsonb
							)
						)
						ORDER BY
							ws.step_order
					),
					'[]'::jsonb
				) AS steps_json
			FROM
				workflow_steps ws
			GROUP BY
				ws.workflow_id
		),
		payload AS (
			SELECT
				w.id AS workflow_id,
				w.series_id,
				w.proposer_id,
				COALESCE(NULLIF(BTRIM(s.title), ''), CONCAT('Workflow ', UPPER(SUBSTRING(w.series_id FROM 1 FOR 8)))) AS title,
				COALESCE(s.description, '') AS description,
				COALESCE(NULLIF(BTRIM(s.recurrence), ''), 'one_time') AS recurrence,
				s.recurrence_end_at,
				w.manager_improver_id AS supervisor_user_id,
				GREATEST(0, COALESCE(w.manager_bounty, 0)) AS supervisor_bounty,
				COALESCE(s.supervisor_data_json, '[]'::jsonb) AS supervisor_data_json,
				COALESCE(rp.roles_json, '[]'::jsonb) AS roles_json,
				COALESCE(sp.steps_json, '[]'::jsonb) AS steps_json
			FROM
				workflows w
			LEFT JOIN
				workflow_series s
			ON
				s.id = w.series_id
			LEFT JOIN
				role_payload rp
			ON
				rp.workflow_id = w.id
			LEFT JOIN
				step_payload sp
			ON
				sp.workflow_id = w.id
			WHERE
				w.workflow_state_id IS NULL
		)
		INSERT INTO workflow_states(
			id,
			series_id,
			proposer_id,
			title,
			description,
			recurrence,
			recurrence_end_at,
			supervisor_user_id,
			supervisor_bounty,
			supervisor_data_json,
			roles_json,
			steps_json,
			source_workflow_id,
			proposed_by_user_id
		)
		SELECT
			MD5(CONCAT('workflow-state:', p.workflow_id)),
			p.series_id,
			p.proposer_id,
			p.title,
			p.description,
			p.recurrence,
			p.recurrence_end_at,
			p.supervisor_user_id,
			p.supervisor_bounty,
			p.supervisor_data_json,
			p.roles_json,
			p.steps_json,
			p.workflow_id,
			p.proposer_id
		FROM
			payload p
		ON CONFLICT (id) DO NOTHING;

		UPDATE workflows w
		SET
			workflow_state_id = MD5(CONCAT('workflow-state:', w.id))
		WHERE
			w.workflow_state_id IS NULL;

		WITH ranked AS (
			SELECT
				w.series_id,
				w.workflow_state_id,
				ROW_NUMBER() OVER (
					PARTITION BY w.series_id
					ORDER BY
						CASE WHEN w.status <> 'deleted' THEN 0 ELSE 1 END,
						w.start_at DESC,
						w.created_at DESC,
						w.id DESC
				) AS row_rank
			FROM
				workflows w
			WHERE
				w.workflow_state_id IS NOT NULL
		)
		UPDATE workflow_series s
		SET
			current_state_id = r.workflow_state_id,
			updated_at = unix_now()
		FROM
			ranked r
		WHERE
			s.id = r.series_id
		AND
			r.row_rank = 1
		AND
			(s.current_state_id IS NULL OR BTRIM(s.current_state_id) = '');

		UPDATE workflow_series s
		SET
			title = st.title,
			description = st.description,
			recurrence = st.recurrence,
			recurrence_end_at = st.recurrence_end_at,
			supervisor_data_json = st.supervisor_data_json,
			updated_at = unix_now()
		FROM
			workflow_states st
		WHERE
			st.id = s.current_state_id
		AND
			st.series_id = s.id;
	`)
	if err != nil {
		return fmt.Errorf("error backfilling workflow state snapshots: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE INDEX IF NOT EXISTS workflow_series_current_state_idx
			ON workflow_series(current_state_id);
		CREATE INDEX IF NOT EXISTS workflows_state_idx
			ON workflows(workflow_state_id);

		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'workflow_series_current_state_fk'
			) THEN
				ALTER TABLE workflow_series
				ADD CONSTRAINT workflow_series_current_state_fk
				FOREIGN KEY (current_state_id)
				REFERENCES workflow_states(id)
				ON DELETE SET NULL;
			END IF;

			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'workflows_workflow_state_fk'
			) THEN
				ALTER TABLE workflows
				ADD CONSTRAINT workflows_workflow_state_fk
				FOREIGN KEY (workflow_state_id)
				REFERENCES workflow_states(id)
				ON DELETE SET NULL;
			END IF;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("error adding workflow state foreign keys: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			DO $$
			BEGIN
			IF NOT EXISTS (
				SELECT
					1
				FROM
					pg_constraint
				WHERE
					conname = 'workflows_series_fk'
			) THEN
				ALTER TABLE workflows
				ADD CONSTRAINT workflows_series_fk
				FOREIGN KEY (series_id) REFERENCES workflow_series(id)
				ON DELETE CASCADE;
			END IF;
			END $$;
		`)
	if err != nil {
		return fmt.Errorf("error adding workflow series foreign key: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			ALTER TABLE workflows
			DROP COLUMN IF EXISTS title;

			ALTER TABLE workflows
			DROP COLUMN IF EXISTS description;

			ALTER TABLE workflows
			DROP COLUMN IF EXISTS recurrence;
		`)
	if err != nil {
		return fmt.Errorf("error dropping legacy workflow definition columns: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			ALTER TABLE workflows
			DROP CONSTRAINT IF EXISTS workflows_status_check;

		ALTER TABLE workflows
		ADD CONSTRAINT workflows_status_check
		CHECK (status IN ('pending', 'approved', 'rejected', 'in_progress', 'completed', 'paid_out', 'blocked', 'expired', 'failed', 'skipped', 'deleted'));
	`)
	if err != nil {
		return fmt.Errorf("error updating workflows status constraint: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		UPDATE
			workflows
		SET
			status = 'skipped',
			updated_at = unix_now()
		WHERE
			status = 'failed';
	`)
	if err != nil {
		return fmt.Errorf("error migrating failed workflow statuses to skipped: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS workflow_templates(
				id TEXT PRIMARY KEY,
				template_title TEXT NOT NULL,
				template_description TEXT NOT NULL,
				owner_user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
				created_by_user_id TEXT NOT NULL REFERENCES users(id),
				is_default BOOLEAN NOT NULL DEFAULT false,
					recurrence TEXT NOT NULL,
					start_at BIGINT NOT NULL,
					series_id TEXT,
					supervisor_user_id TEXT REFERENCES users(id),
					supervisor_bounty BIGINT,
					supervisor_data_json JSONB NOT NULL DEFAULT '[]'::jsonb,
					manager_json JSONB,
					roles_json JSONB NOT NULL DEFAULT '[]'::jsonb,
					steps_json JSONB NOT NULL DEFAULT '[]'::jsonb,
				created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			CHECK (recurrence IN ('one_time', 'daily', 'weekly', 'monthly'))
		);

		CREATE INDEX IF NOT EXISTS workflow_templates_owner_idx ON workflow_templates(owner_user_id);
		CREATE INDEX IF NOT EXISTS workflow_templates_default_idx ON workflow_templates(is_default);
		CREATE INDEX IF NOT EXISTS workflow_templates_created_by_idx ON workflow_templates(created_by_user_id);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_templates table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
				ALTER TABLE workflow_series
				ADD COLUMN IF NOT EXISTS supervisor_data_json JSONB NOT NULL DEFAULT '[]'::jsonb;

				ALTER TABLE workflow_templates
				ADD COLUMN IF NOT EXISTS supervisor_user_id TEXT REFERENCES users(id);

				ALTER TABLE workflow_templates
				ADD COLUMN IF NOT EXISTS supervisor_bounty BIGINT;

				ALTER TABLE workflow_templates
				ADD COLUMN IF NOT EXISTS supervisor_data_json JSONB NOT NULL DEFAULT '[]'::jsonb;

				ALTER TABLE workflow_templates
				ADD COLUMN IF NOT EXISTS manager_json JSONB;
			`)
	if err != nil {
		return fmt.Errorf("error altering workflow_templates columns: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE workflows
		ADD COLUMN IF NOT EXISTS vote_quorum_reached_at BIGINT;

		ALTER TABLE workflows
		ADD COLUMN IF NOT EXISTS vote_finalize_at BIGINT;

		ALTER TABLE workflows
		ADD COLUMN IF NOT EXISTS vote_finalized_at BIGINT;

		ALTER TABLE workflows
		ADD COLUMN IF NOT EXISTS vote_finalized_by_user_id TEXT REFERENCES users(id);

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS vote_decision TEXT;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_required BOOLEAN NOT NULL DEFAULT false;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_role_id TEXT;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_improver_id TEXT REFERENCES users(id);

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_bounty BIGINT NOT NULL DEFAULT 0;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_paid_out_at BIGINT;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_payout_error TEXT;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_payout_last_try_at BIGINT;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_payout_in_progress BOOLEAN NOT NULL DEFAULT false;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_retry_requested_at BIGINT;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_retry_requested_by TEXT REFERENCES users(id);
		`)
	if err != nil {
		return fmt.Errorf("error altering workflows voting columns: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		UPDATE workflows
		SET
			status = 'approved',
			is_start_blocked = false,
			blocked_by_workflow_id = NULL,
			updated_at = unix_now()
		WHERE
			status = 'blocked';
	`)
	if err != nil {
		return fmt.Errorf("error unblocking legacy workflow rows: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS workflow_roles(
				id TEXT PRIMARY KEY,
				workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
				title TEXT NOT NULL
			);

			CREATE INDEX IF NOT EXISTS workflow_roles_workflow_idx ON workflow_roles(workflow_id);
		`)
	if err != nil {
		return fmt.Errorf("error creating workflow_roles table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			ALTER TABLE workflow_roles
			ADD COLUMN IF NOT EXISTS is_manager BOOLEAN NOT NULL DEFAULT false;
		`)
	if err != nil {
		return fmt.Errorf("error altering workflow_roles manager column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE UNIQUE INDEX IF NOT EXISTS workflow_roles_single_manager_idx
				ON workflow_roles(workflow_id)
				WHERE is_manager = true;
		`)
	if err != nil {
		return fmt.Errorf("error creating workflow_roles manager index: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			DO $$
			BEGIN
				IF NOT EXISTS (
					SELECT 1
					FROM pg_constraint
					WHERE conname = 'workflows_manager_role_fk'
				) THEN
					ALTER TABLE workflows
					ADD CONSTRAINT workflows_manager_role_fk
					FOREIGN KEY (manager_role_id) REFERENCES workflow_roles(id)
					DEFERRABLE INITIALLY IMMEDIATE;
				END IF;
			END $$;
		`)
	if err != nil {
		return fmt.Errorf("error adding workflow manager role foreign key: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS workflow_role_credentials(
			role_id TEXT NOT NULL REFERENCES workflow_roles(id) ON DELETE CASCADE,
			credential_type TEXT NOT NULL,
			PRIMARY KEY (role_id, credential_type),
			CHECK (credential_type IN ('dpw_certified', 'sfluv_verifier'))
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_role_credentials table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS workflow_steps(
				id TEXT PRIMARY KEY,
				series_id TEXT NOT NULL REFERENCES workflow_series(id) ON DELETE CASCADE,
				workflow_id TEXT NOT NULL,
				step_order INTEGER NOT NULL,
				title TEXT NOT NULL,
			description TEXT NOT NULL,
			bounty BIGINT NOT NULL DEFAULT 0,
			role_id TEXT REFERENCES workflow_roles(id),
			assigned_improver_id TEXT REFERENCES users(id),
			allow_step_not_possible BOOLEAN NOT NULL DEFAULT false,
			status TEXT NOT NULL DEFAULT 'locked',
			started_at BIGINT,
			completed_at BIGINT,
			payout_error TEXT,
			payout_last_try_at BIGINT,
			payout_in_progress BOOLEAN NOT NULL DEFAULT false,
			retry_requested_at BIGINT,
			retry_requested_by TEXT REFERENCES users(id),
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			UNIQUE (workflow_id, step_order),
			CHECK (status IN ('locked', 'available', 'in_progress', 'completed', 'paid_out'))
			);

			CREATE INDEX IF NOT EXISTS workflow_steps_workflow_idx ON workflow_steps(workflow_id);
			CREATE UNIQUE INDEX IF NOT EXISTS workflow_single_assignment_per_improver_idx
				ON workflow_steps(workflow_id, assigned_improver_id)
				WHERE assigned_improver_id IS NOT NULL;
		`)
	if err != nil {
		return fmt.Errorf("error creating workflow_steps table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			ALTER TABLE workflow_steps
			ADD COLUMN IF NOT EXISTS series_id TEXT;

			ALTER TABLE workflow_steps
			ADD COLUMN IF NOT EXISTS started_at BIGINT;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS completed_at BIGINT;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS payout_error TEXT;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS payout_last_try_at BIGINT;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS payout_in_progress BOOLEAN NOT NULL DEFAULT false;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS retry_requested_at BIGINT;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS retry_requested_by TEXT REFERENCES users(id);

			ALTER TABLE workflow_steps
			ADD COLUMN IF NOT EXISTS allow_step_not_possible BOOLEAN NOT NULL DEFAULT false;

			UPDATE workflow_steps ws
			SET
				series_id = w.series_id
			FROM
				workflows w
			WHERE
				ws.workflow_id = w.id
			AND
				(ws.series_id IS NULL OR ws.series_id <> w.series_id);

			DO $$
			BEGIN
				IF NOT EXISTS (
					SELECT
						1
					FROM
						pg_constraint
					WHERE
						conname = 'workflow_steps_series_fk'
				) THEN
					ALTER TABLE workflow_steps
					ADD CONSTRAINT workflow_steps_series_fk
					FOREIGN KEY (series_id) REFERENCES workflow_series(id)
					ON DELETE CASCADE;
				END IF;
			END $$;

			ALTER TABLE workflow_steps
			DROP CONSTRAINT IF EXISTS workflow_steps_workflow_id_fkey;

			ALTER TABLE workflow_steps
			ALTER COLUMN series_id SET NOT NULL;

			CREATE INDEX IF NOT EXISTS workflow_steps_series_idx ON workflow_steps(series_id);
			CREATE UNIQUE INDEX IF NOT EXISTS workflow_single_assignment_per_improver_idx
				ON workflow_steps(workflow_id, assigned_improver_id)
				WHERE assigned_improver_id IS NOT NULL;
		`)
	if err != nil {
		return fmt.Errorf("error altering workflow_steps execution columns: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS workflow_step_items(
				id TEXT PRIMARY KEY,
				step_id TEXT NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
				item_order INTEGER NOT NULL,
				title TEXT NOT NULL,
				description TEXT NOT NULL,
				is_optional BOOLEAN NOT NULL DEFAULT false,
				requires_photo BOOLEAN NOT NULL DEFAULT false,
				camera_capture_only BOOLEAN NOT NULL DEFAULT false,
				photo_required_count INTEGER NOT NULL DEFAULT 1,
				photo_allow_any_count BOOLEAN NOT NULL DEFAULT false,
				photo_aspect_ratio TEXT NOT NULL DEFAULT 'square',
				requires_written_response BOOLEAN NOT NULL DEFAULT false,
				requires_dropdown BOOLEAN NOT NULL DEFAULT false,
				dropdown_options JSONB NOT NULL DEFAULT '[]'::jsonb,
				dropdown_requires_written_response JSONB NOT NULL DEFAULT '{}'::jsonb,
				notify_emails JSONB NOT NULL DEFAULT '[]'::jsonb,
				notify_on_dropdown_values JSONB NOT NULL DEFAULT '[]'::jsonb,
				UNIQUE (step_id, item_order),
				CHECK (photo_required_count >= 1),
				CHECK (photo_aspect_ratio IN ('vertical', 'square', 'horizontal'))
			);

		CREATE INDEX IF NOT EXISTS workflow_step_items_step_idx ON workflow_step_items(step_id);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_step_items table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			ALTER TABLE workflow_step_items
			ADD COLUMN IF NOT EXISTS camera_capture_only BOOLEAN NOT NULL DEFAULT false;

			ALTER TABLE workflow_step_items
			ADD COLUMN IF NOT EXISTS photo_required_count INTEGER NOT NULL DEFAULT 1;

			ALTER TABLE workflow_step_items
			ADD COLUMN IF NOT EXISTS photo_allow_any_count BOOLEAN NOT NULL DEFAULT false;

			ALTER TABLE workflow_step_items
			ADD COLUMN IF NOT EXISTS photo_aspect_ratio TEXT NOT NULL DEFAULT 'square';

			ALTER TABLE workflow_step_items
			DROP CONSTRAINT IF EXISTS workflow_step_items_photo_required_count_check;

			ALTER TABLE workflow_step_items
			ADD CONSTRAINT workflow_step_items_photo_required_count_check CHECK (photo_required_count >= 1);

			ALTER TABLE workflow_step_items
			DROP CONSTRAINT IF EXISTS workflow_step_items_photo_aspect_ratio_check;

			ALTER TABLE workflow_step_items
			ADD CONSTRAINT workflow_step_items_photo_aspect_ratio_check CHECK (photo_aspect_ratio IN ('vertical', 'square', 'horizontal'));
		`)
	if err != nil {
		return fmt.Errorf("error altering workflow_step_items camera capture column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS workflow_step_submissions(
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			step_id TEXT NOT NULL UNIQUE REFERENCES workflow_steps(id) ON DELETE CASCADE,
			improver_id TEXT NOT NULL REFERENCES users(id),
			step_not_possible BOOLEAN NOT NULL DEFAULT false,
			step_not_possible_details TEXT,
			item_responses JSONB NOT NULL DEFAULT '[]'::jsonb,
			submitted_at BIGINT NOT NULL DEFAULT unix_now(),
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now()
		);

		CREATE INDEX IF NOT EXISTS workflow_step_submissions_workflow_idx ON workflow_step_submissions(workflow_id);
		CREATE INDEX IF NOT EXISTS workflow_step_submissions_improver_idx ON workflow_step_submissions(improver_id);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_step_submissions table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE workflow_step_submissions
		ADD COLUMN IF NOT EXISTS step_not_possible BOOLEAN NOT NULL DEFAULT false;

		ALTER TABLE workflow_step_submissions
		ADD COLUMN IF NOT EXISTS step_not_possible_details TEXT;
	`)
	if err != nil {
		return fmt.Errorf("error altering workflow_step_submissions columns: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS workflow_submission_photos(
				id TEXT PRIMARY KEY,
				workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
				step_id TEXT NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
				item_id TEXT NOT NULL REFERENCES workflow_step_items(id) ON DELETE CASCADE,
				submission_id TEXT NOT NULL REFERENCES workflow_step_submissions(id) ON DELETE CASCADE,
				file_name TEXT NOT NULL DEFAULT '',
				content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
				photo_data BYTEA NOT NULL,
				size_bytes INTEGER NOT NULL DEFAULT 0,
				created_at BIGINT NOT NULL DEFAULT unix_now(),
				updated_at BIGINT NOT NULL DEFAULT unix_now()
			);

			CREATE INDEX IF NOT EXISTS workflow_submission_photos_submission_idx
				ON workflow_submission_photos(submission_id);
			CREATE INDEX IF NOT EXISTS workflow_submission_photos_workflow_idx
				ON workflow_submission_photos(workflow_id);
			CREATE INDEX IF NOT EXISTS workflow_submission_photos_step_item_idx
				ON workflow_submission_photos(step_id, item_id);
		`)
	if err != nil {
		return fmt.Errorf("error creating workflow_submission_photos table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS workflow_step_notifications(
			step_id TEXT NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id),
			notification_type TEXT NOT NULL,
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			PRIMARY KEY (step_id, user_id, notification_type),
			CHECK (notification_type IN ('step_available'))
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_step_notifications table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS workflow_improver_absences(
				id TEXT PRIMARY KEY,
				improver_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			series_id TEXT NOT NULL,
			step_order INTEGER NOT NULL,
			absent_from BIGINT NOT NULL,
			absent_until BIGINT NOT NULL,
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			CHECK (step_order > 0),
			CHECK (absent_until > absent_from)
		);

		CREATE INDEX IF NOT EXISTS workflow_improver_absences_improver_idx
			ON workflow_improver_absences(improver_id, absent_from DESC);
		CREATE INDEX IF NOT EXISTS workflow_improver_absences_series_step_idx
			ON workflow_improver_absences(series_id, step_order, absent_from, absent_until);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_improver_absences table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS workflow_series_step_claims(
				series_id TEXT NOT NULL,
				step_order INTEGER NOT NULL,
				improver_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				created_at BIGINT NOT NULL DEFAULT unix_now(),
				updated_at BIGINT NOT NULL DEFAULT unix_now(),
				PRIMARY KEY (series_id, step_order),
				CHECK (step_order > 0)
			);

			CREATE INDEX IF NOT EXISTS workflow_series_step_claims_improver_idx
				ON workflow_series_step_claims(improver_id, series_id, step_order);
		`)
	if err != nil {
		return fmt.Errorf("error creating workflow_series_step_claims table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
				WITH ranked AS (
					SELECT
						w.series_id,
						ws.step_order,
						ws.assigned_improver_id,
					ROW_NUMBER() OVER (
						PARTITION BY w.series_id, ws.step_order
						ORDER BY
							w.start_at DESC,
							ws.updated_at DESC,
							ws.created_at DESC
					) AS row_rank
					FROM
						workflow_steps ws
					JOIN
						workflows w
					ON
						w.id = ws.workflow_id
					JOIN
						workflow_series sr
					ON
						sr.id = w.series_id
					WHERE
						sr.recurrence <> 'one_time'
					AND
						w.status IN ('approved', 'blocked', 'in_progress', 'completed', 'paid_out')
					AND
					ws.assigned_improver_id IS NOT NULL
			)
			INSERT INTO workflow_series_step_claims(
				series_id,
				step_order,
				improver_id,
				created_at,
				updated_at
			)
			SELECT
				r.series_id,
				r.step_order,
				r.assigned_improver_id,
				unix_now(),
				unix_now()
			FROM
				ranked r
			WHERE
				r.row_rank = 1
			ON CONFLICT (series_id, step_order)
			DO UPDATE SET
				improver_id = EXCLUDED.improver_id,
				updated_at = unix_now();
		`)
	if err != nil {
		return fmt.Errorf("error backfilling workflow_series_step_claims table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS workflow_payout_admin_actions(
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			step_id TEXT REFERENCES workflow_steps(id) ON DELETE SET NULL,
			target_type TEXT NOT NULL,
			action TEXT NOT NULL,
			error_message TEXT NOT NULL DEFAULT '',
			performed_by_user_id TEXT NOT NULL REFERENCES users(id),
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			CHECK (target_type IN ('step', 'supervisor')),
			CHECK (action IN ('mark_paid_out', 'mark_failed')),
			CHECK (
				(target_type = 'step' AND step_id IS NOT NULL)
				OR
				(target_type = 'supervisor' AND step_id IS NULL)
			)
		);

		CREATE INDEX IF NOT EXISTS workflow_payout_admin_actions_workflow_idx
			ON workflow_payout_admin_actions(workflow_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS workflow_payout_admin_actions_performed_by_idx
			ON workflow_payout_admin_actions(performed_by_user_id, created_at DESC);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_payout_admin_actions table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS workflow_votes(
			workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			voter_id TEXT NOT NULL REFERENCES users(id),
			decision TEXT NOT NULL,
			comment TEXT,
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			PRIMARY KEY (workflow_id, voter_id),
			CHECK (decision IN ('approve', 'deny'))
		);

		CREATE INDEX IF NOT EXISTS workflow_votes_workflow_idx ON workflow_votes(workflow_id);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_votes table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS workflow_deletion_proposals(
			id TEXT PRIMARY KEY,
			target_type TEXT NOT NULL,
			target_workflow_id TEXT REFERENCES workflows(id) ON DELETE CASCADE,
			target_series_id TEXT,
			requested_by_user_id TEXT NOT NULL REFERENCES users(id),
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			vote_quorum_reached_at BIGINT,
			vote_finalize_at BIGINT,
			vote_finalized_at BIGINT,
			vote_finalized_by_user_id TEXT REFERENCES users(id),
			vote_decision TEXT,
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			CHECK (target_type IN ('workflow', 'series')),
			CHECK (
				(target_type = 'workflow' AND target_workflow_id IS NOT NULL)
				OR
				(target_type = 'series' AND target_series_id IS NOT NULL)
			),
			CHECK (status IN ('pending', 'approved', 'denied', 'expired')),
			CHECK (vote_decision IN ('approve', 'deny', 'admin_approve') OR vote_decision IS NULL)
		);

		CREATE INDEX IF NOT EXISTS workflow_deletion_proposals_status_idx
			ON workflow_deletion_proposals(status);
		CREATE INDEX IF NOT EXISTS workflow_deletion_proposals_workflow_idx
			ON workflow_deletion_proposals(target_workflow_id);
		CREATE INDEX IF NOT EXISTS workflow_deletion_proposals_series_idx
			ON workflow_deletion_proposals(target_series_id);

		CREATE TABLE IF NOT EXISTS workflow_deletion_votes(
			proposal_id TEXT NOT NULL REFERENCES workflow_deletion_proposals(id) ON DELETE CASCADE,
			voter_id TEXT NOT NULL REFERENCES users(id),
			decision TEXT NOT NULL,
			comment TEXT,
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			PRIMARY KEY (proposal_id, voter_id),
			CHECK (decision IN ('approve', 'deny'))
		);

		CREATE INDEX IF NOT EXISTS workflow_deletion_votes_proposal_idx
			ON workflow_deletion_votes(proposal_id);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow deletion vote tables: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS workflow_edit_proposals(
			id TEXT PRIMARY KEY,
			series_id TEXT NOT NULL REFERENCES workflow_series(id) ON DELETE CASCADE,
			target_workflow_id TEXT REFERENCES workflows(id) ON DELETE SET NULL,
			proposed_state_id TEXT NOT NULL REFERENCES workflow_states(id) ON DELETE CASCADE,
			requested_by_user_id TEXT NOT NULL REFERENCES users(id),
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			vote_quorum_reached_at BIGINT,
			vote_finalize_at BIGINT,
			vote_finalized_at BIGINT,
			vote_finalized_by_user_id TEXT REFERENCES users(id),
			vote_decision TEXT,
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			CHECK (status IN ('pending', 'approved', 'denied', 'expired')),
			CHECK (vote_decision IN ('approve', 'deny', 'admin_approve') OR vote_decision IS NULL)
		);

		CREATE INDEX IF NOT EXISTS workflow_edit_proposals_status_idx
			ON workflow_edit_proposals(status);
		CREATE INDEX IF NOT EXISTS workflow_edit_proposals_series_idx
			ON workflow_edit_proposals(series_id);
		CREATE INDEX IF NOT EXISTS workflow_edit_proposals_target_workflow_idx
			ON workflow_edit_proposals(target_workflow_id);
		CREATE UNIQUE INDEX IF NOT EXISTS workflow_edit_proposals_pending_unique_idx
			ON workflow_edit_proposals(series_id)
			WHERE status = 'pending';

		CREATE TABLE IF NOT EXISTS workflow_edit_votes(
			proposal_id TEXT NOT NULL REFERENCES workflow_edit_proposals(id) ON DELETE CASCADE,
			voter_id TEXT NOT NULL REFERENCES users(id),
			decision TEXT NOT NULL,
			comment TEXT,
			created_at BIGINT NOT NULL DEFAULT unix_now(),
			updated_at BIGINT NOT NULL DEFAULT unix_now(),
			PRIMARY KEY (proposal_id, voter_id),
			CHECK (decision IN ('approve', 'deny'))
		);

		CREATE INDEX IF NOT EXISTS workflow_edit_votes_proposal_idx
			ON workflow_edit_votes(proposal_id);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow edit proposal vote tables: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE affiliates
		ADD COLUMN IF NOT EXISTS affiliate_logo TEXT;

		ALTER TABLE affiliates
		ADD COLUMN IF NOT EXISTS weekly_allocation BIGINT NOT NULL DEFAULT 0;

		UPDATE affiliates
		SET weekly_allocation = weekly_balance
		WHERE weekly_allocation = 0;

		UPDATE affiliates
		SET weekly_balance = LEAST(weekly_balance, weekly_allocation);
	`)
	if err != nil {
		return fmt.Errorf("error updating affiliates weekly allocation: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
				CREATE TABLE IF NOT EXISTS locations (
					id SERIAL PRIMARY KEY,
					google_id TEXT,
					owner_id TEXT REFERENCES users(id),
					name TEXT,
					description TEXT,
					type TEXT,
					approval BOOLEAN,
					approved_at TIMESTAMP,
					street TEXT,
					city TEXT,
					state TEXT,
				zip TEXT,
				lat NUMERIC,
				lng NUMERIC,
				phone TEXT,
				email TEXT,
				admin_phone TEXT,
				admin_email TEXT,
				website TEXT,
				image_url TEXT,
				rating NUMERIC,
				maps_page TEXT,
				contact_firstname TEXT,
				contact_lastname TEXT,
				contact_phone TEXT,
				pos_system TEXT,
				sole_proprietorship TEXT,
				tipping_policy TEXT,
				tipping_division TEXT,
				table_coverage TEXT,
				service_stations INTEGER,
				tablet_model TEXT,
				messaging_service TEXT,
				reference TEXT,
				UNIQUE (google_id)
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating locations table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			ALTER TABLE locations
			ADD COLUMN IF NOT EXISTS approved_at TIMESTAMP;

			UPDATE locations
			SET approved_at = NOW()
			WHERE approval = TRUE
			AND approved_at IS NULL;
		`)
	if err != nil {
		return fmt.Errorf("error ensuring locations.approved_at column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS location_hours(
			location_id INTEGER REFERENCES locations(id),
			weekday INTEGER NOT NULL,
			hours TEXT
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating location_hours table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS contacts(
			id SERIAL PRIMARY KEY NOT NULL,
			owner TEXT NOT NULL REFERENCES users(id),
			name TEXT NOT NULL,
			address TEXT NOT NULL,
			is_favorite BOOLEAN NOT NULL DEFAULT FALSE,
			UNIQUE (owner, address),
			UNIQUE (owner, name)
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating contacts table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS ponder_subscriptions(
			id INTEGER PRIMARY KEY,
			address TEXT NOT NULL,
			type TEXT NOT NULL,
			owner TEXT NOT NULL REFERENCES users(id),
			data TEXT
		);

		CREATE INDEX IF NOT EXISTS ponder_subscription_address ON ponder_subscriptions(address);
	`)
	if err != nil {
		return fmt.Errorf("error creating ponder subscriptions table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS issuers(
			user_id TEXT PRIMARY KEY REFERENCES users(id),
			organization TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			nickname TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS issuers_status_idx ON issuers(status);
	`)
	if err != nil {
		return fmt.Errorf("error creating issuers table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		WITH candidate_emails AS (
			SELECT
				u.id AS user_id,
				TRIM(u.contact_email) AS email
			FROM
				users u
			WHERE
				TRIM(COALESCE(u.contact_email, '')) <> ''
			UNION
			SELECT
				p.user_id,
				TRIM(p.email) AS email
			FROM
				proposers p
			WHERE
				TRIM(COALESCE(p.email, '')) <> ''
			UNION
			SELECT
				i.user_id,
				TRIM(i.email) AS email
			FROM
				improvers i
			WHERE
				TRIM(COALESCE(i.email, '')) <> ''
			UNION
			SELECT
				isr.user_id,
				TRIM(isr.email) AS email
			FROM
				issuers isr
			WHERE
				TRIM(COALESCE(isr.email, '')) <> ''
			UNION
			SELECT
				ps.owner AS user_id,
				TRIM(ps.data) AS email
			FROM
				ponder_subscriptions ps
			WHERE
				TRIM(COALESCE(ps.data, '')) <> ''
		)
		INSERT INTO user_verified_emails
			(
				id,
				user_id,
				email,
				email_normalized,
				verified_at,
				verification_token,
				verification_sent_at,
				verification_token_expires_at
			)
		SELECT
			MD5(user_id || ':' || LOWER(email)) AS id,
			user_id,
			email,
			LOWER(email) AS email_normalized,
			NOW() AS verified_at,
			NULL,
			NULL,
			NULL
		FROM
			candidate_emails
		WHERE
			TRIM(COALESCE(email, '')) <> ''
		ON CONFLICT (user_id, email_normalized) DO UPDATE
		SET
			email = EXCLUDED.email,
			verified_at = COALESCE(user_verified_emails.verified_at, EXCLUDED.verified_at),
			updated_at = NOW();
	`)
	if err != nil {
		return fmt.Errorf("error backfilling verified email rows: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS credential_type_definitions(
			value TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("error creating credential_type_definitions table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE issuer_credential_scopes
		DROP CONSTRAINT IF EXISTS issuer_credential_scopes_credential_type_check;

		ALTER TABLE user_credentials
		DROP CONSTRAINT IF EXISTS user_credentials_credential_type_check;

		ALTER TABLE workflow_role_credentials
		DROP CONSTRAINT IF EXISTS workflow_role_credentials_credential_type_check;
	`)
	if err != nil {
		return fmt.Errorf("error dropping hardcoded credential type constraints: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		INSERT INTO credential_type_definitions (value, label)
		SELECT DISTINCT
			TRIM(wrc.credential_type) AS value,
			TRIM(wrc.credential_type) AS label
		FROM
			workflow_role_credentials wrc
		LEFT JOIN
			credential_type_definitions ctd
		ON
			ctd.value = TRIM(wrc.credential_type)
		WHERE
			TRIM(COALESCE(wrc.credential_type, '')) <> ''
		AND
			ctd.value IS NULL;
	`)
	if err != nil {
		return fmt.Errorf("error seeding credential type definitions: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'workflow_role_credentials_credential_type_fk'
			) THEN
				ALTER TABLE workflow_role_credentials
				ADD CONSTRAINT workflow_role_credentials_credential_type_fk
				FOREIGN KEY (credential_type)
				REFERENCES credential_type_definitions(value)
				ON UPDATE CASCADE
				ON DELETE RESTRICT
				DEFERRABLE INITIALLY IMMEDIATE;
			END IF;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("error adding workflow role credential foreign key: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
			CREATE TABLE IF NOT EXISTS w9_wallet_earnings(
				wallet_address TEXT NOT NULL,
				year INTEGER NOT NULL,
				amount_received NUMERIC(78, 0) NOT NULL DEFAULT 0,
				user_id TEXT,
				w9_required BOOLEAN NOT NULL DEFAULT false,
				w9_required_at TIMESTAMP,
				last_tx_hash TEXT,
				last_tx_timestamp INTEGER,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
				PRIMARY KEY (wallet_address, year)
			);

			CREATE INDEX IF NOT EXISTS w9_wallet_earnings_user_id_idx ON w9_wallet_earnings(user_id);
			CREATE INDEX IF NOT EXISTS w9_wallet_earnings_year_idx ON w9_wallet_earnings(year);
			CREATE INDEX IF NOT EXISTS w9_wallet_earnings_required_idx ON w9_wallet_earnings(w9_required);
		`)
	if err != nil {
		return fmt.Errorf("error creating w9 wallet earnings table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS w9_submissions(
			id SERIAL PRIMARY KEY,
			wallet_address TEXT NOT NULL,
			year INTEGER NOT NULL,
			email TEXT NOT NULL,
			submitted_at TIMESTAMP NOT NULL DEFAULT NOW(),
			pending_approval BOOLEAN NOT NULL DEFAULT TRUE,
			approved_at TIMESTAMP NULL,
			approved_by_user_id TEXT NULL,
			rejected_at TIMESTAMP NULL,
			rejected_by_user_id TEXT NULL,
			rejection_reason TEXT NULL,
			w9_url TEXT NULL,
			UNIQUE (wallet_address, year)
		);

		ALTER TABLE w9_submissions ADD COLUMN IF NOT EXISTS rejected_at TIMESTAMP NULL;
		ALTER TABLE w9_submissions ADD COLUMN IF NOT EXISTS rejected_by_user_id TEXT NULL;
		ALTER TABLE w9_submissions ADD COLUMN IF NOT EXISTS rejection_reason TEXT NULL;

		CREATE INDEX IF NOT EXISTS w9_submissions_pending_idx ON w9_submissions(pending_approval);
		CREATE INDEX IF NOT EXISTS w9_submissions_year_idx ON w9_submissions(year);
	`)
	if err != nil {
		return fmt.Errorf("error creating w9 submissions table: %s", err)
	}

	return nil
}
