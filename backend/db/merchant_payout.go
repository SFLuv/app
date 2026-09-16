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

// MerchantPayoutSchemaDDL is applied by schema migration 1.55.
//
// Four tables, one per fact:
//
//   - merchant_payout_profiles — the business's standing with Bridge. Keyed by
//     the owning user, because KYB verifies a legal entity, and one entity may
//     run several locations.
//   - merchant_bank_accounts — a mirror of the bank accounts Bridge holds for
//     that business. Only what a merchant needs to recognise their own bank
//     is stored; the account number never exists on our side.
//   - location_liquidation_addresses — exactly one row per location. The row is
//     the answer to "where does this location's unwrapped USDC go", and an
//     override replaces it rather than adding to it.
//   - unwraps — the ledger. One row per withdrawTo, matched to Bridge's drain
//     by transaction hash and carried through to the bank.
const MerchantPayoutSchemaDDL = `
	CREATE TABLE IF NOT EXISTS merchant_payout_profiles (
		owner_id            TEXT PRIMARY KEY,
		bridge_customer_id  TEXT NOT NULL DEFAULT '',
		bridge_kyc_link_id  TEXT NOT NULL DEFAULT '',
		kyb_status          TEXT NOT NULL DEFAULT 'not_started',
		tos_status          TEXT NOT NULL DEFAULT 'pending',
		kyb_link_url        TEXT NOT NULL DEFAULT '',
		kyb_requested_at    TIMESTAMPTZ,
		kyb_approved_at     TIMESTAMPTZ,
		last_synced_at      TIMESTAMPTZ,
		created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS merchant_payout_profiles_customer_idx
		ON merchant_payout_profiles (bridge_customer_id) WHERE bridge_customer_id <> '';
	CREATE INDEX IF NOT EXISTS merchant_payout_profiles_kyb_status_idx
		ON merchant_payout_profiles (kyb_status);

	CREATE TABLE IF NOT EXISTS merchant_bank_accounts (
		id                          BIGSERIAL PRIMARY KEY,
		owner_id                    TEXT NOT NULL,
		bridge_external_account_id  TEXT NOT NULL UNIQUE,
		bank_name                   TEXT NOT NULL DEFAULT '',
		last_4                      TEXT NOT NULL DEFAULT '',
		account_owner_name          TEXT NOT NULL DEFAULT '',
		currency                    TEXT NOT NULL DEFAULT '',
		active                      BOOLEAN NOT NULL DEFAULT TRUE,
		created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS merchant_bank_accounts_owner_idx ON merchant_bank_accounts (owner_id);

	CREATE TABLE IF NOT EXISTS location_liquidation_addresses (
		location_id                    INTEGER PRIMARY KEY REFERENCES locations(id),
		owner_id                       TEXT NOT NULL,
		bridge_liquidation_address_id  TEXT NOT NULL DEFAULT '',
		address                        TEXT NOT NULL,
		chain                          TEXT NOT NULL DEFAULT 'celo',
		currency                       TEXT NOT NULL DEFAULT 'usdc',
		destination_payment_rail       TEXT NOT NULL DEFAULT 'ach',
		destination_currency           TEXT NOT NULL DEFAULT 'usd',
		bridge_external_account_id     TEXT NOT NULL DEFAULT '',
		-- 'bridge' when we provisioned it; 'admin' when an admin pasted one.
		source                         TEXT NOT NULL DEFAULT 'bridge',
		set_by_user_id                 TEXT NOT NULL DEFAULT '',
		created_at                     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at                     TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS location_liquidation_addresses_owner_idx ON location_liquidation_addresses (owner_id);

	CREATE TABLE IF NOT EXISTS unwraps (
		id                    BIGSERIAL PRIMARY KEY,
		owner_id              TEXT NOT NULL,
		location_id           INTEGER,
		wallet_id             INTEGER,
		wallet_address        TEXT NOT NULL,
		wallet_role           TEXT NOT NULL DEFAULT 'payment',
		destination_address   TEXT NOT NULL,
		amount_wei            TEXT NOT NULL,
		tx_hash               TEXT NOT NULL UNIQUE,
		status                TEXT NOT NULL DEFAULT 'submitted',
		bridge_drain_id       TEXT NOT NULL DEFAULT '',
		bridge_state          TEXT NOT NULL DEFAULT '',
		bank_reference        TEXT NOT NULL DEFAULT '',
		last_synced_at        TIMESTAMPTZ,
		created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS unwraps_owner_created_idx ON unwraps (owner_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS unwraps_location_idx ON unwraps (location_id);
	CREATE INDEX IF NOT EXISTS unwraps_open_idx ON unwraps (status) WHERE status IN ('submitted','funds_received','payment_submitted');
`

// Unwrap ledger statuses. The first four mirror the merchant's view of a
// drain; anything Bridge reports that is not a success lands in "failed" with
// the raw bridge_state kept beside it for admins.
const (
	UnwrapSubmitted        = "submitted"
	UnwrapFundsReceived    = "funds_received"
	UnwrapPaymentSubmitted = "payment_submitted"
	UnwrapPaymentProcessed = "payment_processed"
	UnwrapFailed           = "failed"
)

// ---------------------------------------------------------------------------
// Profiles
// ---------------------------------------------------------------------------

func scanPayoutProfile(row pgx.Row) (*structs.MerchantPayoutProfile, error) {
	var p structs.MerchantPayoutProfile
	err := row.Scan(
		&p.OwnerID, &p.BridgeCustomerID, &p.BridgeKYCLinkID, &p.KYBStatus, &p.TOSStatus, &p.KYBLinkURL,
		&p.KYBRequestedAt, &p.KYBApprovedAt, &p.LastSyncedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

const payoutProfileColumns = `
	owner_id, bridge_customer_id, bridge_kyc_link_id, kyb_status, tos_status, kyb_link_url,
	kyb_requested_at, kyb_approved_at, last_synced_at, created_at, updated_at`

// GetMerchantPayoutProfile returns nil, nil when the owner has never started.
func (a *AppDB) GetMerchantPayoutProfile(ctx context.Context, ownerID string) (*structs.MerchantPayoutProfile, error) {
	p, err := scanPayoutProfile(a.db.QueryRow(ctx, `
		SELECT `+payoutProfileColumns+` FROM merchant_payout_profiles WHERE owner_id = $1;
	`, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error loading merchant payout profile: %w", err)
	}
	return p, nil
}

func (a *AppDB) GetMerchantPayoutProfileByCustomer(ctx context.Context, customerID string) (*structs.MerchantPayoutProfile, error) {
	p, err := scanPayoutProfile(a.db.QueryRow(ctx, `
		SELECT `+payoutProfileColumns+` FROM merchant_payout_profiles WHERE bridge_customer_id = $1 LIMIT 1;
	`, customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error loading merchant payout profile by customer: %w", err)
	}
	return p, nil
}

// StartMerchantKYB records a freshly issued KYC link. Idempotent on owner: a
// second approval for the same business updates the link and leaves the
// customer id alone.
func (a *AppDB) StartMerchantKYB(ctx context.Context, ownerID, customerID, kycLinkID, kycStatus, tosStatus, linkURL string) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO merchant_payout_profiles (owner_id, bridge_customer_id, bridge_kyc_link_id, kyb_status, tos_status, kyb_link_url, kyb_requested_at, last_synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (owner_id) DO UPDATE SET
			bridge_customer_id = CASE WHEN merchant_payout_profiles.bridge_customer_id = '' THEN EXCLUDED.bridge_customer_id ELSE merchant_payout_profiles.bridge_customer_id END,
			bridge_kyc_link_id = EXCLUDED.bridge_kyc_link_id,
			kyb_status         = EXCLUDED.kyb_status,
			tos_status         = EXCLUDED.tos_status,
			kyb_link_url       = EXCLUDED.kyb_link_url,
			kyb_requested_at   = COALESCE(merchant_payout_profiles.kyb_requested_at, NOW()),
			last_synced_at     = NOW(),
			updated_at         = NOW();
	`, ownerID, customerID, kycLinkID, kycStatus, tosStatus, linkURL)
	if err != nil {
		return fmt.Errorf("error starting merchant KYB: %w", err)
	}
	return nil
}

// AttachBridgeCustomer is for businesses that already exist on Bridge (the
// first merchants were onboarded by hand). It records the customer without a
// KYC link and with whatever status Bridge reports.
func (a *AppDB) AttachBridgeCustomer(ctx context.Context, ownerID, customerID, kybStatus string) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO merchant_payout_profiles (owner_id, bridge_customer_id, kyb_status, last_synced_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (owner_id) DO UPDATE SET
			bridge_customer_id = EXCLUDED.bridge_customer_id,
			kyb_status         = EXCLUDED.kyb_status,
			kyb_approved_at    = CASE WHEN EXCLUDED.kyb_status = 'approved' THEN COALESCE(merchant_payout_profiles.kyb_approved_at, NOW()) ELSE merchant_payout_profiles.kyb_approved_at END,
			last_synced_at     = NOW(),
			updated_at         = NOW();
	`, ownerID, customerID, kybStatus)
	if err != nil {
		return fmt.Errorf("error attaching bridge customer: %w", err)
	}
	return nil
}

func (a *AppDB) SetMerchantKYBStatus(ctx context.Context, ownerID, kybStatus, tosStatus string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE merchant_payout_profiles SET
			kyb_status      = $2,
			tos_status      = CASE WHEN $3 = '' THEN tos_status ELSE $3 END,
			kyb_approved_at = CASE WHEN $2 = 'approved' THEN COALESCE(kyb_approved_at, NOW()) ELSE kyb_approved_at END,
			last_synced_at  = NOW(),
			updated_at      = NOW()
		WHERE owner_id = $1;
	`, ownerID, kybStatus, tosStatus)
	if err != nil {
		return fmt.Errorf("error updating merchant KYB status: %w", err)
	}
	return nil
}

// ListMerchantPayoutProfilesPendingKYB is the sweep's worklist: businesses we
// sent to Bridge whose answer has not come back yet.
func (a *AppDB) ListMerchantPayoutProfilesPendingKYB(ctx context.Context, limit int) ([]*structs.MerchantPayoutProfile, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := a.db.Query(ctx, `
		SELECT `+payoutProfileColumns+`
		FROM merchant_payout_profiles
		WHERE bridge_customer_id <> ''
		AND kyb_status NOT IN ('approved', 'rejected', 'offboarded')
		ORDER BY COALESCE(last_synced_at, created_at) ASC
		LIMIT $1;
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("error listing pending KYB profiles: %w", err)
	}
	defer rows.Close()
	var out []*structs.MerchantPayoutProfile
	for rows.Next() {
		p, err := scanPayoutProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning payout profile: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (a *AppDB) ListAllMerchantPayoutProfiles(ctx context.Context) ([]*structs.MerchantPayoutProfile, error) {
	rows, err := a.db.Query(ctx, `SELECT `+payoutProfileColumns+` FROM merchant_payout_profiles ORDER BY updated_at DESC LIMIT 500;`)
	if err != nil {
		return nil, fmt.Errorf("error listing payout profiles: %w", err)
	}
	defer rows.Close()
	var out []*structs.MerchantPayoutProfile
	for rows.Next() {
		p, err := scanPayoutProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning payout profile: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Bank accounts
// ---------------------------------------------------------------------------

// SyncMerchantBankAccounts replaces the owner's mirror with what Bridge
// currently holds. Bridge is the source of truth for whether an account is
// active; we never delete, we mark.
func (a *AppDB) SyncMerchantBankAccounts(ctx context.Context, ownerID string, accounts []structs.MerchantBankAccount) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting bank account sync: %w", err)
	}
	defer tx.Rollback(ctx)

	seen := make([]string, 0, len(accounts))
	for _, acct := range accounts {
		seen = append(seen, acct.BridgeExternalAccountID)
		if _, err := tx.Exec(ctx, `
			INSERT INTO merchant_bank_accounts (owner_id, bridge_external_account_id, bank_name, last_4, account_owner_name, currency, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (bridge_external_account_id) DO UPDATE SET
				bank_name = EXCLUDED.bank_name, last_4 = EXCLUDED.last_4, account_owner_name = EXCLUDED.account_owner_name,
				currency = EXCLUDED.currency, active = EXCLUDED.active, updated_at = NOW();
		`, ownerID, acct.BridgeExternalAccountID, acct.BankName, acct.Last4, acct.AccountOwnerName, acct.Currency, acct.Active); err != nil {
			return fmt.Errorf("error upserting bank account: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE merchant_bank_accounts SET active = FALSE, updated_at = NOW()
		WHERE owner_id = $1 AND active = TRUE AND NOT (bridge_external_account_id = ANY($2));
	`, ownerID, seen); err != nil {
		return fmt.Errorf("error retiring removed bank accounts: %w", err)
	}
	return tx.Commit(ctx)
}

func (a *AppDB) ListMerchantBankAccounts(ctx context.Context, ownerID string) ([]structs.MerchantBankAccount, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, owner_id, bridge_external_account_id, bank_name, last_4, account_owner_name, currency, active, created_at
		FROM merchant_bank_accounts WHERE owner_id = $1 AND active = TRUE ORDER BY created_at ASC;
	`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("error listing bank accounts: %w", err)
	}
	defer rows.Close()
	out := []structs.MerchantBankAccount{}
	for rows.Next() {
		var b structs.MerchantBankAccount
		if err := rows.Scan(&b.ID, &b.OwnerID, &b.BridgeExternalAccountID, &b.BankName, &b.Last4, &b.AccountOwnerName, &b.Currency, &b.Active, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("error scanning bank account: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Liquidation addresses
// ---------------------------------------------------------------------------

func (a *AppDB) GetLocationLiquidationAddress(ctx context.Context, locationID uint64) (*structs.LocationLiquidationAddress, error) {
	var l structs.LocationLiquidationAddress
	err := a.db.QueryRow(ctx, `
		SELECT location_id, owner_id, bridge_liquidation_address_id, address, chain, currency, destination_payment_rail, destination_currency,
		       bridge_external_account_id, source, set_by_user_id, created_at, updated_at
		FROM location_liquidation_addresses WHERE location_id = $1;
	`, locationID).Scan(&l.LocationID, &l.OwnerID, &l.BridgeLiquidationAddressID, &l.Address, &l.Chain, &l.Currency, &l.DestinationPaymentRail,
		&l.DestinationCurrency, &l.BridgeExternalAccountID, &l.Source, &l.SetByUserID, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error loading location liquidation address: %w", err)
	}
	return &l, nil
}

func (a *AppDB) ListLocationLiquidationAddressesByOwner(ctx context.Context, ownerID string) ([]structs.LocationLiquidationAddress, error) {
	rows, err := a.db.Query(ctx, `
		SELECT location_id, owner_id, bridge_liquidation_address_id, address, chain, currency, destination_payment_rail, destination_currency,
		       bridge_external_account_id, source, set_by_user_id, created_at, updated_at
		FROM location_liquidation_addresses WHERE owner_id = $1 ORDER BY location_id;
	`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("error listing liquidation addresses: %w", err)
	}
	defer rows.Close()
	out := []structs.LocationLiquidationAddress{}
	for rows.Next() {
		var l structs.LocationLiquidationAddress
		if err := rows.Scan(&l.LocationID, &l.OwnerID, &l.BridgeLiquidationAddressID, &l.Address, &l.Chain, &l.Currency, &l.DestinationPaymentRail,
			&l.DestinationCurrency, &l.BridgeExternalAccountID, &l.Source, &l.SetByUserID, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("error scanning liquidation address: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// UpsertLocationLiquidationAddress is the one write for both the provisioned
// path and the admin override: whatever was there is replaced.
func (a *AppDB) UpsertLocationLiquidationAddress(ctx context.Context, l *structs.LocationLiquidationAddress) error {
	address := strings.TrimSpace(l.Address)
	if address == "" {
		return fmt.Errorf("liquidation address is required")
	}
	var err error
	address, err = normalizeEthereumAddressForField(address, "liquidation address")
	if err != nil {
		return err
	}
	_, err = a.db.Exec(ctx, `
		INSERT INTO location_liquidation_addresses (
			location_id, owner_id, bridge_liquidation_address_id, address, chain, currency, destination_payment_rail, destination_currency,
			bridge_external_account_id, source, set_by_user_id
		) VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5,''),'celo'), COALESCE(NULLIF($6,''),'usdc'), COALESCE(NULLIF($7,''),'ach'), COALESCE(NULLIF($8,''),'usd'), $9, $10, $11)
		ON CONFLICT (location_id) DO UPDATE SET
			owner_id = EXCLUDED.owner_id,
			bridge_liquidation_address_id = EXCLUDED.bridge_liquidation_address_id,
			address = EXCLUDED.address, chain = EXCLUDED.chain, currency = EXCLUDED.currency,
			destination_payment_rail = EXCLUDED.destination_payment_rail, destination_currency = EXCLUDED.destination_currency,
			bridge_external_account_id = EXCLUDED.bridge_external_account_id,
			source = EXCLUDED.source, set_by_user_id = EXCLUDED.set_by_user_id, updated_at = NOW();
	`, l.LocationID, l.OwnerID, l.BridgeLiquidationAddressID, address, l.Chain, l.Currency, l.DestinationPaymentRail, l.DestinationCurrency,
		l.BridgeExternalAccountID, l.Source, l.SetByUserID)
	if err != nil {
		return fmt.Errorf("error saving location liquidation address: %w", err)
	}
	return nil
}

// ListApprovedLocationIDsForOwner is what provisioning walks.
func (a *AppDB) ListApprovedLocationIDsForOwner(ctx context.Context, ownerID string) ([]uint64, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id FROM locations WHERE owner_id = $1 AND active = TRUE AND approval = TRUE ORDER BY id;
	`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("error listing approved locations: %w", err)
	}
	defer rows.Close()
	var out []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// LocationOwnedBy reports whether the location is active and owned by ownerID.
func (a *AppDB) LocationOwnedBy(ctx context.Context, locationID uint64, ownerID string) (bool, error) {
	var ok bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM locations WHERE id = $1 AND owner_id = $2 AND active = TRUE);
	`, locationID, ownerID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("error checking location ownership: %w", err)
	}
	return ok, nil
}

func (a *AppDB) GetLocationOwnerID(ctx context.Context, locationID uint64) (string, error) {
	var owner string
	err := a.db.QueryRow(ctx, `SELECT owner_id FROM locations WHERE id = $1 AND active = TRUE;`, locationID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", pgx.ErrNoRows
	}
	if err != nil {
		return "", fmt.Errorf("error loading location owner: %w", err)
	}
	return owner, nil
}

// ---------------------------------------------------------------------------
// Unwrap ledger
// ---------------------------------------------------------------------------

func (a *AppDB) InsertUnwrap(ctx context.Context, u *structs.Unwrap) (int64, error) {
	var id int64
	err := a.db.QueryRow(ctx, `
		INSERT INTO unwraps (owner_id, location_id, wallet_id, wallet_address, wallet_role, destination_address, amount_wei, tx_hash, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'submitted')
		ON CONFLICT (tx_hash) DO UPDATE SET updated_at = NOW()
		RETURNING id;
	`, u.OwnerID, u.LocationID, u.WalletID, strings.ToLower(strings.TrimSpace(u.WalletAddress)), u.WalletRole,
		strings.ToLower(strings.TrimSpace(u.DestinationAddress)), u.AmountWei, strings.ToLower(strings.TrimSpace(u.TxHash))).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("error inserting unwrap: %w", err)
	}
	return id, nil
}

const unwrapColumns = `
	id, owner_id, location_id, wallet_id, wallet_address, wallet_role, destination_address, amount_wei, tx_hash,
	status, bridge_drain_id, bridge_state, bank_reference, last_synced_at, created_at, updated_at`

func scanUnwrap(row pgx.Row) (*structs.Unwrap, error) {
	var u structs.Unwrap
	err := row.Scan(&u.ID, &u.OwnerID, &u.LocationID, &u.WalletID, &u.WalletAddress, &u.WalletRole, &u.DestinationAddress, &u.AmountWei, &u.TxHash,
		&u.Status, &u.BridgeDrainID, &u.BridgeState, &u.BankReference, &u.LastSyncedAt, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (a *AppDB) ListUnwrapsByOwner(ctx context.Context, ownerID string, limit int) ([]*structs.Unwrap, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := a.db.Query(ctx, `SELECT `+unwrapColumns+` FROM unwraps WHERE owner_id = $1 ORDER BY created_at DESC LIMIT $2;`, ownerID, limit)
	if err != nil {
		return nil, fmt.Errorf("error listing unwraps: %w", err)
	}
	defer rows.Close()
	out := []*structs.Unwrap{}
	for rows.Next() {
		u, err := scanUnwrap(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning unwrap: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (a *AppDB) ListAllUnwraps(ctx context.Context, limit int) ([]*structs.Unwrap, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := a.db.Query(ctx, `SELECT `+unwrapColumns+` FROM unwraps ORDER BY created_at DESC LIMIT $1;`, limit)
	if err != nil {
		return nil, fmt.Errorf("error listing unwraps: %w", err)
	}
	defer rows.Close()
	out := []*structs.Unwrap{}
	for rows.Next() {
		u, err := scanUnwrap(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning unwrap: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ListOpenUnwraps is the sweep's worklist: rows Bridge has not finished with.
func (a *AppDB) ListOpenUnwraps(ctx context.Context, limit int) ([]*structs.Unwrap, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := a.db.Query(ctx, `
		SELECT `+unwrapColumns+` FROM unwraps
		WHERE status IN ('submitted','funds_received','payment_submitted')
		ORDER BY COALESCE(last_synced_at, created_at) ASC LIMIT $1;
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("error listing open unwraps: %w", err)
	}
	defer rows.Close()
	out := []*structs.Unwrap{}
	for rows.Next() {
		u, err := scanUnwrap(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning unwrap: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (a *AppDB) UpdateUnwrapFromDrain(ctx context.Context, txHash, status, drainID, bridgeState, bankReference string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE unwraps SET
			status = $2, bridge_drain_id = $3, bridge_state = $4,
			bank_reference = CASE WHEN $5 = '' THEN bank_reference ELSE $5 END,
			last_synced_at = NOW(), updated_at = NOW()
		WHERE tx_hash = $1;
	`, strings.ToLower(strings.TrimSpace(txHash)), status, drainID, bridgeState, bankReference)
	if err != nil {
		return fmt.Errorf("error updating unwrap from drain: %w", err)
	}
	return nil
}

func (a *AppDB) TouchUnwrapSynced(ctx context.Context, id int64) error {
	_, err := a.db.Exec(ctx, `UPDATE unwraps SET last_synced_at = NOW() WHERE id = $1;`, id)
	return err
}

// UnwrapStatusFromDrainState maps Bridge's drain vocabulary onto the ledger's.
func UnwrapStatusFromDrainState(state string) string {
	switch state {
	case "in_review", "funds_received":
		return UnwrapFundsReceived
	case "payment_submitted":
		return UnwrapPaymentSubmitted
	case "payment_processed":
		return UnwrapPaymentProcessed
	case "":
		return UnwrapSubmitted
	default:
		return UnwrapFailed
	}
}

var _ = time.Now

// FindMerchantOwnersByEmail returns every active account whose contact email
// matches (case-insensitive, trimmed), with its approved location names, so a
// caller can attach by email and still notice when the email is ambiguous.
func (a *AppDB) FindMerchantOwnersByEmail(ctx context.Context, email string) ([]structs.MerchantOwnerCandidate, error) {
	rows, err := a.db.Query(ctx, `
		SELECT u.id, COALESCE(u.contact_name, ''), COALESCE(u.contact_email, ''),
		       COALESCE(ARRAY_AGG(l.name ORDER BY l.id) FILTER (WHERE l.id IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN locations l ON l.owner_id = u.id AND l.active = TRUE AND l.approval = TRUE
		WHERE u.active = TRUE AND LOWER(TRIM(COALESCE(u.contact_email, ''))) = LOWER(TRIM($1))
		GROUP BY u.id, u.contact_name, u.contact_email
		ORDER BY COUNT(l.id) DESC, u.id`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []structs.MerchantOwnerCandidate{}
	for rows.Next() {
		var c structs.MerchantOwnerCandidate
		if err := rows.Scan(&c.OwnerID, &c.ContactName, &c.ContactEmail, &c.LocationNames); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
