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

	direction := "ASC"
	if descending {
		direction = "DESC"
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
				t.timestamp %s,
				t.id %s
			LIMIT $3
			OFFSET $4;
		`, direction, direction), address, address, count, offset)
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
			return nil, fmt.Errorf("error scanning transaction history row for address %s: %s", address, err)
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

func (p *PonderDB) GetTransactionPartiesByHash(ctx context.Context, txHash string, chainID int64) (*structs.PonderTransactionParties, error) {
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
			ORDER BY
				t.timestamp DESC,
				t.id DESC
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

	// The Ponder DB is single-chain; report the active chain id supplied by the
	// caller rather than reading a (removed) chain_id column.
	tx.ChainID = chainID
	return &tx, nil
}

// GetTransferByHash returns one indexed transfer with its amount.
//
// GetTransactionPartiesByHash answers "who sent what to whom"; a refund also has
// to know HOW MUCH the original moved, because that is the ceiling on what may
// be given back. Reading it from the chain index rather than trusting the client
// is the whole point: the amount a refund is measured against must not be a
// number the refunding party supplied.
func (p *PonderDB) GetTransferByHash(ctx context.Context, txHash string) (*structs.PonderTransaction, error) {
	normalizedHash := strings.ToLower(strings.TrimSpace(txHash))
	if normalizedHash == "" {
		return nil, nil
	}

	row := p.db.QueryRow(ctx, `
			SELECT
				t.id,
				t.hash,
				t.amount::text,
				t.timestamp,
				t.from,
				t.to
			FROM
				transfer_event t
			WHERE
				t.hash = LOWER($1)
			ORDER BY
				t.timestamp DESC,
				t.id DESC
			LIMIT 1;
		`, normalizedHash)

	var t structs.PonderTransaction
	if err := row.Scan(&t.Id, &t.Hash, &t.Amount, &t.Timestamp, &t.From, &t.To); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("error loading transfer %s: %w", normalizedHash, err)
	}
	return &t, nil
}

// GetTransfersForAddresses pages a location's whole history: money into and out
// of every wallet it takes payment through, newest first, in one ordering.
//
// Separate from GetTransactionsPaginated because a location is not one address —
// a till and its tipping wallet are one shop, and paging them independently
// would interleave wrong at every page boundary.
func (p *PonderDB) GetTransfersForAddresses(ctx context.Context, addresses []string, page, count int) ([]structs.PonderTransaction, uint64, error) {
	lowered := make([]string, 0, len(addresses))
	for _, a := range addresses {
		if trimmed := strings.ToLower(strings.TrimSpace(a)); trimmed != "" {
			lowered = append(lowered, trimmed)
		}
	}
	if len(lowered) == 0 {
		return []structs.PonderTransaction{}, 0, nil
	}
	if count <= 0 || count > 200 {
		count = 50
	}
	if page < 0 {
		page = 0
	}

	var total uint64
	if err := p.db.QueryRow(ctx, `
		SELECT COUNT(t.id) FROM transfer_event t
		WHERE t.from = ANY($1) OR t.to = ANY($1);
	`, lowered).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("error counting location transfers: %w", err)
	}

	rows, err := p.db.Query(ctx, `
		SELECT t.id, t.hash, t.amount::text, t.timestamp, t.from, t.to
		FROM transfer_event t
		WHERE t.from = ANY($1) OR t.to = ANY($1)
		ORDER BY t.timestamp DESC, t.id DESC
		LIMIT $2 OFFSET $3;
	`, lowered, count, page*count)
	if err != nil {
		return nil, 0, fmt.Errorf("error querying location transfers: %w", err)
	}
	defer rows.Close()

	out := []structs.PonderTransaction{}
	for rows.Next() {
		var t structs.PonderTransaction
		if err := rows.Scan(&t.Id, &t.Hash, &t.Amount, &t.Timestamp, &t.From, &t.To); err != nil {
			return nil, 0, fmt.Errorf("error scanning location transfer: %w", err)
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}
