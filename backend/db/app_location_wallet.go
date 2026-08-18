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

func normalizeLocationWalletAddresses(addresses []string, fieldName string) ([]string, error) {
	normalizedAddresses := make([]string, 0, len(addresses))
	seen := map[string]struct{}{}

	for _, rawAddress := range addresses {
		trimmedAddress := strings.TrimSpace(rawAddress)
		if trimmedAddress == "" {
			continue
		}

		normalizedAddress, err := normalizeEthereumAddressForField(trimmedAddress, fieldName)
		if err != nil {
			return nil, err
		}

		key := strings.ToLower(normalizedAddress)
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}
		normalizedAddresses = append(normalizedAddresses, normalizedAddress)
	}

	return normalizedAddresses, nil
}

func containsAddress(addresses []string, candidate string) bool {
	for _, address := range addresses {
		if strings.EqualFold(address, candidate) {
			return true
		}
	}
	return false
}

func int32LocationIDs(locationIDs []uint) []int32 {
	ids := make([]int32, 0, len(locationIDs))
	for _, locationID := range locationIDs {
		ids = append(ids, int32(locationID))
	}
	return ids
}

func (a *AppDB) getLocationPaymentWalletsByLocationIDs(ctx context.Context, locationIDs []uint) (map[uint][]structs.LocationPaymentWallet, error) {
	if len(locationIDs) == 0 {
		return map[uint][]structs.LocationPaymentWallet{}, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			location_id,
			wallet_address,
			is_default
		FROM
			location_payment_wallets
		WHERE
			location_id = ANY($1)
		AND
			active = TRUE
		ORDER BY
			location_id ASC,
			CASE
				WHEN is_default = TRUE THEN 0
				ELSE 1
			END,
			id ASC;
	`, int32LocationIDs(locationIDs))
	if err != nil {
		return nil, fmt.Errorf("error querying location payment wallets: %w", err)
	}
	defer rows.Close()

	walletsByLocationID := map[uint][]structs.LocationPaymentWallet{}
	for rows.Next() {
		var wallet structs.LocationPaymentWallet
		var locationID int64
		if err := rows.Scan(
			&wallet.ID,
			&locationID,
			&wallet.WalletAddress,
			&wallet.IsDefault,
		); err != nil {
			return nil, fmt.Errorf("error scanning location payment wallet row: %w", err)
		}
		wallet.LocationID = uint(locationID)
		walletsByLocationID[wallet.LocationID] = append(walletsByLocationID[wallet.LocationID], wallet)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating location payment wallet rows: %w", err)
	}

	return walletsByLocationID, nil
}

func (a *AppDB) attachLocationPaymentWallets(ctx context.Context, locations []*structs.Location) error {
	locationIDs := make([]uint, 0, len(locations))
	for _, location := range locations {
		if location == nil {
			continue
		}
		locationIDs = append(locationIDs, location.ID)
	}

	walletsByLocationID, err := a.getLocationPaymentWalletsByLocationIDs(ctx, locationIDs)
	if err != nil {
		return err
	}

	for _, location := range locations {
		if location == nil {
			continue
		}
		location.PaymentWallets = walletsByLocationID[location.ID]
	}

	return nil
}

func (a *AppDB) UpdateLocationWalletSettings(ctx context.Context, userID string, locationID uint64, request *structs.LocationWalletSettingsUpdateRequest) (*structs.Location, error) {
	user, err := a.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.IsMerchant {
		return nil, fmt.Errorf("only merchants can update merchant wallet settings")
	}

	normalizedPaymentWallets, err := normalizeLocationWalletAddresses(request.PaymentWalletAddresses, "payment wallet")
	if err != nil {
		return nil, err
	}

	normalizedDefaultPaymentWallet := strings.TrimSpace(request.DefaultPaymentWalletAddress)
	if normalizedDefaultPaymentWallet != "" {
		normalizedDefaultPaymentWallet, err = normalizeEthereumAddressForField(normalizedDefaultPaymentWallet, "default payment wallet")
		if err != nil {
			return nil, err
		}
	}

	// A location must always have somewhere for money to land. This used to be
	// tolerated because the read path fell back to the owner's primary wallet;
	// now that it does not, an empty list is a shop that silently drops payments.
	// Removing the last wallet is therefore not an edit — it is a replacement,
	// and ReplaceLocationWallet is the only way to do it.
	if len(normalizedPaymentWallets) == 0 {
		return nil, fmt.Errorf("payment wallet is required")
	}

	if normalizedDefaultPaymentWallet == "" {
		normalizedDefaultPaymentWallet = normalizedPaymentWallets[0]
	}
	if !containsAddress(normalizedPaymentWallets, normalizedDefaultPaymentWallet) {
		return nil, fmt.Errorf("default payment wallet must be one of the payment wallets")
	}

	normalizedTippingWallet := strings.TrimSpace(request.TippingWalletAddress)
	if normalizedTippingWallet != "" {
		normalizedTippingWallet, err = normalizeEthereumAddressForField(normalizedTippingWallet, "tipping wallet")
		if err != nil {
			return nil, err
		}
	}

	for _, paymentWalletAddress := range normalizedPaymentWallets {
		if normalizedTippingWallet != "" && strings.EqualFold(normalizedTippingWallet, paymentWalletAddress) {
			return nil, fmt.Errorf("tipping wallet must be different from every payment wallet")
		}
	}

	// One address, one role, across the whole estate. The database enforces that
	// no two locations share a payment wallet, and none share a tipping wallet,
	// but it cannot express the cross-role case — payment wallets live in
	// location_payment_wallets and tipping wallets in a column on locations — so
	// that half is checked here. Without it a merchant could point shop B's tips
	// at shop A's till and quietly make both unreportable.
	candidates := append([]string(nil), normalizedPaymentWallets...)
	if normalizedTippingWallet != "" {
		candidates = append(candidates, normalizedTippingWallet)
	}
	for _, candidate := range candidates {
		var clashingLocation string
		err := a.db.QueryRow(ctx, `
			SELECT COALESCE(NULLIF(TRIM(l.name), ''), 'another location')
			FROM locations l
			WHERE l.active = TRUE
			AND l.id <> $1
			AND l.owner_id = $2
			AND (
				LOWER(TRIM(COALESCE(l.tipping_wallet_address, ''))) = LOWER($3)
				OR EXISTS (
					SELECT 1 FROM location_payment_wallets p
					WHERE p.location_id = l.id AND p.active = TRUE
					AND LOWER(TRIM(p.wallet_address)) = LOWER($3)
				)
			)
			LIMIT 1;
		`, locationID, userID, candidate).Scan(&clashingLocation)
		if err == nil {
			return nil, fmt.Errorf("that wallet is already in use by %s — each location needs its own payment and tipping wallets", clashingLocation)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("error checking wallet uniqueness: %w", err)
		}
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error starting location wallet settings transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var locationExists bool
	if err := tx.QueryRow(ctx, `
		SELECT
			EXISTS(
				SELECT 1
				FROM locations
				WHERE id = $1
				AND owner_id = $2
				AND active = TRUE
			);
	`, locationID, userID).Scan(&locationExists); err != nil {
		return nil, fmt.Errorf("error verifying location ownership: %w", err)
	}
	if !locationExists {
		return nil, pgx.ErrNoRows
	}

	if _, err := tx.Exec(ctx, `
		UPDATE location_payment_wallets
		SET
			active = FALSE,
			delete_date = $2,
			delete_reason = $3
		WHERE
			location_id = $1
		AND
			active = TRUE
		AND
			NOT (wallet_address = ANY($4));
	`, locationID, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonWalletSettings, normalizedPaymentWallets); err != nil {
		return nil, fmt.Errorf("error retiring removed location payment wallets: %w", err)
	}

	for _, paymentWalletAddress := range normalizedPaymentWallets {
		if _, err := tx.Exec(ctx, `
			INSERT INTO location_payment_wallets (
				location_id,
				wallet_address,
				is_default
			) VALUES ($1, $2, $3)
			ON CONFLICT (location_id, wallet_address) WHERE active = TRUE
			DO UPDATE SET
				is_default = EXCLUDED.is_default,
				active = TRUE,
				delete_date = NULL,
				delete_reason = NULL;
		`, locationID, paymentWalletAddress, strings.EqualFold(paymentWalletAddress, normalizedDefaultPaymentWallet)); err != nil {
			return nil, fmt.Errorf("error saving location payment wallet: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE locations
		SET tipping_wallet_address = $1
		WHERE id = $2
		AND owner_id = $3;
	`, normalizedTippingWallet, locationID, userID); err != nil {
		return nil, fmt.Errorf("error updating location tipping wallet: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("error committing location wallet settings transaction: %w", err)
	}

	locations, err := a.GetLocationsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	for _, location := range locations {
		if location != nil && location.ID == uint(locationID) {
			return location, nil
		}
	}

	return nil, pgx.ErrNoRows
}
