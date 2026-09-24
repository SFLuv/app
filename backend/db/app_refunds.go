package db

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/SFLuv/app/backend/structs"
)

// RefundSchemaDDL is applied by schema migration 1.58.
//
// A refund is an ordinary transfer on chain — the till sends tokens back to the
// customer — so the chain cannot say what it was for. This table is the only
// place that knows a particular transfer undid a particular payment, which is
// what lets a partly refunded payment be refunded again for the rest and no
// further.
const RefundSchemaDDL = `
	CREATE TABLE IF NOT EXISTS transaction_refunds (
		id               BIGSERIAL PRIMARY KEY,
		original_tx_hash TEXT NOT NULL,
		refund_tx_hash   TEXT NOT NULL UNIQUE,
		location_id      INTEGER,
		owner_id         TEXT NOT NULL,
		from_address     TEXT NOT NULL,
		to_address       TEXT NOT NULL,
		-- Base units, as a decimal string. Kept as text for the same reason the
		-- unwrap ledger does: these are token amounts, not money the database
		-- should be rounding.
		amount           TEXT NOT NULL,
		chain_id         BIGINT,
		created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS transaction_refunds_original_idx ON transaction_refunds (original_tx_hash);
	CREATE INDEX IF NOT EXISTS transaction_refunds_location_idx ON transaction_refunds (location_id);
	CREATE INDEX IF NOT EXISTS transaction_refunds_owner_idx ON transaction_refunds (owner_id, created_at DESC);
`

// InsertRefund records one refund against its original.
//
// Idempotent on the refund's own hash: a client that submits the same
// transaction twice (a retry, a double tap) gets the existing row back rather
// than a second refund against the same money.
func (a *AppDB) InsertRefund(ctx context.Context, r *structs.Refund) (int64, error) {
	var id int64
	err := a.db.QueryRow(ctx, `
		INSERT INTO transaction_refunds
			(original_tx_hash, refund_tx_hash, location_id, owner_id, from_address, to_address, amount, chain_id)
		VALUES (LOWER($1), LOWER($2), $3, $4, LOWER($5), LOWER($6), $7, $8)
		ON CONFLICT (refund_tx_hash) DO UPDATE SET refund_tx_hash = EXCLUDED.refund_tx_hash
		RETURNING id;
	`,
		strings.TrimSpace(r.OriginalTxHash), strings.TrimSpace(r.RefundTxHash), r.LocationID, r.OwnerID,
		strings.TrimSpace(r.FromAddress), strings.TrimSpace(r.ToAddress), r.Amount, r.ChainID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("error inserting refund for %s: %w", r.OriginalTxHash, err)
	}
	return id, nil
}

// RefundedTotals sums what has already been refunded against each of the given
// original hashes. Hashes with nothing against them are absent from the map.
//
// Taken in bulk because the history view needs it for a page of transactions at
// once, and asking per row would be a query per line.
func (a *AppDB) RefundedTotals(ctx context.Context, originalHashes []string) (map[string]*big.Int, error) {
	totals := map[string]*big.Int{}
	if len(originalHashes) == 0 {
		return totals, nil
	}
	lowered := make([]string, 0, len(originalHashes))
	for _, h := range originalHashes {
		if trimmed := strings.ToLower(strings.TrimSpace(h)); trimmed != "" {
			lowered = append(lowered, trimmed)
		}
	}
	if len(lowered) == 0 {
		return totals, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT original_tx_hash, COALESCE(SUM(amount::numeric), 0)::text
		FROM transaction_refunds
		WHERE original_tx_hash = ANY($1)
		GROUP BY original_tx_hash;
	`, lowered)
	if err != nil {
		return nil, fmt.Errorf("error summing refunds: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var hash, sum string
		if err := rows.Scan(&hash, &sum); err != nil {
			return nil, err
		}
		if amount, ok := new(big.Int).SetString(sum, 10); ok {
			totals[hash] = amount
		}
	}
	return totals, rows.Err()
}

// ListRefundsForOriginals returns the individual refunds behind those totals, so
// a transaction can show what was taken off it and when.
func (a *AppDB) ListRefundsForOriginals(ctx context.Context, originalHashes []string) (map[string][]structs.Refund, error) {
	out := map[string][]structs.Refund{}
	if len(originalHashes) == 0 {
		return out, nil
	}
	lowered := make([]string, 0, len(originalHashes))
	for _, h := range originalHashes {
		if trimmed := strings.ToLower(strings.TrimSpace(h)); trimmed != "" {
			lowered = append(lowered, trimmed)
		}
	}
	if len(lowered) == 0 {
		return out, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT id, original_tx_hash, refund_tx_hash, location_id, owner_id, from_address, to_address, amount, chain_id, created_at
		FROM transaction_refunds
		WHERE original_tx_hash = ANY($1)
		ORDER BY created_at ASC;
	`, lowered)
	if err != nil {
		return nil, fmt.Errorf("error listing refunds: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r structs.Refund
		if err := rows.Scan(&r.ID, &r.OriginalTxHash, &r.RefundTxHash, &r.LocationID, &r.OwnerID,
			&r.FromAddress, &r.ToAddress, &r.Amount, &r.ChainID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out[r.OriginalTxHash] = append(out[r.OriginalTxHash], r)
	}
	return out, rows.Err()
}

// RefundOriginalsByRefundHash maps a refund's own hash back to what it refunded,
// so the refund line in a history view can point at the payment it undid.
func (a *AppDB) RefundOriginalsByRefundHash(ctx context.Context, refundHashes []string) (map[string]string, error) {
	out := map[string]string{}
	if len(refundHashes) == 0 {
		return out, nil
	}
	lowered := make([]string, 0, len(refundHashes))
	for _, h := range refundHashes {
		if trimmed := strings.ToLower(strings.TrimSpace(h)); trimmed != "" {
			lowered = append(lowered, trimmed)
		}
	}
	if len(lowered) == 0 {
		return out, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT refund_tx_hash, original_tx_hash FROM transaction_refunds WHERE refund_tx_hash = ANY($1);
	`, lowered)
	if err != nil {
		return nil, fmt.Errorf("error mapping refunds to originals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var refundHash, originalHash string
		if err := rows.Scan(&refundHash, &originalHash); err != nil {
			return nil, err
		}
		out[refundHash] = originalHash
	}
	return out, rows.Err()
}
