package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These four were written and then never called, which is the worst kind of
// bug: the code reads as done, the tests pass, and nothing happens. The
// scheduler one in particular meant escrow never expired, the vendor was never
// polled and no reminder ever went out.
//
// Asserting the wiring rather than the behaviour is deliberate — a unit test
// cannot tell "the sweep ran and had nothing to do" from "the sweep never ran".
func TestTaxSystemIsActuallyWiredUp(t *testing.T) {
	for _, wiring := range []struct {
		file string
		call string
		why  string
	}{
		{
			file: "workflow_maintenance_scheduler.go",
			call: "payouts.RunW9Maintenance",
			why:  "without it escrow never expires, the provider is never polled and no reminder is ever sent",
		},
		{
			file: "app_wallet.go",
			call: "AttributeUnlinkedPayoutsForUser",
			why:  "without it, redeeming to an unlinked address and linking it later permanently dodges the threshold",
		},
		{
			file: "bot.go",
			call: "s.payouts.Pay(",
			why:  "the redemption path must go through the choke point",
		},
		{
			file: "app_workflow.go",
			call: "a.payouts.Pay(",
			why:  "workflow bounties must go through the choke point",
		},
	} {
		contents, err := os.ReadFile(filepath.Join(".", wiring.file))
		if err != nil {
			t.Fatalf("could not read %s: %v", wiring.file, err)
		}
		if !strings.Contains(string(contents), wiring.call) {
			t.Errorf("%s no longer calls %s — %s", wiring.file, wiring.call, wiring.why)
		}
	}
}
