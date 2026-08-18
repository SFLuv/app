package handlers

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Files allowed to move money on chain directly.
//
//   - payout.go is the choke point itself.
//   - recovery.go returns a user their own recovered balance. That is not
//     income, so gating it behind a tax form would withhold somebody's own
//     money from them.
//   - bot.go still calls Drain, an admin sweep of the faucet, which is not a
//     payout to a person.
var directSendAllowlist = map[string]string{
	"payout.go":   "the choke point",
	"recovery.go": "returns a user their own balance, which is not income",
	"bot.go":      "Drain: an admin sweep, not a payout",
}

var directSendPattern = regexp.MustCompile(`\b(?:Send|SubmitTransfer|SubmitTransferBaseUnits)\(`)

// This is the test that keeps the tax gate from rotting.
//
// The old system had one check on one of three send paths, so a bounty could
// pay someone past the reporting threshold and nothing noticed. Routing
// everything through PayoutService fixed that, but nothing stops a future
// change from adding a fourth path — except this.
//
// If it fails, the fix is almost never to add a file to the allowlist. It is to
// call PayoutService.Pay.
func TestNoDirectSendsOutsideThePayoutService(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("could not read the handlers package: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, allowed := directSendAllowlist[name]; allowed {
			continue
		}

		contents, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("could not read %s: %v", name, err)
		}

		for i, line := range strings.Split(string(contents), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || !directSendPattern.MatchString(trimmed) {
				continue
			}
			// Only calls on a bot client count. Plenty of unrelated things are
			// called Send.
			if !strings.Contains(trimmed, "bot.") {
				continue
			}
			t.Errorf(
				"%s:%d sends on chain outside PayoutService:\n\t%s\n"+
					"Every payout must go through PayoutService.Pay so it is recorded and "+
					"checked against the annual reporting threshold.",
				name, i+1, trimmed,
			)
		}
	}
}

// The allowlist is a liability, so it stays short and each entry says why.
func TestDirectSendAllowlistIsJustified(t *testing.T) {
	if len(directSendAllowlist) > 3 {
		t.Errorf("the direct-send allowlist has grown to %d entries; each one is a way around the tax gate", len(directSendAllowlist))
	}
	for file, reason := range directSendAllowlist {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s is allowlisted with no stated reason", file)
		}
		if _, err := os.Stat(file); err != nil {
			t.Errorf("%s is allowlisted but no longer exists; remove it from the list", file)
		}
	}
}
