package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func normalizePushToken(value string) string {
	return strings.TrimSpace(value)
}

func normalizePushAddresses(addresses []string) []string {
	seen := make(map[string]struct{}, len(addresses))
	normalized := make([]string, 0, len(addresses))

	for _, rawAddress := range addresses {
		address := strings.ToLower(strings.TrimSpace(rawAddress))
		if address == "" {
			continue
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		normalized = append(normalized, address)
	}

	return normalized
}

func (a *AppDB) SyncMobilePushSubscriptions(
	ctx context.Context,
	owner string,
	token string,
	addresses []string,
) error {
	normalizedToken := normalizePushToken(token)
	normalizedAddresses := normalizePushAddresses(addresses)

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE mobile_push_subscriptions
		SET
			active = FALSE,
			delete_date = $3,
			delete_reason = $4
		WHERE
			token = $1
		AND
			owner <> $2
		AND
			active = TRUE;
	`, normalizedToken, owner, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonPonderDelete); err != nil {
		return fmt.Errorf("error clearing push subscriptions for other owners: %w", err)
	}

	if len(normalizedAddresses) == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE mobile_push_subscriptions
			SET
				active = FALSE,
				delete_date = $3,
				delete_reason = $4
			WHERE
				owner = $1
			AND
				token = $2
			AND
				active = TRUE;
		`, owner, normalizedToken, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonPonderDelete); err != nil {
			return fmt.Errorf("error disabling push subscriptions: %w", err)
		}

		return tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE mobile_push_subscriptions
		SET
			active = FALSE,
			delete_date = $4,
			delete_reason = $5
		WHERE
			owner = $1
		AND
			token = $2
		AND
			address <> ALL($3)
		AND
			active = TRUE;
	`, owner, normalizedToken, normalizedAddresses, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonPonderDelete); err != nil {
		return fmt.Errorf("error pruning mobile push subscriptions: %w", err)
	}

	for _, address := range normalizedAddresses {
		if _, err := tx.Exec(ctx, `
			INSERT INTO mobile_push_subscriptions(
				owner,
				token,
				address
			) VALUES (
				$1,
				$2,
				$3
			)
			ON CONFLICT (token, address) DO UPDATE
			SET
				owner = EXCLUDED.owner,
				active = TRUE,
				delete_date = NULL,
				delete_reason = NULL;
		`, owner, normalizedToken, address); err != nil {
			return fmt.Errorf("error upserting mobile push subscription for %s: %w", address, err)
		}
	}

	return tx.Commit(ctx)
}

func scanMobilePushSubscriptions(
	rows pgx.Rows,
) ([]*structs.PonderSubscription, error) {
	defer rows.Close()

	subscriptions := make([]*structs.PonderSubscription, 0)
	for rows.Next() {
		var (
			id      int
			owner   string
			address string
			token   string
		)
		if err := rows.Scan(&id, &owner, &address, &token); err != nil {
			return nil, err
		}

		subscriptions = append(subscriptions, &structs.PonderSubscription{
			Id:      id,
			Owner:   owner,
			Address: address,
			Type:    structs.PushSubscription,
			Data:    []byte(token),
		})
	}

	return subscriptions, rows.Err()
}

func (a *AppDB) GetMobilePushSubscriptionsByUser(
	ctx context.Context,
	userDid string,
) ([]*structs.PonderSubscription, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			owner,
			address,
			token
		FROM
			mobile_push_subscriptions
		WHERE
			owner = $1
		AND
			active = TRUE
		ORDER BY
			id ASC;
	`, userDid)
	if err != nil {
		return nil, fmt.Errorf("error querying for mobile push subscriptions for user %s: %w", userDid, err)
	}

	return scanMobilePushSubscriptions(rows)
}

func (a *AppDB) GetMobilePushSubscriptionsByAddress(
	ctx context.Context,
	address string,
) ([]*structs.PonderSubscription, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			owner,
			address,
			token
		FROM
			mobile_push_subscriptions
		WHERE
			address = LOWER($1)
		AND
			active = TRUE
		ORDER BY
			id ASC;
	`, address)
	if err != nil {
		return nil, fmt.Errorf("error querying for mobile push subscriptions for address %s: %w", address, err)
	}

	return scanMobilePushSubscriptions(rows)
}

func (a *AppDB) DeleteMobilePushSubscription(
	ctx context.Context,
	id int,
	owner string,
) error {
	_, err := a.db.Exec(ctx, `
		UPDATE mobile_push_subscriptions
		SET
			active = FALSE,
			delete_date = $3,
			delete_reason = $4
		WHERE
			id = $1
		AND
			owner = $2
		AND
			active = TRUE;
	`, id, owner, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonPonderDelete)
	return err
}
