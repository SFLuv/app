package db

import (
	"context"
	"fmt"
	"math/big"

	"github.com/SFLuv/app/backend/structs"
)

// TransfersInto returns every indexed transfer into any of the given addresses
// within [start, end). Addresses are compared lowercased, which is how the
// indexer writes them.
func (p *PonderDB) TransfersInto(ctx context.Context, addresses []string, start, end int64) ([]structs.MerchantTransfer, error) {
	return p.transfers(ctx, `"to"`, addresses, start, end)
}

// TransfersOutOf returns transfers leaving the given addresses. Merchant mode
// has no send flow, so on that screen these are refunds or a manual correction
// made from another device — rare, but money the till log should not omit.
func (p *PonderDB) TransfersOutOf(ctx context.Context, addresses []string, start, end int64) ([]structs.MerchantTransfer, error) {
	return p.transfers(ctx, `"from"`, addresses, start, end)
}

func (p *PonderDB) transfers(ctx context.Context, column string, addresses []string, start, end int64) ([]structs.MerchantTransfer, error) {
	if len(addresses) == 0 {
		return nil, nil
	}

	lowered := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if trimmed := normalizeAddress(address); trimmed != "" {
			lowered = append(lowered, trimmed)
		}
	}
	if len(lowered) == 0 {
		return nil, nil
	}

	// column is one of two literals chosen above, never caller input.
	rows, err := p.db.Query(ctx, fmt.Sprintf(`
		SELECT
			t.hash,
			LOWER(t."from"),
			LOWER(t."to"),
			t.amount::text,
			t.timestamp
		FROM
			transfer_event t
		WHERE
			LOWER(t.%s) = ANY($1)
		AND
			t.timestamp >= $2
		AND
			t.timestamp < $3
		ORDER BY
			t.timestamp ASC;
	`, column), lowered, start, end)
	if err != nil {
		return nil, fmt.Errorf("error querying merchant transfers: %w", err)
	}
	defer rows.Close()

	transfers := []structs.MerchantTransfer{}
	for rows.Next() {
		var transfer structs.MerchantTransfer
		var amount string
		if err := rows.Scan(&transfer.Hash, &transfer.From, &transfer.To, &amount, &transfer.Timestamp); err != nil {
			return nil, fmt.Errorf("error scanning merchant transfer: %w", err)
		}
		value, ok := new(big.Int).SetString(amount, 10)
		if !ok {
			// A row we cannot read is skipped rather than counted as zero: a
			// silent zero would understate the till, which is worse than a gap.
			continue
		}
		transfer.Amount = value
		transfers = append(transfers, transfer)
	}

	return transfers, rows.Err()
}
