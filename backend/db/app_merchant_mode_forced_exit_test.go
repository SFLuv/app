package db

import (
	"strings"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func TestForcedExitReason(t *testing.T) {
	healthy := func() *structs.MerchantModeDevice {
		return &structs.MerchantModeDevice{
			LocationActive:   true,
			LocationApproved: true,
			WalletAddress:    "0x89250ba4f791521225D1dc841DA1Fc7B34F79b57",
		}
	}

	t.Run("a trading, approved, paid-into shop keeps its till", func(t *testing.T) {
		if reason := merchantModeForcedExitReason(healthy()); reason != "" {
			t.Fatalf("expected no forced exit, got %q", reason)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(*structs.MerchantModeDevice)
		expect string
	}{
		{
			name:   "closed location",
			mutate: func(d *structs.MerchantModeDevice) { d.LocationActive = false },
			expect: "closed",
		},
		{
			name:   "approval pulled",
			mutate: func(d *structs.MerchantModeDevice) { d.LocationApproved = false },
			expect: "no longer approved",
		},
		{
			name:   "nowhere for money to land",
			mutate: func(d *structs.MerchantModeDevice) { d.WalletAddress = "" },
			expect: "no payment wallet",
		},
		{
			name:   "a blank wallet counts as none",
			mutate: func(d *structs.MerchantModeDevice) { d.WalletAddress = "   " },
			expect: "no payment wallet",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			device := healthy()
			tc.mutate(device)
			reason := merchantModeForcedExitReason(device)
			if reason == "" {
				t.Fatalf("expected a forced exit for %s", tc.name)
			}
			if !strings.Contains(reason, tc.expect) {
				t.Fatalf("reason %q does not explain %q", reason, tc.expect)
			}
			// The reason is shown to whoever is standing at the counter, so it
			// has to say what happened, not just that something did.
			if !strings.Contains(reason, "merchant mode") {
				t.Errorf("reason %q does not say the device left merchant mode", reason)
			}
		})
	}

	// A closed shop is also unapproved and often wallet-less. The most
	// fundamental fact should be the one reported.
	t.Run("closure is reported ahead of its consequences", func(t *testing.T) {
		device := &structs.MerchantModeDevice{}
		if reason := merchantModeForcedExitReason(device); !strings.Contains(reason, "closed") {
			t.Fatalf("reason = %q; want the closure reported first", reason)
		}
	})
}
