package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) GetAnalyticsWalletOwners(ctx context.Context) ([]*structs.AnalyticsWalletOwner, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			w.owner,
			u.is_admin,
			u.is_merchant,
			LOWER(TRIM(w.eoa_address)),
			LOWER(TRIM(COALESCE(w.smart_address, '')))
		FROM
			wallets w
		JOIN
			users u
		ON
			u.id = w.owner
		WHERE
			COALESCE(w.active, TRUE) = TRUE
		AND
			COALESCE(u.active, TRUE) = TRUE;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying analytics wallet owners: %w", err)
	}
	defer rows.Close()

	owners := make([]*structs.AnalyticsWalletOwner, 0)
	seen := make(map[string]struct{})
	for rows.Next() {
		var ownerID string
		var isAdmin bool
		var isMerchant bool
		var eoaAddress string
		var smartAddress string
		if err := rows.Scan(&ownerID, &isAdmin, &isMerchant, &eoaAddress, &smartAddress); err != nil {
			return nil, fmt.Errorf("error scanning analytics wallet owner: %w", err)
		}

		appendOwner := func(address string) {
			normalized := strings.TrimSpace(strings.ToLower(address))
			if normalized == "" {
				return
			}
			key := ownerID + "::" + normalized
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
			owners = append(owners, &structs.AnalyticsWalletOwner{
				UserID:     ownerID,
				Address:    normalized,
				IsAdmin:    isAdmin,
				IsMerchant: isMerchant,
			})
		}

		appendOwner(eoaAddress)
		appendOwner(smartAddress)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analytics wallet owners: %w", err)
	}

	return owners, nil
}

func (a *AppDB) GetAnalyticsMerchantWallets(ctx context.Context) ([]*structs.AnalyticsMerchantWallet, error) {
	rows, err := a.db.Query(ctx, `
		WITH approved_locations AS (
			SELECT
				id,
				owner_id,
				COALESCE(NULLIF(TRIM(name), ''), 'Approved Merchant') AS location_name,
				LOWER(TRIM(COALESCE(tipping_wallet_address, ''))) AS tipping_wallet_address
			FROM
				locations
			WHERE
				approval = TRUE
			AND
				owner_id IS NOT NULL
			AND
				COALESCE(active, TRUE) = TRUE
		),
		merchant_addresses AS (
			SELECT
				al.owner_id,
				al.id AS location_id,
				al.location_name,
				LOWER(TRIM(w.eoa_address)) AS address
			FROM
				approved_locations al
			JOIN
				wallets w
			ON
				w.owner = al.owner_id
			WHERE
				COALESCE(w.active, TRUE) = TRUE
			UNION
			SELECT
				al.owner_id,
				al.id AS location_id,
				al.location_name,
				LOWER(TRIM(COALESCE(w.smart_address, ''))) AS address
			FROM
				approved_locations al
			JOIN
				wallets w
			ON
				w.owner = al.owner_id
			WHERE
				COALESCE(w.active, TRUE) = TRUE
			UNION
			SELECT
				al.owner_id,
				al.id AS location_id,
				al.location_name,
				LOWER(TRIM(lpw.wallet_address)) AS address
			FROM
				approved_locations al
			JOIN
				location_payment_wallets lpw
			ON
				lpw.location_id = al.id
			WHERE
				COALESCE(lpw.active, TRUE) = TRUE
			UNION
			SELECT
				al.owner_id,
				al.id AS location_id,
				al.location_name,
				al.tipping_wallet_address AS address
			FROM
				approved_locations al
		)
		SELECT DISTINCT
			owner_id,
			location_id,
			location_name,
			address
		FROM
			merchant_addresses
		WHERE
			address <> ''
		ORDER BY
			location_name ASC,
			address ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying analytics merchant wallets: %w", err)
	}
	defer rows.Close()

	wallets := make([]*structs.AnalyticsMerchantWallet, 0)
	for rows.Next() {
		var wallet structs.AnalyticsMerchantWallet
		if err := rows.Scan(&wallet.OwnerID, &wallet.LocationID, &wallet.LocationName, &wallet.Address); err != nil {
			return nil, fmt.Errorf("error scanning analytics merchant wallet: %w", err)
		}
		wallets = append(wallets, &wallet)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analytics merchant wallets: %w", err)
	}

	return wallets, nil
}

func (a *AppDB) GetAnalyticsActiveUserCount(ctx context.Context) (int, error) {
	var count int
	if err := a.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM users
		WHERE COALESCE(active, TRUE) = TRUE;
	`).Scan(&count); err != nil {
		return 0, fmt.Errorf("error counting active users for analytics: %w", err)
	}
	return count, nil
}

func (a *AppDB) GetAnalyticsWorkflowCosts(ctx context.Context) ([]*structs.AnalyticsWorkflowCost, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			w.id,
			COALESCE(
				w.manager_paid_out_at,
				MAX(ws.completed_at),
				w.updated_at,
				w.start_at,
				w.created_at
			) AS completed_at,
			(
				CASE
					WHEN COALESCE(w.total_bounty, 0) > 0 THEN COALESCE(w.total_bounty, 0)
					ELSE COALESCE(SUM(ws.bounty), 0)
				END
				+ COALESCE(w.manager_bounty, 0)
			)::text AS cost_wei
		FROM
			workflows w
		LEFT JOIN
			workflow_steps ws
		ON
			ws.workflow_id = w.id
		WHERE
			w.status IN ('completed', 'paid_out')
		GROUP BY
			w.id
		ORDER BY
			completed_at ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying analytics workflow costs: %w", err)
	}
	defer rows.Close()

	costs := make([]*structs.AnalyticsWorkflowCost, 0)
	for rows.Next() {
		var row structs.AnalyticsWorkflowCost
		var completedAt sql.NullInt64
		if err := rows.Scan(&row.ID, &completedAt, &row.CostWei); err != nil {
			return nil, fmt.Errorf("error scanning analytics workflow cost: %w", err)
		}
		if completedAt.Valid {
			row.CompletedAt = completedAt.Int64
		}
		costs = append(costs, &row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analytics workflow costs: %w", err)
	}

	return costs, nil
}
