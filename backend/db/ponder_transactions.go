package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func (p *PonderDB) GetTransactionsPaginated(ctx context.Context, address string, page int, count int, descending bool) (*structs.PonderTransactionsPage, error) {
	offset := page * count

	row := p.db.QueryRow(ctx, `
		SELECT
			COUNT(t.id)
		FROM
			transfer_event t
		WHERE
			t.from = LOWER($1)
		OR
			t.to = LOWER($2);
	`, address, address)
	var total uint64
	err := row.Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("error getting total transaction count for address %s: %s", address, err)
	}

	desc := ""
	if descending {
		desc = "DESC"
	}

	rows, err := p.db.Query(ctx, fmt.Sprintf(`
		SELECT
			t.id,
			t.hash,
			t.amount,
			t.timestamp,
			t.from,
			t.to
		FROM
			transfer_event t
		WHERE
			t.from = LOWER($1)
		OR
			t.to = LOWER($2)
		ORDER BY
			t.timestamp
		%s
		LIMIT $3
		OFFSET $4;
	`, desc), address, address, count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying for transaction history for address %s: %s", address, err)
	}
	defer rows.Close()

	transactions := make([]*structs.PonderTransaction, 0)
	for rows.Next() {
		var t structs.PonderTransaction
		err = rows.Scan(
			&t.Id,
			&t.Hash,
			&t.Amount,
			&t.Timestamp,
			&t.From,
			&t.To,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning transaction history ro for address %s: %s", address, err)
		}

		transactions = append(transactions, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transaction history rows for address %s: %s", address, err)
	}

	transactionsPage := structs.PonderTransactionsPage{
		Transactions: transactions,
		Total:        total,
	}

	return &transactionsPage, nil
}

func (p *PonderDB) GetBalanceAtTimestamp(ctx context.Context, address string, timestamp int64) (string, error) {
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
			t.timestamp <= $2
		AND (
			t.from = LOWER($1)
			OR
			t.to = LOWER($1)
		);
	`, address, timestamp)

	var balance string
	err := row.Scan(&balance)
	if err != nil {
		return "", fmt.Errorf("error getting balance at timestamp for address %s: %s", address, err)
	}

	return balance, nil
}

func (p *PonderDB) GetTransactionPartiesByHash(ctx context.Context, txHash string) (*structs.PonderTransactionParties, error) {
	normalizedHash := strings.ToLower(strings.TrimSpace(txHash))
	if normalizedHash == "" {
		return nil, nil
	}

	row := p.db.QueryRow(ctx, `
		SELECT
			t.hash,
			t.from,
			t.to
		FROM
			transfer_event t
		WHERE
			t.hash = LOWER($1)
		LIMIT 1;
	`, normalizedHash)

	var tx structs.PonderTransactionParties
	err := row.Scan(
		&tx.Hash,
		&tx.From,
		&tx.To,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error querying transaction by hash %s: %w", normalizedHash, err)
	}

	return &tx, nil
}
