package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

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

type AnalyticsWalletRoleCandidate struct {
	Address    string
	Role       string
	ChainID    int64
	UserID     string
	LocationID int
	Source     string
}

func (a *AppDB) SyncAnalyticsWalletRoleHistory(ctx context.Context, chainID int64, candidates []AnalyticsWalletRoleCandidate) error {
	normalized := make([]AnalyticsWalletRoleCandidate, 0, len(candidates))
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		address := strings.TrimSpace(strings.ToLower(candidate.Address))
		role := strings.TrimSpace(strings.ToLower(candidate.Role))
		if address == "" || role == "" || chainID == 0 {
			continue
		}
		source := strings.TrimSpace(candidate.Source)
		key := fmt.Sprintf("%s|%s|%d|%s|%d|%s", address, role, chainID, candidate.UserID, candidate.LocationID, source)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidate.Address = address
		candidate.Role = role
		candidate.ChainID = chainID
		candidate.Source = source
		normalized = append(normalized, candidate)
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error beginning analytics wallet role sync: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_analytics_wallet_roles(
			address TEXT NOT NULL,
			role TEXT NOT NULL,
			chain_id BIGINT NOT NULL,
			user_id TEXT NOT NULL DEFAULT '',
			location_id INTEGER NOT NULL DEFAULT 0,
			source TEXT NOT NULL DEFAULT ''
		) ON COMMIT DROP;
	`); err != nil {
		return fmt.Errorf("error creating analytics wallet role sync temp table: %w", err)
	}

	for _, candidate := range normalized {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tmp_analytics_wallet_roles(address, role, chain_id, user_id, location_id, source)
			VALUES($1, $2, $3, $4, $5, $6);
		`, candidate.Address, candidate.Role, candidate.ChainID, candidate.UserID, candidate.LocationID, candidate.Source); err != nil {
			return fmt.Errorf("error staging analytics wallet role %s %s: %w", candidate.Role, candidate.Address, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE analytics_wallet_role_history h
		SET
			ended_at = NOW(),
			updated_at = NOW()
		WHERE
			h.ended_at IS NULL
		AND
			(
				h.chain_id <> $1
				OR NOT EXISTS (
					SELECT 1
					FROM tmp_analytics_wallet_roles t
					WHERE LOWER(h.address) = t.address
					AND h.role = t.role
					AND h.chain_id = t.chain_id
					AND COALESCE(h.user_id, '') = t.user_id
					AND COALESCE(h.location_id, 0) = t.location_id
					AND h.source = t.source
				)
			);
	`, chainID); err != nil {
		return fmt.Errorf("error closing inactive analytics wallet role records: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO analytics_wallet_role_history(address, role, chain_id, user_id, location_id, source, started_at)
		SELECT
			t.address,
			t.role,
			t.chain_id,
			NULLIF(t.user_id, ''),
			NULLIF(t.location_id, 0),
			t.source,
			CASE
				WHEN EXISTS (
					SELECT 1
					FROM analytics_wallet_role_history existing
					WHERE LOWER(existing.address) = t.address
					AND existing.role = t.role
					AND existing.chain_id = t.chain_id
					AND COALESCE(existing.user_id, '') = t.user_id
					AND COALESCE(existing.location_id, 0) = t.location_id
					AND existing.source = t.source
				) THEN NOW()
				ELSE TO_TIMESTAMP(0)
			END
		FROM
			tmp_analytics_wallet_roles t
		WHERE NOT EXISTS (
			SELECT 1
			FROM analytics_wallet_role_history h
			WHERE h.ended_at IS NULL
			AND LOWER(h.address) = t.address
			AND h.role = t.role
			AND h.chain_id = t.chain_id
			AND COALESCE(h.user_id, '') = t.user_id
			AND COALESCE(h.location_id, 0) = t.location_id
			AND h.source = t.source
		);
	`); err != nil {
		return fmt.Errorf("error inserting active analytics wallet role records: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing analytics wallet role sync: %w", err)
	}
	return nil
}

func (a *AppDB) GetAnalyticsWalletRoleHistory(ctx context.Context) ([]*structs.AnalyticsWalletRoleRecord, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			LOWER(address),
			role,
			chain_id,
			COALESCE(user_id, ''),
			COALESCE(location_id, 0),
			EXTRACT(EPOCH FROM started_at)::bigint,
			COALESCE(EXTRACT(EPOCH FROM ended_at)::bigint, 0)
		FROM
			analytics_wallet_role_history
		ORDER BY
			started_at ASC,
			id ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying analytics wallet role history: %w", err)
	}
	defer rows.Close()

	records := make([]*structs.AnalyticsWalletRoleRecord, 0)
	for rows.Next() {
		var record structs.AnalyticsWalletRoleRecord
		if err := rows.Scan(&record.Address, &record.Role, &record.ChainID, &record.UserID, &record.LocationID, &record.StartedAt, &record.EndedAt); err != nil {
			return nil, fmt.Errorf("error scanning analytics wallet role history: %w", err)
		}
		records = append(records, &record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analytics wallet role history: %w", err)
	}
	return records, nil
}

func (a *AppDB) RecordAnalyticsUserActivity(ctx context.Context, userID string, platform string, observedAt time.Time) error {
	userID = strings.TrimSpace(userID)
	platform = strings.TrimSpace(strings.ToLower(platform))
	if userID == "" {
		return nil
	}
	if platform == "" {
		platform = "web"
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	_, err := a.db.Exec(ctx, `
		INSERT INTO analytics_user_activity(user_id, activity_date, platform, first_seen_at, last_seen_at)
		VALUES($1, $2, $3, $4, $4)
		ON CONFLICT(user_id, activity_date, platform)
		DO UPDATE SET
			last_seen_at = GREATEST(analytics_user_activity.last_seen_at, EXCLUDED.last_seen_at);
	`, userID, observedAt.UTC().Format("2006-01-02"), platform, observedAt.UTC())
	if err != nil {
		return fmt.Errorf("error recording analytics user activity: %w", err)
	}
	return nil
}

func (a *AppDB) GetAnalyticsUserActivity(ctx context.Context, start time.Time) (map[int64]map[string]struct{}, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			EXTRACT(EPOCH FROM activity_date::timestamp)::bigint AS activity_day,
			user_id
		FROM
			analytics_user_activity
		WHERE
			activity_date >= $1::date;
	`, start.UTC().Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("error querying analytics user activity: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]map[string]struct{})
	for rows.Next() {
		var day int64
		var userID string
		if err := rows.Scan(&day, &userID); err != nil {
			return nil, fmt.Errorf("error scanning analytics user activity: %w", err)
		}
		if result[day] == nil {
			result[day] = make(map[string]struct{})
		}
		result[day][userID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analytics user activity: %w", err)
	}
	return result, nil
}
