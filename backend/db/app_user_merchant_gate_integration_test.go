package db

import (
	"context"
	"testing"
	"time"
)

// The gate's predicate is two conditions projected into one boolean, and which
// rows come back at all depends on the WHERE. Getting either wrong reads as a
// working gate — it just answers false for everybody, and the merchant it was
// built for writes freely. Only Postgres can say.
//
// Same throwaway database as the other db integration tests:
//
//	LOCATION_DB_TEST_URL=postgres://localhost:5432/sfluv_loc_test \
//	  go test -vet=off ./db -run Integration -v
func TestIntegrationMerchantOnboardingPending(t *testing.T) {
	a := newAccountTypeTestDB(t)
	ctx := context.Background()
	stamped := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	seedAccountTypeUser(t, a, "did:privy:new-merchant", "merchant", nil)
	seedAccountTypeUser(t, a, "did:privy:listed-merchant", "merchant", &stamped)
	seedAccountTypeUser(t, a, "did:privy:regular", "regular", nil)

	cases := []struct {
		userID string
		want   bool
		why    string
	}{
		{"did:privy:new-merchant", true, "a merchant with no listing behind them is the whole population the gate is for"},
		{"did:privy:listed-merchant", false, "every merchant on live data looks like this one, and none of them may be gated"},
		{"did:privy:regular", false, "regular accounts never see the gate, stamp or no stamp"},
		{"did:privy:nobody", false, "no user row means the privacy-policy gate is the one answering them"},
	}

	for _, testCase := range cases {
		pending, err := a.MerchantOnboardingPending(ctx, testCase.userID)
		if err != nil {
			t.Fatalf("MerchantOnboardingPending(%s) error = %v", testCase.userID, err)
		}
		if pending != testCase.want {
			t.Errorf("MerchantOnboardingPending(%s) = %v, want %v: %s",
				testCase.userID, pending, testCase.want, testCase.why)
		}
	}
}
