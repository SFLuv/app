package structs

import (
	"math/big"
	"sort"
)

// MerchantTransfer is one indexed token movement, as the merchant-mode day view
// needs it. Amounts stay in base units end to end; the client divides once for
// display so no intermediate rounding can drift the till total.
type MerchantTransfer struct {
	Hash      string
	From      string
	To        string
	Amount    *big.Int
	Timestamp int64
}

// MerchantDayRow is one line of the merchant's day: a payment, the tip that came
// with it, or one without the other.
//
// Payment and Tip are separate on-chain transfers — the app sends the tip as a
// second transaction — so a row is a pairing, not a single event. Amounts are
// decimal strings in base units.
type MerchantDayRow struct {
	At          int64  `json:"at"`
	PaymentBase string `json:"payment_base"`
	TipBase     string `json:"tip_base"`
	From        string `json:"from"`
	PaymentHash string `json:"payment_hash,omitempty"`
	TipHash     string `json:"tip_hash,omitempty"`
	// Refund marks money leaving the payment wallet. Payment is negative on
	// these. Rare in practice: merchant mode has no send flow.
	Refund bool `json:"refund"`
}

// MerchantTodayResponse is the whole merchant-mode home screen in one payload.
// Totals are computed here rather than summed on the device: the same rule has
// to hold across clients, and a till figure should not depend on an app version.
type MerchantTodayResponse struct {
	BusinessDate  string           `json:"business_date"`
	TimeZone      string           `json:"timezone"`
	PaymentsBase  string           `json:"payments_base"`
	TipsBase      string           `json:"tips_base"`
	TokenDecimals int              `json:"token_decimals"`
	Transactions  []MerchantDayRow `json:"transactions"`
	// TipsWalletConfigured is false when the location has no tipping wallet, or
	// one that fails the ownership check. The client shows the tips figure as
	// unavailable rather than as a confident zero, which would read as "no tips
	// today" when the truth is "tips are not set up".
	TipsWalletConfigured bool `json:"tips_wallet_configured"`
}

// DefaultTipPairingWindowSeconds bounds how far after a payment a transfer into
// the tipping wallet may be and still count as that payment's tip. The two are
// sequential user operations from the same customer, so the real gap is seconds;
// the window is generous because a slow bundler should not orphan a tip.
const DefaultTipPairingWindowSeconds int64 = 120

// PairPaymentsAndTips builds the day's rows from the two transfer streams.
//
// The two sides share nothing on chain — no common hash, no memo — so pairing is
// a heuristic: same sender, tip close behind the payment. It is deliberately
// one-to-one and nearest-first, and anything left over is still shown. Two
// customers paying identical amounts within the same window are genuinely
// indistinguishable; the totals stay correct either way, only the attribution of
// a tip to a specific line can be wrong.
func PairPaymentsAndTips(payments, tips, refunds []MerchantTransfer, windowSeconds int64) []MerchantDayRow {
	if windowSeconds <= 0 {
		windowSeconds = DefaultTipPairingWindowSeconds
	}

	sorted := append([]MerchantTransfer(nil), payments...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })

	candidates := append([]MerchantTransfer(nil), tips...)
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Timestamp < candidates[j].Timestamp })

	// Every viable (payment, tip) combination, closest first. Matching each
	// payment in turn against its own best tip is not the same thing and gets it
	// wrong: an earlier payment sitting at the edge of the window will claim a
	// tip that belongs to a payment two seconds before it. Ranking all the pairs
	// globally means the tightest gap always wins, whichever payment it belongs to.
	type pairing struct {
		payment, tip int
		gap          int64
	}
	var options []pairing
	for pi, payment := range sorted {
		for ti, tip := range candidates {
			if !sameAddress(tip.From, payment.From) {
				continue
			}
			gap := tip.Timestamp - payment.Timestamp
			if gap < 0 {
				// A tip indexed a moment before its payment happens when both land
				// in the same block, so a small negative gap still counts.
				gap = -gap
			}
			if gap > windowSeconds {
				continue
			}
			options = append(options, pairing{payment: pi, tip: ti, gap: gap})
		}
	}
	sort.SliceStable(options, func(i, j int) bool { return options[i].gap < options[j].gap })

	tipUsed := make([]bool, len(candidates))
	tipForPayment := make([]int, len(sorted))
	for i := range tipForPayment {
		tipForPayment[i] = -1
	}
	for _, option := range options {
		if tipUsed[option.tip] || tipForPayment[option.payment] >= 0 {
			continue
		}
		tipUsed[option.tip] = true
		tipForPayment[option.payment] = option.tip
	}

	rows := make([]MerchantDayRow, 0, len(sorted)+len(candidates)+len(refunds))
	for pi, payment := range sorted {
		row := MerchantDayRow{
			At:          payment.Timestamp,
			PaymentBase: amountString(payment.Amount),
			TipBase:     "0",
			From:        payment.From,
			PaymentHash: payment.Hash,
		}
		if ti := tipForPayment[pi]; ti >= 0 {
			row.TipBase = amountString(candidates[ti].Amount)
			row.TipHash = candidates[ti].Hash
		}
		rows = append(rows, row)
	}

	// A tip with no payment behind it is still the merchant's money. Dropping it
	// would make the rows disagree with the day's tip total.
	for i, tip := range candidates {
		if tipUsed[i] {
			continue
		}
		rows = append(rows, MerchantDayRow{
			At:          tip.Timestamp,
			PaymentBase: "0",
			TipBase:     amountString(tip.Amount),
			From:        tip.From,
			TipHash:     tip.Hash,
		})
	}

	for _, refund := range refunds {
		rows = append(rows, MerchantDayRow{
			At:          refund.Timestamp,
			PaymentBase: negativeAmountString(refund.Amount),
			TipBase:     "0",
			From:        refund.To,
			PaymentHash: refund.Hash,
			Refund:      true,
		})
	}

	// Newest first: an employee checking the till cares about the last sale.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].At > rows[j].At })
	return rows
}

// SumTransfers totals a stream in base units.
func SumTransfers(transfers []MerchantTransfer) *big.Int {
	total := new(big.Int)
	for _, transfer := range transfers {
		if transfer.Amount != nil {
			total.Add(total, transfer.Amount)
		}
	}
	return total
}

func sameAddress(a, b string) bool {
	return len(a) == len(b) && equalFoldASCII(a, b)
}

// equalFoldASCII compares hex addresses without allocating. Addresses arrive
// lowercased from the index, but callers may pass a checksummed form.
func equalFoldASCII(a, b string) bool {
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func amountString(amount *big.Int) string {
	if amount == nil {
		return "0"
	}
	return amount.String()
}

func negativeAmountString(amount *big.Int) string {
	if amount == nil {
		return "0"
	}
	return new(big.Int).Neg(amount).String()
}
