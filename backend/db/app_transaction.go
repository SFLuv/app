package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) AddTransactionSubscription(ctx context.Context, address string, walletId string) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO transaction_subscriptions(
			address,
			wallet
		) VALUES (
			$1,
			$2
		);
	`, address, walletId)

	return err
}

func (a *AppDB) GetTransactionSubscription(ctx context.Context, address string) (string, error) {
	var walletId string
	row := a.db.QueryRow(ctx, `
		SELECT
			wallet
		FROM
			transaction_subscriptions
		WHERE
			address = $1;
	`, address)
	err := row.Scan(&walletId)
	if err != nil {
		return "", err
	}

	return walletId, nil
}

func (a *AppDB) RemoveTransactionSubscription(ctx context.Context, address string) error {
	_, err := a.db.Exec(ctx, `
		DELETE FROM
			transaction_subscriptions
		WHERE
			address = $1;
	`, address)

	return err
}

func (a *AppDB) AddTransaction(ctx context.Context, tx *structs.FormattedTransaction) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO transactions(
			id,
			wallet,
			direction,
			counterparty,
			type,
			amount,
			timestamp_seconds
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7
		);
	`, tx.Id, tx.Wallet, tx.Direction, tx.Counterparty, tx.Type, tx.Amount, tx.Timestamp)

	return err
}

func (a *AppDB) GetTransactions(ctx context.Context, wallet string, page uint, count uint) ([]*structs.FormattedTransaction, error) {
	offset := page * count

	rows, err := a.db.Query(ctx, `
		SELECT
			t.id,
			t.wallet,
			t.direction,
			t.counterparty,
			t.type,
			t.amount,
			t.timestamp_seconds
		FROM
			transactions t
		JOIN
			wallets w
		WHERE
			t.wallet = w.id
		AND
			w.id = $1
		ORDER BY
			t.timestamp_seconds
		DESC
		LIMIT
			$2
		OFFSET
			$3;
	`, wallet, count, offset)

	transactions := []*structs.FormattedTransaction{}
	for rows.Next() {
		var transaction structs.FormattedTransaction
		err = rows.Scan(
			&transaction.Id,
			&transaction.Wallet,
			&transaction.Direction,
			&transaction.Counterparty,
			&transaction.Type,
			&transaction.Amount,
			&transaction.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning transaction row %s", err)
		}
		transactions = append(transactions, &transaction)
	}

	return transactions, nil
}

func (a *AppDB) GetTransactionCount(ctx context.Context, wallet string) (uint64, error) {
	var count uint64

	row := a.db.QueryRow(ctx, `
		SELECT
			COUNT(id)
		AS
			total_rows
		FROM
			transactions t
		JOIN
			wallets w
		WHERE
			t.wallet = w.id
		AND
			w.id = $1;
	`, wallet)

	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (a *AppDB) UpdateTransactionTimestamp(ctx context.Context, id string, timestamp uint64) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			transactions
		SET
			timestamp_seconds = $1
		WHERE
			id = $2;
	`, timestamp, id)
	return err
}
