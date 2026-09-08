package handlers

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/w9provider"
)

// onTierReached records a warning tier and, the first time, tells the person.
//
// Deliberately outside the payout transaction, and deliberately unable to fail
// the payout: a push that does not send is a worse notification, not a worse
// payment. The recording is what matters — the modal is driven by stored state,
// so someone who never receives the push still sees it when they next open the
// app.
func (p *PayoutService) onTierReached(ctx context.Context, userID string, taxYear int, tier string) {
	if strings.TrimSpace(userID) == "" || tier == "" {
		return
	}

	fresh, err := p.appDb.RecordW9TierReached(ctx, userID, taxYear, tier)
	if err != nil {
		if p.logger != nil {
			p.logger.Logf("w9: could not record tier %s for %s: %s", tier, userID, err)
		}
		return
	}

	// A tier is news once. Crossing it again on the next reward is not.
	if !fresh || p.notify == nil {
		return
	}

	// The form has to exist before we point anyone at it.
	if _, err := p.EnsureW9Request(ctx, userID, taxYear); err != nil && p.logger != nil {
		p.logger.Logf("w9: could not prepare a form for %s at tier %s: %s", userID, tier, err)
	}
	p.notify.pushW9Tier(ctx, userID, taxYear, tier)
}

// onBlocked runs after a payout was refused.
//
// Re-arms the blocked modal every time, unlike the warning tiers which are shown
// once. Being refused is not a state somebody should be able to dismiss and
// forget: it is costing them money each time it happens, and the explanation
// belongs with the failure.
func (p *PayoutService) onBlocked(ctx context.Context, userID string, taxYear int) {
	if strings.TrimSpace(userID) == "" {
		return
	}

	if err := p.appDb.RearmW9BlockedTier(ctx, userID, taxYear); err != nil && p.logger != nil {
		p.logger.Logf("w9: could not re-arm the blocked notice for %s: %s", userID, err)
	}
	if _, err := p.EnsureW9Request(ctx, userID, taxYear); err != nil && p.logger != nil {
		p.logger.Logf("w9: could not prepare a form for blocked user %s: %s", userID, err)
	}
	if p.notify != nil {
		p.notify.pushW9Tier(ctx, userID, taxYear, db.W9TierBlocked)
	}
}

// onEscrowed runs after money has been held.
//
// Deliberately outside the payout transaction: asking a vendor for a form and
// pushing to a phone are both slow and both allowed to fail. Neither may hold a
// database lock, and neither may undo the hold — the money is already correctly
// withheld whether or not the person is successfully told about it.
func (p *PayoutService) onEscrowed(ctx context.Context, userID string, taxYear int, amount *big.Int) {
	if strings.TrimSpace(userID) == "" {
		return
	}

	// The crossing is a tier like the two warnings before it, and the modal
	// explaining it reads stored state. Without this row GetOutstandingW9Tier
	// keeps answering with the warning tier, so somebody whose money is
	// actually being held is shown "you are approaching the limit" — the one
	// tier whose whole point is that the money has already stopped.
	if _, err := p.appDb.RecordW9TierReached(ctx, userID, taxYear, db.W9TierEscrowed); err != nil && p.logger != nil {
		p.logger.Logf("w9: could not record the escrow tier for %s: %s", userID, err)
	}

	if _, err := p.EnsureW9Request(ctx, userID, taxYear); err != nil && p.logger != nil {
		// Not fatal. The person can still start the form from the app, and the
		// sweeper retries.
		p.logger.Logf("w9: could not prepare a form for %s: %s", userID, err)
	}

	if p.notify != nil {
		p.notify.pushW9EscrowHeld(ctx, userID, taxYear)
	}
}

// EnsureW9Request makes sure this person has somewhere to go and fill the form
// in, creating the vendor payee and request the first time.
func (p *PayoutService) EnsureW9Request(ctx context.Context, userID string, taxYear int) (*structs.W9Filing, error) {
	if p.provider == nil {
		return nil, w9provider.ErrProviderDisabled
	}

	filing, err := p.appDb.GetW9Filing(ctx, userID, taxYear)
	if err != nil {
		return nil, err
	}
	if filing != nil && db.W9StatusClears(filing.Status) {
		return filing, nil
	}

	email, _ := p.appDb.GetUserContactEmail(ctx, userID)
	contactEmail := ""
	if email != nil {
		contactEmail = *email
	}

	payeeID, err := p.appDb.GetTaxPayeeID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if payeeID == "" {
		payee, err := p.provider.EnsurePayee(ctx, w9provider.PayeeInput{UserID: userID, Email: contactEmail})
		if err != nil {
			return nil, err
		}
		payeeID = payee.ProviderPayeeID
		if err := p.appDb.UpsertTaxPayee(ctx, userID, p.provider.Name(), payeeID); err != nil {
			return nil, err
		}
	}

	// A stored link may have expired, so an existing request is re-linked rather
	// than replayed. Somebody tapping "complete your tax form" three days later
	// must not land on a dead page.
	if filing != nil && filing.ProviderRequestID != "" {
		refreshed, err := p.provider.HostedFormURL(ctx, filing.ProviderRequestID, w9provider.W9RequestInput{
			UserID:          userID,
			ProviderPayeeID: payeeID,
			TaxYear:         taxYear,
			Email:           contactEmail,
			ReturnURL:       w9ReturnURL(),
			ExistingFormURL: filing.FormURL,
		})
		if err != nil {
			return filing, err
		}
		if err := p.appDb.SaveW9ProviderRequest(ctx, userID, taxYear, p.provider.Name(),
			refreshed.ProviderRequestID, refreshed.FormURL, timePtr(refreshed.FormURLExpiresAt)); err != nil {
			return nil, err
		}
		return p.appDb.GetW9Filing(ctx, userID, taxYear)
	}

	created, err := p.provider.CreateW9Request(ctx, w9provider.W9RequestInput{
		UserID:          userID,
		ProviderPayeeID: payeeID,
		TaxYear:         taxYear,
		Email:           contactEmail,
		ReturnURL:       w9ReturnURL(),
	})
	if err != nil {
		return nil, err
	}
	if err := p.appDb.SaveW9ProviderRequest(ctx, userID, taxYear, p.provider.Name(),
		created.ProviderRequestID, created.FormURL, timePtr(created.FormURLExpiresAt)); err != nil {
		return nil, err
	}
	return p.appDb.GetW9Filing(ctx, userID, taxYear)
}

// w9ReturnURL is where the vendor sends someone once they submit.
//
// Deliberately a web page and NOT the app's URL scheme, which is what this used
// to be. A scheme here does not return somebody to the app so much as ambush
// it: iOS switches over to handle sfluv://, which dismisses the browser as a
// side effect — so it looks like it worked — and the app receives a navigation
// intent in the middle of the filing flow, before the status it is waiting on
// has arrived. That is what stopped the confirmation appearing, and the real
// vendor would have reproduced it on day one.
//
// The app closes the sheet itself, when the backend confirms the filing
// actually cleared. So this only has to be somewhere harmless to land for the
// second or two in between.
func w9ReturnURL() string {
	if raw := strings.TrimSpace(envString("W9_RETURN_URL", "")); raw != "" {
		return raw
	}
	base := strings.TrimSpace(envString("PUBLIC_BACKEND_URL", ""))
	if base == "" {
		base = strings.TrimSpace(envString("NEXT_PUBLIC_BACKEND_URL", ""))
	}
	if base == "" {
		// Nowhere safe to send them. Empty means the vendor shows its own
		// confirmation, which is a worse page but not a broken flow — and far
		// better than a scheme that disrupts the app.
		return ""
	}
	return strings.TrimSuffix(base, "/") + "/w9/complete"
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// ReleaseEscrowForUserYear pays out everything being held for a person, and
// turns anything that already lapsed into an admin back-pay claim.
//
// Reached from four directions — a vendor webhook, the poller, an admin
// override, and boot-time repair — so it must be safe to call repeatedly. It is:
// claiming rows is a state transition, and a second caller finds nothing left
// to claim.
func (p *PayoutService) ReleaseEscrowForUserYear(ctx context.Context, userID string, taxYear int) (released int, backPayRequested int64, err error) {
	claimed, err := p.appDb.ClaimEscrowedPayoutsForRelease(ctx, userID, taxYear,
		[]string{db.PayoutStateEscrowed})
	if err != nil {
		return 0, 0, err
	}

	var releasedTotal = big.NewInt(0)
	for _, row := range claimed {
		amount, ok := new(big.Int).SetString(row.AmountBase, 10)
		if !ok || amount.Sign() <= 0 {
			_ = p.appDb.ReturnPayoutToEscrow(ctx, row.ID, db.PayoutStateEscrowed, "unreadable amount")
			continue
		}

		txHash, transferErr := p.transfer(ctx, row.ID, row.RecipientAddress, amount)
		if transferErr != nil {
			// Insufficient faucet balance is the expected failure here. The row
			// goes back to escrowed and the next sweep retries it; the money is
			// never lost, only delayed.
			_ = p.appDb.ReturnPayoutToEscrow(ctx, row.ID, db.PayoutStateEscrowed, transferErr.Error())
			if p.logger != nil {
				p.logger.Logf("w9: could not release payout %d for %s: %s", row.ID, userID, transferErr)
			}
			continue
		}

		if err := p.appDb.MarkPayoutPaid(ctx, row.ID, txHash, time.Now().UTC().Year()); err != nil {
			return released, 0, err
		}
		p.settleSource(ctx, row)
		releasedTotal.Add(releasedTotal, amount)
		released++
	}

	// The warnings existed to get the form filed. It is filed, so they go —
	// leaving them would mean a modal asking for something already done.
	if err := p.appDb.ClearW9TierNotices(ctx, userID, taxYear); err != nil && p.logger != nil {
		p.logger.Logf("w9: released escrow for %s but could not clear their tier notices: %s", userID, err)
	}

	if p.notify != nil && released > 0 {
		p.notify.pushW9EscrowReleased(ctx, userID, taxYear, releasedTotal, 0)
	}
	return released, 0, nil
}

// settleSource closes the record the payout came from, now that the money has
// actually moved. A workflow step is only paid out once its transfer lands, not
// when it was first attempted.
func (p *PayoutService) settleSource(ctx context.Context, row *structs.PayoutLedgerRow) {
	if p.notify == nil {
		return
	}
	switch row.Source {
	case db.PayoutSourceWorkflowStep, db.PayoutSourceWorkflowManager:
		if err := p.notify.settleWorkflowPayoutFromLedger(ctx, row); err != nil && p.logger != nil {
			p.logger.Logf("w9: released payout %d but could not settle its workflow: %s", row.ID, err)
		}
	}
	// Redemption codes were consumed at scan time, so there is nothing further
	// to settle for them.
}

// RunW9Maintenance is the periodic pass: ask the vendor about outstanding
// filings, release anything that has been completed, expire stale holds, and
// nudge people who still owe a form.
//
// Polling rather than trusting webhooks alone is deliberate. A single dropped
// delivery would otherwise hold someone's money indefinitely.
func (p *PayoutService) RunW9Maintenance(ctx context.Context) {
	p.pollProviderFilings(ctx)
	p.repairCompletedFilings(ctx)
	p.sendEscrowReminders(ctx)
}

func (p *PayoutService) pollProviderFilings(ctx context.Context) {
	if p.provider == nil {
		return
	}
	filings, err := p.appDb.ListW9FilingsAwaitingProvider(ctx, 100, p.pollCutoff())
	if err != nil {
		if p.logger != nil {
			p.logger.Logf("w9: could not list filings awaiting the provider: %s", err)
		}
		return
	}

	for _, filing := range filings {
		if err := p.SyncFilingFromProvider(ctx, filing); err != nil && p.logger != nil {
			p.logger.Logf("w9: could not sync filing %s: %s", filing.ProviderRequestID, err)
		}
		// Stamped whether or not the read succeeded. A provider that is down
		// should not have every filing pile onto the next sweep the moment it
		// recovers; they will come round again on the normal cadence.
		if err := p.appDb.MarkW9FilingPolled(ctx, filing.ID); err != nil && p.logger != nil {
			p.logger.Logf("w9: could not stamp filing %d as polled: %s", filing.ID, err)
		}
	}
}

// webhookBackoff is how long a filing rests between sweeps once callbacks are
// carrying the news.
const webhookBackoff = time.Hour

// pollCutoff decides how stale a filing must be before the sweep asks again.
//
// Nil — meaning ask about everything, every pass — when the provider does not
// sign callbacks, because then polling is not a backstop, it is the only way we
// will ever hear. Track1099 publishes no callbacks at all, and a provider with
// no credentials configured cannot verify one, so both keep the old behaviour.
//
// Where callbacks do arrive, an outstanding filing is re-read hourly rather
// than every five minutes. That is the difference between ~288 vendor calls a
// day per unfilled form and ~24, and it costs nothing in latency: the callback
// already released the money, and this only exists for a delivery lost past all
// nine of their retries.
func (p *PayoutService) pollCutoff() *time.Time {
	if _, ok := p.provider.(w9provider.WebhookVerifier); !ok {
		return nil
	}
	cutoff := time.Now().Add(-webhookBackoff)
	return &cutoff
}

// SyncFilingFromProvider reads one filing's authoritative status and applies it.
//
// The single place a provider status turns into money moving, shared by the
// sweeper and the webhook receiver. The webhook could act on its own payload —
// it is signed, so it is not a spoofing risk — but re-reading keeps exactly one
// path that interprets a vendor status, rather than a second one that only runs
// on callbacks and is therefore the one nobody tests.
//
// Idempotent, which is what makes it safe to call from a delivery that will be
// retried up to nine times: MarkW9FilingCompleted and RecordTINMatch each
// report whether they changed anything, and the work behind them only runs when
// they did. A duplicate callback costs one API read.
func (p *PayoutService) SyncFilingFromProvider(ctx context.Context, filing *structs.W9Filing) error {
	if p.provider == nil || filing == nil {
		return nil
	}

	status, err := p.provider.GetW9Status(ctx, filing.ProviderRequestID)
	if err != nil {
		return err
	}

	{
		// Completion is the signature, and nothing else. The TIN match resolves
		// separately and can take a day; waiting on it would hold somebody's
		// money long after they had done everything asked of them.
		if status.Status == w9provider.StatusCompleted {
			changed, err := p.appDb.MarkW9FilingCompleted(ctx, filing.UserID, filing.TaxYear, status.TINType, status.Status)
			if err == nil && changed {
				if _, _, relErr := p.ReleaseEscrowForUserYear(ctx, filing.UserID, filing.TaxYear); relErr != nil && p.logger != nil {
					p.logger.Logf("w9: could not release escrow for %s after polling: %s", filing.UserID, relErr)
				}
			}
		}

		// Recorded whenever it lands, including long after release — which is
		// why completed filings stay in the poll set until it resolves. A
		// rejection marks the filing invalid so it stops clearing the NEXT
		// payout; it never reaches back for money already sent.
		if status.TINMatch != "" && status.TINMatch != w9provider.TINMatchPending {
			recorded, err := p.appDb.RecordTINMatch(ctx, filing.UserID, filing.TaxYear, status.TINMatch)
			if err != nil && p.logger != nil {
				p.logger.Logf("w9: could not record the tin match for %s: %s", filing.UserID, err)
			}
			if recorded && status.TINMatch == w9provider.TINMatchRejected && p.notify != nil {
				// They need to file again, and they need to know before the next
				// reward is held rather than after.
				p.notify.pushW9Notice(ctx, filing.UserID, "w9_required",
					"We need a corrected W-9",
					"The tax details on your W-9 could not be verified. Please complete a new one so your next rewards aren't held.",
				)
			}
		}
	}
	return nil
}

// repairCompletedFilings catches the case where a filing completed but its
// escrow did not release — a crash mid-release, or a webhook that arrived while
// the transfer was failing.
func (p *PayoutService) repairCompletedFilings(ctx context.Context) {
	stuck, err := p.appDb.ListUsersWithHeldMoneyAndClearedFilings(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Logf("w9: could not look for unreleased escrow: %s", err)
		}
		return
	}
	for _, entry := range stuck {
		if _, _, err := p.ReleaseEscrowForUserYear(ctx, entry.UserID, entry.TaxYear); err != nil && p.logger != nil {
			p.logger.Logf("w9: repair pass could not release escrow for %s: %s", entry.UserID, err)
		}
	}
}

// sendEscrowReminders nudges people who still owe a form, including one warning
// before the automatic window closes. The warning matters more than the
// reminders: crossing that line turns a self-service release into a manual one.
func (p *PayoutService) sendEscrowReminders(ctx context.Context) {
	if p.notify == nil {
		return
	}
	holders, err := p.appDb.ListUsersWithOpenEscrow(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Logf("w9: could not list people with held money: %s", err)
		}
		return
	}

	window := escrowWindow()
	for _, holder := range holders {
		if holder.OldestEscrowedAt == nil {
			continue
		}
		age := time.Since(*holder.OldestEscrowedAt)
		seq := escrowReminderSequence(age, window)
		if seq == 0 {
			continue
		}
		ok, err := p.appDb.ShouldSendEscrowReminder(ctx, holder.UserID, holder.TaxYear, seq)
		if err != nil || !ok {
			continue
		}
		p.notify.pushW9Reminder(ctx, holder.UserID, holder.TaxYear, seq, window-age)
	}
}

// escrowReminderSequence maps how long money has been held to which nudge is
// due. Sequence 1 is the pre-expiry warning; later ones are periodic.
func escrowReminderSequence(age time.Duration, window time.Duration) int {
	switch {
	case age >= window:
		// Past the window: remind about once a fortnight, forever, because the
		// money is still owed.
		return 100 + int(age/(14*24*time.Hour))
	case age >= window-48*time.Hour:
		return 1
	default:
		return 0
	}
}

// settleWorkflowPayoutFromLedger closes a workflow step or manager bounty once
// its money has actually landed.
//
// Escrow separates "earned" from "paid" for the first time in this codebase. A
// step whose bounty is being held must not be marked paid_out — it would read
// as settled to the improver and to the sweeper, which would then stop
// retrying. So settlement waits here, for the transfer.
//
// source_ref carries the identifiers: "<workflowId>:<stepId>" for a step,
// "<workflowId>" for a manager bounty.
func (a *AppService) settleWorkflowPayoutFromLedger(ctx context.Context, row *structs.PayoutLedgerRow) error {
	parts := strings.Split(row.SourceRef, ":")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return fmt.Errorf("workflow payout %d has no workflow reference", row.ID)
	}
	workflowID := parts[0]

	switch row.Source {
	case db.PayoutSourceWorkflowStep:
		if len(parts) < 2 {
			return fmt.Errorf("workflow step payout %d has no step reference", row.ID)
		}
		if _, err := a.db.MarkWorkflowStepPaidOut(ctx, workflowID, parts[1]); err != nil {
			return err
		}
	case db.PayoutSourceWorkflowManager:
		if _, err := a.db.MarkWorkflowManagerPaidOut(ctx, workflowID); err != nil {
			return err
		}
	default:
		return nil
	}

	if _, err := a.db.FinalizeWorkflowPaidOutIfSettled(ctx, workflowID); err != nil {
		return err
	}
	return nil
}
