package structs

import "time"

// Refund is one recorded refund: a transfer out of a till that undoes part or
// all of a payment into it.
type Refund struct {
	ID             int64  `json:"id"`
	OriginalTxHash string `json:"original_tx_hash"`
	RefundTxHash   string `json:"refund_tx_hash"`
	LocationID     *int64 `json:"location_id,omitempty"`
	OwnerID        string `json:"owner_id"`
	FromAddress    string `json:"from_address"`
	ToAddress      string `json:"to_address"`
	// Amount is in token base units, as a decimal string.
	Amount    string    `json:"amount"`
	ChainID   *int64    `json:"chain_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// RecordRefundRequest is what the client reports after the till's refund
// transfer is on chain. Mirrors the unwrap flow: the wallet signs, the backend
// records, and the backend is what decides whether the amount was allowed.
type RecordRefundRequest struct {
	OriginalTxHash string `json:"original_tx_hash"`
	RefundTxHash   string `json:"refund_tx_hash"`
	WalletAddress  string `json:"wallet_address"`
	// Amount in base units. Checked against what remains unrefunded on the
	// original, so a client cannot refund more than was taken.
	Amount     string  `json:"amount"`
	LocationID *uint64 `json:"location_id,omitempty"`
}

type RecordRefundResponse struct {
	Recorded bool   `json:"recorded"`
	RefundID int64  `json:"refund_id,omitempty"`
	Refunded  string `json:"refunded_total"`
	Remaining string `json:"remaining_refundable"`
}

// RefundabilityRequest asks what may still be refunded on a payment, before the
// merchant is shown a refund form.
type RefundabilityRequest struct {
	OriginalTxHash string  `json:"original_tx_hash"`
	LocationID     *uint64 `json:"location_id,omitempty"`
}

// RefundState is how one transaction looks once the refund ledger is applied.
//
// The three marks the merchant sees — refunded, partially refunded, refund —
// are derived here rather than in each client, so the web panel and the till
// app cannot disagree about whether something is fully refunded.
type RefundState struct {
	// Status is "none", "partially_refunded", "refunded", or "refund".
	Status string `json:"status"`
	// RefundedBase is what has been refunded against this payment so far.
	RefundedBase string `json:"refunded_base,omitempty"`
	// RemainingBase is what may still be refunded. Zero means the refund
	// button should not be offered at all.
	RemainingBase string `json:"remaining_base,omitempty"`
	// Refunds are the refunds taken off this payment, newest last.
	Refunds []Refund `json:"refunds,omitempty"`
	// RefundsOriginalHash is set on a refund line and points at the payment it
	// undid, so the two cross-reference in the history view.
	RefundsOriginalHash string `json:"refunds_original_hash,omitempty"`
}

// Refund status vocabulary, shared by every surface that renders it.
const (
	RefundStatusNone     = "none"
	RefundStatusPartial  = "partially_refunded"
	RefundStatusFull     = "refunded"
	RefundStatusIsRefund = "refund"
)

// LocationTransaction is one line of a location's history.
type LocationTransaction struct {
	Hash       string `json:"hash"`
	From       string `json:"from"`
	To         string `json:"to"`
	AmountBase string `json:"amount_base"`
	Timestamp  int64  `json:"timestamp"`
	// Direction is "in" (a payment to the till) or "out" (money leaving it).
	Direction string `json:"direction"`
	// Wallet is "payment" or "tipping" — which of the location's wallets this
	// moved through.
	Wallet string      `json:"wallet"`
	Refund RefundState `json:"refund"`
}

type LocationTransactionsResponse struct {
	LocationID    uint64                `json:"location_id"`
	TokenDecimals int                   `json:"token_decimals"`
	Transactions  []LocationTransaction `json:"transactions"`
	Total         uint64                `json:"total"`
	Page          int                   `json:"page"`
}
