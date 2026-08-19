package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The repair SQL returns the pre-update account type from the same statement
// that writes the new one, which no unit test can check: the self-join either
// reads the old row or it does not, and only Postgres knows. The audit line
// that says who changed what depends on it being the old value.
//
// Runs against the same throwaway database as the location integration tests:
//
//	LOCATION_DB_TEST_URL=postgres://localhost:5432/sfluv_loc_test \
//	  go test -vet=off ./db -run Integration -v
const accountTypeTestSchema = `
DROP TABLE IF EXISTS users CASCADE;

CREATE TABLE users (
	id TEXT PRIMARY KEY,
	active BOOLEAN NOT NULL DEFAULT true,
	account_type TEXT NOT NULL DEFAULT 'regular'
		CHECK (account_type IN ('regular', 'merchant')),
	merchant_onboarding_completed_at TIMESTAMPTZ
);
`

func newAccountTypeTestDB(t *testing.T) *AppDB {
	t.Helper()

	url := os.Getenv("LOCATION_DB_TEST_URL")
	if url == "" {
		t.Skip("LOCATION_DB_TEST_URL not set; skipping account type SQL integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connecting to %s: %v", url, err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, accountTypeTestSchema); err != nil {
		t.Fatalf("creating test schema: %v", err)
	}

	return &AppDB{db: pool}
}

func seedAccountTypeUser(t *testing.T, a *AppDB, userID string, accountType string, stamp *time.Time) {
	t.Helper()

	if _, err := a.db.Exec(context.Background(),
		`INSERT INTO users (id, account_type, merchant_onboarding_completed_at) VALUES ($1, $2, $3);`,
		userID, accountType, stamp,
	); err != nil {
		t.Fatalf("seeding %s user %s: %v", accountType, userID, err)
	}
}

// The case the route exists for: somebody signed up on a client that never
// asked, took the 'regular' default, and can never reach merchant onboarding.
// Afterwards they must be a merchant with the stamp still NULL, because that
// NULL is what sends them through setup.
func TestIntegrationSetUserAccountTypeRepairsRegularToMerchant(t *testing.T) {
	a := newAccountTypeTestDB(t)
	ctx := context.Background()
	seedAccountTypeUser(t, a, "did:privy:owner1", "regular", nil)

	result, err := a.SetUserAccountType(ctx, "did:privy:owner1", structs.AccountTypeMerchant)
	if err != nil {
		t.Fatalf("SetUserAccountType() error = %v", err)
	}

	if result.PreviousAccountType != structs.AccountTypeRegular {
		t.Errorf("previous account type = %q; want %q", result.PreviousAccountType, structs.AccountTypeRegular)
	}
	if result.AccountType != structs.AccountTypeMerchant {
		t.Errorf("account type = %q; want %q", result.AccountType, structs.AccountTypeMerchant)
	}
	if result.MerchantOnboardingCompletedAt != nil {
		t.Errorf("merchant_onboarding_completed_at = %v; want NULL so they are gated into onboarding", *result.MerchantOnboardingCompletedAt)
	}
}

// A merchant who already finished onboarding and is sent back to 'regular'
// keeps the stamp: it records that onboarding happened, and erasing it would
// march a shop owner with a live listing back through setup if anybody ever
// flipped them back.
func TestIntegrationSetUserAccountTypeKeepsOnboardingStamp(t *testing.T) {
	a := newAccountTypeTestDB(t)
	ctx := context.Background()
	stamped := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	seedAccountTypeUser(t, a, "did:privy:owner1", "merchant", &stamped)

	if _, err := a.SetUserAccountType(ctx, "did:privy:owner1", structs.AccountTypeRegular); err != nil {
		t.Fatalf("SetUserAccountType() to regular error = %v", err)
	}

	result, err := a.SetUserAccountType(ctx, "did:privy:owner1", structs.AccountTypeMerchant)
	if err != nil {
		t.Fatalf("SetUserAccountType() back to merchant error = %v", err)
	}
	if result.MerchantOnboardingCompletedAt == nil || !result.MerchantOnboardingCompletedAt.UTC().Equal(stamped) {
		t.Fatalf("merchant_onboarding_completed_at = %v; want it left at %v", result.MerchantOnboardingCompletedAt, stamped)
	}
}

// A value outside the enum must be refused before it reaches the CHECK
// constraint, so a caller that skips the handler cannot turn it into a 500.
func TestIntegrationSetUserAccountTypeRejectsUnknownType(t *testing.T) {
	a := newAccountTypeTestDB(t)
	ctx := context.Background()
	seedAccountTypeUser(t, a, "did:privy:owner1", "regular", nil)

	if _, err := a.SetUserAccountType(ctx, "did:privy:owner1", "wholesaler"); err == nil {
		t.Fatal("SetUserAccountType() accepted an account type outside the enum")
	}

	var stored string
	if err := a.db.QueryRow(ctx, `SELECT account_type FROM users WHERE id = $1;`, "did:privy:owner1").Scan(&stored); err != nil {
		t.Fatalf("reading account type: %v", err)
	}
	if stored != structs.AccountTypeRegular {
		t.Errorf("account type = %q after a rejected write; want it untouched at %q", stored, structs.AccountTypeRegular)
	}
}

// A deactivated or unknown account reports no rows, which is what the handler
// turns into a 404 rather than silently reporting a repair that never happened.
func TestIntegrationSetUserAccountTypeSkipsInactiveUsers(t *testing.T) {
	a := newAccountTypeTestDB(t)
	ctx := context.Background()
	seedAccountTypeUser(t, a, "did:privy:owner1", "regular", nil)
	if _, err := a.db.Exec(ctx, `UPDATE users SET active = FALSE WHERE id = $1;`, "did:privy:owner1"); err != nil {
		t.Fatalf("deactivating user: %v", err)
	}

	if _, err := a.SetUserAccountType(ctx, "did:privy:owner1", structs.AccountTypeMerchant); err != pgx.ErrNoRows {
		t.Fatalf("SetUserAccountType() on an inactive user error = %v; want %v", err, pgx.ErrNoRows)
	}
	if _, err := a.SetUserAccountType(ctx, "did:privy:nobody", structs.AccountTypeMerchant); err != pgx.ErrNoRows {
		t.Fatalf("SetUserAccountType() on an unknown user error = %v; want %v", err, pgx.ErrNoRows)
	}
}
