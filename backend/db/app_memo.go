package db

import (
	"context"
	"fmt"
	"strings"
)

func (a *AppDB) UpsertTransactionMemo(ctx context.Context, txHash string, memo string, owner string) error {
	normalizedHash := strings.ToLower(strings.TrimSpace(txHash))
	trimmedMemo := strings.TrimSpace(memo)

	_, err := a.db.Exec(ctx, `
		INSERT INTO memos (
			tx_hash,
			memo,
			owner
		) VALUES (
			$1,
			$2,
			$3
		)
		ON CONFLICT (tx_hash)
		DO UPDATE SET
			memo = EXCLUDED.memo,
			updated_at = NOW();
	`, normalizedHash, trimmedMemo, owner)
	if err != nil {
		return fmt.Errorf("error upserting transaction memo for tx %s: %w", normalizedHash, err)
	}

	return nil
}

func (a *AppDB) GetTransactionMemosByHashes(ctx context.Context, hashes []string) (map[string]string, error) {
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
			tx_hash = ANY($1);
	`, normalizedHashes)
	if err != nil {
		return nil, fmt.Errorf("error querying transaction memos: %w", err)
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
