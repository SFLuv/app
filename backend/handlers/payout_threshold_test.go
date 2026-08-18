package handlers

import (
	"math/big"
	"testing"
	"time"

	"github.com/SFLuv/app/backend/db"
)

func sfluv(n int64) *big.Int { return big.NewInt(n * 1_000_000) }

var threshold = sfluv(600)

// The gate, exhaustively. Every case here is somebody's money, and the
// escalation is the point: two warnings while still being paid, then a hold,
// then a refusal.
func TestDecidePayout(t *testing.T) {
	tiers := payoutThresholds{Notice: sfluv(400), Warning: sfluv(500), Limit: sfluv(600)}

	for _, tc := range []struct {
		name       string
		prior      *big.Int
		amount     *big.Int
		hasOpen    bool
		filing     string
		wantAction string
		wantTier   string
	}{
		{name: "well below anything", prior: sfluv(100), amount: sfluv(50),
			filing: db.W9StatusNotStarted, wantAction: payoutActionPay, wantTier: ""},

		// The first line is a courtesy: still paid, just told.
		{name: "one unit short of the notice", prior: sfluv(350),
			amount: new(big.Int).Sub(sfluv(50), big.NewInt(1)),
			filing: db.W9StatusNotStarted, wantAction: payoutActionPay, wantTier: ""},
		{name: "lands exactly on the notice", prior: sfluv(350), amount: sfluv(50),
			filing: db.W9StatusNotStarted, wantAction: payoutActionPay, wantTier: db.W9TierNotice},
		{name: "between notice and warning", prior: sfluv(430), amount: sfluv(20),
			filing: db.W9StatusNotStarted, wantAction: payoutActionPay, wantTier: db.W9TierNotice},

		// The second is firmer, still paid.
		{name: "lands exactly on the warning", prior: sfluv(450), amount: sfluv(50),
			filing: db.W9StatusNotStarted, wantAction: payoutActionPay, wantTier: db.W9TierWarning},
		{name: "one unit short of the limit", prior: sfluv(550),
			amount: new(big.Int).Sub(sfluv(50), big.NewInt(1)),
			filing: db.W9StatusNotStarted, wantAction: payoutActionPay, wantTier: db.W9TierWarning},

		// The limit itself holds, in full — half a reward cannot be explained.
		{name: "lands exactly on the limit", prior: sfluv(550), amount: sfluv(50),
			filing: db.W9StatusNotStarted, wantAction: payoutActionEscrow, wantTier: db.W9TierEscrowed},
		{name: "the crossing payment is held whole", prior: sfluv(550), amount: sfluv(100),
			filing: db.W9StatusNotStarted, wantAction: payoutActionEscrow, wantTier: db.W9TierEscrowed},
		{name: "a single payment past the limit from zero", prior: big.NewInt(0), amount: sfluv(900),
			filing: db.W9StatusNotStarted, wantAction: payoutActionEscrow, wantTier: db.W9TierEscrowed},

		// And everything after is refused, not held again. This is what keeps
		// escrow to one payment and removes any owed-money bookkeeping.
		{name: "the payment after a hold is refused", prior: sfluv(620), amount: sfluv(30),
			hasOpen: true, filing: db.W9StatusNotStarted,
			wantAction: payoutActionBlock, wantTier: db.W9TierBlocked},
		{name: "refused even for a tiny amount", prior: sfluv(620), amount: big.NewInt(1),
			hasOpen: true, filing: db.W9StatusNotStarted,
			wantAction: payoutActionBlock, wantTier: db.W9TierBlocked},
		{name: "refused even while below the limit", prior: sfluv(10), amount: sfluv(1),
			hasOpen: true, filing: db.W9StatusNotStarted,
			wantAction: payoutActionBlock, wantTier: db.W9TierBlocked},

		// A cleared filing pays through at any amount, and outranks a hold.
		{name: "completed pays through", prior: sfluv(5000), amount: sfluv(1000),
			filing: db.W9StatusCompleted, wantAction: payoutActionPay, wantTier: ""},
		{name: "legacy approval pays through", prior: sfluv(5000), amount: sfluv(1000),
			filing: db.W9StatusLegacyApproved, wantAction: payoutActionPay, wantTier: ""},
		{name: "a cleared filing beats an open hold", prior: sfluv(5000), amount: sfluv(1),
			hasOpen: true, filing: db.W9StatusCompleted, wantAction: payoutActionPay, wantTier: ""},

		// Requested is not cleared. Neither is a rejected TIN match.
		{name: "requested does not clear", prior: sfluv(590), amount: sfluv(20),
			filing: db.W9StatusRequested, wantAction: payoutActionEscrow, wantTier: db.W9TierEscrowed},
		{name: "invalid does not clear", prior: sfluv(590), amount: sfluv(20),
			filing: db.W9StatusInvalid, wantAction: payoutActionEscrow, wantTier: db.W9TierEscrowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decidePayout(tc.prior, tc.amount, tiers, tc.hasOpen, tc.filing)
			if got.Action != tc.wantAction {
				t.Fatalf("action = %q (%s); want %q", got.Action, got.Reason, tc.wantAction)
			}
			if got.Tier != tc.wantTier {
				t.Fatalf("tier = %q; want %q", got.Tier, tc.wantTier)
			}
			if got.Reason == "" {
				t.Error("every decision carries a reason; it is logged and shown to admins")
			}
		})
	}
}

// Escrow must never grow beyond a single payment. That property is what removes
// expiry, back pay and the admin queue that went with them.
func TestEscrowCannotAccumulate(t *testing.T) {
	tiers := payoutThresholds{Notice: sfluv(400), Warning: sfluv(500), Limit: sfluv(600)}

	first := decidePayout(sfluv(590), sfluv(50), tiers, false, db.W9StatusNotStarted)
	if first.Action != payoutActionEscrow {
		t.Fatalf("the crossing payment should be held; got %q", first.Action)
	}
	// From here on, hasOpenEscrow is true for every subsequent payout.
	for i, amount := range []*big.Int{sfluv(10), sfluv(500), big.NewInt(1)} {
		got := decidePayout(sfluv(640), amount, tiers, true, db.W9StatusNotStarted)
		if got.Action != payoutActionBlock {
			t.Fatalf("payout %d after a hold: %q; a second hold would let escrow accumulate", i+2, got.Action)
		}
	}
}

// Degenerate inputs must not panic or silently hold everybody's money.
func TestDecidePayoutHandlesMissingValues(t *testing.T) {
	tiers := payoutThresholds{Notice: sfluv(400), Warning: sfluv(500), Limit: sfluv(600)}

	if got := decidePayout(nil, nil, tiers, false, db.W9StatusNotStarted); got.Action != payoutActionPay {
		t.Error("a nil prior total and amount should pay through")
	}
	// An unset limit must fail open. Failing closed would stop every payout on
	// the platform the moment an env var went missing.
	for _, name := range []string{"unset", "zero"} {
		broken := payoutThresholds{Notice: sfluv(400), Warning: sfluv(500)}
		if name == "zero" {
			broken.Limit = big.NewInt(0)
		}
		if got := decidePayout(sfluv(10000), sfluv(1), broken, false, db.W9StatusNotStarted); got.Action != payoutActionPay {
			t.Errorf("a %s limit should pay through, not withhold everything", name)
		}
	}
	// Disabling a warning tier must not disable the limit.
	noWarnings := payoutThresholds{Limit: sfluv(600)}
	if got := decidePayout(sfluv(590), sfluv(50), noWarnings, false, db.W9StatusNotStarted); got.Action != payoutActionEscrow {
		t.Error("the limit must still hold when the warning tiers are turned off")
	}
	if got := decidePayout(sfluv(450), sfluv(10), noWarnings, false, db.W9StatusNotStarted); got.Tier != "" {
		t.Error("a disabled warning tier must not fire")
	}
}

// The sequence a volunteer actually lives through: a run of shifts, two
// warnings while still being paid, a hold, a refusal, then filing.
func TestTheWholeEscalation(t *testing.T) {
	tiers := payoutThresholds{Notice: sfluv(400), Warning: sfluv(500), Limit: sfluv(600)}

	steps := []struct {
		amount     *big.Int
		wantAction string
		wantTier   string
	}{
		{sfluv(200), payoutActionPay, ""},                   // 200
		{sfluv(200), payoutActionPay, db.W9TierNotice},      // 400 — polite
		{sfluv(120), payoutActionPay, db.W9TierWarning},     // 520 — firmer
		{sfluv(100), payoutActionEscrow, db.W9TierEscrowed}, // 620 — held
		{sfluv(50), payoutActionBlock, db.W9TierBlocked},    // refused
		{sfluv(50), payoutActionBlock, db.W9TierBlocked},    // still refused
	}

	prior := big.NewInt(0)
	hasOpen := false
	for i, step := range steps {
		got := decidePayout(prior, step.amount, tiers, hasOpen, db.W9StatusNotStarted)
		if got.Action != step.wantAction || got.Tier != step.wantTier {
			t.Fatalf("step %d (%s on top of %s): %s/%s; want %s/%s",
				i+1, step.amount, prior, got.Action, got.Tier, step.wantAction, step.wantTier)
		}
		switch got.Action {
		case payoutActionEscrow:
			hasOpen = true
			// Held money still counts: it is theirs and it is reportable.
			prior = new(big.Int).Add(prior, step.amount)
		case payoutActionPay:
			prior = new(big.Int).Add(prior, step.amount)
		case payoutActionBlock:
			// Refused money never moved, so it must not push the total up.
		}
	}

	// Filing clears everything, and payments resume.
	if got := decidePayout(prior, sfluv(500), tiers, hasOpen, db.W9StatusCompleted); got.Action != payoutActionPay {
		t.Fatalf("after filing, payments should go straight out; got %q (%s)", got.Action, got.Reason)
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
