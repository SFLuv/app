package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

// InsertUnwrap records one withdrawTo cash-out in the unwraps audit ledger.
func (a *AppDB) InsertUnwrap(ctx context.Context, entry *structs.UnwrapLedgerEntry) (int, error) {
	var id int
	err := a.db.QueryRow(ctx, `
		INSERT INTO unwraps (
			user_id,
			wallet_id,
			wallet_address,
			location_id,
			destination_address,
			amount_wei,
			tx_hash,
			status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id;
	`,
		entry.UserID,
		entry.WalletID,
		entry.WalletAddress,
		entry.LocationID,
		entry.DestinationAddress,
		entry.AmountWei,
		entry.TxHash,
		entry.Status,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("error inserting unwrap ledger entry: %w", err)
	}

	return id, nil
}

// GetUnwrapsByUser returns a user's cash-out history, newest first.
func (a *AppDB) GetUnwrapsByUser(ctx context.Context, userID string, limit int) ([]*structs.UnwrapLedgerEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			user_id,
			wallet_id,
			wallet_address,
			location_id,
			destination_address,
			amount_wei,
			tx_hash,
			status,
			created_at
		FROM unwraps
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2;
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("error querying unwrap ledger: %w", err)
	}
	defer rows.Close()

	entries := []*structs.UnwrapLedgerEntry{}
	for rows.Next() {
		var entry structs.UnwrapLedgerEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.UserID,
			&entry.WalletID,
			&entry.WalletAddress,
			&entry.LocationID,
			&entry.DestinationAddress,
			&entry.AmountWei,
			&entry.TxHash,
			&entry.Status,
			&entry.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning unwrap ledger entry: %w", err)
		}
		entries = append(entries, &entry)
	}

	return entries, nil
}
