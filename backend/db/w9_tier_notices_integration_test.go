package db

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Real SQL against real Postgres, for the same reason the location tests exist:
// a placeholder whose type nobody agrees on compiles, runs, affects no rows and
// reports no error. This one shipped. `'w9_required:' || $2::text` makes
// Postgres infer $2 as text, so the year was sent as a parameter of a type the
// query had not asked for and the DELETE quietly matched nothing — leaving a
// bell entry dismissed through every tier that followed it.
//
//	createdb sfluv_w9_test
//	W9_DB_TEST_URL=postgres://localhost:5432/sfluv_w9_test \
//	  go test -vet=off ./db -run Integration -v
const w9NoticesTestSchema = `
DROP TABLE IF EXISTS improver_notification_reads;
DROP TABLE IF EXISTS w9_tier_notices;

CREATE TABLE w9_tier_notices (
	user_id TEXT NOT NULL,
	tax_year INTEGER NOT NULL,
	tier TEXT NOT NULL,
	notified_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	acknowledged_at TIMESTAMPTZ,
	PRIMARY KEY (user_id, tax_year, tier)
);

CREATE TABLE improver_notification_reads (
	user_id TEXT NOT NULL,
	notification_key TEXT NOT NULL,
	seen_at BIGINT NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, notification_key)
);
`

func newW9NoticesTestDB(t *testing.T) *AppDB {
	t.Helper()

	url := os.Getenv("W9_DB_TEST_URL")
	if url == "" {
		t.Skip("W9_DB_TEST_URL not set; skipping w9 notice SQL integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connecting to %s: %v", url, err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, w9NoticesTestSchema); err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
	return &AppDB{db: pool}
}

func readKeys(t *testing.T, a *AppDB, userID string) []string {
	t.Helper()
	rows, err := a.db.Query(context.Background(),
		`SELECT notification_key FROM improver_notification_reads WHERE user_id = $1 ORDER BY notification_key;`, userID)
	if err != nil {
		t.Fatalf("reading notification reads: %v", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatalf("scanning notification read: %v", err)
		}
		keys = append(keys, key)
	}
	return keys
}

func seedRead(t *testing.T, a *AppDB, userID, key string) {
	t.Helper()
	if _, err := a.db.Exec(context.Background(),
		`INSERT INTO improver_notification_reads (user_id, notification_key, seen_at) VALUES ($1, $2, 1)
		 ON CONFLICT (user_id, notification_key) DO NOTHING;`, userID, key); err != nil {
		t.Fatalf("seeding notification read: %v", err)
	}
}

// Reaching a new tier un-dismisses the bell entry. Without this, somebody who
// dismissed the 400 notice and then crossed 500 got the modal with nothing
// behind it — closing it left no way back to the form.
func TestRecordW9TierReachedClearsDismissalIntegration(t *testing.T) {
	a := newW9NoticesTestDB(t)
	ctx := context.Background()
	const user = "did:privy:tier-clear"

	fresh, err := a.RecordW9TierReached(ctx, user, 2026, W9TierNotice)
	if err != nil || !fresh {
		t.Fatalf("recording the first tier: fresh=%v err=%v", fresh, err)
	}
	seedRead(t, a, user, "w9_required:2026")

	fresh, err = a.RecordW9TierReached(ctx, user, 2026, W9TierWarning)
	if err != nil || !fresh {
		t.Fatalf("recording the second tier: fresh=%v err=%v", fresh, err)
	}
	if keys := readKeys(t, a, user); len(keys) != 0 {
		t.Fatalf("escalating a tier should clear the dismissal, still had %v", keys)
	}

	// A tier already recorded is not news, so it must not un-dismiss anything.
	seedRead(t, a, user, "w9_required:2026")
	fresh, err = a.RecordW9TierReached(ctx, user, 2026, W9TierWarning)
	if err != nil {
		t.Fatalf("re-recording the same tier: %v", err)
	}
	if fresh {
		t.Fatal("re-recording the same tier reported it as new")
	}
	if keys := readKeys(t, a, user); len(keys) != 1 {
		t.Fatalf("re-recording the same tier should leave the dismissal alone, had %v", keys)
	}
}

// Another year's dismissal is somebody else's business and must survive.
func TestClearW9NotificationReadsScopeIntegration(t *testing.T) {
	a := newW9NoticesTestDB(t)
	ctx := context.Background()
	const user = "did:privy:tier-scope"

	seedRead(t, a, user, "w9_required:2026")
	seedRead(t, a, user, "w9_escrow_held:2026")
	seedRead(t, a, user, "w9_required:2025")
	seedRead(t, a, user, "workflow_payout_pending:abc")
	seedRead(t, a, "did:privy:someone-else", "w9_required:2026")

	if err := a.clearW9NotificationReads(ctx, user, 2026); err != nil {
		t.Fatalf("clearing reads: %v", err)
	}

	got := readKeys(t, a, user)
	want := map[string]bool{"w9_required:2025": true, "workflow_payout_pending:abc": true}
	if len(got) != len(want) {
		t.Fatalf("cleared the wrong rows, left %v", got)
	}
	for _, key := range got {
		if !want[key] {
			t.Fatalf("cleared the wrong rows, left %v", got)
		}
	}
	if other := readKeys(t, a, "did:privy:someone-else"); len(other) != 1 {
		t.Fatalf("another person's dismissal was cleared, left %v", other)
	}
}

// Every refusal is news, so the blocked re-arm un-dismisses too — including the
// escrow entry, which is the one showing once money is held.
func TestRearmW9BlockedTierClearsDismissalIntegration(t *testing.T) {
	a := newW9NoticesTestDB(t)
	ctx := context.Background()
	const user = "did:privy:tier-rearm"

	if err := a.RearmW9BlockedTier(ctx, user, 2026); err != nil {
		t.Fatalf("first re-arm: %v", err)
	}
	seedRead(t, a, user, "w9_escrow_held:2026")

	if err := a.RearmW9BlockedTier(ctx, user, 2026); err != nil {
		t.Fatalf("second re-arm: %v", err)
	}
	if keys := readKeys(t, a, user); len(keys) != 0 {
		t.Fatalf("a refusal should clear the dismissal, still had %v", keys)
	}
}
