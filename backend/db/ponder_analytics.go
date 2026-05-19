package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
)

func (p *PonderDB) GetAnalyticsTransfersSince(ctx context.Context, startTimestamp int64) ([]*structs.AnalyticsTransfer, error) {
	rows, err := p.db.Query(ctx, `
		SELECT
			hash,
			amount::text,
			timestamp,
			LOWER("from"),
			LOWER("to")
		FROM
			transfer_event
		WHERE
			timestamp >= $1
		ORDER BY
			timestamp ASC,
			id ASC;
	`, startTimestamp)
	if err != nil {
		return nil, fmt.Errorf("error querying analytics transfers: %w", err)
	}
	defer rows.Close()

	transfers := make([]*structs.AnalyticsTransfer, 0)
	for rows.Next() {
		var tx structs.AnalyticsTransfer
		if err := rows.Scan(&tx.Hash, &tx.Amount, &tx.Timestamp, &tx.From, &tx.To); err != nil {
			return nil, fmt.Errorf("error scanning analytics transfer: %w", err)
		}
		transfers = append(transfers, &tx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analytics transfers: %w", err)
	}

	return transfers, nil
}

func (p *PonderDB) GetAnalyticsAddressBalances(ctx context.Context, addresses []string) ([]*structs.AnalyticsAddressBalance, error) {
	normalized := make([]string, 0, len(addresses))
	seen := make(map[string]struct{})
	for _, address := range addresses {
		value := strings.TrimSpace(strings.ToLower(address))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []*structs.AnalyticsAddressBalance{}, nil
	}

	rows, err := p.db.Query(ctx, `
		SELECT
			LOWER(address),
			balance::text
		FROM
			transfer_account
		WHERE
			LOWER(address) = ANY($1::text[]);
	`, normalized)
	if err != nil {
		return nil, fmt.Errorf("error querying analytics address balances: %w", err)
	}
	defer rows.Close()

	balances := make([]*structs.AnalyticsAddressBalance, 0)
	for rows.Next() {
		var balance structs.AnalyticsAddressBalance
		if err := rows.Scan(&balance.Address, &balance.Balance); err != nil {
			return nil, fmt.Errorf("error scanning analytics address balance: %w", err)
		}
		balances = append(balances, &balance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analytics address balances: %w", err)
	}

	return balances, nil
}
