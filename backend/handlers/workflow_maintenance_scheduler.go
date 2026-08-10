package handlers

import (
	"context"
	"time"
)

// WorkflowMaintenanceScheduler runs the workflow upkeep that used to happen
// only as a side effect of user traffic.
//
// Recurrence catch-up, stale payout lock recovery, payout reconciliation, and
// paid_out finalization were all triggered opportunistically from user-facing
// handlers. That had two failure modes seen in production:
//
//  1. The work ran on the requesting client's context, so a user navigating
//     away cancelled maintenance that belongs to everyone.
//  2. When nobody hit the right endpoint, the work simply never ran — recurring
//     series stopped advancing, and payouts whose confirmation wait had timed
//     out stayed "completed" forever even though the transfer had settled
//     on-chain, because reconciliation only existed behind a manual retry.
//
// Running it on a timer makes the system self-healing: correctness no longer
// depends on which pages users happen to open.
type WorkflowMaintenanceScheduler struct {
	app      *AppService
	interval time.Duration
}

const (
	defaultWorkflowMaintenanceInterval = 5 * time.Minute
	workflowMaintenanceSweepLimit      = 200
	workflowReconcileSweepTimeout      = 10 * time.Minute
)

func NewWorkflowMaintenanceScheduler(app *AppService, interval time.Duration) *WorkflowMaintenanceScheduler {
	if interval <= 0 {
		interval = defaultWorkflowMaintenanceInterval
	}
	return &WorkflowMaintenanceScheduler{app: app, interval: interval}
}

func (s *WorkflowMaintenanceScheduler) Start(ctx context.Context) {
	if s == nil || s.app == nil {
		return
	}

	go func() {
		// Run once shortly after boot so a restart repairs anything that was
		// interrupted mid-payout, rather than waiting a full interval.
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.RunOnce(ctx)
		}

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.RunOnce(ctx)
			}
		}
	}()
}

// RunOnce performs a single maintenance pass. Exported so it can be invoked
// directly (tests, an admin-triggered repair) without waiting for the timer.
func (s *WorkflowMaintenanceScheduler) RunOnce(ctx context.Context) {
	if s == nil || s.app == nil {
		return
	}
	if ctx.Err() != nil {
		return
	}

	s.app.runWorkflowAvailabilityMaintenance("scheduled workflow maintenance")
	s.reconcileUnsettledPayouts(ctx)
	s.finalizeSettledWorkflows(ctx)
	s.pruneResolvedNotificationReads(ctx)
	s.runVolunteerMaintenance(ctx)
}

// runVolunteerMaintenance advances volunteer series whose latest occurrence has
// ended, mints codes for any created underfunded once the faucet can cover
// them, sends due event reminders, releases spots held by portal signups that
// were never confirmed, and clears cover photos staged for an event that was
// never submitted.
func (s *WorkflowMaintenanceScheduler) runVolunteerMaintenance(parent context.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), workflowReconcileSweepTimeout)
	defer cancel()

	s.app.GenerateRecurringVolunteerEvents(ctx)
	s.app.SendDueVolunteerReminders(ctx)
	s.app.ExpireUnconfirmedSignups(ctx)
	s.app.ExpireStagedEventPhotos(ctx)
}

// pruneResolvedNotificationReads drops seen-markers whose notification has been
// resolved, so the table tracks only live notifications.
func (s *WorkflowMaintenanceScheduler) pruneResolvedNotificationReads(parent context.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), workflowMaintenanceTimeout)
	defer cancel()

	if _, err := s.app.db.PruneResolvedImproverNotificationReads(ctx); err != nil {
		s.app.logger.Logf("error pruning resolved improver notification reads: %s", err)
	}
}

// reconcileUnsettledPayouts verifies on-chain the payouts that recorded a
// transaction hash but never settled, and advances the ones that actually
// succeeded. This is the sweep that clears "completed but not paid out" steps
// without anyone pressing retry.
func (s *WorkflowMaintenanceScheduler) reconcileUnsettledPayouts(parent context.Context) {
	app := s.app
	if app.bot == nil || app.bot.bot == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), workflowReconcileSweepTimeout)
	defer cancel()

	targets, err := app.db.GetUnsettledWorkflowPayoutsWithTxHash(ctx, workflowMaintenanceSweepLimit)
	if err != nil {
		app.logger.Logf("error loading unsettled workflow payouts for reconciliation: %s", err)
		return
	}

	reconciledCount := 0
	pendingCount := 0
	for _, target := range targets {
		if ctx.Err() != nil {
			return
		}

		var reconciled, pending bool
		var reconcileErr error
		if target.IsManager {
			reconciled, pending, reconcileErr = app.reconcileWorkflowManagerPayoutByHash(ctx, target.WorkflowId, target.ImproverId)
		} else {
			reconciled, pending, reconcileErr = app.reconcileWorkflowStepPayoutByHash(ctx, target.WorkflowId, target.StepId, target.ImproverId)
		}

		if reconcileErr != nil {
			app.logger.Logf(
				"error reconciling workflow payout (workflow %s step %q manager %t): %s",
				target.WorkflowId, target.StepId, target.IsManager, reconcileErr,
			)
			continue
		}
		if reconciled {
			reconciledCount++
		}
		if pending {
			pendingCount++
		}
	}

	if reconciledCount > 0 || pendingCount > 0 {
		app.logger.Logf(
			"workflow payout reconciliation sweep: checked=%d reconciled=%d still_pending=%d",
			len(targets), reconciledCount, pendingCount,
		)
	}
}

// finalizeSettledWorkflows advances completed workflows to paid_out once every
// step and manager payout has settled. Reconciliation above can settle the last
// outstanding payout, so this runs immediately after it.
func (s *WorkflowMaintenanceScheduler) finalizeSettledWorkflows(parent context.Context) {
	app := s.app

	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), workflowMaintenanceTimeout)
	defer cancel()

	workflowIDs, err := app.db.GetWorkflowIDsAwaitingPayoutSettlement(ctx, workflowMaintenanceSweepLimit)
	if err != nil {
		app.logger.Logf("error loading workflows awaiting payout settlement: %s", err)
		return
	}

	settledCount := 0
	for _, workflowID := range workflowIDs {
		if ctx.Err() != nil {
			return
		}
		settled, err := app.db.FinalizeWorkflowPaidOutIfSettled(ctx, workflowID)
		if err != nil {
			app.logger.Logf("error finalizing workflow %s during maintenance sweep: %s", workflowID, err)
			continue
		}
		if settled {
			settledCount++
		}
	}

	if settledCount > 0 {
		app.logger.Logf("workflow finalization sweep: finalized %d workflow(s) to paid_out", settledCount)
	}
}
