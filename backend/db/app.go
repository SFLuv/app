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
		CREATE TABLE IF NOT EXISTS workflows(
			id TEXT PRIMARY KEY,
			series_id TEXT NOT NULL,
			proposer_id TEXT NOT NULL REFERENCES users(id),
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			recurrence TEXT NOT NULL,
			start_at TIMESTAMP NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			is_start_blocked BOOLEAN NOT NULL DEFAULT false,
			blocked_by_workflow_id TEXT,
			total_bounty BIGINT NOT NULL DEFAULT 0,
			weekly_bounty_requirement BIGINT NOT NULL DEFAULT 0,
			budget_weekly_deducted BIGINT NOT NULL DEFAULT 0,
			budget_one_time_deducted BIGINT NOT NULL DEFAULT 0,
				vote_quorum_reached_at TIMESTAMP,
				vote_finalize_at TIMESTAMP,
				vote_finalized_at TIMESTAMP,
				vote_finalized_by_user_id TEXT REFERENCES users(id),
				vote_decision TEXT,
				manager_required BOOLEAN NOT NULL DEFAULT false,
				manager_role_id TEXT,
				manager_improver_id TEXT REFERENCES users(id),
				manager_bounty BIGINT NOT NULL DEFAULT 0,
				manager_paid_out_at TIMESTAMP,
				manager_payout_error TEXT,
				manager_payout_last_try_at TIMESTAMP,
				manager_retry_requested_at TIMESTAMP,
				manager_retry_requested_by TEXT REFERENCES users(id),
				approved_at TIMESTAMP,
				approved_by_user_id TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			CHECK (recurrence IN ('one_time', 'daily', 'weekly', 'monthly')),
			CHECK (status IN ('pending', 'approved', 'rejected', 'in_progress', 'completed', 'paid_out', 'blocked', 'expired', 'deleted')),
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
		ALTER TABLE workflows
		DROP CONSTRAINT IF EXISTS workflows_status_check;

		ALTER TABLE workflows
		ADD CONSTRAINT workflows_status_check
		CHECK (status IN ('pending', 'approved', 'rejected', 'in_progress', 'completed', 'paid_out', 'blocked', 'expired', 'deleted'));
	`)
	if err != nil {
		return fmt.Errorf("error updating workflows status constraint: %s", err)
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
				start_at TIMESTAMP NOT NULL,
				series_id TEXT,
				manager_json JSONB,
				roles_json JSONB NOT NULL DEFAULT '[]'::jsonb,
				steps_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
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
			ALTER TABLE workflow_templates
			ADD COLUMN IF NOT EXISTS manager_json JSONB;
		`)
	if err != nil {
		return fmt.Errorf("error altering workflow_templates manager_json column: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE workflows
		ADD COLUMN IF NOT EXISTS vote_quorum_reached_at TIMESTAMP;

		ALTER TABLE workflows
		ADD COLUMN IF NOT EXISTS vote_finalize_at TIMESTAMP;

		ALTER TABLE workflows
		ADD COLUMN IF NOT EXISTS vote_finalized_at TIMESTAMP;

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
			ADD COLUMN IF NOT EXISTS manager_paid_out_at TIMESTAMP;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_payout_error TEXT;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_payout_last_try_at TIMESTAMP;

			ALTER TABLE workflows
			ADD COLUMN IF NOT EXISTS manager_retry_requested_at TIMESTAMP;

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
			updated_at = NOW()
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
			workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			step_order INTEGER NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			bounty BIGINT NOT NULL DEFAULT 0,
			role_id TEXT REFERENCES workflow_roles(id),
			assigned_improver_id TEXT REFERENCES users(id),
			status TEXT NOT NULL DEFAULT 'locked',
			started_at TIMESTAMP,
			completed_at TIMESTAMP,
			payout_error TEXT,
			payout_last_try_at TIMESTAMP,
			retry_requested_at TIMESTAMP,
			retry_requested_by TEXT REFERENCES users(id),
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
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
		ADD COLUMN IF NOT EXISTS started_at TIMESTAMP;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS payout_error TEXT;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS payout_last_try_at TIMESTAMP;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS retry_requested_at TIMESTAMP;

		ALTER TABLE workflow_steps
		ADD COLUMN IF NOT EXISTS retry_requested_by TEXT REFERENCES users(id);

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
			requires_written_response BOOLEAN NOT NULL DEFAULT false,
			requires_dropdown BOOLEAN NOT NULL DEFAULT false,
			dropdown_options JSONB NOT NULL DEFAULT '[]'::jsonb,
			dropdown_requires_written_response JSONB NOT NULL DEFAULT '{}'::jsonb,
			notify_emails JSONB NOT NULL DEFAULT '[]'::jsonb,
			notify_on_dropdown_values JSONB NOT NULL DEFAULT '[]'::jsonb,
			UNIQUE (step_id, item_order)
		);

		CREATE INDEX IF NOT EXISTS workflow_step_items_step_idx ON workflow_step_items(step_id);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_step_items table: %s", err)
	}

	_, err = s.db.Exec(context.Background(), `
		ALTER TABLE workflow_step_items
		ADD COLUMN IF NOT EXISTS camera_capture_only BOOLEAN NOT NULL DEFAULT false;
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
			item_responses JSONB NOT NULL DEFAULT '[]'::jsonb,
			submitted_at TIMESTAMP NOT NULL DEFAULT NOW(),
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS workflow_step_submissions_workflow_idx ON workflow_step_submissions(workflow_id);
		CREATE INDEX IF NOT EXISTS workflow_step_submissions_improver_idx ON workflow_step_submissions(improver_id);
	`)
	if err != nil {
		return fmt.Errorf("error creating workflow_step_submissions table: %s", err)
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
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW()
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
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
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
			absent_from TIMESTAMP NOT NULL,
			absent_until TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
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
		CREATE TABLE IF NOT EXISTS workflow_votes(
			workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
			voter_id TEXT NOT NULL REFERENCES users(id),
			decision TEXT NOT NULL,
			comment TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
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
			vote_quorum_reached_at TIMESTAMP,
			vote_finalize_at TIMESTAMP,
			vote_finalized_at TIMESTAMP,
			vote_finalized_by_user_id TEXT REFERENCES users(id),
			vote_decision TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
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
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
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
