package db

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"testing"
)

// These run against the same throwaway database as the rest of the location
// integration tests:
//
//	LOCATION_DB_TEST_URL=postgres://localhost:5432/sfluv_loc_test \
//	  go test -vet=off ./db -run Integration -v
//
// The account factory is not involved. Derivation is CREATE2 arithmetic the
// caller does before any transaction opens, so a stand-in that is deterministic
// in the same way — one address per owner and index, always the same one — puts
// the collision under test exactly where the real one is.
func fakeDerivedAddress(ownerEOA string, index int) string {
	sum := fnv.New64a()
	fmt.Fprintf(sum, "%s:%d", ownerEOA, index)
	return fmt.Sprintf("0x%040x", sum.Sum64())
}

func fakeDerivation(provisioning *LocationProvisioningContext) *DerivedLocationWallets {
	paymentIndex := provisioning.DerivationStartIndex()
	return &DerivedLocationWallets{
		PaymentAddress: fakeDerivedAddress(provisioning.OwnerEOA, paymentIndex),
		PaymentIndex:   paymentIndex,
		TippingAddress: fakeDerivedAddress(provisioning.OwnerEOA, paymentIndex+1),
		TippingIndex:   paymentIndex + 1,
		Street:         provisioning.Street,
	}
}

// seedMerchant gives an owner the shape a real merchant has by the time they
// submit a listing: a Privy signer, a smart wallet at index 0, and that wallet
// recorded as their primary.
func seedMerchant(t *testing.T, a *AppDB, ownerID, eoa string) string {
	t.Helper()
	ctx := context.Background()
	primary := fakeDerivedAddress(eoa, 0)

	if _, err := a.db.Exec(ctx, `
		INSERT INTO users (id, account_type, primary_wallet_address)
		VALUES ($1, 'merchant', $2);
	`, ownerID, primary); err != nil {
		t.Fatalf("seeding user %s: %v", ownerID, err)
	}
	if _, err := a.db.Exec(ctx, `
		INSERT INTO wallets (owner, name, is_eoa, eoa_address, smart_address, smart_index)
		VALUES ($1, 'Personal', FALSE, $2, $3, 0);
	`, ownerID, eoa, primary); err != nil {
		t.Fatalf("seeding wallet for %s: %v", ownerID, err)
	}

	return primary
}

func readPaymentWallet(t *testing.T, a *AppDB, locationID uint) (paymentColumn, attached, tipping string) {
	t.Helper()
	if err := a.db.QueryRow(context.Background(), `
		SELECT
			COALESCE(l.payment_wallet_address, ''),
			COALESCE((
				SELECT p.wallet_address FROM location_payment_wallets p
				WHERE p.location_id = l.id AND p.active = TRUE AND p.is_default = TRUE
				LIMIT 1
			), ''),
			COALESCE(l.tipping_wallet_address, '')
		FROM locations l WHERE l.id = $1;
	`, locationID).Scan(&paymentColumn, &attached, &tipping); err != nil {
		t.Fatalf("reading wallets for location %d: %v", locationID, err)
	}
	return paymentColumn, attached, tipping
}

// A location created now must come out of the form with a till of its own, so
// the Locations tab has something to show before an admin has looked at it.
func TestIntegrationNewLocationIsProvisionedAtCreation(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()
	primary := seedMerchant(t, a, "did:privy:owner1", "0xEOA1")

	location := newTestLocation("ChIJshiba", "did:privy:owner1")
	if err := a.AddLocation(ctx, location); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	provisioning, err := a.GetLocationProvisioningContext(ctx, location.ID)
	if err != nil {
		t.Fatalf("GetLocationProvisioningContext() error = %v", err)
	}
	provisioning.AlwaysDerive = true

	if !provisioning.NeedsDerivedWallets() {
		t.Fatal("NeedsDerivedWallets() = false for a first location on the creation path; want its own wallets")
	}
	if got := provisioning.DerivationStartIndex(); got != 1 {
		t.Fatalf("DerivationStartIndex() = %d; want 1, past the owner's own wallet", got)
	}

	derived := fakeDerivation(provisioning)
	if err := a.ProvisionLocationWallets(ctx, location.ID, provisioning, derived); err != nil {
		t.Fatalf("ProvisionLocationWallets() error = %v", err)
	}

	paymentColumn, attached, tipping := readPaymentWallet(t, a, location.ID)
	if attached != derived.PaymentAddress {
		t.Fatalf("attached payment wallet = %q; want the derived %q", attached, derived.PaymentAddress)
	}
	if paymentColumn != derived.PaymentAddress {
		t.Fatalf("payment_wallet_address = %q; want it synced to %q", paymentColumn, derived.PaymentAddress)
	}
	if tipping != derived.TippingAddress {
		t.Fatalf("tipping_wallet_address = %q; want the derived %q", tipping, derived.TippingAddress)
	}
	if attached == primary {
		t.Fatal("the shop's till is the merchant's personal wallet; a first location must no longer inherit it")
	}

	var walletRows int
	if err := a.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM wallets WHERE owner = $1 AND smart_index IN (1, 2);
	`, "did:privy:owner1").Scan(&walletRows); err != nil {
		t.Fatalf("counting derived wallets: %v", err)
	}
	if walletRows != 2 {
		t.Fatalf("derived wallet rows = %d; want 2 (payments and tips)", walletRows)
	}
}

// Approval stays in place for locations that reach it without a till, and must
// keep its hands off the ones that arrive with one.
func TestIntegrationApprovalBackstopLeavesAProvisionedTillAlone(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()
	seedMerchant(t, a, "did:privy:owner1", "0xEOA1")

	location := newTestLocation("ChIJshiba", "did:privy:owner1")
	if err := a.AddLocation(ctx, location); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	atCreation, err := a.GetLocationProvisioningContext(ctx, location.ID)
	if err != nil {
		t.Fatalf("GetLocationProvisioningContext() error = %v", err)
	}
	atCreation.AlwaysDerive = true
	derived := fakeDerivation(atCreation)
	if err := a.ProvisionLocationWallets(ctx, location.ID, atCreation, derived); err != nil {
		t.Fatalf("ProvisionLocationWallets() error = %v", err)
	}

	atApproval, err := a.GetLocationProvisioningContext(ctx, location.ID)
	if err != nil {
		t.Fatalf("GetLocationProvisioningContext() at approval error = %v", err)
	}
	if atApproval.NeedsDerivedWallets() {
		t.Fatal("NeedsDerivedWallets() = true for an already-provisioned location; approval would mint a second till")
	}

	approved := true
	if err := a.UpdateLocationApproval(ctx, location.ID, &approved, atApproval, nil); err != nil {
		t.Fatalf("UpdateLocationApproval() error = %v", err)
	}

	if _, attached, _ := readPaymentWallet(t, a, location.ID); attached != derived.PaymentAddress {
		t.Fatalf("payment wallet after approval = %q; want the one minted at creation %q", attached, derived.PaymentAddress)
	}
}

// A location submitted before minting moved to creation still reaches approval
// with no till, and must be provisioned under the rule it was submitted under:
// a first location keeps the merchant's primary wallet.
func TestIntegrationApprovalBackstopProvisionsALocationCreatedWithoutATill(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()
	primary := seedMerchant(t, a, "did:privy:owner1", "0xEOA1")

	location := newTestLocation("ChIJshiba", "did:privy:owner1")
	if err := a.AddLocation(ctx, location); err != nil {
		t.Fatalf("AddLocation() error = %v", err)
	}

	provisioning, err := a.GetLocationProvisioningContext(ctx, location.ID)
	if err != nil {
		t.Fatalf("GetLocationProvisioningContext() error = %v", err)
	}
	if provisioning.NeedsDerivedWallets() {
		t.Fatal("NeedsDerivedWallets() = true at approval for a first location; want the inherited primary wallet")
	}

	approved := true
	if err := a.UpdateLocationApproval(ctx, location.ID, &approved, provisioning, nil); err != nil {
		t.Fatalf("UpdateLocationApproval() error = %v", err)
	}

	paymentColumn, attached, _ := readPaymentWallet(t, a, location.ID)
	if attached != primary || paymentColumn != primary {
		t.Fatalf("payment wallet = %q / column %q; want the owner's primary %q", attached, paymentColumn, primary)
	}
}

// The bug this pass exists for. The smart index is read outside the transaction
// so the chain call does not hold write locks, which means two submissions for
// one owner can derive the same pair of addresses. Writing both is two shops on
// one till, and no later migration can unmix the transfers.
func TestIntegrationConcurrentProvisioningNeverPutsTwoShopsOnOneTill(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()
	seedMerchant(t, a, "did:privy:owner1", "0xEOA1")

	first := newTestLocation("ChIJshiba", "did:privy:owner1")
	if err := a.AddLocation(ctx, first); err != nil {
		t.Fatalf("AddLocation() first error = %v", err)
	}
	second := newTestLocation("ChIJshiba2", "did:privy:owner1")
	second.Street = "600 Balboa Street"
	if err := a.AddLocation(ctx, second); err != nil {
		t.Fatalf("AddLocation() second error = %v", err)
	}

	// Both read before either writes, which is what overlapping submissions do.
	contexts := map[uint]*LocationProvisioningContext{}
	derivations := map[uint]*DerivedLocationWallets{}
	for _, id := range []uint{first.ID, second.ID} {
		provisioning, err := a.GetLocationProvisioningContext(ctx, id)
		if err != nil {
			t.Fatalf("GetLocationProvisioningContext(%d) error = %v", id, err)
		}
		provisioning.AlwaysDerive = true
		contexts[id] = provisioning
		derivations[id] = fakeDerivation(provisioning)
	}
	if derivations[first.ID].PaymentAddress != derivations[second.ID].PaymentAddress {
		t.Fatal("the two derivations differ; the test is not reproducing the race it exists for")
	}

	results := map[uint]error{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range []uint{first.ID, second.ID} {
		wg.Add(1)
		go func(locationID uint) {
			defer wg.Done()
			err := a.ProvisionLocationWallets(context.Background(), locationID, contexts[locationID], derivations[locationID])
			mu.Lock()
			results[locationID] = err
			mu.Unlock()
		}(id)
	}
	wg.Wait()

	var winner, loser uint
	for id, err := range results {
		switch {
		case err == nil:
			if winner != 0 {
				t.Fatal("both provisioning runs succeeded; they wrote the same address to two locations")
			}
			winner = id
		case errors.Is(err, ErrLocationWalletIndexMoved):
			loser = id
		default:
			t.Fatalf("ProvisionLocationWallets(%d) error = %v; want nil or ErrLocationWalletIndexMoved", id, err)
		}
	}
	if winner == 0 || loser == 0 {
		t.Fatalf("results = %v; want one success and one ErrLocationWalletIndexMoved", results)
	}

	// The retry is the caller's, and it must land on addresses of its own.
	retry, err := a.GetLocationProvisioningContext(ctx, loser)
	if err != nil {
		t.Fatalf("GetLocationProvisioningContext(%d) on retry error = %v", loser, err)
	}
	retry.AlwaysDerive = true
	if err := a.ProvisionLocationWallets(ctx, loser, retry, fakeDerivation(retry)); err != nil {
		t.Fatalf("retrying ProvisionLocationWallets(%d) error = %v", loser, err)
	}

	_, winnerWallet, winnerTips := readPaymentWallet(t, a, winner)
	_, loserWallet, loserTips := readPaymentWallet(t, a, loser)
	if winnerWallet == "" || loserWallet == "" {
		t.Fatalf("payment wallets = %q and %q; both shops must have one", winnerWallet, loserWallet)
	}
	if winnerWallet == loserWallet || winnerTips == loserTips {
		t.Fatalf("two shops share an address: payments %q/%q, tips %q/%q", winnerWallet, loserWallet, winnerTips, loserTips)
	}

	// Four derived rows, one per address. The dropped row was the other half of
	// the bug: the address was recorded against the location while the wallet it
	// named never existed.
	var derivedWallets int
	if err := a.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM wallets WHERE owner = $1 AND smart_index > 0 AND active = TRUE;
	`, "did:privy:owner1").Scan(&derivedWallets); err != nil {
		t.Fatalf("counting derived wallets: %v", err)
	}
	if derivedWallets != 4 {
		t.Fatalf("derived wallet rows = %d; want 4, one behind every address in use", derivedWallets)
	}
}

// The startup sweep grants the on-chain role per address, so what it needs is
// every address a location takes money at — payment tills and tipping wallets
// alike — and each of them once.
func TestIntegrationGetLocationWalletsListsEveryAddressOnce(t *testing.T) {
	a := newLocationTestDB(t)
	ctx := context.Background()
	seedMerchant(t, a, "did:privy:owner1", "0xEOA1")

	provisioned := map[uint]*DerivedLocationWallets{}
	for _, seed := range []struct {
		googleID string
		street   string
	}{
		{"ChIJshiba", "517 Balboa Street"},
		{"ChIJshiba2", "600 Balboa Street"},
	} {
		location := newTestLocation(seed.googleID, "did:privy:owner1")
		location.Street = seed.street
		if err := a.AddLocation(ctx, location); err != nil {
			t.Fatalf("AddLocation(%s) error = %v", seed.googleID, err)
		}

		provisioning, err := a.GetLocationProvisioningContext(ctx, location.ID)
		if err != nil {
			t.Fatalf("GetLocationProvisioningContext(%d) error = %v", location.ID, err)
		}
		provisioning.AlwaysDerive = true
		derived := fakeDerivation(provisioning)
		if err := a.ProvisionLocationWallets(ctx, location.ID, provisioning, derived); err != nil {
			t.Fatalf("ProvisionLocationWallets(%d) error = %v", location.ID, err)
		}
		provisioned[location.ID] = derived
	}

	wallets, err := a.GetLocationWallets(ctx)
	if err != nil {
		t.Fatalf("GetLocationWallets() error = %v", err)
	}
	if len(wallets) != 4 {
		t.Fatalf("location wallets = %d; want 4 (a till and a tips wallet for each of two shops)", len(wallets))
	}

	seen := map[string]string{}
	for _, wallet := range wallets {
		if previous, repeated := seen[wallet.Address]; repeated {
			t.Fatalf("address %s listed twice (%s and %s); the sweep would grant it twice", wallet.Address, previous, wallet.Role)
		}
		seen[wallet.Address] = wallet.Role
		if wallet.WalletID == nil {
			t.Fatalf("wallet %s has no wallets row; the sweep would re-read it from the chain every boot", wallet.Address)
		}
		if wallet.IsRedeemer {
			t.Fatalf("wallet %s is already recorded as a redeemer; it has never been granted", wallet.Address)
		}
	}

	for locationID, derived := range provisioned {
		if role, listed := seen[derived.PaymentAddress]; !listed || role != "payment" {
			t.Fatalf("location %d payment wallet %s listed as %q; want payment", locationID, derived.PaymentAddress, role)
		}
		if role, listed := seen[derived.TippingAddress]; !listed || role != "tipping" {
			t.Fatalf("location %d tipping wallet %s listed as %q; want tipping", locationID, derived.TippingAddress, role)
		}
	}
}
