package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Admin liquidation addresses are stored per admin user, separate from the
// per-location merchant addresses in app_location_wallet.go. Admins do not own
// locations, so their cash-out destination hangs off the user row instead.

func (a *AppDB) GetAdminLiquidationAddress(ctx context.Context, userID string) (string, error) {
	var liquidationAddress string
	err := a.db.QueryRow(ctx, `
		SELECT
			COALESCE(NULLIF(TRIM(liquidation_address), ''), '')
		FROM
			users
		WHERE
			id = $1
		AND
			active = TRUE;
	`, userID).Scan(&liquidationAddress)
	if err == pgx.ErrNoRows {
		return "", pgx.ErrNoRows
	}
	if err != nil {
		return "", fmt.Errorf("error reading admin liquidation address: %w", err)
	}

	return liquidationAddress, nil
}

// UpdateAdminLiquidationAddress sets (or clears, with an empty address) the
// admin's own cash-out destination.
func (a *AppDB) UpdateAdminLiquidationAddress(ctx context.Context, userID string, liquidationAddress string) (string, error) {
	normalizedAddress := strings.TrimSpace(liquidationAddress)
	if normalizedAddress != "" {
		var err error
		normalizedAddress, err = normalizeEthereumAddressForField(normalizedAddress, "liquidation address")
		if err != nil {
			return "", err
		}
	}

	tag, err := a.db.Exec(ctx, `
		UPDATE users
		SET liquidation_address = $1
		WHERE id = $2
		AND active = TRUE;
	`, normalizedAddress, userID)
	if err != nil {
		return "", fmt.Errorf("error updating admin liquidation address: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", pgx.ErrNoRows
	}

	return normalizedAddress, nil
}
