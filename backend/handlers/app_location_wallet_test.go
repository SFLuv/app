package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SFLuv/app/backend/db"
	"github.com/go-chi/chi/v5"
)

// The swap errors name the offending location, so they cannot be matched
// exactly. If the classifier misses them the merchant gets a bare 500 and never
// learns which shop already holds the wallet.
func TestSwapValidationErrorsAreClassifiedAsTheMerchantsMistake(t *testing.T) {
	shouldBe400 := []string{
		"that wallet is already in use by O'Leary Labs — each location needs its own payment and tipping wallets",
		"that wallet is already this location's tipping wallet — payments and tips must stay separate",
		"that wallet does not belong to this account",
		"a location must always have a payment wallet — choose another wallet or create a new one",
		"this account has no signing wallet to derive a new address from",
		`unknown wallet role "cashbox"`,
		"only merchants can change location wallets",
		"payment wallet is required",
	}
	for _, errMsg := range shouldBe400 {
		if !containsLocationWalletValidationError(errMsg) {
			t.Errorf("expected a 400 for %q, got a 500", errMsg)
		}
	}

	// Genuine faults must keep returning 500 rather than being echoed to the user.
	shouldBe500 := []string{
		"error committing wallet replacement: connection refused",
		"error checking wallet uniqueness: timeout",
		"",
	}
	for _, errMsg := range shouldBe500 {
		if containsLocationWalletValidationError(errMsg) {
			t.Errorf("expected a 500 for %q, got a 400", errMsg)
		}
	}
}

func requestWithRole(role string, query string) *http.Request {
	r := httptest.NewRequest(http.MethodPut, "/locations/1/wallets/x"+query, nil)
	routeCtx := chi.NewRouteContext()
	if role != "" {
		routeCtx.URLParams.Add("role", role)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
}

func TestLocationWalletRoleParsing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		role    string
		query   string
		want    string
		wantErr bool
	}{
		{name: "payment from path", role: "payment", want: db.LocationWalletRolePayment},
		{name: "tipping from path", role: "tipping", want: db.LocationWalletRoleTipping},
		{name: "case insensitive", role: "TIPPING", want: db.LocationWalletRoleTipping},
		{name: "whitespace tolerated", role: "  payment  ", want: db.LocationWalletRolePayment},
		{name: "query fallback", role: "", query: "?role=tipping", want: db.LocationWalletRoleTipping},
		// Payments are the role that must always be filled, so an unspecified
		// role defaults there rather than to the optional one.
		{name: "defaults to payment", role: "", want: db.LocationWalletRolePayment},
		{name: "rejects nonsense", role: "cashbox", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := locationWalletRole(requestWithRole(tc.role, tc.query))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("role %q: expected an error, got %q", tc.role, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("role %q: unexpected error %v", tc.role, err)
			}
			if got != tc.want {
				t.Fatalf("role %q = %q; want %q", tc.role, got, tc.want)
			}
		})
	}
}

// A bare signing key must not be offered or accepted as a till: the paymaster
// and bundler act on smart accounts, so an EOA could take payments and then be
// unable to spend them.
func TestSigningKeyRejectionIsTheMerchantsMistakeNotAFault(t *testing.T) {
	msg := "that wallet is a signing key, not a smart account — locations must be paid into a smart account"
	if !containsLocationWalletValidationError(msg) {
		t.Fatal("expected a 400 explaining the wallet type, got a 500")
	}
}
