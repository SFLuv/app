package db

import (
	"context"
	"fmt"
	"strings"
)

// MerchantDayWallets is the pair of addresses a merchant-mode till reports on.
type MerchantDayWallets struct {
	// Payment is the wallet this device receives into — the same address its
	// QR encodes, so the day total always describes the till in front of you.
	Payment string
	// Tipping is empty when the location has none configured, or when the one
	// configured fails OwnerOwnsTipping.
	Tipping string
	// OwnerOwnsTipping records whether the tipping wallet actually belongs to the
	// location's owner. A tipping wallet pointing somewhere else is a
	// misconfiguration, and counting a stranger's incoming transfers as this
	// merchant's tips would be worse than showing none.
	OwnerOwnsTipping bool
	TimeZone         string
}

// GetMerchantDayWallets resolves the addresses behind a merchant-mode device.
func (a *AppDB) GetMerchantDayWallets(ctx context.Context, locationID uint, deviceWallet string) (*MerchantDayWallets, error) {
	wallets := &MerchantDayWallets{
		Payment:  normalizeAddress(deviceWallet),
		TimeZone: MerchantDayTimeZone,
	}

	var tipping, ownerID string
	err := a.db.QueryRow(ctx, `
		SELECT
			COALESCE(NULLIF(TRIM(l.tipping_wallet_address), ''), ''),
			COALESCE(l.owner_id, '')
		FROM locations l
		WHERE l.id = $1
		AND l.active = TRUE;
	`, locationID).Scan(&tipping, &ownerID)
	if err != nil {
		return nil, fmt.Errorf("error resolving merchant day wallets for location %d: %w", locationID, err)
	}

	tipping = normalizeAddress(tipping)
	if tipping == "" || ownerID == "" {
		return wallets, nil
	}

	// The ownership gate. Both addresses come off the same location row, so this
	// is not guarding against cross-tenant leakage so much as against a location
	// configured with an address its owner does not hold — at which point those
	// incoming transfers are somebody else's money, not tips.
	var owned bool
	err = a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM wallets w
			WHERE w.owner = $1
			AND w.active = TRUE
			AND LOWER(COALESCE(NULLIF(TRIM(w.smart_address), ''), TRIM(w.eoa_address))) = $2
		);
	`, ownerID, tipping).Scan(&owned)
	if err != nil {
		return nil, fmt.Errorf("error checking tipping wallet ownership for location %d: %w", locationID, err)
	}

	wallets.OwnerOwnsTipping = owned
	if owned {
		wallets.Tipping = tipping
	}

	return wallets, nil
}

// MerchantDayTimeZone is the business day every location is measured in. SFLuv
// is a San Francisco currency and every merchant sits in one zone, so this is a
// constant rather than a column or a setting. Give locations a timezone column
// the day one opens outside it — the day boundary is the only thing that reads
// this, and it is computed server-side precisely so that change lands in one place.
const MerchantDayTimeZone = "America/Los_Angeles"

func normalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}
