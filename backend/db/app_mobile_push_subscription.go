package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

const (
	deleteReasonPushPreferenceDisabled  = "push_preference_disabled"
	deleteReasonPushDeviceUnregistered  = "push_device_unregistered"
	deleteReasonPushDeviceNotRegistered = "push_device_not_registered"
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

func addUniquePushAddress(addresses map[string]struct{}, address string) {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return
	}
	addresses[address] = struct{}{}
}

func collectPushAddresses(rows pgx.Rows, addresses map[string]struct{}) error {
	defer rows.Close()

	for rows.Next() {
		var address string
		if err := rows.Scan(&address); err != nil {
			return err
		}
		addUniquePushAddress(addresses, address)
	}

	return rows.Err()
}

func mapKeysSortedByInput(values []string, present map[string]struct{}) []string {
	result := make([]string, 0, len(present))
	for _, value := range values {
		if _, ok := present[value]; ok {
			result = append(result, value)
			delete(present, value)
		}
	}
	for value := range present {
		result = append(result, value)
	}
	return result
}

func pushAddressMapValues(addresses map[string]struct{}) []string {
	result := make([]string, 0, len(addresses))
	for address := range addresses {
		result = append(result, address)
	}
	return result
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func pushInactiveReason(preferenceEnabled *bool, deviceRegistered *bool, fallback string) string {
	if preferenceEnabled != nil && !*preferenceEnabled {
		return deleteReasonPushPreferenceDisabled
	}
	if deviceRegistered != nil && !*deviceRegistered {
		return deleteReasonPushDeviceUnregistered
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return deleteReasonPonderDelete
}

func collectDisabledPushAddresses(rows pgx.Rows, addresses map[string]struct{}) error {
	defer rows.Close()

	for rows.Next() {
		var (
			address   string
			wasActive bool
			isActive  bool
		)
		if err := rows.Scan(&address, &wasActive, &isActive); err != nil {
			return err
		}
		if wasActive && !isActive {
			addUniquePushAddress(addresses, address)
		}
	}

	return rows.Err()
}

func (a *AppDB) SyncMobilePushSubscriptions(
	ctx context.Context,
	owner string,
	token string,
	addresses []string,
	ponderHookIDsByAddress map[string]int,
	preferenceEnabled *bool,
	deviceRegistered *bool,
	pruneMissing bool,
) ([]string, error) {
	normalizedToken := normalizePushToken(token)
	normalizedAddresses := normalizePushAddresses(addresses)
	disabledAddresses := make(map[string]struct{})
	preferenceEnabledArg := nullableBool(preferenceEnabled)
	deviceRegisteredArg := nullableBool(deviceRegistered)
	inactiveReason := pushInactiveReason(preferenceEnabled, deviceRegistered, deleteReasonPonderDelete)

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		UPDATE mobile_push_subscriptions
		SET
			active = FALSE,
			device_registered = FALSE,
			delete_date = $3::TIMESTAMPTZ,
			delete_reason = $4::TEXT
		WHERE
			token = $1
		AND
			owner <> $2
		AND
			active = TRUE
		RETURNING
			address;
	`, normalizedToken, owner, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonPonderDelete)
	if err != nil {
		return nil, fmt.Errorf("error clearing push subscriptions for other owners: %w", err)
	}
	if err := collectPushAddresses(rows, disabledAddresses); err != nil {
		return nil, fmt.Errorf("error reading cleared push subscriptions for other owners: %w", err)
	}

	if len(normalizedAddresses) == 0 && (preferenceEnabled != nil || deviceRegistered != nil) {
		rows, err := tx.Query(ctx, `
			WITH candidates AS (
				SELECT
					id,
					address,
					active AS was_active
				FROM
					mobile_push_subscriptions
				WHERE
					owner = $1
				AND
					token = $2
			)
			UPDATE mobile_push_subscriptions AS mps
			SET
				preference_enabled = COALESCE($3::BOOLEAN, mps.preference_enabled),
				device_registered = COALESCE($4::BOOLEAN, mps.device_registered),
				active = COALESCE($3::BOOLEAN, mps.preference_enabled) AND COALESCE($4::BOOLEAN, mps.device_registered),
				delete_date = CASE
					WHEN COALESCE($3::BOOLEAN, mps.preference_enabled) AND COALESCE($4::BOOLEAN, mps.device_registered)
						THEN NULL::TIMESTAMPTZ
					ELSE $5::TIMESTAMPTZ
				END,
				delete_reason = CASE
					WHEN COALESCE($3::BOOLEAN, mps.preference_enabled) AND COALESCE($4::BOOLEAN, mps.device_registered)
						THEN NULL::TEXT
					ELSE $6::TEXT
				END,
				updated_at = NOW()
			FROM
				candidates
			WHERE
				mps.id = candidates.id
			RETURNING
				mps.address,
				candidates.was_active,
				mps.active;
		`, owner, normalizedToken, preferenceEnabledArg, deviceRegisteredArg, time.Now().UTC().Add(accountDeletionGracePeriod), inactiveReason)
		if err != nil {
			return nil, fmt.Errorf("error updating mobile push registration state: %w", err)
		}
		if err := collectDisabledPushAddresses(rows, disabledAddresses); err != nil {
			return nil, fmt.Errorf("error reading updated mobile push registration state: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return mapKeysSortedByInput(normalizedAddresses, disabledAddresses), nil
	}

	if pruneMissing {
		rows, err = tx.Query(ctx, `
			WITH candidates AS (
				SELECT
					id,
					address,
					active AS was_active
				FROM
					mobile_push_subscriptions
				WHERE
					owner = $1
				AND
					token = $2
				AND
					LOWER(address) <> ALL($3)
			)
			UPDATE mobile_push_subscriptions AS mps
			SET
				preference_enabled = FALSE,
				active = FALSE,
				delete_date = $4::TIMESTAMPTZ,
				delete_reason = $5::TEXT,
				updated_at = NOW()
			FROM
				candidates
			WHERE
				mps.id = candidates.id
			RETURNING
				mps.address,
				candidates.was_active,
				mps.active;
		`, owner, normalizedToken, normalizedAddresses, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonPushPreferenceDisabled)
		if err != nil {
			return nil, fmt.Errorf("error pruning mobile push subscriptions: %w", err)
		}
		if err := collectDisabledPushAddresses(rows, disabledAddresses); err != nil {
			return nil, fmt.Errorf("error reading pruned mobile push subscriptions: %w", err)
		}
	}

	if len(normalizedAddresses) > 0 {
		rows, err = tx.Query(ctx, `
			WITH candidates AS (
				SELECT
					id,
					address,
					active AS was_active
				FROM
					mobile_push_subscriptions
				WHERE
					owner = $1
				AND
					token = $2
				AND
					LOWER(address) = ANY($3)
			)
			UPDATE mobile_push_subscriptions AS mps
			SET
				preference_enabled = COALESCE($4::BOOLEAN, mps.preference_enabled),
				device_registered = COALESCE($5::BOOLEAN, mps.device_registered),
				active = COALESCE($4::BOOLEAN, mps.preference_enabled) AND COALESCE($5::BOOLEAN, mps.device_registered),
				ponder_hook_id = COALESCE($6::INTEGER, mps.ponder_hook_id),
				delete_date = CASE
					WHEN COALESCE($4::BOOLEAN, mps.preference_enabled) AND COALESCE($5::BOOLEAN, mps.device_registered)
						THEN NULL::TIMESTAMPTZ
					ELSE $7::TIMESTAMPTZ
				END,
				delete_reason = CASE
					WHEN COALESCE($4::BOOLEAN, mps.preference_enabled) AND COALESCE($5::BOOLEAN, mps.device_registered)
						THEN NULL::TEXT
					ELSE $8::TEXT
				END,
				updated_at = NOW()
			FROM
				candidates
			WHERE
				mps.id = candidates.id
			RETURNING
				mps.address,
				candidates.was_active,
				mps.active;
		`, owner, normalizedToken, normalizedAddresses, preferenceEnabledArg, deviceRegisteredArg, nil, time.Now().UTC().Add(accountDeletionGracePeriod), inactiveReason)
		if err != nil {
			return nil, fmt.Errorf("error updating mobile push subscriptions: %w", err)
		}
		if err := collectDisabledPushAddresses(rows, disabledAddresses); err != nil {
			return nil, fmt.Errorf("error reading updated mobile push subscriptions: %w", err)
		}
	}

	for _, address := range normalizedAddresses {
		hookID, hasHookID := ponderHookIDsByAddress[address]
		if preferenceEnabled != nil && !*preferenceEnabled {
			continue
		}
		if preferenceEnabled == nil && !hasHookID {
			continue
		}
		var ponderHookID any
		if hasHookID && hookID > 0 {
			ponderHookID = hookID
		}
		nextDeviceRegistered := true
		if deviceRegistered != nil {
			nextDeviceRegistered = *deviceRegistered
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO mobile_push_subscriptions(
				owner,
				token,
				address,
				ponder_hook_id,
				preference_enabled,
				device_registered,
				active
			) VALUES (
				$1,
				$2,
				$3,
				$4,
				TRUE,
				$5,
				$5
			)
			ON CONFLICT (token, address) DO UPDATE
			SET
				owner = EXCLUDED.owner,
				preference_enabled = TRUE,
				device_registered = EXCLUDED.device_registered,
				active = EXCLUDED.device_registered,
				ponder_hook_id = COALESCE(EXCLUDED.ponder_hook_id, mobile_push_subscriptions.ponder_hook_id),
				delete_date = NULL,
				delete_reason = NULL,
				updated_at = NOW();
		`, owner, normalizedToken, address, ponderHookID, nextDeviceRegistered); err != nil {
			return nil, fmt.Errorf("error upserting mobile push subscription for %s: %w", address, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return mapKeysSortedByInput(normalizedAddresses, disabledAddresses), nil
}

func scanMobilePushSubscriptions(
	rows pgx.Rows,
) ([]*structs.MobilePushSubscription, error) {
	defer rows.Close()

	subscriptions := make([]*structs.MobilePushSubscription, 0)
	for rows.Next() {
		var (
			subscription structs.MobilePushSubscription
			ponderHookID sql.NullInt64
		)
		if err := rows.Scan(
			&subscription.Id,
			&subscription.Owner,
			&subscription.Address,
			&subscription.Token,
			&subscription.Active,
			&subscription.PreferenceEnabled,
			&subscription.DeviceRegistered,
			&ponderHookID,
		); err != nil {
			return nil, err
		}
		if ponderHookID.Valid {
			hookID := int(ponderHookID.Int64)
			subscription.PonderHookId = &hookID
		}
		subscription.Type = structs.PushSubscription
		subscription.Data = []byte(subscription.Token)

		subscriptions = append(subscriptions, &subscription)
	}

	return subscriptions, rows.Err()
}

func scanMobilePushSubscriptionRecord(row pgx.Row) (*structs.MobilePushSubscription, error) {
	var (
		subscription structs.MobilePushSubscription
		ponderHookID sql.NullInt64
	)
	err := row.Scan(
		&subscription.Id,
		&subscription.Owner,
		&subscription.Token,
		&subscription.Address,
		&subscription.Active,
		&subscription.PreferenceEnabled,
		&subscription.DeviceRegistered,
		&ponderHookID,
	)
	if err != nil {
		return nil, err
	}
	if ponderHookID.Valid {
		hookID := int(ponderHookID.Int64)
		subscription.PonderHookId = &hookID
	}
	subscription.Type = structs.PushSubscription
	subscription.Data = []byte(subscription.Token)

	return &subscription, nil
}

func (a *AppDB) GetMobilePushSubscriptionsByOwnerToken(
	ctx context.Context,
	owner string,
	token string,
) ([]*structs.MobilePushSubscription, error) {
	normalizedToken := normalizePushToken(token)
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			owner,
			token,
			address,
			active,
			preference_enabled,
			device_registered,
			ponder_hook_id
		FROM
			mobile_push_subscriptions
		WHERE
			owner = $1
		AND
			token = $2
		ORDER BY
			id ASC;
	`, owner, normalizedToken)
	if err != nil {
		return nil, fmt.Errorf("error querying mobile push subscriptions for owner %s and token: %w", owner, err)
	}
	defer rows.Close()

	subscriptions := make([]*structs.MobilePushSubscription, 0)
	for rows.Next() {
		subscription, err := scanMobilePushSubscriptionRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning mobile push subscription for owner %s and token: %w", owner, err)
		}
		subscriptions = append(subscriptions, subscription)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading mobile push subscriptions for owner %s and token: %w", owner, err)
	}

	return subscriptions, nil
}

func (a *AppDB) GetMobilePushSubscription(
	ctx context.Context,
	id int,
	owner string,
) (*structs.MobilePushSubscription, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id,
			owner,
			token,
			address,
			active,
			preference_enabled,
			device_registered,
			ponder_hook_id
		FROM
			mobile_push_subscriptions
		WHERE
			id = $1
		AND
			owner = $2;
	`, id, owner)

	return scanMobilePushSubscriptionRecord(row)
}

func (a *AppDB) GetMobilePushSubscriptionsByUser(
	ctx context.Context,
	userDid string,
) ([]*structs.MobilePushSubscription, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			owner,
			address,
			token,
			active,
			preference_enabled,
			device_registered,
			ponder_hook_id
		FROM
			mobile_push_subscriptions
		WHERE
			owner = $1
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
) ([]*structs.MobilePushSubscription, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			owner,
			address,
			token,
			active,
			preference_enabled,
			device_registered,
			ponder_hook_id
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
) (string, error) {
	var address string
	err := a.db.QueryRow(ctx, `
		UPDATE mobile_push_subscriptions
		SET
			preference_enabled = FALSE,
			active = FALSE,
			delete_date = $3::TIMESTAMPTZ,
			delete_reason = $4::TEXT
		WHERE
			id = $1
		AND
			owner = $2
		RETURNING
			address;
	`, id, owner, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonPushPreferenceDisabled).Scan(&address)
	return strings.ToLower(strings.TrimSpace(address)), err
}

func (a *AppDB) DeactivateMobilePushSubscriptionsByToken(
	ctx context.Context,
	token string,
	reason string,
) ([]string, error) {
	normalizedToken := normalizePushToken(token)
	if normalizedToken == "" {
		return nil, nil
	}
	if strings.TrimSpace(reason) == "" {
		reason = deleteReasonPushDeviceNotRegistered
	}

	rows, err := a.db.Query(ctx, `
		UPDATE mobile_push_subscriptions
		SET
			device_registered = FALSE,
			active = FALSE,
			delete_date = $2::TIMESTAMPTZ,
			delete_reason = $3::TEXT
		WHERE
			token = $1
		AND
			active = TRUE
		RETURNING
			address;
	`, normalizedToken, time.Now().UTC().Add(accountDeletionGracePeriod), reason)
	if err != nil {
		return nil, fmt.Errorf("error deactivating mobile push subscriptions for token: %w", err)
	}

	disabledAddresses := make(map[string]struct{})
	if err := collectPushAddresses(rows, disabledAddresses); err != nil {
		return nil, fmt.Errorf("error reading deactivated mobile push subscriptions for token: %w", err)
	}

	return pushAddressMapValues(disabledAddresses), nil
}

func (a *AppDB) SetMobilePushSubscriptionPonderHook(
	ctx context.Context,
	owner string,
	token string,
	address string,
	hookID int,
) error {
	normalizedToken := normalizePushToken(token)
	normalizedAddress := strings.ToLower(strings.TrimSpace(address))
	if normalizedToken == "" || normalizedAddress == "" || hookID <= 0 {
		return nil
	}

	tag, err := a.db.Exec(ctx, `
		UPDATE mobile_push_subscriptions
		SET
			ponder_hook_id = $4,
			updated_at = NOW()
		WHERE
			owner = $1
		AND
			token = $2
		AND
			address = $3;
	`, owner, normalizedToken, normalizedAddress, hookID)
	if err != nil {
		return fmt.Errorf("error setting mobile push ponder hook for address %s: %w", normalizedAddress, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mobile push subscription for address %s was not found", normalizedAddress)
	}

	return nil
}

func (a *AppDB) ClearMobilePushSubscriptionPonderHook(
	ctx context.Context,
	hookID int,
) error {
	if hookID <= 0 {
		return nil
	}

	_, err := a.db.Exec(ctx, `
		UPDATE mobile_push_subscriptions
		SET
			ponder_hook_id = NULL,
			updated_at = NOW()
		WHERE
			ponder_hook_id = $1;
	`, hookID)
	if err != nil {
		return fmt.Errorf("error clearing mobile push ponder hook %d: %w", hookID, err)
	}

	return nil
}

func (a *AppDB) AddMobilePushNotificationTicket(
	ctx context.Context,
	owner string,
	token string,
	address string,
	ticketID string,
) error {
	normalizedToken := normalizePushToken(token)
	normalizedAddress := strings.ToLower(strings.TrimSpace(address))
	ticketID = strings.TrimSpace(ticketID)
	if normalizedToken == "" || normalizedAddress == "" || ticketID == "" {
		return nil
	}

	_, err := a.db.Exec(ctx, `
		INSERT INTO mobile_push_notification_tickets(
			ticket_id,
			owner,
			token,
			address,
			status
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			'pending'
		)
		ON CONFLICT (ticket_id) DO UPDATE
		SET
			owner = EXCLUDED.owner,
			token = EXCLUDED.token,
			address = EXCLUDED.address,
			status = 'pending',
			receipt_status = NULL,
			receipt_message = NULL,
			receipt_error_code = NULL,
			checked_at = NULL;
	`, ticketID, owner, normalizedToken, normalizedAddress)
	if err != nil {
		return fmt.Errorf("error storing mobile push notification ticket %s: %w", ticketID, err)
	}

	return nil
}

func (a *AppDB) MarkMobilePushNotificationTicketReceipt(
	ctx context.Context,
	ticketID string,
	status string,
	message string,
	errorCode string,
) error {
	ticketID = strings.TrimSpace(ticketID)
	if ticketID == "" {
		return nil
	}

	_, err := a.db.Exec(ctx, `
		UPDATE mobile_push_notification_tickets
		SET
			status = CASE
				WHEN $2 = 'error' THEN 'error'
				WHEN $2 = 'ok' THEN 'ok'
				ELSE status
			END,
			receipt_status = $2,
			receipt_message = $3,
			receipt_error_code = $4,
			checked_at = NOW()
		WHERE
			ticket_id = $1;
	`, ticketID, strings.TrimSpace(status), strings.TrimSpace(message), strings.TrimSpace(errorCode))
	if err != nil {
		return fmt.Errorf("error marking mobile push notification ticket %s receipt: %w", ticketID, err)
	}

	return nil
}
