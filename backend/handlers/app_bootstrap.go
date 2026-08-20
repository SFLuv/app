package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// GetUserBootstrap combines the three requests every client makes at startup —
// GET /users/policy-status, GET /users/delete-account/status, and GET /users —
// into one round trip. It delegates to the existing handlers in-process, so the
// nested payload shapes are exactly what the standalone endpoints return and
// stay in lockstep with them. The standalone endpoints are unchanged, so
// existing clients are unaffected; this is purely an additive fast path.
//
// The policy status block is the only part a brand-new account gets: the
// profile is withheld until the privacy policy is accepted. That is why
// account_type and the merchant onboarding timestamp ride along in it — a
// client deciding whether to send somebody into merchant onboarding has to be
// able to answer that on this response alone.
func (a *AppService) GetUserBootstrap(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	run := func(handler http.HandlerFunc) (int, json.RawMessage) {
		rec := httptest.NewRecorder()
		handler(rec, r)
		body := rec.Body.Bytes()
		if !json.Valid(body) {
			body = nil
		}
		return rec.Code, json.RawMessage(body)
	}

	policyCode, policyBody := run(a.GetUserPolicyStatus)
	deleteCode, deleteBody := run(a.GetDeleteAccountStatus)

	// Only load the full profile when the user exists and has accepted the
	// privacy policy — mirroring the withActiveAuth gate on GET /users.
	profileCode := 0
	var profileBody json.RawMessage
	if policyCode == http.StatusOK {
		var policy struct {
			AcceptedPrivacyPolicy bool `json:"accepted_privacy_policy"`
		}
		if err := json.Unmarshal(policyBody, &policy); err == nil && policy.AcceptedPrivacyPolicy {
			profileCode, profileBody = run(a.GetUserAuthed)
		}
	}

	response := map[string]any{
		"policy_status_code": policyCode,
		"delete_status_code": deleteCode,
	}

	// The router's read-only gate, answered before the client makes a request
	// that trips it. Derived here rather than left to the client to work out
	// from account_type and the onboarding stamp: those say what the account
	// is, this says what the backend is currently doing about it, and the kill
	// switch can make those two different.
	merchantOnboardingRequired := a.MerchantOnboardingRequired(r.Context(), *userDid)
	response["merchant_onboarding_required"] = merchantOnboardingRequired
	if merchantOnboardingRequired {
		// The same string the 403 carries in X-SFLUV-Auth-Reason, so first
		// paint and a refused write are keyed off one value.
		response["auth_gate_reason"] = structs.AuthReasonMerchantOnboardingRequired
	}
	if policyCode == http.StatusOK && policyBody != nil {
		response["policy_status"] = policyBody
	}
	if deleteCode == http.StatusOK && deleteBody != nil {
		response["delete_status"] = deleteBody
	}
	if profileCode != 0 {
		response["profile_status_code"] = profileCode
		if profileCode == http.StatusOK && profileBody != nil {
			response["profile"] = profileBody
		}
	}

	writeJSON(w, http.StatusOK, response)
}
