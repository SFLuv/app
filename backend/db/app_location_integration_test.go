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

// These tests run the real location SQL against a real Postgres. They are the
// only way to catch a placeholder/argument mismatch, which compiles and passes
// every unit test while writing values into the wrong columns.
//
// Point LOCATION_DB_TEST_URL at a throwaway database to run them:
//
//	createdb sfluv_loc_test
//	LOCATION_DB_TEST_URL=postgres://localhost:5432/sfluv_loc_test \
//	  go test -vet=off ./db -run Integration -v
const locationTestSchema = `
DROP TABLE IF EXISTS location_hours;
DROP TABLE IF EXISTS location_payment_wallets;
DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS users;

CREATE TABLE users (
	id TEXT PRIMARY KEY,
	account_type TEXT NOT NULL DEFAULT 'regular',
	merchant_onboarding_completed_at TIMESTAMPTZ
);

CREATE TABLE locations (
	id SERIAL PRIMARY KEY,
	google_id TEXT,
	listing_source TEXT NOT NULL DEFAULT 'google_place',
	owner_id TEXT,
	name TEXT,
	description TEXT,
	type TEXT,
	approval BOOLEAN,
	approved_at TIMESTAMP,
	street TEXT,
	city TEXT,
	state TEXT,
	zip TEXT,
	lat NUMERIC,
	lng NUMERIC,
	phone TEXT,
	email TEXT,
	admin_phone TEXT,
	admin_email TEXT,
	website TEXT,
	image_url TEXT,
	icon_updated_at TIMESTAMPTZ,
	rating NUMERIC,
	maps_page TEXT,
	contact_firstname TEXT,
	contact_lastname TEXT,
	contact_phone TEXT,
	pos_system TEXT,
	sole_proprietorship TEXT,
	tipping_policy TEXT,
	tipping_division TEXT,
	table_coverage TEXT,
	service_stations INTEGER,
	tablet_model TEXT,
	messaging_service TEXT,
	reference TEXT,
	tipping_wallet_address TEXT NOT NULL DEFAULT '',
	payment_wallet_address TEXT NOT NULL DEFAULT '',
	active BOOLEAN NOT NULL DEFAULT true
);

CREATE UNIQUE INDEX locations_google_id_active_idx
	ON locations(google_id)
	WHERE active = TRUE AND google_id IS NOT NULL;

CREATE TABLE location_hours(
	location_id INTEGER REFERENCES locations(id),
	weekday INTEGER NOT NULL,
	hours TEXT,
	active BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE location_payment_wallets(
	id SERIAL PRIMARY KEY,
	location_id INTEGER NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
	wallet_address TEXT NOT NULL,
	is_default BOOLEAN NOT NULL DEFAULT false,
	active BOOLEAN NOT NULL DEFAULT true,
	delete_date TIMESTAMPTZ,
	delete_reason TEXT
);
`

func newLocationTestDB(t *testing.T) *AppDB {
	t.Helper()

	url := os.Getenv("LOCATION_DB_TEST_URL")
	if url == "" {
		t.Skip("LOCATION_DB_TEST_URL not set; skipping location SQL integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connecting to %s: %v", url, err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, locationTestSchema); err != nil {
		t.Fatalf("creating test schema: %v", err)
	}

	return &AppDB{db: pool}
}

func newTestLocation(googleID, ownerID string) *structs.Location {
	return &structs.Location{
		GoogleID:           googleID,
		OwnerID:            ownerID,
		Name:               "Shiba SF",
		Description:        "Neighborhood restaurant.",
		Type:               "Restaurant",
		Street:             "517 Balboa Street",
		City:               "San Francisco",
		State:              "California",
		ZIP:                "94118",
		Lat:                37.7766,
		Lng:                -122.4646,
		Phone:              "(415) 555-0100",
		Email:              "hello@shiba.example",
		AdminPhone:         "(415) 555-0134",
		AdminEmail:         "owner@shiba.example",
		Website:            "https://shiba.example",
		ImageURL:           "https://images.example/shiba.jpg",
		Rating:             4.6,
		MapsPage:           "https://maps.google.com/?cid=1",
		OpeningHours:       []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"},
		ContactFirstName:   "Sam",
		ContactLastName:    "Rivera",
		ContactPhone:       "(415) 555-0134",
		PosSystem:          "Square",
		SoleProprietorship: "No",
		TippingPolicy:      "Customers leave tips at their discretion",
		TippingDivision:    "All tips are pooled and divided between the team",
		TableCoverage:      "Servers are assigned to specific sections",
		ServiceStations:    4,
		TabletModel:        "iPad",
		MessagingService:   "Zapier",
		Reference:          "Word of mouth",
	}
}

type storedLocation struct {
	Name       string
	Street     string
	Phone      string
	Email      string
	Website    string
	AdminPhone string
	AdminEmail string
	ImageURL   string
	City       string
	Lat        float64
	Lng        float64
	GoogleID   string
	Approval   *bool
	OwnerID    string
}

func readLocation(t *testing.T, a *AppDB, id uint) storedLocation {
	t.Helper()

	var got storedLocation
	err := a.db.QueryRow(context.Background(), `
		SELECT
			COALESCE(name, ''), COALESCE(street, ''), COALESCE(phone, ''), COALESCE(email, ''),
			COALESCE(website, ''), COALESCE(admin_phone, ''), COALESCE(admin_email, ''),
			COALESCE(image_url, ''), COALESCE(city, ''), COALESCE(lat, 0), COALESCE(lng, 0),
			COALESCE(google_id, ''), approval, COALESCE(owner_id, '')
		FROM locations WHERE id = $1;
	`, id).Scan(
		&got.Name, &got.Street, &got.Phone, &got.Email, &got.Website, &got.AdminPhone,
		&got.AdminEmail, &got.ImageURL, &got.City, &got.Lat, &got.Lng, &got.GoogleID,
		&got.Approval, &got.OwnerID,
	)
	if err != nil {
		t.Fatalf("reading location %d: %v", id, err)
	}
	return got
}

func readHours(t *testing.T, a *AppDB, id uint) map[int]string {
	t.Helper()

	rows, err := a.db.Query(context.Background(), `
		SELECT weekday, COALESCE(hours, '') FROM location_hours WHERE location_id = $1 ORDER BY weekday;
	`, id)
	if err != nil {
		t.Fatalf("reading hours for %d: %v", id, err)
	}
	defer rows.Close()

	hours := map[int]string{}
	for rows.Next() {
		var weekday int
		var value string
		if err := rows.Scan(&weekday, &value); err != nil {
			t.Fatalf("scanning hours: %v", err)
		}
		if existing, seen := hours[weekday]; seen {
			t.Fatalf("weekday %d has more than one row (%q and %q)", weekday, existing, value)
		}
		hours[weekday] = value
	}
	return hours
}

func TestIntegrationAddLocationStoresEveryColumnInTheRightPlace(t *testing.T) {
	a := newLocationTestDB(t)
	location := newTestLocation("ChIJshiba", "did:privy:owner1")

	if err := a.AddLocation(context.Background(), location); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}
	if location.ID == 0 {
		t.Fatal("AddLocation() left ID unset; want the inserted row id")
	}

	got := readLocation(t, a, location.ID)
	if got.Website != "https://shiba.example" {
		t.Fatalf("website = %q; want the website, not another column's value", got.Website)
	}
	if got.AdminPhone != "(415) 555-0134" {
		t.Fatalf("admin_phone = %q; want the admin phone", got.AdminPhone)
	}
	if got.AdminEmail != "owner@shiba.example" {
		t.Fatalf("admin_email = %q; want the admin email", got.AdminEmail)
	}
	if got.Phone != "(415) 555-0100" {
		t.Fatalf("phone = %q; want the public phone", got.Phone)
	}
	if got.Approval != nil {
		t.Fatalf("approval = %v; want NULL so the location starts pending", *got.Approval)
	}

	hours := readHours(t, a, location.ID)
	if len(hours) != 7 {
		t.Fatalf("hours rows = %d; want 7", len(hours))
	}
	for weekday, want := range map[int]string{0: "Mon", 3: "Thu", 6: "Sun"} {
		if hours[weekday] != want {
			t.Fatalf("hours[%d] = %q; want %q", weekday, hours[weekday], want)
		}
	}
}

func TestIntegrationAddLocationRejectsDuplicateGooglePlace(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()

	if err := a.AddLocation(ctx, newTestLocation("ChIJshiba", "did:privy:owner1")); err != nil {
		t.Fatalf("first AddLocation() error = %v", err)
	}

	err := a.AddLocation(ctx, newTestLocation("ChIJshiba", "did:privy:owner2"))
	if err != ErrDuplicateGoogleLocation {
		t.Fatalf("second AddLocation() error = %v; want ErrDuplicateGoogleLocation", err)
	}

	var count int
	if err := a.db.QueryRow(ctx, `SELECT COUNT(*) FROM locations;`).Scan(&count); err != nil {
		t.Fatalf("counting locations: %v", err)
	}
	if count != 1 {
		t.Fatalf("locations = %d; want the rejected insert to have rolled back", count)
	}
}

// The regression this whole pass exists for on the update side: the SET list and
// the argument list drifted, so admin_phone was written into the public website
// column and served on the unauthenticated map endpoint.
func TestIntegrationUpdateLocationWritesColumnsInTheRightOrder(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()

	location := newTestLocation("ChIJshiba", "did:privy:owner1")
	if err := a.AddLocation(ctx, location); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	update := newTestLocation("ChIJshiba", "did:privy:owner1")
	update.ID = location.ID
	update.Name = "Shiba SF Restaurant"
	update.Street = "519 Balboa Street"
	update.Phone = "(415) 555-1111"
	update.Email = "new@shiba.example"
	update.Website = "https://new.shiba.example"
	update.AdminPhone = "(415) 555-2222"
	update.AdminEmail = "newowner@shiba.example"

	if err := a.UpdateLocation(ctx, update); err != nil {
		t.Fatalf("UpdateLocation() error = %v", err)
	}

	got := readLocation(t, a, location.ID)
	checks := []struct {
		column string
		got    string
		want   string
	}{
		{"name", got.Name, "Shiba SF Restaurant"},
		{"street", got.Street, "519 Balboa Street"},
		{"phone", got.Phone, "(415) 555-1111"},
		{"email", got.Email, "new@shiba.example"},
		{"website", got.Website, "https://new.shiba.example"},
		{"admin_phone", got.AdminPhone, "(415) 555-2222"},
		{"admin_email", got.AdminEmail, "newowner@shiba.example"},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Fatalf("%s = %q; want %q", check.column, check.got, check.want)
		}
	}
}

func TestIntegrationUpdateLocationCannotSelfApproveOrRelocate(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()

	location := newTestLocation("ChIJshiba", "did:privy:owner1")
	if err := a.AddLocation(ctx, location); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	approved := true
	update := newTestLocation("ChIJsomewhereelse", "did:privy:owner1")
	update.ID = location.ID
	update.Approval = &approved
	update.Lat = 40.7128
	update.Lng = -74.0060
	update.City = "New York"

	if err := a.UpdateLocation(ctx, update); err != nil {
		t.Fatalf("UpdateLocation() error = %v", err)
	}

	got := readLocation(t, a, location.ID)
	if got.Approval != nil {
		t.Fatalf("approval = %v; want it untouched by an owner-scoped update", *got.Approval)
	}
	if got.GoogleID != "ChIJshiba" {
		t.Fatalf("google_id = %q; want the original place", got.GoogleID)
	}
	if got.Lat != 37.7766 || got.Lng != -122.4646 {
		t.Fatalf("coords = %v,%v; want the map pin unmoved", got.Lat, got.Lng)
	}
	if got.City != "San Francisco" {
		t.Fatalf("city = %q; want it unchanged", got.City)
	}
}

func TestIntegrationUpdateLocationIsOwnerScoped(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()

	location := newTestLocation("ChIJshiba", "did:privy:owner1")
	if err := a.AddLocation(ctx, location); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	update := newTestLocation("ChIJshiba", "did:privy:attacker")
	update.ID = location.ID
	update.Name = "Taken Over"

	if err := a.UpdateLocation(ctx, update); err != pgx.ErrNoRows {
		t.Fatalf("UpdateLocation() error = %v; want pgx.ErrNoRows for a non-owner", err)
	}

	if got := readLocation(t, a, location.ID); got.Name != "Shiba SF" {
		t.Fatalf("name = %q; want the original", got.Name)
	}
}

// The old hours "update" matched only on location_id, so every row ended up
// holding the last weekday's value. A shorter new week must also shrink the row
// count rather than leaving stale weekdays behind.
func TestIntegrationGooglePlaceUpdateReplacesHoursPerWeekday(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()

	location := newTestLocation("ChIJshiba", "did:privy:owner1")
	if err := a.AddLocation(ctx, location); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	verified := &structs.VerifiedGooglePlace{
		GoogleID:     "ChIJshiba2",
		Name:         "Shiba SF",
		Type:         "Restaurant",
		Street:       "600 Balboa Street",
		City:         "San Francisco",
		State:        "California",
		ZIP:          "94118",
		Lat:          37.78,
		Lng:          -122.47,
		Website:      "https://shiba2.example",
		Rating:       4.8,
		MapsPage:     "https://maps.google.com/?cid=2",
		OpeningHours: []string{"NewMon", "NewTue", "NewWed"},
	}

	if err := a.UpdateLocationGooglePlace(ctx, "did:privy:owner1", location.ID, verified); err != nil {
		t.Fatalf("UpdateLocationGooglePlace() error = %v", err)
	}

	hours := readHours(t, a, location.ID)
	if len(hours) != 3 {
		t.Fatalf("hours rows = %d; want 3 after replacing a 7-day week with a 3-day one", len(hours))
	}
	for weekday, want := range map[int]string{0: "NewMon", 1: "NewTue", 2: "NewWed"} {
		if hours[weekday] != want {
			t.Fatalf("hours[%d] = %q; want %q", weekday, hours[weekday], want)
		}
	}

	got := readLocation(t, a, location.ID)
	if got.GoogleID != "ChIJshiba2" || got.Street != "600 Balboa Street" {
		t.Fatalf("google fields not applied: google_id=%q street=%q", got.GoogleID, got.Street)
	}
	// Merchant-authored fields are not part of a Google resync.
	if got.AdminEmail != "owner@shiba.example" {
		t.Fatalf("admin_email = %q; want it untouched by a Google resync", got.AdminEmail)
	}
}

func TestIntegrationGooglePlaceUpdateIsOwnerScopedAndRejectsTakenPlaces(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()

	first := newTestLocation("ChIJfirst", "did:privy:owner1")
	if err := a.AddLocation(ctx, first); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}
	second := newTestLocation("ChIJsecond", "did:privy:owner2")
	if err := a.AddLocation(ctx, second); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	taken := &structs.VerifiedGooglePlace{GoogleID: "ChIJfirst", Name: "Shiba SF", Lat: 37.7, Lng: -122.4}
	if err := a.UpdateLocationGooglePlace(ctx, "did:privy:owner2", second.ID, taken); err != ErrDuplicateGoogleLocation {
		t.Fatalf("UpdateLocationGooglePlace() error = %v; want ErrDuplicateGoogleLocation", err)
	}

	free := &structs.VerifiedGooglePlace{GoogleID: "ChIJthird", Name: "Shiba SF", Lat: 37.7, Lng: -122.4}
	if err := a.UpdateLocationGooglePlace(ctx, "did:privy:attacker", first.ID, free); err != pgx.ErrNoRows {
		t.Fatalf("UpdateLocationGooglePlace() error = %v; want pgx.ErrNoRows for a non-owner", err)
	}

	if got := readLocation(t, a, first.ID); got.GoogleID != "ChIJfirst" {
		t.Fatalf("google_id = %q; want the original", got.GoogleID)
	}
}

// Manual listings carry no place id. Stored as ” they would all collide on
// locations_google_id_active_idx, which covers google_id IS NOT NULL — so the
// second shop with no Google presence could never onboard. NULL is what makes
// the partial index ignore them.
func TestIntegrationAddLocationAllowsManyManualListings(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()

	first := newTestLocation("", "did:privy:owner1")
	first.ListingSource = structs.ListingSourceManual
	first.Name = "Corner Bodega"
	if err := a.AddLocation(ctx, first); err != nil {
		t.Fatalf("first manual AddLocation() error = %v", err)
	}

	second := newTestLocation("", "did:privy:owner2")
	second.ListingSource = structs.ListingSourceManual
	second.Name = "Second Street Barbers"
	if err := a.AddLocation(ctx, second); err != nil {
		t.Fatalf("second manual AddLocation() error = %v; want manual listings not to collide", err)
	}

	var googleID *string
	var listingSource string
	err := a.db.QueryRow(ctx,
		`SELECT google_id, listing_source FROM locations WHERE id = $1;`, first.ID,
	).Scan(&googleID, &listingSource)
	if err != nil {
		t.Fatalf("reading stored manual location: %v", err)
	}
	if googleID != nil {
		t.Errorf("google_id = %q; want NULL so the partial unique index skips the row", *googleID)
	}
	if listingSource != structs.ListingSourceManual {
		t.Errorf("listing_source = %q; want %q", listingSource, structs.ListingSourceManual)
	}
}

// A Google listing must still be deduplicated — the manual path widens who can
// onboard, not how many times the same Google business can.
func TestIntegrationAddLocationStillRejectsDuplicateGooglePlaceAlongsideManual(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()

	manual := newTestLocation("", "did:privy:owner1")
	manual.ListingSource = structs.ListingSourceManual
	if err := a.AddLocation(ctx, manual); err != nil {
		t.Fatalf("manual AddLocation() error = %v", err)
	}

	if err := a.AddLocation(ctx, newTestLocation("ChIJshiba", "did:privy:owner2")); err != nil {
		t.Fatalf("google AddLocation() error = %v", err)
	}
	if err := a.AddLocation(ctx, newTestLocation("ChIJshiba", "did:privy:owner3")); err != ErrDuplicateGoogleLocation {
		t.Fatalf("duplicate google AddLocation() error = %v; want ErrDuplicateGoogleLocation", err)
	}
}

func seedUser(t *testing.T, a *AppDB, userID string, accountType string) {
	t.Helper()

	if _, err := a.db.Exec(context.Background(),
		`INSERT INTO users (id, account_type) VALUES ($1, $2);`, userID, accountType,
	); err != nil {
		t.Fatalf("seeding %s user %s: %v", accountType, userID, err)
	}
}

func readOnboardingStamp(t *testing.T, a *AppDB, userID string) *time.Time {
	t.Helper()

	var stamp *time.Time
	if err := a.db.QueryRow(context.Background(),
		`SELECT merchant_onboarding_completed_at FROM users WHERE id = $1;`, userID,
	).Scan(&stamp); err != nil {
		t.Fatalf("reading onboarding stamp for %s: %v", userID, err)
	}
	return stamp
}

// The forced-onboarding gate reads this column, so a merchant who has listed a
// shop and still has NULL here is locked out of the app permanently.
func TestIntegrationAddLocationStampsMerchantOnboardingOnce(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()
	seedUser(t, a, "did:privy:owner1", "merchant")

	if err := a.AddLocation(ctx, newTestLocation("ChIJshiba", "did:privy:owner1")); err != nil {
		t.Fatalf("first AddLocation() error = %v", err)
	}

	first := readOnboardingStamp(t, a, "did:privy:owner1")
	if first == nil {
		t.Fatal("merchant_onboarding_completed_at = NULL after listing a shop; want it stamped")
	}

	second := newTestLocation("ChIJsecond", "did:privy:owner1")
	second.Name = "Shiba Outer Sunset"
	if err := a.AddLocation(ctx, second); err != nil {
		t.Fatalf("second AddLocation() error = %v", err)
	}

	// A second shop years later is not a second onboarding; the column records
	// when they first finished, and restating it would lose that.
	if again := readOnboardingStamp(t, a, "did:privy:owner1"); again == nil || !again.Equal(*first) {
		t.Fatalf("merchant_onboarding_completed_at = %v after a second location; want it left at %v", again, *first)
	}
}

// A regular account that submits a listing has never seen merchant onboarding.
// Stamping it here would skip that flow for good if they later say they are one.
func TestIntegrationAddLocationLeavesRegularAccountsUnstamped(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()
	seedUser(t, a, "did:privy:owner1", "regular")

	if err := a.AddLocation(ctx, newTestLocation("ChIJshiba", "did:privy:owner1")); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	if stamp := readOnboardingStamp(t, a, "did:privy:owner1"); stamp != nil {
		t.Fatalf("merchant_onboarding_completed_at = %v for a regular account; want NULL", *stamp)
	}
}

func attachPaymentWallet(t *testing.T, a *AppDB, locationID uint, address string, isDefault bool, active bool) {
	t.Helper()

	if _, err := a.db.Exec(context.Background(), `
		INSERT INTO location_payment_wallets (location_id, wallet_address, is_default, active)
		VALUES ($1, $2, $3, $4);
	`, locationID, address, isDefault, active); err != nil {
		t.Fatalf("attaching wallet %s to location %d: %v", address, locationID, err)
	}
}

func syncAndReadPaymentAddress(t *testing.T, a *AppDB, locationID uint) string {
	t.Helper()
	ctx := context.Background()

	tx, err := a.db.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning sync transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := syncLocationPaymentWalletAddress(ctx, tx, uint64(locationID)); err != nil {
		t.Fatalf("syncLocationPaymentWalletAddress() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing sync transaction: %v", err)
	}

	var stored string
	if err := a.db.QueryRow(ctx,
		`SELECT payment_wallet_address FROM locations WHERE id = $1;`, locationID,
	).Scan(&stored); err != nil {
		t.Fatalf("reading payment_wallet_address for %d: %v", locationID, err)
	}
	return stored
}

// The column is what the Pay button will send to, so it has to resolve to the
// same row the map's lateral join resolves to: active only, default first,
// oldest next — and retired rows must not keep their claim on it.
func TestIntegrationSyncLocationPaymentWalletAddressMatchesTheReaderTieBreak(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()

	location := newTestLocation("ChIJshiba", "did:privy:owner1")
	if err := a.AddLocation(ctx, location); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	if got := syncAndReadPaymentAddress(t, a, location.ID); got != "" {
		t.Fatalf("payment_wallet_address = %q with no wallets; want empty", got)
	}

	attachPaymentWallet(t, a, location.ID, "0xaaaa", false, true)
	if got := syncAndReadPaymentAddress(t, a, location.ID); got != "0xaaaa" {
		t.Fatalf("payment_wallet_address = %q; want the only active wallet", got)
	}

	// A later row marked default outranks an older non-default one, exactly as
	// the readers order them.
	attachPaymentWallet(t, a, location.ID, "0xbbbb", true, true)
	if got := syncAndReadPaymentAddress(t, a, location.ID); got != "0xbbbb" {
		t.Fatalf("payment_wallet_address = %q; want the default wallet to win", got)
	}

	// This is the replacement case: retire the default and the column must move
	// off it rather than keep advertising a wallet the merchant walked away from.
	if _, err := a.db.Exec(ctx,
		`UPDATE location_payment_wallets SET active = FALSE WHERE location_id = $1 AND wallet_address = '0xbbbb';`,
		location.ID,
	); err != nil {
		t.Fatalf("retiring the default wallet: %v", err)
	}
	if got := syncAndReadPaymentAddress(t, a, location.ID); got != "0xaaaa" {
		t.Fatalf("payment_wallet_address = %q after retiring the default; want the remaining active wallet", got)
	}
}

// Migration 1.46 skipped blank addresses and fell through to the next row. The
// readers do not: a blank default resolves to '' and the shop shows as having
// nowhere to be paid. The column has to agree with what is on the map, or the
// two cannot be swapped for each other later.
func TestIntegrationSyncLocationPaymentWalletAddressDoesNotSkipBlankRows(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()

	location := newTestLocation("ChIJshiba", "did:privy:owner1")
	if err := a.AddLocation(ctx, location); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	attachPaymentWallet(t, a, location.ID, "   ", true, true)
	attachPaymentWallet(t, a, location.ID, "0xaaaa", false, true)

	got := syncAndReadPaymentAddress(t, a, location.ID)
	if got != "" {
		t.Fatalf("payment_wallet_address = %q; want '' to match the reader, which does not look past a blank default", got)
	}

	var readerAddress string
	if err := a.db.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(TRIM(default_payment_wallet.wallet_address), ''), '')
		FROM locations l
		LEFT JOIN LATERAL (
			SELECT lpw.wallet_address
			FROM location_payment_wallets lpw
			WHERE lpw.location_id = l.id AND lpw.active = TRUE
			ORDER BY CASE WHEN lpw.is_default = TRUE THEN 0 ELSE 1 END, lpw.id ASC
			LIMIT 1
		) default_payment_wallet ON TRUE
		WHERE l.id = $1;
	`, location.ID).Scan(&readerAddress); err != nil {
		t.Fatalf("running the reader's lateral join: %v", err)
	}
	if got != readerAddress {
		t.Fatalf("column = %q but the reader returns %q; they must agree", got, readerAddress)
	}
}
