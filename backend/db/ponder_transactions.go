package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
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

	var transactions []*structs.PonderTransaction
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

	transactionsPage := structs.PonderTransactionsPage{
		Transactions: transactions,
		Total:        total,
	}

	return &transactionsPage, nil
}
