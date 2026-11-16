package handlers

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

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
	// DroppedForeign are hooks we never made, pointing at another origin,
	// deleted from Ponder. ForeignSkipReason says why that step did not run.
	DroppedForeign    []db.PonderHook
	DropFailures      int
	ForeignSkipReason string
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
// Writes go to our own database, and to Ponder only through its HTTP API, which
// is the sanctioned path: hooks are recreated there when asked, and hooks we
// never made that point at another origin are deleted there (see
// dropForeignPonderHooks). The Ponder database itself is only ever read.
func (a *AppService) ReconcilePonderHooks(ctx context.Context, recreateMissing bool) (*PonderHookReconcileReport, error) {
	if a.ponderDb == nil {
		return &PonderHookReconcileReport{Skipped: true, SkipReason: "no ponder database configured"}, nil
	}

	references, err := a.db.GetPonderHookReferences(ctx)
	if err != nil {
		return nil, err
	}

	existing, err := a.ponderDb.GetHooks(ctx)
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

	a.dropForeignPonderHooks(ctx, existing, references, report)

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

// dropForeignPonderHooks deletes, through Ponder's API, every hook we never made
// that points at an origin other than our own callback's.
//
// Ponder sends every stored hook the admin key with each transfer it covers.
// Until Ponder's /hooks checked that key, anybody could store one, so a hook
// planted that way keeps receiving the key — and any key it is rotated to —
// for as long as it exists. This runs at every boot so nobody has to find and
// delete them by hand.
//
// Kept, whatever they look like: any hook one of our rows references, active
// or not, and any hook on our own callback's origin, which at worst is stale
// bookkeeping and never a leak. Skipped altogether when our callback URL is
// unknown, or when we reference nothing while Ponder holds hooks — a wrong
// database is far likelier than every hook being foreign. PONDER_HOOK_DROP_FOREIGN=false
// turns it off.
func (a *AppService) dropForeignPonderHooks(ctx context.Context, existing map[int]db.PonderHook, references []db.PonderHookReference, report *PonderHookReconcileReport) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("PONDER_HOOK_DROP_FOREIGN")), "false") {
		report.ForeignSkipReason = "PONDER_HOOK_DROP_FOREIGN is false"
		return
	}
	ownOrigin := ponderHookOrigin(os.Getenv("PONDER_CALLBACK_URL"))
	if ownOrigin == "" {
		report.ForeignSkipReason = "PONDER_CALLBACK_URL is not a usable url, so our own hooks cannot be told apart"
		return
	}
	if len(references) == 0 && len(existing) > 0 {
		report.ForeignSkipReason = fmt.Sprintf("we reference no hooks while ponder holds %d; refusing to judge any of them foreign", len(existing))
		return
	}

	referenced := map[int]struct{}{}
	for _, reference := range references {
		referenced[reference.HookID] = struct{}{}
	}

	ids := make([]int, 0, len(existing))
	for id := range existing {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		hook := existing[id]
		if _, ok := referenced[id]; ok {
			continue
		}
		if ponderHookOrigin(hook.URL) == ownOrigin {
			continue
		}
		if err := a.deletePonderHook(ctx, id); err != nil {
			report.DropFailures++
			a.logger.Logf("ponder hook reconcile: could not drop foreign hook %d (%s → %s): %s", id, hook.Address, hook.URL, err)
			continue
		}
		report.DroppedForeign = append(report.DroppedForeign, hook)
	}
}

// ponderHookOrigin is a hook url's scheme and host, lowercased, or empty when
// it is not an http(s) url.
func ponderHookOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
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
	for _, hook := range report.DroppedForeign {
		a.logger.Logf("ponder hook reconcile: dropped foreign hook %d (%s → %s)", hook.ID, hook.Address, hook.URL)
	}
	if report.DropFailures > 0 {
		a.logger.Logf("ponder hook reconcile: %d foreign hook(s) could not be dropped and are still sent the admin key", report.DropFailures)
	}
	if report.ForeignSkipReason != "" {
		a.logger.Logf("ponder hook reconcile: foreign hooks not checked — %s", report.ForeignSkipReason)
	}
}
