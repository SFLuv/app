package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
)

type stubMerchantGate struct{ required bool }

func (s stubMerchantGate) MerchantOnboardingRequired(ctx context.Context, userId string) bool {
	return s.required
}

// gatedTestRouter mounts the real gate in front of a handler that answers 200
// to anything reaching it, so a 200 means "the gate let this through" and a 403
// means it did not.
//
// The gated state has to be built here: no account in production is behind the
// gate — the ten merchants that exist were all stamped as onboarded by the
// migration — so a test that went looking for one would pass by finding
// nothing.
func gatedTestRouter(t *testing.T, required bool, userId string) *chi.Mux {
	t.Helper()

	mux := chi.NewRouter()
	if userId != "" {
		mux.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(r.Context(), "userDid", userId)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})
	}
	mux.Use(merchantOnboardingGate(stubMerchantGate{required: required}))
	mux.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	return mux
}

func serve(t *testing.T, mux *chi.Mux, method string, target string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, target, strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	return recorder
}

// A merchant who has not listed a shop may look at the app and may not act in
// it. The split is by method, so a route added tomorrow is covered without
// anybody remembering to cover it.
func TestMerchantOnboardingGateHoldsMerchantToReads(t *testing.T) {
	mux := gatedTestRouter(t, true, "did:privy:gated-merchant")

	refused := []struct{ method, target string }{
		{http.MethodPost, "/contacts"},
		{http.MethodPut, "/locations"},
		{http.MethodPatch, "/merchant-mode/devices/device-1"},
		{http.MethodDelete, "/ponder"},
		{http.MethodPut, "/wallets"},
		{http.MethodPost, "/improvers/request"},
		{http.MethodPost, "/merchant-mode/enable"},
	}
	for _, testCase := range refused {
		recorder := serve(t, mux, testCase.method, testCase.target)

		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s %s as a gated merchant = %d, want %d",
				testCase.method, testCase.target, recorder.Code, http.StatusForbidden)
			continue
		}
		if reason := recorder.Header().Get("X-SFLUV-Auth-Reason"); reason != structs.AuthReasonMerchantOnboardingRequired {
			t.Errorf("%s %s reason header = %q, want %q",
				testCase.method, testCase.target, reason, structs.AuthReasonMerchantOnboardingRequired)
		}

		// The body has to be JSON: GetUserBootstrap re-dispatches handlers
		// through a recorder and discards anything that will not parse, so a
		// plain-text refusal reaches that client as a bare 403.
		var body map[string]string
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Errorf("%s %s body %q is not JSON: %s",
				testCase.method, testCase.target, recorder.Body.String(), err)
			continue
		}
		if body["reason"] != structs.AuthReasonMerchantOnboardingRequired {
			t.Errorf("%s %s body reason = %q, want %q",
				testCase.method, testCase.target, body["reason"], structs.AuthReasonMerchantOnboardingRequired)
		}
	}

	allowed := []struct{ method, target string }{
		// Reads. The map is the app for most people, and a merchant part way
		// through setup is still allowed to look at it.
		{http.MethodGet, "/locations"},
		{http.MethodGet, "/users/bootstrap"},
		{http.MethodHead, "/locations/1"},
		{http.MethodGet, "/config"},

		// The write that clears the gate, and the writes that keep it from
		// being a trap.
		{http.MethodPost, "/locations"},
		{http.MethodPost, "/users"},
		{http.MethodPost, "/users/policies/accept"},
		{http.MethodPost, "/users/delete-account"},
		{http.MethodPost, "/users/delete-account/cancel"},
		{http.MethodPost, "/users/oauth/apple"},
	}
	for _, testCase := range allowed {
		recorder := serve(t, mux, testCase.method, testCase.target)

		if recorder.Code != http.StatusOK {
			t.Errorf("%s %s as a gated merchant = %d, want %d",
				testCase.method, testCase.target, recorder.Code, http.StatusOK)
		}
	}
}

// The other half: the gate must be invisible to everybody it is not about.
// Regular accounts are 384 of the 394 on the platform, and an onboarded
// merchant is every merchant there is today.
func TestMerchantOnboardingGateLeavesEveryoneElseAlone(t *testing.T) {
	onboarded := gatedTestRouter(t, false, "did:privy:regular-user")
	if recorder := serve(t, onboarded, http.MethodPost, "/contacts"); recorder.Code != http.StatusOK {
		t.Errorf("POST /contacts as an ungated user = %d, want %d", recorder.Code, http.StatusOK)
	}

	// No identified caller: webhooks, the code redemption flow and the public
	// portal signups all arrive this way, and their authorization is a token or
	// a shared key, not a session this gate could read.
	anonymous := gatedTestRouter(t, true, "")
	if recorder := serve(t, anonymous, http.MethodPost, "/redeem"); recorder.Code != http.StatusOK {
		t.Errorf("POST /redeem with no caller = %d, want %d", recorder.Code, http.StatusOK)
	}
}

// Every exemption must name a route that exists. A typo — /users/delete_account,
// say — is a dead entry that reads as a safety valve and refuses like everything
// else, and the person it strands is somebody trying to leave.
func TestMerchantOnboardingExemptPathsAreRealRoutes(t *testing.T) {
	router := newTestRouter(t)

	registered := map[string]bool{}
	err := chi.Walk(router, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		registered[method+" "+strings.TrimSuffix(route, "/")] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walking routes: %s", err)
	}

	for route := range merchantOnboardingOpenRoutes {
		if !registered[route] {
			t.Errorf("%q is exempt from the merchant gate but is not a registered route", route)
		}
	}

	// Named individually because these are the ones that matter: the shop
	// listing is the only way to clear the gate, the delete-account pair is the
	// only way out from behind it, and wallet registration is what the web
	// client does unprompted on every sign-in — gating that one made the gate
	// unsatisfiable, because the client rethrows the failure and logs the person
	// out before they ever see the onboarding form.
	for _, path := range []string{"/locations", "/users/delete-account", "/users/delete-account/cancel", "/wallets"} {
		if !merchantOnboardingGateAllows(http.MethodPost, path) {
			t.Errorf("POST %s is refused to a gated merchant; they can never finish or leave", path)
		}
	}

	// And the scoping stays a method rule, not a route rule. PUT /locations is
	// the one worth stating: it shares a path with the exemption and edits a
	// listing rather than making one.
	refused := []struct{ method, path string }{
		{http.MethodPut, "/locations"},
		{http.MethodPut, "/users"},
		{http.MethodPost, "/contacts"},
		// PUT, not POST. Registering a wallet is how the client says "this
		// address exists and is mine", which it does unprompted at every
		// sign-in; editing one is an action a gated merchant has no business
		// taking.
		{http.MethodPut, "/wallets"},
		{http.MethodPost, "/locations/1/icon"},
		{http.MethodPost, "/merchant-mode/enable"},
	}
	for _, testCase := range refused {
		if merchantOnboardingGateAllows(testCase.method, testCase.path) {
			t.Errorf("%s %s is exempt from the merchant gate; only signing in, onboarding and leaving may be",
				testCase.method, testCase.path)
		}
	}
}
