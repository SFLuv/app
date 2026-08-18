package handlers

import (
	"math/big"
	"testing"
	"time"

	"github.com/SFLuv/app/backend/db"
)

func sfluv(n int64) *big.Int { return big.NewInt(n * 1_000_000) }

var threshold = sfluv(600)

// The threshold rules, exhaustively. This is the whole gate, and every case
// here is somebody's money.
func TestDecideEscrow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		prior      *big.Int
		amount     *big.Int
		hasOpen    bool
		filing     string
		wantEscrow bool
	}{
		{name: "well below the line pays through",
			prior: sfluv(100), amount: sfluv(50), filing: db.W9StatusNotStarted, wantEscrow: false},

		// The comparison is >=, matching the behaviour that preceded this.
		// Landing exactly on the threshold is reaching it, not approaching it.
		{name: "landing exactly on the threshold is held",
			prior: sfluv(550), amount: sfluv(50), filing: db.W9StatusNotStarted, wantEscrow: true},
		{name: "one unit short still pays through",
			prior: sfluv(550), amount: new(big.Int).Sub(sfluv(50), big.NewInt(1)),
			filing: db.W9StatusNotStarted, wantEscrow: false},

		// The payment that crosses is held in full. Splitting it would mean two
		// transfers and a half-paid reward nobody can explain to a volunteer.
		{name: "the crossing payment is held whole",
			prior: sfluv(550), amount: sfluv(100), filing: db.W9StatusNotStarted, wantEscrow: true},

		// Once anything is held, everything after is held. A payment slipping
		// between two held ones would be incoherent, and would let someone
		// wear the obligation down by earning in small pieces.
		{name: "a tiny payment after a crossing is still held",
			prior: sfluv(700), amount: big.NewInt(1), hasOpen: true,
			filing: db.W9StatusNotStarted, wantEscrow: true},
		{name: "open escrow holds even below the line",
			prior: sfluv(10), amount: sfluv(1), hasOpen: true,
			filing: db.W9StatusNotStarted, wantEscrow: true},

		// A cleared filing pays through regardless of amount or open holds.
		{name: "a completed filing pays through",
			prior: sfluv(5000), amount: sfluv(1000), filing: db.W9StatusCompleted, wantEscrow: false},
		{name: "a legacy approval pays through",
			prior: sfluv(5000), amount: sfluv(1000), filing: db.W9StatusLegacyApproved, wantEscrow: false},
		{name: "a manual clear pays through",
			prior: sfluv(5000), amount: sfluv(1000), filing: db.W9StatusManuallyCleared, wantEscrow: false},
		{name: "a cleared filing beats open escrow",
			prior: sfluv(5000), amount: sfluv(1), hasOpen: true,
			filing: db.W9StatusCompleted, wantEscrow: false},

		// A requested-but-unfinished filing is not a cleared one.
		{name: "a requested filing does not clear",
			prior: sfluv(590), amount: sfluv(20), filing: db.W9StatusRequested, wantEscrow: true},
		{name: "an invalid filing does not clear",
			prior: sfluv(590), amount: sfluv(20), filing: db.W9StatusInvalid, wantEscrow: true},

		{name: "a fresh year starts from zero",
			prior: big.NewInt(0), amount: sfluv(100), filing: db.W9StatusNotStarted, wantEscrow: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decideEscrow(tc.prior, tc.amount, threshold, tc.hasOpen, tc.filing)
			if got.Escrow != tc.wantEscrow {
				t.Fatalf("escrow = %v (%s); want %v", got.Escrow, got.Reason, tc.wantEscrow)
			}
			if got.Reason == "" {
				t.Error("every decision must carry a reason; it is logged and shown to admins")
			}
		})
	}
}

// Degenerate inputs must not panic or silently hold everybody's money.
func TestDecideEscrowHandlesMissingValues(t *testing.T) {
	if got := decideEscrow(nil, nil, threshold, false, db.W9StatusNotStarted); got.Escrow {
		t.Error("a nil prior total and amount should not trigger a hold")
	}
	// No threshold configured must fail open. Failing closed would hold every
	// payout on the platform the moment an env var went missing.
	if got := decideEscrow(sfluv(10000), sfluv(1), nil, false, db.W9StatusNotStarted); got.Escrow {
		t.Error("an unset threshold should pay through, not hold everything")
	}
	if got := decideEscrow(sfluv(10000), sfluv(1), big.NewInt(0), false, db.W9StatusNotStarted); got.Escrow {
		t.Error("a zero threshold should pay through, not hold everything")
	}
}

// The sequence a volunteer actually experiences: several shifts, a crossing,
// then filing.
func TestRunningTotalCrossesOnceAndStaysHeld(t *testing.T) {
	prior := big.NewInt(0)
	amounts := []*big.Int{sfluv(200), sfluv(200), sfluv(150), sfluv(100), sfluv(50)}
	wantEscrow := []bool{false, false, false, true, true}

	hasOpen := false
	for i, amount := range amounts {
		got := decideEscrow(prior, amount, threshold, hasOpen, db.W9StatusNotStarted)
		if got.Escrow != wantEscrow[i] {
			t.Fatalf("payment %d of %s: escrow = %v (%s); want %v",
				i+1, amount, got.Escrow, got.Reason, wantEscrow[i])
		}
		if got.Escrow {
			hasOpen = true
		}
		prior = new(big.Int).Add(prior, amount)
	}

	// Filing releases the hold and later payments pay through again.
	if got := decideEscrow(prior, sfluv(500), threshold, hasOpen, db.W9StatusCompleted); got.Escrow {
		t.Fatalf("after filing, payments should go straight out; got %s", got.Reason)
	}
}

func TestEscrowReminderSequence(t *testing.T) {
	window := escrowWindowForTest()

	if seq := escrowReminderSequence(window/2, window); seq != 0 {
		t.Errorf("no reminder is due halfway through the window; got %d", seq)
	}
	// The warning before the window closes is the important one: after it,
	// releasing stops being automatic.
	if seq := escrowReminderSequence(window-time24(), window); seq != 1 {
		t.Errorf("a warning is due a day before expiry; got %d", seq)
	}
	if seq := escrowReminderSequence(window, window); seq < 100 {
		t.Errorf("past the window the reminder should switch to the periodic series; got %d", seq)
	}
	// Periodic reminders must advance, or the dedup table silences them forever.
	first := escrowReminderSequence(window, window)
	later := escrowReminderSequence(window+15*24*time24()/24, window)
	if later <= first {
		t.Errorf("periodic reminders must keep advancing: %d then %d", first, later)
	}
}

func time24() time.Duration              { return 24 * time.Hour }
func escrowWindowForTest() time.Duration { return 7 * 24 * time.Hour }
