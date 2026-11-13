package handlers

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/ethereum/go-ethereum/common"
)

// The payout confirmation sweep.
//
// A faucet payout can reach the chain and still be recorded as unconfirmed: the
// public Celo RPC drops a search, or the caller stops waiting before the
// transfer is seen. Such a row may not be sent again, because the money may be
// out, and it may not be reported as paid, because it may not be. Before this
// sweep it stayed that way until somebody checked by hand, and the workflow
// series behind it stayed blocked.
//
// So every few minutes each unresolved row is looked up on the chain.
//
// A row whose operation was written down before it was sent is settled from
// what the chain says became of that operation, as soon as there is an answer:
// run with its transfer, it is paid; run without it, or past the paymaster's
// window without running at all, it never sent anything. A row abandoned
// before anything was written down never reached a send, and is not sent.
//
// Older rows, and the old key's sends, have only a recipient and an amount. A
// transfer found is claimed and the row settles as paid, along with whatever it
// was paying for. A transfer still absent once the paymaster's seven-day window
// has closed on any operation the row could have gone out as never will land,
// so the row becomes an ordinary not-sent failure and is paid again through
// the normal path.
//
// That used to be an hour, once a later faucet operation had landed, on the
// reasoning that operations take their nonces in order. On this deployment
// they do not: the engine gives each operation a random nonce key of its own,
// so an earlier operation still queued in the engine could land after a later
// one, after its payout had been concluded lost and paid again.
const (
	// Leaves the send that made a row time to find its own transfer.
	payoutConfirmationSettleAfter = 5 * time.Minute
	payoutConfirmationSweepLimit  = 25
	// A pending row with no outcome recorded this long after its attempt began
	// was abandoned mid-send. Must match unresolvedPayoutClause in the db
	// package.
	payoutAbandonedAfter = 15 * time.Minute
)

// ResolveUnconfirmedPayouts runs one pass of the sweep.
func (a *AppService) ResolveUnconfirmedPayouts(ctx context.Context) {
	if a.db == nil || a.protocolSvc == nil || a.protocolSvc.Available() != nil {
		return
	}

	rows, err := a.db.ListUnresolvedPayouts(ctx, a.activeChainID(), payoutConfirmationSettleAfter, payoutConfirmationSweepLimit)
	if err != nil {
		a.logger.Logf("payout confirmation sweep: %s", err)
		return
	}

	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		if err := a.resolveUnconfirmedPayout(ctx, row); err != nil {
			a.logger.Logf("payout confirmation sweep: payout %d: %s", row.ID, err)
		}
	}
}

func (a *AppService) resolveUnconfirmedPayout(ctx context.Context, row *structs.PayoutLedgerRow) error {
	// Only the Safe's transfers can be searched. A row from the old key's era
	// is left for an admin rather than concluded on evidence that cannot exist.
	held, err := a.protocolSvc.FaucetSafeHeldAt(ctx, row.CreatedAt)
	if err != nil {
		return err
	}
	if !held {
		return nil
	}
	if !common.IsHexAddress(row.RecipientAddress) {
		return fmt.Errorf("recipient %q is not an address", row.RecipientAddress)
	}
	amount, ok := new(big.Int).SetString(row.AmountBase, 10)
	if !ok || amount.Sign() <= 0 {
		return fmt.Errorf("amount %q is not a positive integer", row.AmountBase)
	}

	if row.OperationHash != "" {
		return a.resolveJournaledPayout(ctx, row, amount)
	}
	// Abandoned before any send: its attempt began, but neither path wrote
	// anything down, and both write before they send. The queue it waited in
	// died with the process, so it cannot go out now.
	if row.State == db.PayoutStatePending && row.LastError == "" && row.AttemptStartedAt != nil && row.OperationSender == "" {
		return a.concludePayoutNotSent(ctx, row, fmt.Sprintf(
			"abandoned before it was sent: its attempt began at %s and nothing was recorded as sent",
			row.AttemptStartedAt.UTC().Format(time.RFC3339)))
	}

	search, err := a.protocolSvc.FindFaucetPayout(ctx, common.HexToAddress(row.RecipientAddress), amount, row.CreatedAt)
	if err != nil {
		return err
	}

	if search.TxHash != "" {
		return a.settleConfirmedPayout(ctx, row, search.TxHash)
	}
	// No operation was written down for this row, so nothing can be asked
	// about it directly. It is concluded lost only once the paymaster's window
	// has closed for any operation its last attempt could have made: until
	// then one may still be waiting in the engine's queue and land.
	if time.Since(lastPayoutAttempt(row)) >= faucetSponsorshipWindow+faucetSponsorshipMargin {
		return a.concludePayoutNotSent(ctx, row, search.describeSearch(row.CreatedAt))
	}
	return nil
}

// lastPayoutAttempt is the latest a row's last attempt can have been sponsored:
// every write to the row comes at or after the attempt that made it.
func lastPayoutAttempt(row *structs.PayoutLedgerRow) time.Time {
	latest := row.CreatedAt
	if row.UpdatedAt.After(latest) {
		latest = row.UpdatedAt
	}
	if row.AttemptStartedAt != nil && row.AttemptStartedAt.After(latest) {
		latest = *row.AttemptStartedAt
	}
	return latest
}

// resolveJournaledPayout settles a payout from what the chain says became of
// the operation it was written down as. There is no waiting period: every
// conclusion here rests on the chain's own record, not on how long it has been.
func (a *AppService) resolveJournaledPayout(ctx context.Context, row *structs.PayoutLedgerRow, amount *big.Int) error {
	nonce, ok := new(big.Int).SetString(row.OperationNonce, 10)
	if !ok {
		return fmt.Errorf("operation nonce %q is not a number", row.OperationNonce)
	}
	since := row.CreatedAt
	if row.AttemptStartedAt != nil {
		since = *row.AttemptStartedAt
	}

	search, err := a.protocolSvc.FindFaucetOperation(ctx, JournaledOperation{
		Hash: row.OperationHash, Sender: row.OperationSender, Nonce: nonce,
		ValidUntil: row.OperationValidUntil, Since: since,
	}, common.HexToAddress(row.RecipientAddress), amount)
	if err != nil {
		return err
	}
	switch {
	case search.Sent:
		return a.settleConfirmedPayout(ctx, row, search.TxHash)
	case search.NeverSent():
		return a.concludePayoutNotSent(ctx, row, search.Describe(row.OperationHash))
	}
	// Not run yet and still inside its window: it may land. Asked again on
	// the next pass.
	return nil
}

// settleConfirmedPayout marks the row paid with the transfer that was found,
// then settles what it paid for.
func (a *AppService) settleConfirmedPayout(ctx context.Context, row *structs.PayoutLedgerRow, txHash string) error {
	settled, err := a.db.SettleUnresolvedPayout(ctx, row.ID, txHash, row.CreatedAt.UTC().Year())
	if err != nil || !settled {
		return err
	}
	a.logger.Logf("payout confirmation sweep: payout %d (%s %s) found on chain as %s", row.ID, row.Source, row.SourceRef, txHash)

	workflowID, stepID, isWorkflow := workflowPayoutRef(row)
	if !isWorkflow {
		// A redemption's code stayed spent while the payout was unresolved, so
		// there is nothing to undo or redo.
		return nil
	}

	chainID := a.activeChainID()
	alreadyPaid, err := a.db.WorkflowPayoutTargetSettled(ctx, workflowID, stepID)
	if err != nil {
		return err
	}
	if alreadyPaid {
		// Marked paid earlier without a transaction. Give it the one it had.
		if stepID == "" {
			_, err = a.db.BackfillWorkflowManagerPayoutTxHash(ctx, workflowID, txHash, chainID)
		} else {
			_, err = a.db.BackfillWorkflowStepPayoutTxHash(ctx, workflowID, stepID, txHash, chainID)
		}
		return err
	}

	if stepID == "" {
		err = a.db.RecordWorkflowManagerPayoutTxHash(ctx, workflowID, txHash, chainID)
	} else {
		err = a.db.RecordWorkflowStepPayoutTxHash(ctx, workflowID, stepID, txHash, chainID)
	}
	if err != nil {
		return err
	}
	if err := a.settleWorkflowPayoutFromLedger(ctx, row); err != nil {
		return err
	}
	// The series was held behind this payout; let it move on now rather than
	// at whatever next touches the workflow.
	a.processWorkflowSeriesPayouts(ctx, workflowID)
	return nil
}

// concludePayoutNotSent turns a row that provably never landed into an ordinary
// not-sent failure and puts what it paid for back in line to be paid.
func (a *AppService) concludePayoutNotSent(ctx context.Context, row *structs.PayoutLedgerRow, reason string) error {
	concluded, err := a.db.ConcludePayoutNotSent(ctx, row.ID, reason)
	if err != nil || !concluded {
		return err
	}
	a.logger.Logf("payout confirmation sweep: payout %d (%s %s) never landed: %s", row.ID, row.Source, row.SourceRef, reason)

	switch row.Source {
	case db.PayoutSourceRedemptionCode:
		// Hand the code back so the volunteer can scan it again; the retry
		// re-arms this row and pays it.
		if a.bot != nil && a.bot.db != nil {
			if _, err := a.bot.releaseRedemption(ctx, row.SourceRef, row.RecipientAddress); err != nil {
				return fmt.Errorf("could not hand code %s back: %w", row.SourceRef, err)
			}
		}
		return nil
	case db.PayoutSourceWorkflowStep, db.PayoutSourceWorkflowManager:
	default:
		return nil
	}

	workflowID, stepID, _ := workflowPayoutRef(row)
	alreadyPaid, err := a.db.WorkflowPayoutTargetSettled(ctx, workflowID, stepID)
	if err != nil {
		return err
	}
	if alreadyPaid {
		// Marked paid by a retry that reported this row as settled without
		// sending anything. The improver is owed this payout; reopening a paid
		// step is left to a person.
		a.sendUnpaidSettledPayoutEmail(row, workflowID, stepID)
		return nil
	}

	// A payout error holds a step out of the payout loop until a retry is
	// requested, which is what an improver pressing retry does. The sweep asks
	// on their behalf: the row is now a plain not-sent failure, so the retry
	// re-arms it and sends it once.
	if stepID == "" {
		err = a.db.MarkWorkflowManagerPayoutFailed(ctx, workflowID, workflowPayoutErrorTransferFailed)
		if err == nil {
			err = a.db.RequestWorkflowManagerPayoutRetry(ctx, workflowID, row.UserID)
		}
	} else {
		err = a.db.MarkWorkflowStepPayoutFailed(ctx, workflowID, stepID, workflowPayoutErrorTransferFailed)
		if err == nil {
			err = a.db.RequestWorkflowStepPayoutRetry(ctx, workflowID, stepID, row.UserID)
		}
	}
	if err != nil {
		return fmt.Errorf("could not queue workflow %s step %q for a retry: %w", workflowID, stepID, err)
	}
	a.processWorkflowSeriesPayouts(ctx, workflowID)
	return nil
}

// workflowPayoutRef splits a workflow payout's source_ref. The step is empty
// for a manager bounty.
func workflowPayoutRef(row *structs.PayoutLedgerRow) (workflowID string, stepID string, ok bool) {
	parts := strings.SplitN(row.SourceRef, ":", 2)
	switch row.Source {
	case db.PayoutSourceWorkflowStep:
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], true
		}
	case db.PayoutSourceWorkflowManager:
		if parts[0] != "" {
			return parts[0], "", true
		}
	}
	return "", "", false
}

func (a *AppService) sendUnpaidSettledPayoutEmail(row *structs.PayoutLedgerRow, workflowID string, stepID string) {
	amount := row.AmountBase
	if multiplier, err := getTokenMultiplier(); err == nil && multiplier.Sign() > 0 {
		if value, ok := new(big.Int).SetString(row.AmountBase, 10); ok {
			amount = new(big.Int).Div(value, multiplier).String()
		}
	}
	target := "Manager bounty"
	if stepID != "" {
		target = "Step " + stepID
	}
	a.sendRoleRequestEmail(
		"WORKFLOW_ADMIN_EMAIL",
		"Workflow Payout Never Sent",
		"A workflow payout is marked paid, but the money never left the faucet.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr><td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:180px;">Workflow ID</td><td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td></tr>
  <tr><td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Payout</td><td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td></tr>
  <tr><td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Amount</td><td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s SFLuv</td></tr>
  <tr><td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Recipient</td><td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td></tr>
  <tr><td style="padding:12px 0; font-size:13px; color:#6b7280;">Ledger ID</td><td style="padding:12px 0; font-size:13px; color:#111827;">%d</td></tr>
</table>
<p style="font-size:13px; color:#374151;">The improver has not been paid. Send it manually or reopen the payout.</p>`,
			utils.EscapeEmailHTML(workflowID), utils.EscapeEmailHTML(target), utils.EscapeEmailHTML(amount),
			utils.EscapeEmailHTML(row.RecipientAddress), row.ID),
	)
}
