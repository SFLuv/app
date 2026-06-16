package db

import (
	"context"
	"fmt"
	"strings"
)

func (a *AppDB) UpsertTransactionMemo(ctx context.Context, txHash string, chainID int64, memo string, owner string) error {
	normalizedHash := strings.ToLower(strings.TrimSpace(txHash))
	trimmedMemo := strings.TrimSpace(memo)

	_, err := a.db.Exec(ctx, `
		INSERT INTO memos (
			tx_hash,
			chain_id,
			memo,
			owner
		) VALUES (
			$1,
			$2,
			$3,
			$4
		)
		ON CONFLICT (chain_id, tx_hash)
		DO UPDATE SET
			memo = EXCLUDED.memo,
			owner = EXCLUDED.owner,
			active = TRUE,
			delete_date = NULL,
			delete_reason = NULL,
			updated_at = NOW();
	`, normalizedHash, chainID, trimmedMemo, owner)
	if err != nil {
		return fmt.Errorf("error upserting transaction memo for tx %s chain %d: %w", normalizedHash, chainID, err)
	}

	return nil
}

func (a *AppDB) GetTransactionMemosByHashes(ctx context.Context, hashes []string, chainID int64) (map[string]string, error) {
	normalizedHashes := make([]string, 0, len(hashes))
	seen := map[string]struct{}{}
	for _, hash := range hashes {
		normalized := strings.ToLower(strings.TrimSpace(hash))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		normalizedHashes = append(normalizedHashes, normalized)
	}

	if len(normalizedHashes) == 0 {
		return map[string]string{}, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			tx_hash,
			memo
			FROM
				memos
			WHERE
				chain_id = $2
			AND
				tx_hash = ANY($1)
			AND
				active = TRUE;
	`, normalizedHashes, chainID)
	if err != nil {
		return nil, fmt.Errorf("error querying transaction memos for chain %d: %w", chainID, err)
	}
	defer rows.Close()

	memosByHash := map[string]string{}
	for rows.Next() {
		var txHash string
		var memo string
		if err := rows.Scan(&txHash, &memo); err != nil {
			return nil, fmt.Errorf("error scanning transaction memo row: %w", err)
		}
		memosByHash[strings.ToLower(strings.TrimSpace(txHash))] = memo
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transaction memo rows: %w", err)
	}

	return memosByHash, nil
}
