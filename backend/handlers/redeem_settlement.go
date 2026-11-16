package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
)

// Settling a redemption code whose payout did not go through.
//
// A code is spent the moment it is scanned, before its payout is sent, so two
// scans of one code can never both be paid. The cost is that a payout which
// then sends nothing leaves the code spent with nobody paid. The send-failure
// path hands it back at once, and the confirmation sweep does once the chain
// shows nothing went out, but anything that slipped past both — a process
// stopped mid-send, a payout that never reached the ledger — left it spent for
// good, and every rescan said "already redeemed".
//
// So "already redeemed" is now checked rather than taken at its word. A rescan
// looks at the code's payouts: one that was paid or held keeps the code spent;
// one still unresolved is asked about on the chain there and then; and a code
// whose every payout provably sent nothing is handed back and redeemed for the
// person scanning it.

const (
	// redeemReleaseGrace is how old a redemption must be before a rescan may
	// hand its code back for want of a payout. Far longer than the moment
	// between a code being spent and its payout being recorded, so a rescan
	// cannot release a code whose own redemption is still being handled.
	redeemReleaseGrace = 15 * time.Minute
	// redeemRecheckEvery limits how often rescans of one code may ask the chain
	// about its payout. Each check is several RPC calls, and a rescan is
	// unauthenticated.
	redeemRecheckEvery = time.Minute
)

// redeemedCodeState is what a rescan of a spent code found.
type redeemedCodeState int

const (
	// redeemedCodeSpent: paid, held, or not checkable. "Already redeemed"
	// stands.
	redeemedCodeSpent redeemedCodeState = iota
	// redeemedCodePending: its payout is still settling, so it may yet be paid
	// or handed back.
	redeemedCodePending
	// redeemedCodeReopened: its payout provably sent nothing, and the code has
	// been handed back to be redeemed again.
	redeemedCodeReopened
)

// releaseRedemption hands a code back to be scanned again — but only if no
// payout for it may have sent money or still might, asked while the code is
// locked. Every path that releases a code goes through here, so none can hand
// one back behind a payout that is in flight or already paid.
func (s *BotService) releaseRedemption(ctx context.Context, code string, address string) (bool, error) {
	var mayRelease func(context.Context) (bool, error)
	if s.appDb != nil {
		mayRelease = func(ctx context.Context) (bool, error) {
			standing, err := s.appDb.RedemptionCodeHasStandingPayout(ctx, code)
			return !standing, err
		}
	}
	return s.db.UndoRedeemIf(ctx, code, address, s.chainID(), mayRelease)
}

// releaseAfterFailedPayout hands a code back after its payout failed before
// sending or was refused. If a payout for the code still stands, it is kept and
// the sweep or a rescan settles it.
func (s *BotService) releaseAfterFailedPayout(code string, address string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	released, err := s.releaseRedemption(ctx, code, address)
	if err != nil {
		fmt.Printf("error handing back code %s for %s after its payout did not go out: %s\n", code, address, err)
		return
	}
	if !released {
		fmt.Printf("kept code %s spent for %s: a payout for it still stands\n", code, address)
	}
}

// recheckRedeemedCode decides whether a code that answered "already redeemed"
// really was. holder is the address the code's payout is for, when one is
// still settling.
//
// Every way out of here that is not certain keeps the code spent. An error
// reading either database, a payout that cannot be resolved yet, a redemption
// too recent or too old to date: each answers spent or pending, never
// reopened, because reopening a code whose money went out pays it twice.
func (s *BotService) recheckRedeemedCode(ctx context.Context, code string) (state redeemedCodeState, holder string) {
	if s.appDb == nil {
		return redeemedCodeSpent, ""
	}

	payouts, err := s.appDb.ListRedemptionCodePayouts(ctx, code)
	if err != nil {
		fmt.Printf("rechecking spent code %s: %s\n", code, err)
		return redeemedCodeSpent, ""
	}

	// An unresolved payout is put to the chain now rather than left for the
	// sweep, so the person scanning gets the answer while they stand there.
	// Only one the sweep would take up: a send still running is watching for
	// its own transfer, and claiming it from here would make that send report
	// a failure it did not have.
	if open := openRedemptionPayout(payouts); open != nil && payoutAwaitingSweep(open) && s.app != nil && s.allowPayoutRecheck(open.ID) {
		if err := s.app.resolveUnconfirmedPayout(ctx, open); err != nil {
			fmt.Printf("rechecking spent code %s: payout %d: %s\n", code, open.ID, err)
		}
		if payouts, err = s.appDb.ListRedemptionCodePayouts(ctx, code); err != nil {
			fmt.Printf("rechecking spent code %s: %s\n", code, err)
			return redeemedCodeSpent, ""
		}
	}

	redemption, err := s.db.GetCodeRedemption(ctx, code)
	if err != nil {
		fmt.Printf("rechecking spent code %s: %s\n", code, err)
		return redeemedCodeSpent, ""
	}
	if redemption == nil {
		// Handed back already, by the check above or by the sweep.
		return redeemedCodeReopened, ""
	}

	for _, payout := range payouts {
		switch payout.State {
		case db.PayoutStatePaid, db.PayoutStateEscrowed, db.PayoutStateReleasing:
			return redeemedCodeSpent, ""
		}
	}
	if open := openRedemptionPayout(payouts); open != nil {
		return redeemedCodePending, open.RecipientAddress
	}

	// Nothing stands: every attempt failed before sending or was refused, or
	// none reached the ledger at all. The ledger row is written before any
	// money moves, so either way nothing was sent — provided the redemption is
	// not one whose payout is being recorded right now.
	if !redemptionSettledLongEnough(redemption, payouts) {
		if redemption.RedeemedAt == nil && len(payouts) == 0 {
			// From before redemptions were dated or payouts recorded: paid by
			// the old flow, as far as anything here can tell.
			return redeemedCodeSpent, ""
		}
		return redeemedCodePending, redemption.Address
	}

	released, err := s.releaseRedemption(ctx, code, redemption.Address)
	if err != nil {
		fmt.Printf("rechecking spent code %s: could not hand it back: %s\n", code, err)
		return redeemedCodeSpent, ""
	}
	if !released {
		return redeemedCodeSpent, ""
	}
	fmt.Printf("handed back code %s: no payout for its redemption by %s ever sent anything\n", code, redemption.Address)
	return redeemedCodeReopened, ""
}

// openRedemptionPayout is the payout whose outcome is not known yet: in flight,
// unconfirmed, abandoned, or a failure from before the not-sent marker.
func openRedemptionPayout(payouts []*structs.PayoutLedgerRow) *structs.PayoutLedgerRow {
	for _, payout := range payouts {
		if payout.State == db.PayoutStatePending {
			return payout
		}
		if payout.State == db.PayoutStateFailed && !strings.HasPrefix(payout.LastError, db.PayoutNotSentPrefix) {
			return payout
		}
	}
	return nil
}

// payoutAwaitingSweep is the confirmation sweep's own test for a row, from
// ListUnresolvedPayouts: past the settling period, and unconfirmed, a failure
// from before the not-sent marker, or abandoned mid-send.
func payoutAwaitingSweep(row *structs.PayoutLedgerRow) bool {
	if time.Since(row.CreatedAt) < payoutConfirmationSettleAfter {
		return false
	}
	switch row.State {
	case db.PayoutStatePending:
		if strings.HasPrefix(row.LastError, db.PayoutUnconfirmedPrefix) {
			return true
		}
		started := row.CreatedAt
		if row.AttemptStartedAt != nil {
			started = *row.AttemptStartedAt
		}
		return row.LastError == "" && time.Since(started) >= payoutAbandonedAfter
	case db.PayoutStateFailed:
		return !strings.HasPrefix(row.LastError, db.PayoutNotSentPrefix)
	}
	return false
}

// redemptionSettledLongEnough reports whether a redemption with no standing
// payout is old enough that its own payout cannot still be on its way into the
// ledger. A redemption from before they were dated is judged by its newest
// payout instead; with neither, it cannot be dated and is never released.
func redemptionSettledLongEnough(redemption *db.CodeRedemption, payouts []*structs.PayoutLedgerRow) bool {
	if redemption.RedeemedAt != nil {
		return time.Since(*redemption.RedeemedAt) >= redeemReleaseGrace
	}
	if len(payouts) == 0 {
		return false
	}
	return time.Since(payouts[0].CreatedAt) >= redeemReleaseGrace
}

// allowPayoutRecheck rations chain checks from rescans to one a minute for each
// payout. The sweep still checks every one regardless.
func (s *BotService) allowPayoutRecheck(id int64) bool {
	now := time.Now()
	if last, ok := s.payoutRechecks.Load(id); ok && now.Sub(last.(time.Time)) < redeemRecheckEvery {
		return false
	}
	s.payoutRechecks.Store(id, now)
	return true
}
