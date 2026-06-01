package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

const (
	PonderNotificationChannelEmail = "email"
	PonderNotificationChannelPush  = "push"

	maxPonderNotificationErrorMessageLength = 1000
)

func normalizePonderHookNotificationDestination(channel string, destination string) string {
	destination = strings.TrimSpace(destination)
	if strings.EqualFold(strings.TrimSpace(channel), PonderNotificationChannelEmail) {
		return strings.ToLower(destination)
	}
	return destination
}

func truncatePonderNotificationErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	runes := []rune(message)
	if len(runes) <= maxPonderNotificationErrorMessageLength {
		return message
	}
	return string(runes[:maxPonderNotificationErrorMessageLength])
}

func (a *AppDB) RecordPonderHookTrigger(ctx context.Context, hook structs.PonderHookData) (int64, error) {
	txHash := strings.ToLower(strings.TrimSpace(hook.Hash))
	fromAddress := strings.ToLower(strings.TrimSpace(hook.From))
	toAddress := strings.ToLower(strings.TrimSpace(hook.To))
	amount := strings.TrimSpace(hook.Amount)
	if txHash == "" || fromAddress == "" || toAddress == "" || amount == "" {
		return 0, fmt.Errorf("ponder hook trigger requires tx hash, from address, to address, and amount")
	}

	var id int64
	err := a.db.QueryRow(ctx, `
		INSERT INTO ponder_hook_triggers(
			tx_hash,
			from_address,
			to_address,
			amount
		) VALUES (
			$1,
			$2,
			$3,
			$4
		)
		ON CONFLICT (tx_hash, from_address, to_address, amount)
		DO UPDATE SET
			last_seen_at = NOW(),
			trigger_count = ponder_hook_triggers.trigger_count + 1
		RETURNING
			id;
	`, txHash, fromAddress, toAddress, amount).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("error recording ponder hook trigger for tx %s: %w", txHash, err)
	}

	return id, nil
}

func (a *AppDB) ClaimPonderHookNotificationDelivery(
	ctx context.Context,
	hookTriggerID int64,
	txHash string,
	channel string,
	destination string,
	owner string,
	subscriptionType string,
	subscriptionID int,
	address string,
) (int64, bool, error) {
	txHash = strings.ToLower(strings.TrimSpace(txHash))
	channel = strings.ToLower(strings.TrimSpace(channel))
	destination = normalizePonderHookNotificationDestination(channel, destination)
	owner = strings.TrimSpace(owner)
	subscriptionType = strings.TrimSpace(subscriptionType)
	address = strings.ToLower(strings.TrimSpace(address))
	if hookTriggerID <= 0 || txHash == "" || channel == "" || destination == "" {
		return 0, false, fmt.Errorf("ponder hook notification delivery requires hook trigger, tx hash, channel, and destination")
	}

	var deliveryID int64
	err := a.db.QueryRow(ctx, `
		INSERT INTO ponder_hook_notification_deliveries(
			hook_trigger_id,
			tx_hash,
			channel,
			destination,
			owner,
			subscription_type,
			subscription_id,
			address,
			status
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			'pending'
		)
		ON CONFLICT (tx_hash, channel, destination)
		DO NOTHING
		RETURNING
			id;
	`, hookTriggerID, txHash, channel, destination, owner, subscriptionType, subscriptionID, address).Scan(&deliveryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("error claiming ponder hook notification delivery for tx %s channel %s destination %s: %w", txHash, channel, destination, err)
	}

	return deliveryID, true, nil
}

func (a *AppDB) MarkPonderHookNotificationDeliverySent(ctx context.Context, deliveryID int64, providerReference string) error {
	if deliveryID <= 0 {
		return nil
	}

	_, err := a.db.Exec(ctx, `
		UPDATE ponder_hook_notification_deliveries
		SET
			status = 'sent',
			provider_reference = $2,
			error_message = '',
			sent_at = NOW(),
			failed_at = NULL,
			updated_at = NOW()
		WHERE
			id = $1;
	`, deliveryID, strings.TrimSpace(providerReference))
	if err != nil {
		return fmt.Errorf("error marking ponder hook notification delivery %d sent: %w", deliveryID, err)
	}

	return nil
}

func (a *AppDB) MarkPonderHookNotificationDeliveryFailed(ctx context.Context, deliveryID int64, message string) error {
	if deliveryID <= 0 {
		return nil
	}

	_, err := a.db.Exec(ctx, `
		UPDATE ponder_hook_notification_deliveries
		SET
			status = 'failed',
			provider_reference = '',
			error_message = $2,
			failed_at = NOW(),
			updated_at = NOW()
		WHERE
			id = $1;
	`, deliveryID, truncatePonderNotificationErrorMessage(message))
	if err != nil {
		return fmt.Errorf("error marking ponder hook notification delivery %d failed: %w", deliveryID, err)
	}

	return nil
}

func (a *AppDB) AddPonderSubscription(ctx context.Context, s *structs.PonderSubscription) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO ponder_subscriptions(
			id,
			address,
			type,
			owner,
			data
		) VALUES (
			$1,
			LOWER($2),
			$3,
			$4,
			$5
		)
		ON CONFLICT (id) DO UPDATE
		SET
			address = EXCLUDED.address,
			type = EXCLUDED.type,
			owner = EXCLUDED.owner,
			data = EXCLUDED.data,
			active = TRUE,
			delete_date = NULL,
			delete_reason = NULL;
	`, s.Id, s.Address, s.Type, s.Owner, s.Data)

	return err
}

func (a *AppDB) GetPonderSubscriptions(ctx context.Context, address string) ([]*structs.PonderSubscription, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			address,
			type,
			owner,
			data
		FROM
			ponder_subscriptions
		WHERE
			address = LOWER($1)
		AND
			active = TRUE
	`, address)
	if err != nil {
		return nil, fmt.Errorf("error querying for ponder subscriptions for address %s: %s", address, err)
	}

	var subscriptions []*structs.PonderSubscription
	for rows.Next() {
		var subscription structs.PonderSubscription
		err = rows.Scan(
			&subscription.Id,
			&subscription.Address,
			&subscription.Type,
			&subscription.Owner,
			&subscription.Data,
		)
		if err != nil {
			a.logger.Logf("error scanning row into subscription struct: %s", err)
			continue
		}

		subscriptions = append(subscriptions, &subscription)
	}

	return subscriptions, nil
}

func (a *AppDB) GetPonderSubscriptionsByUser(ctx context.Context, userDid string) ([]*structs.PonderSubscription, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			address,
			type,
			owner,
			data
		FROM
			ponder_subscriptions
		WHERE
			owner = $1
		AND
			active = TRUE
	`, userDid)
	if err != nil {
		return nil, fmt.Errorf("error querying for ponder subscriptions for userDid %s: %s", userDid, err)
	}

	var subscriptions []*structs.PonderSubscription
	for rows.Next() {
		var subscription structs.PonderSubscription
		err = rows.Scan(
			&subscription.Id,
			&subscription.Address,
			&subscription.Type,
			&subscription.Owner,
			&subscription.Data,
		)
		if err != nil {
			a.logger.Logf("error scanning row into subscription struct: %s", err)
			continue
		}

		subscriptions = append(subscriptions, &subscription)
	}

	return subscriptions, nil
}

func (a *AppDB) GetPonderSubscription(ctx context.Context, id int) (*structs.PonderSubscription, error) {
	rows := a.db.QueryRow(ctx, `
		SELECT
			id,
			address,
			type,
			owner,
			data
		FROM
			ponder_subscriptions
		WHERE
			id = $1
		AND
			active = TRUE
	`, id)

	var subscription structs.PonderSubscription
	err := rows.Scan(
		&subscription.Id,
		&subscription.Address,
		&subscription.Type,
		&subscription.Owner,
		&subscription.Data,
	)
	if err != nil {
		return nil, err
	}

	return &subscription, nil
}

func (a *AppDB) DeletePonderSubscription(ctx context.Context, id int, owner string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			ponder_subscriptions
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

func (a *AppDB) HasActivePonderNotificationDependency(ctx context.Context, address string) (bool, error) {
	var exists bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT
				1
			FROM
				ponder_subscriptions
			WHERE
				address = LOWER($1)
			AND
				active = TRUE
			UNION ALL
			SELECT
				1
			FROM
				mobile_push_subscriptions
			WHERE
				address = LOWER($1)
			AND
				active = TRUE
		);
	`, address).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking active ponder notification dependencies for address %s: %s", address, err)
	}

	return exists, nil
}

func (a *AppDB) GetKnownPonderHookIDsForAddress(ctx context.Context, address string) ([]int, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			ponder_subscriptions
		WHERE
			address = LOWER($1)
		UNION
		SELECT
			ponder_hook_id
		FROM
			mobile_push_subscriptions
		WHERE
			address = LOWER($1)
		AND
			ponder_hook_id IS NOT NULL
		ORDER BY
			id ASC;
	`, address)
	if err != nil {
		return nil, fmt.Errorf("error querying known ponder hook ids for address %s: %s", address, err)
	}
	defer rows.Close()

	hookIDs := make([]int, 0)
	for rows.Next() {
		var hookID int
		if err := rows.Scan(&hookID); err != nil {
			return nil, fmt.Errorf("error scanning ponder hook id for address %s: %s", address, err)
		}
		hookIDs = append(hookIDs, hookID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading ponder hook ids for address %s: %s", address, err)
	}

	return hookIDs, nil
}
