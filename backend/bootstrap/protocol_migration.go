package bootstrap

import (
	"context"
	"fmt"
)

// migrateProtocolAccounts is the 1.68 migration body: the backend's own smart
// accounts, the signing keys behind them, and the record of which address held
// which protocol role when.
//
// The version skips ahead deliberately. On main the highest migration is 1.65,
// but 1.66 and 1.67 are already claimed by the bulletin board and the internal
// notifications work on their own branches, and a third claim on either number
// would mean renumbering whichever landed last. 1.68 is free in every branch
// in flight.
//
// Everything is in the app database. Nothing here touches the ponder database,
// which is the indexer's and is never migrated from this repository.
func migrateProtocolAccounts(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		-- One row per protocol function the backend signs for: the faucet, the
		-- redeemer admin, and whatever comes later.
		--
		-- safe_address is the permanent identity. It is derived from the factory
		-- before anything is deployed, so it can be recorded, funded and
		-- compared against while deployment is still pending — which is also
		-- why it is nullable: a row exists from the moment the function is
		-- known, and gains its address when the signer that owns it is minted.
		CREATE TABLE IF NOT EXISTS protocol_accounts(
			key TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL,
			chain_id INTEGER NOT NULL,
			safe_address TEXT,
			owner_index INTEGER NOT NULL DEFAULT 0,
			deployed_at TIMESTAMPTZ,
			deploy_tx TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS protocol_accounts_safe_idx
			ON protocol_accounts(LOWER(safe_address), chain_id)
			WHERE safe_address IS NOT NULL;

		-- Every signer an account has ever had, encrypted.
		--
		-- sealed_key is nonce ‖ AES-256-GCM ciphertext, with the account key as
		-- additional authenticated data, so a row cannot be moved to another
		-- account and still open. The master key is in the environment and is
		-- never stored here: a dump of this table on its own buys nothing.
		CREATE TABLE IF NOT EXISTS protocol_account_signers(
			id BIGSERIAL PRIMARY KEY,
			account_key TEXT NOT NULL REFERENCES protocol_accounts(key) ON DELETE CASCADE,
			eoa_address TEXT NOT NULL,
			sealed_key TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			-- Set once the Safe itself has been read back and agrees this key is
			-- an owner. A key that was minted but never took is left unactivated
			-- rather than deleted, because the rotation that minted it may have
			-- landed on chain after the request that gave up on it.
			activated_at TIMESTAMPTZ,
			activation_tx TEXT NOT NULL DEFAULT '',
			retired_at TIMESTAMPTZ,
			retired_reason TEXT NOT NULL DEFAULT '',
			rotated_by TEXT
		);
		-- One active signer per account. The predicate is "activated and not
		-- retired" rather than just "not retired" on purpose: a rotation mints
		-- the replacement before the chain has accepted it, so for the length of
		-- one operation two rows are unretired and only one of them is live.
		CREATE UNIQUE INDEX IF NOT EXISTS protocol_signers_active_idx
			ON protocol_account_signers(account_key)
			WHERE activated_at IS NOT NULL AND retired_at IS NULL;
		CREATE INDEX IF NOT EXISTS protocol_signers_account_idx
			ON protocol_account_signers(account_key, created_at DESC);
		CREATE UNIQUE INDEX IF NOT EXISTS protocol_signers_eoa_idx
			ON protocol_account_signers(LOWER(eoa_address));

		-- Which address held which protocol role, and when.
		--
		-- This is what makes a check on an old item answerable. A payout made in
		-- March was sent by March's faucet; comparing it against today's would
		-- report a payment that plainly happened as missing. Rows are closed
		-- rather than updated, so the windows are a history and not a current
		-- value with amnesia.
		--
		-- started_at for an address we have always used is the epoch, not the
		-- migration date: the address really did hold the role for every item
		-- that exists, and dating it to the migration would strand everything
		-- older on the wrong side of the window.
		CREATE TABLE IF NOT EXISTS protocol_address_history(
			id BIGSERIAL PRIMARY KEY,
			address TEXT NOT NULL,
			role TEXT NOT NULL,
			chain_id INTEGER NOT NULL,
			account_key TEXT,
			started_at TIMESTAMPTZ NOT NULL,
			ended_at TIMESTAMPTZ,
			source TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS protocol_address_history_role_idx
			ON protocol_address_history(role, chain_id, started_at DESC);
		CREATE UNIQUE INDEX IF NOT EXISTS protocol_address_history_live_idx
			ON protocol_address_history(role, chain_id, LOWER(address))
			WHERE ended_at IS NULL;

		-- The cutover ledger.
		--
		-- The migration moves real money with keys that are presumed exposed, so
		-- every step records that it ran before it is allowed to run again. A
		-- restart, a crash loop or a second deploy finds the step done and does
		-- nothing: without this, "sweep the faucet" is a transfer that repeats
		-- once per boot.
		CREATE TABLE IF NOT EXISTS protocol_migration_steps(
			step TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'pending'
				CHECK (status IN ('pending', 'running', 'done', 'failed', 'skipped')),
			detail TEXT NOT NULL DEFAULT '',
			tx_hash TEXT NOT NULL DEFAULT '',
			amount_base NUMERIC(78,0),
			attempts INTEGER NOT NULL DEFAULT 0,
			started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			completed_at TIMESTAMPTZ
		);
	`); err != nil {
		return fmt.Errorf("error creating protocol account tables: %w", err)
	}

	return nil
}
