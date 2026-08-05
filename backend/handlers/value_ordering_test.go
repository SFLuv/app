package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %s", path, err)
	}
	return string(data)
}

// The standing per-cycle organization balance system was retired: affiliates no
// longer hold a spendable balance, and faucet capacity is measured directly
// from outstanding codes. Reintroducing a reserve/refund call in a handler
// would resurrect the ledger that produced the repeated-refund bug — a failed
// delete used to credit the org on every retry.
func TestRetiredOrganizationBalanceLedgerIsNotUsedByHandlers(t *testing.T) {
	entries, err := filepath.Glob(filepath.Join(".", "*.go"))
	if err != nil {
		t.Fatalf("globbing handlers: %s", err)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry, "_test.go") {
			continue
		}
		source := readSource(t, entry)
		for _, retired := range []string{
			"ReserveOrganizationBalance(",
			"RefundOrganizationBalance(",
			"ResetOrganizationAllocations(",
		} {
			if strings.Contains(source, retired) {
				t.Errorf("%s calls %s — the standing organization balance ledger is retired; approval-time event allocations replace it", entry, retired)
			}
		}
	}
}

// Deleting an event must not perform any value mutation: removing the codes is
// what releases the committed faucet value, so there is nothing to credit and
// therefore nothing that can fire twice on a retry.
func TestDeleteEventPerformsNoValueMutation(t *testing.T) {
	source := readSource(t, "bot.go")

	start := strings.Index(source, "func (s *BotService) DeleteEvent(")
	if start < 0 {
		t.Fatal("DeleteEvent is missing")
	}
	body := source[start:]
	body = body[:strings.Index(body, "\n}\n")]

	for _, mutation := range []string{"Refund", "Reserve", "Transfer", "Send("} {
		if strings.Contains(body, mutation) {
			t.Errorf("DeleteEvent performs a value mutation (%s); deleting codes already releases the value", mutation)
		}
	}
}

// Approval is the moment faucet funds are committed: it mints the codes AND
// records the allocation. Both must land in one transaction, or an approval
// that half-failed would leave codes with no allocation record (or the reverse)
// and a retry would double one of them.
func TestApprovalCommitsCodesAndAllocationAtomically(t *testing.T) {
	source := readSource(t, filepath.Join("..", "db", "volunteer_events.go"))

	start := strings.Index(source, "func (s *BotDB) ApproveVolunteerEvent(")
	if start < 0 {
		t.Fatal("ApproveVolunteerEvent is missing")
	}
	body := source[start:]
	body = body[:strings.Index(body, "\n}\n")]

	if !strings.Contains(body, "s.db.Begin(ctx)") {
		t.Fatal("ApproveVolunteerEvent must run in a transaction")
	}

	codesAt := strings.Index(body, "INSERT INTO codes")
	allocAt := strings.Index(body, "INSERT INTO event_allocations")
	commitAt := strings.Index(body, "tx.Commit(ctx)")

	if codesAt < 0 || allocAt < 0 || commitAt < 0 {
		t.Fatal("expected code minting, allocation recording, and a commit in ApproveVolunteerEvent")
	}
	if codesAt > commitAt || allocAt > commitAt {
		t.Error("code minting and allocation recording must happen before the commit, inside the same transaction")
	}
}

// Cancelling releases the allocation. The status change guards against a repeat
// (a second cancel affects no rows), and both statements share a transaction so
// there is no window where an event is cancelled but still reserving faucet
// capacity.
func TestCancelReleasesAllocationInSameTransaction(t *testing.T) {
	source := readSource(t, filepath.Join("..", "db", "volunteer_events.go"))

	start := strings.Index(source, "func (s *BotDB) CancelVolunteerEvent(")
	if start < 0 {
		t.Fatal("CancelVolunteerEvent is missing")
	}
	body := source[start:]
	body = body[:strings.Index(body, "\n}\n")]

	if !strings.Contains(body, "s.db.Begin(ctx)") {
		t.Fatal("CancelVolunteerEvent must run in a transaction")
	}
	if !strings.Contains(body, "RowsAffected() == 0") {
		t.Error("CancelVolunteerEvent must no-op on an already-cancelled event so the release cannot fire twice")
	}
	if !strings.Contains(body, "UPDATE event_allocations") {
		t.Error("CancelVolunteerEvent must release the event's faucet allocation")
	}
}
