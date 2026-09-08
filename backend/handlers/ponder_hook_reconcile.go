package handlers

import (
	"context"
	"fmt"
	"sort"

	"github.com/SFLuv/app/backend/db"
)

// PonderHookReconcileReport is what a boot-time check found.
type PonderHookReconcileReport struct {
	ReferencedHooks  int
	PonderHooks      int
	MissingActive    []db.PonderHookReference
	MissingInactive  int
	OrphanedInPonder []int
	ClearedIDs       int64
	Recreated        int
	RecreateFailures int
	Skipped          bool
	SkipReason       string
}

// ReconcilePonderHooks compares the webhooks our database believes in against
// the ones Ponder actually holds.
//
// This exists because the two can drift silently and in the worst direction:
// notifications simply stop, with nothing logged and nothing to see in the app.
// A Ponder redeploy that resets ponder_hooks orphans every id we hold, and a
// failed teardown leaves an id pointing at a hook that is already gone. In both
// cases the sync path reads the id, concludes a hook exists, and never creates
// one.
//
// Writes go only to our own database. The Ponder database is the indexer's and
// is read here, never modified; hooks are created through Ponder's HTTP API,
// which is the sanctioned path.
func (a *AppService) ReconcilePonderHooks(ctx context.Context, recreateMissing bool) (*PonderHookReconcileReport, error) {
	if a.ponderDb == nil {
		return &PonderHookReconcileReport{Skipped: true, SkipReason: "no ponder database configured"}, nil
	}

	references, err := a.db.GetPonderHookReferences(ctx)
	if err != nil {
		return nil, err
	}

	existing, err := a.ponderDb.GetHookIDs(ctx)
	if err != nil {
		// Unreadable is not the same as empty. Repairing on a failed read would
		// wipe ids we cannot prove are dangling.
		return nil, err
	}

	report := &PonderHookReconcileReport{
		ReferencedHooks: len(references),
		PonderHooks:     len(existing),
	}

	referencedActive := map[int]struct{}{}
	danglingIDs := map[int]struct{}{}
	for _, reference := range references {
		if reference.Active {
			referencedActive[reference.HookID] = struct{}{}
		}
		if _, ok := existing[reference.HookID]; ok {
			continue
		}
		if reference.Active {
			report.MissingActive = append(report.MissingActive, reference)
		} else {
			report.MissingInactive++
		}
		if reference.Source == db.PonderHookSourcePush {
			danglingIDs[reference.HookID] = struct{}{}
		}
	}

	for hookID := range existing {
		if _, ok := referencedActive[hookID]; !ok {
			report.OrphanedInPonder = append(report.OrphanedInPonder, hookID)
		}
	}
	sort.Ints(report.OrphanedInPonder)

	// A wholesale mismatch is far more likely to be a misconfiguration — wrong
	// database, indexer mid-reset — than a genuine loss of every hook. Repairing
	// that automatically would delete every id we hold and then mass-create
	// replacements against whatever we are pointed at, so it stops here and asks
	// for a human instead.
	if len(existing) == 0 && len(referencedActive) > 0 {
		report.Skipped = true
		report.SkipReason = fmt.Sprintf(
			"ponder holds no hooks at all while %d active references exist; refusing to repair automatically",
			len(referencedActive),
		)
		return report, nil
	}

	if len(danglingIDs) > 0 {
		ids := make([]int, 0, len(danglingIDs))
		for hookID := range danglingIDs {
			ids = append(ids, hookID)
		}
		sort.Ints(ids)
		cleared, err := a.db.ClearMobilePushSubscriptionPonderHookIDs(ctx, ids)
		if err != nil {
			return report, err
		}
		report.ClearedIDs = cleared
	}

	if !recreateMissing {
		return report, nil
	}

	// Only push subscriptions are recreated. A merchant subscription's row id is
	// its hook id, so restoring one means minting a row with a specific id —
	// that needs a decision about identity, not a boot-time guess.
	recreatedByAddress := map[string]int{}
	for _, reference := range report.MissingActive {
		if reference.Source != db.PonderHookSourcePush {
			continue
		}
		hookID, done := recreatedByAddress[reference.Address]
		if !done {
			hook, err := a.createPonderHook(ctx, reference.Address)
			if err != nil {
				report.RecreateFailures++
				a.logger.Logf("ponder hook reconcile: could not recreate hook for %s: %s", reference.Address, err)
				continue
			}
			hookID = hook.Id
			recreatedByAddress[reference.Address] = hookID
			report.Recreated++
		}
		if err := a.db.SetMobilePushSubscriptionPonderHook(ctx, reference.Owner, reference.Token, reference.Address, hookID); err != nil {
			report.RecreateFailures++
			a.logger.Logf("ponder hook reconcile: recreated hook %d for %s but could not record it: %s", hookID, reference.Address, err)
		}
	}

	return report, nil
}

// LogPonderHookReconciliation runs the check and reports it. Never fatal: a
// drifted hook table is a problem to see, not a reason to refuse to boot.
func (a *AppService) LogPonderHookReconciliation(ctx context.Context, recreateMissing bool) {
	report, err := a.ReconcilePonderHooks(ctx, recreateMissing)
	if err != nil {
		a.logger.Logf("ponder hook reconcile: check failed: %s", err)
		return
	}
	if report.Skipped {
		a.logger.Logf("ponder hook reconcile: skipped — %s", report.SkipReason)
		return
	}

	a.logger.Logf(
		"ponder hook reconcile: %d references, %d hooks in ponder, %d active references missing a hook, %d stale ids on inactive rows, %d orphaned hooks in ponder, %d ids cleared, %d recreated, %d failures",
		report.ReferencedHooks,
		report.PonderHooks,
		len(report.MissingActive),
		report.MissingInactive,
		len(report.OrphanedInPonder),
		report.ClearedIDs,
		report.Recreated,
		report.RecreateFailures,
	)

	for _, reference := range report.MissingActive {
		a.logger.Logf(
			"ponder hook reconcile: active %s subscription for %s expected hook %d, which ponder does not have",
			reference.Source,
			reference.Address,
			reference.HookID,
		)
	}
	if len(report.OrphanedInPonder) > 0 {
		a.logger.Logf("ponder hook reconcile: hooks in ponder with no active subscription: %v", report.OrphanedInPonder)
	}
}
