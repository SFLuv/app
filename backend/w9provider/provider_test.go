package w9provider

import (
	"context"
	"testing"
)

func TestAdaptersSatisfyTheInterface(t *testing.T) {
	var _ Provider = NewFake(Config{})
	var _ Provider = NewTrack1099(Config{})
	var _ Provider = disabled{}
}

// Completion is the signature alone. The TIN match is a separate, slower event,
// and gating release on it would hold someone's money for up to a day after
// they had done everything asked of them.
func TestCompletionIsDrivenBySigningNotByTheTINMatch(t *testing.T) {
	fake := NewFake(Config{})
	// Resolve the match only after several reads, so "signed but unmatched" is
	// the state under test.
	fake.SetTINMatchOutcome(TINMatchMatched, 3)

	created, err := fake.CreateW9Request(context.Background(), W9RequestInput{UserID: "u1", TaxYear: 2026})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	before, err := fake.GetW9Status(context.Background(), created.ProviderRequestID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if before.Status == StatusCompleted {
		t.Fatal("an unsigned form must not read as completed")
	}

	if err := fake.Sign(created.ProviderRequestID, "ssn"); err != nil {
		t.Fatalf("sign: %v", err)
	}

	after, err := fake.GetW9Status(context.Background(), created.ProviderRequestID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if after.Status != StatusCompleted {
		t.Fatalf("status = %q; signing alone must complete the filing", after.Status)
	}
	if after.TINMatch != TINMatchPending {
		t.Fatalf("tin match = %q; want pending immediately after signing", after.TINMatch)
	}
	if after.CompletedAt == nil {
		t.Fatal("a completed filing must carry the time it was signed")
	}
	if after.TINType != "ssn" {
		t.Fatalf("tin type = %q; a 1099 differs by type, so it must be captured", after.TINType)
	}
}

// The match resolves later, on a subsequent poll — which is why completed
// filings stay in the poll set.
func TestTINMatchResolvesOnALaterPoll(t *testing.T) {
	fake := NewFake(Config{})
	fake.SetTINMatchOutcome(TINMatchRejected, 2)

	created, _ := fake.CreateW9Request(context.Background(), W9RequestInput{UserID: "u1", TaxYear: 2026})
	if err := fake.Sign(created.ProviderRequestID, "ein"); err != nil {
		t.Fatalf("sign: %v", err)
	}

	first, _ := fake.GetW9Status(context.Background(), created.ProviderRequestID)
	if first.TINMatch != TINMatchPending {
		t.Fatalf("first poll: %q; want pending", first.TINMatch)
	}
	second, _ := fake.GetW9Status(context.Background(), created.ProviderRequestID)

	// THE load-bearing assertion. The caller only records a match when this is
	// non-empty, so an empty value here means the rejection is dropped on the
	// floor: the filing keeps clearing future payouts, nobody is asked for a
	// corrected form, and the row polls forever.
	//
	// That is not hypothetical. The live vendor erases its TINMatching object
	// on failure and signals only through the status, and an earlier version of
	// this adapter faithfully reported the resulting emptiness — which silently
	// disabled every rejection path in the system.
	if second.TINMatch == "" {
		t.Fatal("a resolved rejection reported as empty is silently ignored by the caller")
	}
	if second.TINMatch != TINMatchRejected {
		t.Fatalf("second poll: %q; want rejected", second.TINMatch)
	}

	// The filing DOES become invalid, and that is correct — it is how the next
	// payout gets blocked. Verified against the vendor, which returns INVALID,
	// and consistent with RecordTINMatch, which independently sets status
	// 'invalid' on a rejection.
	//
	// What must never happen is money coming back. That is enforced in the
	// payout layer, not here: nothing in the release path is reversible, and a
	// rejection only ever changes what happens NEXT.
	if second.Status != StatusInvalid {
		t.Fatalf("status = %q; a rejected match must stop the filing clearing future payouts", second.Status)
	}
}

// Reusing a reference id surfaces the prior submission rather than opening a
// second one. That is the vendor's stated idempotency mechanism and the reason
// EnsurePayee can be a local no-op.
func TestReusingAReferenceIDReturnsTheSameRequest(t *testing.T) {
	fake := NewFake(Config{})
	a, err := fake.CreateW9Request(context.Background(), W9RequestInput{UserID: "u1", TaxYear: 2026})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := fake.CreateW9Request(context.Background(), W9RequestInput{UserID: "u1", TaxYear: 2026})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if a.ProviderRequestID != b.ProviderRequestID {
		t.Fatalf("two requests for one user-year: %q vs %q", a.ProviderRequestID, b.ProviderRequestID)
	}
	// A different year is a different form.
	c, _ := fake.CreateW9Request(context.Background(), W9RequestInput{UserID: "u1", TaxYear: 2027})
	if c.ProviderRequestID == a.ProviderRequestID {
		t.Fatal("a new tax year must open its own form request")
	}
}

// EnsurePayee is a no-op for this vendor: there is no recipients resource to
// create anything against.
func TestEnsurePayeeIsALocalNoOp(t *testing.T) {
	for _, p := range []Provider{NewFake(Config{}), NewTrack1099(Config{APIKey: "k", TeamAPIID: "t"})} {
		got, err := p.EnsurePayee(context.Background(), PayeeInput{UserID: "u1"})
		if err != nil {
			t.Fatalf("%s: %v", p.Name(), err)
		}
		if got.ProviderPayeeID != "u1" {
			t.Fatalf("%s returned %q; want the user id back", p.Name(), got.ProviderPayeeID)
		}
	}
}

// Every real path is team-scoped. Without the id the call would 404 against the
// vendor, so it fails early and says why instead.
func TestTrack1099RefusesWithoutATeamID(t *testing.T) {
	client := NewTrack1099(Config{APIKey: "k"})
	_, err := client.CreateW9Request(context.Background(), W9RequestInput{UserID: "u1", TaxYear: 2026})
	if err == nil {
		t.Fatal("expected a missing-team-id error")
	}
	if got := err.Error(); got == "" || !contains(got, "W9_PROVIDER_TEAM_ID") {
		t.Fatalf("error %q should name the missing setting", got)
	}
}

func TestDisabledProviderFailsLoudly(t *testing.T) {
	if _, err := New(Config{}).CreateW9Request(context.Background(), W9RequestInput{UserID: "u1"}); err != ErrProviderDisabled {
		t.Fatalf("expected ErrProviderDisabled, got %v", err)
	}
}

func TestTINMatchMapping(t *testing.T) {
	for raw, want := range map[string]string{
		"matched": TINMatchMatched, "Valid": TINMatchMatched,
		"rejected": TINMatchRejected, "mismatch": TINMatchRejected,
		"pending": TINMatchPending, "processing": TINMatchPending,
		"": "", "who knows": "",
	} {
		if got := normaliseTINMatch(raw); got != want {
			t.Errorf("%q mapped to %q; want %q", raw, got, want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
