package handlers

import (
	"testing"

	"github.com/SFLuv/app/backend/db"
)

// classify mirrors the comparison ReconcilePonderHooks performs, so the rules
// can be exercised without a database. The reconciler's own DB access is thin;
// what is worth pinning down is which references count as broken, which ids are
// safe to clear, and when the whole thing must refuse to act.
func classify(references []db.PonderHookReference, existing map[int]struct{}) (missingActive int, missingInactive int, dangling []int, orphaned []int, refuse bool) {
	referencedActive := map[int]struct{}{}
	danglingSet := map[int]struct{}{}
	for _, reference := range references {
		if reference.Active {
			referencedActive[reference.HookID] = struct{}{}
		}
		if _, ok := existing[reference.HookID]; ok {
			continue
		}
		if reference.Active {
			missingActive++
		} else {
			missingInactive++
		}
		if reference.Source == db.PonderHookSourcePush {
			danglingSet[reference.HookID] = struct{}{}
		}
	}
	for hookID := range existing {
		if _, ok := referencedActive[hookID]; !ok {
			orphaned = append(orphaned, hookID)
		}
	}
	for hookID := range danglingSet {
		dangling = append(dangling, hookID)
	}
	refuse = len(existing) == 0 && len(referencedActive) > 0
	return
}

func TestReconcileFlagsActiveSubscriptionWhoseHookIsGone(t *testing.T) {
	references := []db.PonderHookReference{
		{HookID: 1, Address: "0xaaa", Source: db.PonderHookSourcePush, Active: true},
		{HookID: 2, Address: "0xbbb", Source: db.PonderHookSourcePush, Active: true},
	}
	existing := map[int]struct{}{1: {}}

	missingActive, _, dangling, _, refuse := classify(references, existing)
	if refuse {
		t.Fatal("should not refuse when ponder still holds hooks")
	}
	if missingActive != 1 {
		t.Fatalf("expected 1 active reference missing its hook, got %d", missingActive)
	}
	if len(dangling) != 1 || dangling[0] != 2 {
		t.Fatalf("expected hook 2 to be cleared, got %v", dangling)
	}
}

// The bug this whole check exists for: a disabled subscription keeps its hook
// id, the hook is torn down, and the sync path then treats the stale id as
// proof a hook exists and never creates one. Clearing it is what lets the
// subscription recover.
func TestReconcileClearsStaleIDsLeftOnInactiveRows(t *testing.T) {
	references := []db.PonderHookReference{
		{HookID: 87, Address: "0xccc", Source: db.PonderHookSourcePush, Active: false},
		{HookID: 88, Address: "0xddd", Source: db.PonderHookSourcePush, Active: false},
		{HookID: 5, Address: "0xeee", Source: db.PonderHookSourcePush, Active: true},
	}
	existing := map[int]struct{}{5: {}}

	missingActive, missingInactive, dangling, orphaned, refuse := classify(references, existing)
	if refuse {
		t.Fatal("should not refuse: ponder holds a hook")
	}
	if missingActive != 0 {
		t.Fatalf("no active reference is broken here, got %d", missingActive)
	}
	if missingInactive != 2 {
		t.Fatalf("expected 2 stale inactive references, got %d", missingInactive)
	}
	if len(dangling) != 2 {
		t.Fatalf("both stale ids should be cleared, got %v", dangling)
	}
	if len(orphaned) != 0 {
		t.Fatalf("hook 5 is actively referenced and is not orphaned, got %v", orphaned)
	}
}

func TestReconcileReportsHooksNoActiveSubscriptionNeeds(t *testing.T) {
	references := []db.PonderHookReference{
		{HookID: 1, Address: "0xaaa", Source: db.PonderHookSourcePush, Active: true},
		{HookID: 2, Address: "0xbbb", Source: db.PonderHookSourcePush, Active: false},
	}
	existing := map[int]struct{}{1: {}, 2: {}, 3: {}}

	_, _, _, orphaned, _ := classify(references, existing)
	if len(orphaned) != 2 {
		t.Fatalf("hooks 2 and 3 have no active subscription; got %v", orphaned)
	}
}

// A merchant subscription's row id is its hook id, so it cannot be repaired by
// clearing a column. It must be reported, never silently "fixed".
func TestReconcileNeverClearsMerchantReferences(t *testing.T) {
	references := []db.PonderHookReference{
		{HookID: 9, Address: "0xaaa", Source: db.PonderHookSourceMerchant, Active: true},
		{HookID: 4, Address: "0xbbb", Source: db.PonderHookSourcePush, Active: true},
	}
	existing := map[int]struct{}{7: {}}

	missingActive, _, dangling, _, refuse := classify(references, existing)
	if refuse {
		t.Fatal("ponder holds a hook, so this is genuine drift, not a misconfiguration")
	}
	if missingActive != 2 {
		t.Fatalf("both references are broken, got %d", missingActive)
	}
	for _, hookID := range dangling {
		if hookID == 9 {
			t.Fatal("merchant hook id must never be cleared")
		}
	}
}

// Wholesale mismatch reads as a misconfiguration — wrong database, indexer
// mid-reset — and repairing it would wipe every id and mass-create hooks
// against whatever we happen to be pointed at.
func TestReconcileRefusesWhenPonderReportsNoHooksAtAll(t *testing.T) {
	references := []db.PonderHookReference{
		{HookID: 1, Address: "0xaaa", Source: db.PonderHookSourcePush, Active: true},
		{HookID: 2, Address: "0xbbb", Source: db.PonderHookSourcePush, Active: true},
	}

	_, _, _, _, refuse := classify(references, map[int]struct{}{})
	if !refuse {
		t.Fatal("an empty ponder hook table alongside active references must not be repaired automatically")
	}
}

func TestReconcileDoesNotRefuseWhenNothingIsExpected(t *testing.T) {
	_, _, _, _, refuse := classify(nil, map[int]struct{}{})
	if refuse {
		t.Fatal("no references and no hooks is a consistent, empty state")
	}
}
