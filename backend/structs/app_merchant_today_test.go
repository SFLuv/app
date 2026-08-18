package structs

import (
	"math/big"
	"testing"
)

func transfer(hash, from string, amount int64, at int64) MerchantTransfer {
	return MerchantTransfer{Hash: hash, From: from, Amount: big.NewInt(amount), Timestamp: at}
}

const (
	customerA = "0xaaaa000000000000000000000000000000000001"
	customerB = "0xbbbb000000000000000000000000000000000002"
)

func TestPairsATipWithThePaymentItFollowed(t *testing.T) {
	rows := PairPaymentsAndTips(
		[]MerchantTransfer{transfer("p1", customerA, 25_000000, 1000)},
		[]MerchantTransfer{transfer("t1", customerA, 3_000000, 1004)},
		nil, DefaultTipPairingWindowSeconds,
	)

	if len(rows) != 1 {
		t.Fatalf("rows = %d; want the payment and its tip on one line", len(rows))
	}
	if rows[0].PaymentBase != "25000000" || rows[0].TipBase != "3000000" {
		t.Fatalf("row = %+v; want payment 25000000 and tip 3000000", rows[0])
	}
}

// The tip is a second transaction; if it lands in the same block the index can
// report it a moment before the payment. That must still pair.
func TestPairsATipIndexedJustBeforeItsPayment(t *testing.T) {
	rows := PairPaymentsAndTips(
		[]MerchantTransfer{transfer("p1", customerA, 25_000000, 1000)},
		[]MerchantTransfer{transfer("t1", customerA, 3_000000, 998)},
		nil, DefaultTipPairingWindowSeconds,
	)

	if len(rows) != 1 || rows[0].TipBase != "3000000" {
		t.Fatalf("rows = %+v; want the tip paired despite the earlier timestamp", rows)
	}
}

func TestDoesNotPairATipFromADifferentCustomer(t *testing.T) {
	rows := PairPaymentsAndTips(
		[]MerchantTransfer{transfer("p1", customerA, 25_000000, 1000)},
		[]MerchantTransfer{transfer("t1", customerB, 3_000000, 1004)},
		nil, DefaultTipPairingWindowSeconds,
	)

	if len(rows) != 2 {
		t.Fatalf("rows = %d; want the payment and the stray tip kept apart", len(rows))
	}
	for _, row := range rows {
		if row.PaymentBase != "0" && row.TipBase != "0" {
			t.Fatalf("row = %+v; want no row carrying both", row)
		}
	}
}

func TestDoesNotPairATipOutsideTheWindow(t *testing.T) {
	rows := PairPaymentsAndTips(
		[]MerchantTransfer{transfer("p1", customerA, 25_000000, 1000)},
		[]MerchantTransfer{transfer("t1", customerA, 3_000000, 1000+DefaultTipPairingWindowSeconds+1)},
		nil, DefaultTipPairingWindowSeconds,
	)

	if len(rows) != 2 {
		t.Fatalf("rows = %d; want the late tip on its own line", len(rows))
	}
}

// One tip cannot be credited to two payments, or the rows would total more than
// the day did.
func TestATipIsUsedOnce(t *testing.T) {
	rows := PairPaymentsAndTips(
		[]MerchantTransfer{
			transfer("p1", customerA, 25_000000, 1000),
			transfer("p2", customerA, 40_000000, 1010),
		},
		[]MerchantTransfer{transfer("t1", customerA, 3_000000, 1002)},
		nil, DefaultTipPairingWindowSeconds,
	)

	tipped := 0
	for _, row := range rows {
		if row.TipBase != "0" {
			tipped++
		}
	}
	if tipped != 1 {
		t.Fatalf("rows with a tip = %d; want exactly one", tipped)
	}
}

// The nearest payment should win, not merely the first one scanned.
func TestATipGoesToTheNearestPayment(t *testing.T) {
	rows := PairPaymentsAndTips(
		[]MerchantTransfer{
			transfer("early", customerA, 25_000000, 1000),
			transfer("late", customerA, 40_000000, 1100),
		},
		[]MerchantTransfer{transfer("t1", customerA, 3_000000, 1102)},
		nil, DefaultTipPairingWindowSeconds,
	)

	for _, row := range rows {
		if row.PaymentHash == "late" && row.TipBase != "3000000" {
			t.Fatalf("the tip should belong to the payment 2s before it, got %+v", row)
		}
		if row.PaymentHash == "early" && row.TipBase != "0" {
			t.Fatalf("the earlier payment should not take the tip, got %+v", row)
		}
	}
}

func TestUnmatchedTipStillAppears(t *testing.T) {
	rows := PairPaymentsAndTips(
		nil,
		[]MerchantTransfer{transfer("t1", customerA, 3_000000, 1000)},
		nil, DefaultTipPairingWindowSeconds,
	)

	if len(rows) != 1 || rows[0].TipBase != "3000000" || rows[0].PaymentBase != "0" {
		t.Fatalf("rows = %+v; want the orphan tip shown with a zero payment", rows)
	}
}

func TestRefundsAreNegative(t *testing.T) {
	rows := PairPaymentsAndTips(
		nil, nil,
		[]MerchantTransfer{{Hash: "r1", To: customerA, Amount: big.NewInt(5_000000), Timestamp: 1000}},
		DefaultTipPairingWindowSeconds,
	)

	if len(rows) != 1 || rows[0].PaymentBase != "-5000000" || !rows[0].Refund {
		t.Fatalf("rows = %+v; want a single negative refund row", rows)
	}
}

func TestRowsAreNewestFirst(t *testing.T) {
	rows := PairPaymentsAndTips(
		[]MerchantTransfer{
			transfer("old", customerA, 10_000000, 1000),
			transfer("new", customerB, 20_000000, 5000),
		},
		nil, nil, DefaultTipPairingWindowSeconds,
	)

	if len(rows) != 2 || rows[0].PaymentHash != "new" {
		t.Fatalf("rows = %+v; want the most recent sale first", rows)
	}
}

// The headline figures come from the raw streams, never from the paired rows, so
// a pairing mistake can misattribute a tip without ever changing the day total.
func TestTotalsAreIndependentOfPairing(t *testing.T) {
	payments := []MerchantTransfer{
		transfer("p1", customerA, 25_000000, 1000),
		transfer("p2", customerB, 40_000000, 2000),
	}
	tips := []MerchantTransfer{
		transfer("t1", customerA, 3_000000, 1002),
		transfer("t2", customerB, 99_000000, 90000), // far outside any window
	}

	if got := SumTransfers(payments).String(); got != "65000000" {
		t.Fatalf("payments total = %s; want 65000000", got)
	}
	if got := SumTransfers(tips).String(); got != "102000000" {
		t.Fatalf("tips total = %s; want 102000000", got)
	}

	rows := PairPaymentsAndTips(payments, tips, nil, DefaultTipPairingWindowSeconds)
	rowTips := new(big.Int)
	for _, row := range rows {
		value, ok := new(big.Int).SetString(row.TipBase, 10)
		if !ok {
			t.Fatalf("row tip %q is not a number", row.TipBase)
		}
		rowTips.Add(rowTips, value)
	}
	if rowTips.String() != "102000000" {
		t.Fatalf("tips across rows = %s; want every tip represented once", rowTips)
	}
}
