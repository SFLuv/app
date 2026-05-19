package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrMerchantModeForbidden    = errors.New("merchant mode is only available to merchants")
	ErrMerchantModePINRequired  = errors.New("set a merchant mode PIN before enabling merchant mode")
	ErrMerchantModeOldPINNeeded = errors.New("enter the current merchant mode PIN before resetting it")
	ErrMerchantModeInvalidPIN   = errors.New("merchant mode PIN must be exactly 6 digits")
	ErrMerchantModeBadPIN       = errors.New("incorrect merchant mode PIN")
	ErrMerchantModePINLocked    = errors.New("merchant mode PIN is temporarily locked")
	ErrMerchantModeDeviceNeeded = errors.New("merchant mode installation ID is required")
)

const merchantModePINLockoutThreshold = 5

func validateMerchantModePIN(pin string) error {
	if len(pin) != 6 {
		return ErrMerchantModeInvalidPIN
	}
	for _, value := range pin {
		if value < '0' || value > '9' {
			return ErrMerchantModeInvalidPIN
		}
	}
	return nil
}

func hashMerchantModeInstallationID(rawInstallationID string) (string, error) {
	trimmed := strings.TrimSpace(rawInstallationID)
	if trimmed == "" {
		return "", ErrMerchantModeDeviceNeeded
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:]), nil
}

func scanMerchantModeDevice(row interface {
	Scan(...any) error
}) (*structs.MerchantModeDevice, error) {
	var device structs.MerchantModeDevice
	var enabledAt sql.NullTime
	var disabledAt sql.NullTime
	var locationID int64
	if err := row.Scan(
		&device.ID,
		&device.UserID,
		&locationID,
		&device.LocationName,
		&device.WalletAddress,
		&device.DisplayName,
		&device.Platform,
		&device.AppVersion,
		&device.MerchantModeEnabled,
		&enabledAt,
		&disabledAt,
		&device.LastSeenAt,
		&device.CreatedAt,
		&device.UpdatedAt,
	); err != nil {
		return nil, err
	}
	device.LocationID = uint(locationID)
	if enabledAt.Valid {
		device.EnabledAt = &enabledAt.Time
	}
	if disabledAt.Valid {
		device.DisabledAt = &disabledAt.Time
	}
	return &device, nil
}

func (a *AppDB) merchantModePasscodeSet(ctx context.Context, userID string) (bool, error) {
	var passcodeSet bool
	if err := a.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM merchant_mode_settings
			WHERE owner_id = $1
			AND TRIM(pin_hash) <> ''
		);
	`, userID).Scan(&passcodeSet); err != nil {
		return false, fmt.Errorf("error checking merchant mode settings: %w", err)
	}
	return passcodeSet, nil
}

func (a *AppDB) getMerchantModeDeviceByInstallationHash(ctx context.Context, userID string, installationHash string) (*structs.MerchantModeDevice, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			mmd.id,
			mmd.owner_id,
			mmd.location_id,
			COALESCE(NULLIF(TRIM(l.name), ''), 'Merchant location') AS location_name,
			mmd.wallet_address,
			mmd.display_name,
			mmd.platform,
			mmd.app_version,
			mmd.merchant_mode_enabled,
			mmd.enabled_at,
			mmd.disabled_at,
			mmd.last_seen_at,
			mmd.created_at,
			mmd.updated_at
		FROM
			merchant_mode_devices mmd
		JOIN
			locations l
		ON
			l.id = mmd.location_id
		AND
			l.active = TRUE
		WHERE
			mmd.owner_id = $1
		AND
			mmd.installation_id_hash = $2
		AND
			mmd.active = TRUE;
	`, userID, installationHash)

	device, err := scanMerchantModeDevice(row)
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (a *AppDB) getMerchantModeDeviceByID(ctx context.Context, userID string, deviceID string) (*structs.MerchantModeDevice, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			mmd.id,
			mmd.owner_id,
			mmd.location_id,
			COALESCE(NULLIF(TRIM(l.name), ''), 'Merchant location') AS location_name,
			mmd.wallet_address,
			mmd.display_name,
			mmd.platform,
			mmd.app_version,
			mmd.merchant_mode_enabled,
			mmd.enabled_at,
			mmd.disabled_at,
			mmd.last_seen_at,
			mmd.created_at,
			mmd.updated_at
		FROM
			merchant_mode_devices mmd
		JOIN
			locations l
		ON
			l.id = mmd.location_id
		AND
			l.active = TRUE
		WHERE
			mmd.owner_id = $1
		AND
			mmd.id = $2
		AND
			mmd.active = TRUE;
	`, userID, deviceID)

	device, err := scanMerchantModeDevice(row)
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (a *AppDB) ListMerchantModeDevices(ctx context.Context, userID string) (*structs.MerchantModeDevicesResponse, error) {
	user, err := a.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.IsMerchant {
		return nil, ErrMerchantModeForbidden
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			mmd.id,
			mmd.owner_id,
			mmd.location_id,
			COALESCE(NULLIF(TRIM(l.name), ''), 'Merchant location') AS location_name,
			mmd.wallet_address,
			mmd.display_name,
			mmd.platform,
			mmd.app_version,
			mmd.merchant_mode_enabled,
			mmd.enabled_at,
			mmd.disabled_at,
			mmd.last_seen_at,
			mmd.created_at,
			mmd.updated_at
		FROM
			merchant_mode_devices mmd
		JOIN
			locations l
		ON
			l.id = mmd.location_id
		AND
			l.active = TRUE
		WHERE
			mmd.owner_id = $1
		AND
			mmd.active = TRUE
		ORDER BY
			l.name ASC,
			mmd.last_seen_at DESC,
			mmd.updated_at DESC;
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("error listing merchant mode devices: %w", err)
	}
	defer rows.Close()

	devices := []*structs.MerchantModeDevice{}
	for rows.Next() {
		device, err := scanMerchantModeDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning merchant mode device: %w", err)
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating merchant mode devices: %w", err)
	}

	return &structs.MerchantModeDevicesResponse{Devices: devices}, nil
}

func (a *AppDB) GetMerchantModeStatus(ctx context.Context, userID string, installationID string) (*structs.MerchantModeStatusResponse, error) {
	user, err := a.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}

	response := &structs.MerchantModeStatusResponse{
		UserID:     userID,
		IsMerchant: user.IsMerchant,
	}
	if !user.IsMerchant {
		return response, nil
	}

	passcodeSet, err := a.merchantModePasscodeSet(ctx, userID)
	if err != nil {
		return nil, err
	}
	response.PasscodeSet = passcodeSet

	if strings.TrimSpace(installationID) == "" {
		return response, nil
	}
	installationHash, err := hashMerchantModeInstallationID(installationID)
	if err != nil {
		return nil, err
	}
	device, err := a.getMerchantModeDeviceByInstallationHash(ctx, userID, installationHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return response, nil
		}
		return nil, fmt.Errorf("error loading merchant mode device: %w", err)
	}
	response.Device = device

	if _, err := a.db.Exec(ctx, `
		UPDATE merchant_mode_devices
		SET last_seen_at = NOW(), updated_at = NOW()
		WHERE id = $1;
	`, device.ID); err != nil {
		return nil, fmt.Errorf("error updating merchant mode device heartbeat: %w", err)
	}

	return response, nil
}

func (a *AppDB) SetMerchantModePIN(ctx context.Context, userID string, pin string, currentPIN string) (*structs.MerchantModeStatusResponse, error) {
	if err := validateMerchantModePIN(pin); err != nil {
		return nil, err
	}
	user, err := a.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.IsMerchant {
		return nil, ErrMerchantModeForbidden
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error starting merchant mode PIN transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var existingPINHash string
	var failedAttemptCount int
	var lockedUntil sql.NullTime
	existingErr := tx.QueryRow(ctx, `
		SELECT
			pin_hash,
			failed_attempt_count,
			locked_until
		FROM
			merchant_mode_settings
		WHERE
			owner_id = $1
		FOR UPDATE;
	`, userID).Scan(&existingPINHash, &failedAttemptCount, &lockedUntil)
	if existingErr != nil && existingErr != pgx.ErrNoRows {
		return nil, fmt.Errorf("error loading merchant mode PIN settings: %w", existingErr)
	}

	hasExistingPIN := existingErr == nil && strings.TrimSpace(existingPINHash) != ""
	if hasExistingPIN {
		if strings.TrimSpace(currentPIN) == "" {
			return nil, ErrMerchantModeOldPINNeeded
		}
		if err := validateMerchantModePIN(currentPIN); err != nil {
			return nil, err
		}
		if lockedUntil.Valid && lockedUntil.Time.After(time.Now().UTC()) {
			return nil, ErrMerchantModePINLocked
		}

		if err := bcrypt.CompareHashAndPassword([]byte(existingPINHash), []byte(currentPIN)); err != nil {
			failedAttemptCount++
			var nextLockedUntil any
			if failedAttemptCount >= merchantModePINLockoutThreshold {
				nextLockedUntil = time.Now().UTC().Add(5 * time.Minute)
			}
			if _, updateErr := tx.Exec(ctx, `
				UPDATE merchant_mode_settings
				SET
					failed_attempt_count = $2,
					locked_until = $3,
					updated_at = NOW()
				WHERE
					owner_id = $1;
			`, userID, failedAttemptCount, nextLockedUntil); updateErr != nil {
				return nil, fmt.Errorf("error recording merchant mode PIN reset failure: %w", updateErr)
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return nil, fmt.Errorf("error committing merchant mode PIN reset failure: %w", commitErr)
			}
			return nil, ErrMerchantModeBadPIN
		}
	}

	pinHash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("error hashing merchant mode PIN: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO merchant_mode_settings (
			owner_id,
			pin_hash,
			pin_hash_version,
			failed_attempt_count,
			locked_until,
			updated_at
		) VALUES (
			$1,
			$2,
			'bcrypt:v1',
			0,
			NULL,
			NOW()
		)
		ON CONFLICT (owner_id)
		DO UPDATE SET
			pin_hash = EXCLUDED.pin_hash,
			pin_hash_version = EXCLUDED.pin_hash_version,
			failed_attempt_count = 0,
			locked_until = NULL,
			updated_at = NOW();
	`, userID, string(pinHash)); err != nil {
		return nil, fmt.Errorf("error saving merchant mode PIN: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("error committing merchant mode PIN transaction: %w", err)
	}

	return a.GetMerchantModeStatus(ctx, userID, "")
}

func (a *AppDB) resolveMerchantModeLocationAndWallet(ctx context.Context, userID string, request *structs.MerchantModeEnableRequest) (string, string, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COALESCE(NULLIF(TRIM(l.name), ''), 'Merchant location') AS location_name,
			COALESCE(
				NULLIF(TRIM(default_payment_wallet.wallet_address), ''),
				NULLIF(TRIM(u.primary_wallet_address), ''),
				NULLIF(TRIM(legacy_wallet.smart_address), ''),
				''
			) AS default_wallet_address
		FROM
			locations l
		JOIN
			users u
		ON
			u.id = l.owner_id
		AND
			u.active = TRUE
		LEFT JOIN LATERAL (
			SELECT
				lpw.wallet_address
			FROM
				location_payment_wallets lpw
			WHERE
				lpw.location_id = l.id
			AND
				lpw.active = TRUE
			ORDER BY
				CASE
					WHEN lpw.is_default = TRUE THEN 0
					ELSE 1
				END,
				lpw.id ASC
			LIMIT 1
		) default_payment_wallet
			ON TRUE
		LEFT JOIN LATERAL (
			SELECT
				w.smart_address
			FROM
				wallets w
			WHERE
				w.owner = l.owner_id
			AND
				w.active = TRUE
			AND
				w.is_eoa = FALSE
			AND
				w.smart_address IS NOT NULL
			AND
				TRIM(w.smart_address) <> ''
			ORDER BY
				w.smart_index ASC NULLS LAST,
				w.id ASC
			LIMIT 1
		) legacy_wallet
			ON TRUE
		WHERE
			l.id = $1
		AND
			l.owner_id = $2
		AND
			l.active = TRUE
		AND
			COALESCE(l.approval, FALSE) = TRUE;
	`, request.LocationID, userID)

	var locationName string
	var defaultWalletAddress string
	if err := row.Scan(&locationName, &defaultWalletAddress); err != nil {
		return "", "", err
	}

	walletAddress := strings.TrimSpace(request.WalletAddress)
	if walletAddress == "" {
		walletAddress = defaultWalletAddress
	}
	if walletAddress == "" {
		return "", "", fmt.Errorf("merchant mode requires a merchant payment wallet")
	}

	normalizedWalletAddress, err := normalizeEthereumAddressForField(walletAddress, "merchant mode wallet")
	if err != nil {
		return "", "", err
	}

	var walletOwned bool
	if err := a.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM wallets
			WHERE owner = $1
			AND active = TRUE
			AND (
				LOWER(COALESCE(smart_address, '')) = LOWER($2)
				OR LOWER(COALESCE(eoa_address, '')) = LOWER($2)
			)
		);
	`, userID, normalizedWalletAddress).Scan(&walletOwned); err != nil {
		return "", "", fmt.Errorf("error verifying merchant mode wallet ownership: %w", err)
	}
	if !walletOwned {
		return "", "", fmt.Errorf("merchant mode wallet must belong to the merchant account")
	}

	return locationName, normalizedWalletAddress, nil
}

func (a *AppDB) EnableMerchantModeDevice(ctx context.Context, userID string, request *structs.MerchantModeEnableRequest) (*structs.MerchantModeStatusResponse, error) {
	user, err := a.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.IsMerchant {
		return nil, ErrMerchantModeForbidden
	}

	passcodeSet, err := a.merchantModePasscodeSet(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !passcodeSet {
		return nil, ErrMerchantModePINRequired
	}

	installationHash, err := hashMerchantModeInstallationID(request.InstallationID)
	if err != nil {
		return nil, err
	}
	_, walletAddress, err := a.resolveMerchantModeLocationAndWallet(ctx, userID, request)
	if err != nil {
		return nil, err
	}

	displayName := strings.TrimSpace(request.DisplayName)
	platform := strings.TrimSpace(request.Platform)
	appVersion := strings.TrimSpace(request.AppVersion)

	var deviceID string
	if err := a.db.QueryRow(ctx, `
		INSERT INTO merchant_mode_devices (
			id,
			owner_id,
			location_id,
			installation_id_hash,
			display_name,
			platform,
			app_version,
			wallet_address,
			merchant_mode_enabled,
			enabled_at,
			enabled_by,
			disabled_at,
			disabled_by,
			last_seen_at,
			updated_at
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			TRUE,
			NOW(),
			$2,
			NULL,
			'',
			NOW(),
			NOW()
		)
		ON CONFLICT (owner_id, installation_id_hash) WHERE active = TRUE
		DO UPDATE SET
			location_id = EXCLUDED.location_id,
			display_name = EXCLUDED.display_name,
			platform = EXCLUDED.platform,
			app_version = EXCLUDED.app_version,
			wallet_address = EXCLUDED.wallet_address,
			merchant_mode_enabled = TRUE,
			enabled_at = NOW(),
			enabled_by = EXCLUDED.enabled_by,
			disabled_at = NULL,
			disabled_by = '',
			last_seen_at = NOW(),
			updated_at = NOW()
		RETURNING id;
	`, uuid.NewString(), userID, request.LocationID, installationHash, displayName, platform, appVersion, walletAddress).Scan(&deviceID); err != nil {
		return nil, fmt.Errorf("error enabling merchant mode device: %w", err)
	}

	return a.GetMerchantModeStatus(ctx, userID, request.InstallationID)
}

func (a *AppDB) SetMerchantModeDeviceEnabled(ctx context.Context, userID string, deviceID string, enabled bool) (*structs.MerchantModeDevice, error) {
	user, err := a.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.IsMerchant {
		return nil, ErrMerchantModeForbidden
	}

	if enabled {
		passcodeSet, err := a.merchantModePasscodeSet(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !passcodeSet {
			return nil, ErrMerchantModePINRequired
		}
	}

	var updatedDeviceID string
	if enabled {
		err = a.db.QueryRow(ctx, `
			UPDATE merchant_mode_devices
			SET
				merchant_mode_enabled = TRUE,
				enabled_at = NOW(),
				enabled_by = $1,
				disabled_at = NULL,
				disabled_by = '',
				updated_at = NOW()
			WHERE
				owner_id = $1
			AND
				id = $2
			AND
				active = TRUE
			RETURNING id;
		`, userID, deviceID).Scan(&updatedDeviceID)
	} else {
		err = a.db.QueryRow(ctx, `
			UPDATE merchant_mode_devices
			SET
				merchant_mode_enabled = FALSE,
				disabled_at = NOW(),
				disabled_by = $1,
				updated_at = NOW()
			WHERE
				owner_id = $1
			AND
				id = $2
			AND
				active = TRUE
			RETURNING id;
		`, userID, deviceID).Scan(&updatedDeviceID)
	}
	if err != nil {
		return nil, fmt.Errorf("error updating merchant mode device: %w", err)
	}

	return a.getMerchantModeDeviceByID(ctx, userID, updatedDeviceID)
}

func (a *AppDB) DisableMerchantModeDevice(ctx context.Context, userID string, request *structs.MerchantModeDisableRequest) (*structs.MerchantModeStatusResponse, error) {
	if err := validateMerchantModePIN(request.PIN); err != nil {
		return nil, err
	}
	installationHash, err := hashMerchantModeInstallationID(request.InstallationID)
	if err != nil {
		return nil, err
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error starting merchant mode disable transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var pinHash string
	var failedAttemptCount int
	var lockedUntil sql.NullTime
	if err := tx.QueryRow(ctx, `
		SELECT
			pin_hash,
			failed_attempt_count,
			locked_until
		FROM
			merchant_mode_settings
		WHERE
			owner_id = $1
		FOR UPDATE;
	`, userID).Scan(&pinHash, &failedAttemptCount, &lockedUntil); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrMerchantModePINRequired
		}
		return nil, fmt.Errorf("error loading merchant mode settings: %w", err)
	}
	if lockedUntil.Valid && lockedUntil.Time.After(time.Now().UTC()) {
		return nil, ErrMerchantModePINLocked
	}

	if err := bcrypt.CompareHashAndPassword([]byte(pinHash), []byte(request.PIN)); err != nil {
		failedAttemptCount++
		var nextLockedUntil any
		if failedAttemptCount >= merchantModePINLockoutThreshold {
			nextLockedUntil = time.Now().UTC().Add(5 * time.Minute)
		}
		if _, updateErr := tx.Exec(ctx, `
			UPDATE merchant_mode_settings
			SET
				failed_attempt_count = $2,
				locked_until = $3,
				updated_at = NOW()
			WHERE
				owner_id = $1;
		`, userID, failedAttemptCount, nextLockedUntil); updateErr != nil {
			return nil, fmt.Errorf("error recording merchant mode PIN failure: %w", updateErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("error committing merchant mode PIN failure: %w", commitErr)
		}
		return nil, ErrMerchantModeBadPIN
	}

	if _, err := tx.Exec(ctx, `
		UPDATE merchant_mode_settings
		SET
			failed_attempt_count = 0,
			locked_until = NULL,
			updated_at = NOW()
		WHERE
			owner_id = $1;
	`, userID); err != nil {
		return nil, fmt.Errorf("error resetting merchant mode PIN failure state: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE merchant_mode_devices
		SET
			merchant_mode_enabled = FALSE,
			disabled_at = NOW(),
			disabled_by = $2,
			last_seen_at = NOW(),
			updated_at = NOW()
		WHERE
			owner_id = $1
		AND
			installation_id_hash = $3
		AND
			active = TRUE;
	`, userID, userID, installationHash); err != nil {
		return nil, fmt.Errorf("error disabling merchant mode device: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("error committing merchant mode disable transaction: %w", err)
	}

	return a.GetMerchantModeStatus(ctx, userID, request.InstallationID)
}
