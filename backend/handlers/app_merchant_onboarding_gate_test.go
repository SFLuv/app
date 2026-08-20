package handlers

import (
	"context"
	"testing"
)

// The switch has to be usable in the direction that matters: the gate is on by
// default, and turning it off is a restart, not a release.
func TestMerchantOnboardingGateEnabledDefaultsOn(t *testing.T) {
	t.Setenv("MERCHANT_ONBOARDING_GATE_ENABLED", "")
	if !MerchantOnboardingGateEnabled() {
		t.Fatal("MerchantOnboardingGateEnabled() = false with the variable unset; the gate must default on")
	}

	for _, value := range []string{"false", "0", "off", "no"} {
		t.Setenv("MERCHANT_ONBOARDING_GATE_ENABLED", value)
		if MerchantOnboardingGateEnabled() {
			t.Errorf("MerchantOnboardingGateEnabled() = true with the variable set to %q; want the gate off", value)
		}
	}
}

// With the switch off nobody is gated, whatever their account says. Checked
// through the predicate the router and the bootstrap response both call, since
// that is where the switch has to take effect for them to agree.
func TestMerchantOnboardingRequiredHonoursKillSwitch(t *testing.T) {
	t.Setenv("MERCHANT_ONBOARDING_GATE_ENABLED", "false")

	// A service with no database is enough here: a switched-off gate must
	// answer before it would ever look anything up.
	service := &AppService{}
	if service.MerchantOnboardingRequired(context.Background(), "did:privy:someone") {
		t.Fatal("MerchantOnboardingRequired() = true with the gate switched off")
	}
}

// The router builds its middleware from whatever service it was handed, and the
// route-table tests hand it none. A nil service must report nobody gated rather
// than panic on the first write of the process.
func TestMerchantOnboardingRequiredIsNilSafe(t *testing.T) {
	t.Setenv("MERCHANT_ONBOARDING_GATE_ENABLED", "true")

	var service *AppService
	if service.MerchantOnboardingRequired(context.Background(), "did:privy:someone") {
		t.Fatal("MerchantOnboardingRequired() = true on a nil service")
	}
}
