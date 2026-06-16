package db

import (
	"context"
	"fmt"
)

func (a *AppDB) BackfillTransactionChainIDs(ctx context.Context, chainID int64) error {
	if chainID <= 0 {
		return fmt.Errorf("chain id must be positive")
	}

	stmts := []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE memos ADD COLUMN IF NOT EXISTS chain_id BIGINT`, nil},
		{`ALTER TABLE memos DROP CONSTRAINT IF EXISTS memos_pkey`, nil},
		{`CREATE UNIQUE INDEX IF NOT EXISTS memos_chain_tx_hash_unique_idx ON memos(chain_id, tx_hash)`, nil},
		{`CREATE INDEX IF NOT EXISTS memos_chain_tx_hash_idx ON memos(chain_id, LOWER(tx_hash))`, nil},
		{`UPDATE memos SET chain_id = $1 WHERE chain_id IS NULL`, []any{chainID}},
		{`ALTER TABLE workflows ADD COLUMN IF NOT EXISTS manager_payout_chain_id BIGINT`, nil},
		{`UPDATE workflows SET manager_payout_chain_id = $1 WHERE manager_payout_chain_id IS NULL AND COALESCE(NULLIF(TRIM(manager_payout_tx_hash), ''), '') <> ''`, []any{chainID}},
		{`ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS payout_chain_id BIGINT`, nil},
		{`UPDATE workflow_steps SET payout_chain_id = $1 WHERE payout_chain_id IS NULL AND COALESCE(NULLIF(TRIM(payout_tx_hash), ''), '') <> ''`, []any{chainID}},
		{`ALTER TABLE w9_wallet_earnings ADD COLUMN IF NOT EXISTS chain_id BIGINT`, nil},
		{`ALTER TABLE w9_wallet_earnings ADD COLUMN IF NOT EXISTS last_tx_chain_id BIGINT`, nil},
		{`ALTER TABLE w9_wallet_earnings DROP CONSTRAINT IF EXISTS w9_wallet_earnings_pkey`, nil},
		{`CREATE UNIQUE INDEX IF NOT EXISTS w9_wallet_earnings_chain_wallet_year_unique_idx ON w9_wallet_earnings(wallet_address, year, chain_id)`, nil},
		{`CREATE INDEX IF NOT EXISTS w9_wallet_earnings_chain_idx ON w9_wallet_earnings(chain_id, year)`, nil},
		{`UPDATE w9_wallet_earnings SET chain_id = $1 WHERE chain_id IS NULL`, []any{chainID}},
		{`UPDATE w9_wallet_earnings SET last_tx_chain_id = $1 WHERE last_tx_chain_id IS NULL AND COALESCE(NULLIF(TRIM(last_tx_hash), ''), '') <> ''`, []any{chainID}},
	}

	for _, s := range stmts {
		if _, err := a.db.Exec(ctx, s.sql, s.args...); err != nil {
			return fmt.Errorf("error backfilling app transaction chain ids (%s): %w", s.sql[:40], err)
		}
	}

	return nil
}

func (b *BotDB) BackfillTransactionChainIDs(ctx context.Context, chainID int64) error {
	if chainID <= 0 {
		return fmt.Errorf("chain id must be positive")
	}

	stmts := []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE redemptions ADD COLUMN IF NOT EXISTS chain_id BIGINT`, nil},
		{`CREATE INDEX IF NOT EXISTS redemptions_chain_idx ON redemptions(chain_id)`, nil},
		{`UPDATE redemptions SET chain_id = $1 WHERE chain_id IS NULL`, []any{chainID}},
	}

	for _, s := range stmts {
		if _, err := b.db.Exec(ctx, s.sql, s.args...); err != nil {
			return fmt.Errorf("error backfilling bot transaction chain ids (%s): %w", s.sql[:40], err)
		}
	}

	return nil
}

func (p *PonderDB) BackfillTransactionChainIDs(ctx context.Context, chainID int64) error {
	if chainID <= 0 {
		return fmt.Errorf("chain id must be positive")
	}

	type tableSpec struct {
		name    string
		indexes []string
	}
	tables := []tableSpec{
		{
			name: "transfer_event",
			indexes: []string{
				`CREATE INDEX IF NOT EXISTS transfer_event_chain_hash_idx ON transfer_event(chain_id, hash);`,
				`CREATE INDEX IF NOT EXISTS transfer_event_chain_from_idx ON transfer_event(chain_id, "from");`,
				`CREATE INDEX IF NOT EXISTS transfer_event_chain_to_idx ON transfer_event(chain_id, "to");`,
			},
		},
		{
			name: "transfer_account",
			indexes: []string{
				`CREATE INDEX IF NOT EXISTS transfer_account_chain_address_idx ON transfer_account(chain_id, address);`,
			},
		},
		{
			name: "allowance",
			indexes: []string{
				`CREATE INDEX IF NOT EXISTS allowance_chain_owner_spender_idx ON allowance(chain_id, owner, spender);`,
			},
		},
		{
			name: "approval_event",
			indexes: []string{
				`CREATE INDEX IF NOT EXISTS approval_event_chain_owner_idx ON approval_event(chain_id, owner);`,
				`CREATE INDEX IF NOT EXISTS approval_event_chain_spender_idx ON approval_event(chain_id, spender);`,
			},
		},
	}

	for _, table := range tables {
		exists, err := p.tableExists(ctx, table.name)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}

		if _, err := p.db.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS chain_id BIGINT`, table.name)); err != nil {
			return fmt.Errorf("error backfilling ponder table %s chain ids (alter): %w", table.name, err)
		}
		if _, err := p.db.Exec(ctx, fmt.Sprintf(`UPDATE %s SET chain_id = $1 WHERE chain_id IS NULL`, table.name), chainID); err != nil {
			return fmt.Errorf("error backfilling ponder table %s chain ids (update): %w", table.name, err)
		}

		for _, indexQuery := range table.indexes {
			if _, err := p.db.Exec(ctx, indexQuery); err != nil {
				return fmt.Errorf("error creating chain index for ponder table %s: %w", table.name, err)
			}
		}
	}

	return nil
}

func (p *PonderDB) tableExists(ctx context.Context, tableName string) (bool, error) {
	var exists bool
	err := p.db.QueryRow(ctx, `
		SELECT to_regclass($1) IS NOT NULL;
	`, "public."+tableName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking table %s: %w", tableName, err)
	}
	return exists, nil
}
