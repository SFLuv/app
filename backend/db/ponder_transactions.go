package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func (p *PonderDB) GetTransactionsPaginated(ctx context.Context, address string, chainID int64, page int, count int, descending bool) (*structs.PonderTransactionsPage, error) {
	offset := page * count

	row := p.db.QueryRow(ctx, `
		SELECT
			COUNT(t.id)
			FROM
				transfer_event t
			WHERE
				t.chain_id = $3
			AND
				(
					t.from = LOWER($1)
				OR
					t.to = LOWER($2)
				);
		`, address, address, chainID)
	var total uint64
	err := row.Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("error getting total transaction count for address %s chain %d: %s", address, chainID, err)
	}

	desc := ""
	if descending {
		desc = "DESC"
	}

	rows, err := p.db.Query(ctx, fmt.Sprintf(`
			SELECT
				t.id,
				t.chain_id,
				t.hash,
				t.amount,
				t.timestamp,
			t.from,
			t.to
			FROM
				transfer_event t
			WHERE
				t.chain_id = $5
			AND
				(
					t.from = LOWER($1)
				OR
					t.to = LOWER($2)
				)
			ORDER BY
				t.timestamp
			%s
			LIMIT $3
			OFFSET $4;
		`, desc), address, address, count, offset, chainID)
	if err != nil {
		return nil, fmt.Errorf("error querying for transaction history for address %s chain %d: %s", address, chainID, err)
	}
	defer rows.Close()

	transactions := make([]*structs.PonderTransaction, 0)
	for rows.Next() {
		var t structs.PonderTransaction
		err = rows.Scan(
			&t.Id,
			&t.ChainID,
			&t.Hash,
			&t.Amount,
			&t.Timestamp,
			&t.From,
			&t.To,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning transaction history row for address %s chain %d: %s", address, chainID, err)
		}

		transactions = append(transactions, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transaction history rows for address %s chain %d: %s", address, chainID, err)
	}

	transactionsPage := structs.PonderTransactionsPage{
		Transactions: transactions,
		Total:        total,
	}

	return &transactionsPage, nil
}

func (p *PonderDB) GetBalanceAtTimestamp(ctx context.Context, address string, chainID int64, timestamp int64) (string, error) {
	row := p.db.QueryRow(ctx, `
		SELECT
			COALESCE(
				SUM(CASE WHEN t.to = LOWER($1) THEN t.amount ELSE 0 END)
				-
				SUM(CASE WHEN t.from = LOWER($1) THEN t.amount ELSE 0 END),
				0
			)::text
			FROM
				transfer_event t
			WHERE
				t.chain_id = $3
			AND
				t.timestamp <= $2
			AND (
				t.from = LOWER($1)
				OR
				t.to = LOWER($1)
			);
		`, address, timestamp, chainID)

	var balance string
	err := row.Scan(&balance)
	if err != nil {
		return "", fmt.Errorf("error getting balance at timestamp for address %s chain %d: %s", address, chainID, err)
	}

	return balance, nil
}

func (p *PonderDB) GetTransactionPartiesByHash(ctx context.Context, txHash string, chainID int64) (*structs.PonderTransactionParties, error) {
	normalizedHash := strings.ToLower(strings.TrimSpace(txHash))
	if normalizedHash == "" {
		return nil, nil
	}

	row := p.db.QueryRow(ctx, `
			SELECT
				t.chain_id,
				t.hash,
				t.from,
				t.to
			FROM
				transfer_event t
			WHERE
				t.chain_id = $2
			AND
				t.hash = LOWER($1)
			LIMIT 1;
		`, normalizedHash, chainID)

	var tx structs.PonderTransactionParties
	err := row.Scan(
		&tx.ChainID,
		&tx.Hash,
		&tx.From,
		&tx.To,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error querying transaction by hash %s chain %d: %w", normalizedHash, chainID, err)
	}

	return &tx, nil
}
