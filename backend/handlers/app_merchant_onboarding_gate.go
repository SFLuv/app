package handlers

import "context"

// MerchantOnboardingGateEnabled is the kill switch for the read-only gate.
//
// It defaults to on, and the whole point of it is the other direction: the
// predicate decides who may write, so if it is ever wrong about somebody in
// production the fix has to be a restart with MERCHANT_ONBOARDING_GATE_ENABLED
// =false, not a build and a deploy.
func MerchantOnboardingGateEnabled() bool {
	return envBool("MERCHANT_ONBOARDING_GATE_ENABLED", true)
}

// MerchantOnboardingRequired reports whether this account is currently held to
// reads only. It is the single answer behind both the router's 403 and the
// flag the client bootstraps from, so the screen a merchant is shown and the
// refusal they would collect can never disagree.
//
// It fails open. A database blip here would otherwise refuse every write on
// the platform, merchant or not, because "we could not tell" and "yes, gated"
// would be the same answer — and every handler behind the gate still does its
// own authorization.
func (a *AppService) MerchantOnboardingRequired(ctx context.Context, userId string) bool {
	if a == nil || a.db == nil || userId == "" {
		return false
	}
	if !MerchantOnboardingGateEnabled() {
		return false
	}

	pending, err := a.db.MerchantOnboardingPending(ctx, userId)
	if err != nil {
		if a.logger != nil {
			a.logger.Logf("error checking merchant onboarding state for user %s: %s", userId, err)
		}
		return false
	}
	if !pending {
		return false
	}

	// Admins are never held to reads. The gate exists to walk a self-declared
	// merchant through setup, and one of the ten merchant accounts on live data
	// is already staff — locking the people who fix the platform out of it
	// because their own shop is not listed yet costs far more than it protects.
	// Asked only of accounts already behind the gate, so it is a query on a
	// request that was about to be refused rather than one on every write.
	return !a.IsAdmin(ctx, userId)
}
