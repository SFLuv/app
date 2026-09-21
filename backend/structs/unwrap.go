package structs

import "time"

// UnwrapEligibilityRequest asks whether a redemption may go ahead.
//
// AmountWei is the amount of the WHOLE redemption, not of one transaction: a
// till swept together with its tipping wallet is one redemption against the
// location's monthly allowance, so the client sends the combined figure and
// submits the transactions afterwards.
type UnwrapEligibilityRequest struct {
	WalletAddress string `json:"wallet_address"`
	AmountWei     string `json:"amount_wei"`
	// LocationID names the shop the allowance belongs to. Optional only for
	// older clients, which fall back to a per-wallet allowance.
	LocationID *uint64 `json:"location_id,omitempty"`
}

type UnwrapEligibilityResponse struct {
	Allowed                  bool       `json:"allowed"`
	Reason                   string     `json:"reason,omitempty"`
	LastUnwrapAt             *time.Time `json:"last_unwrap_at,omitempty"`
	MinimumFollowupAmountWei string     `json:"minimum_followup_amount_wei"`
}

// UnwrapRecordRequest is what the web client reports after withdrawTo is
// submitted. WalletAddress alone still works (it only stamps last_unwrap_at);
// the rest is what turns the call into a ledger row that can be followed to
// the bank.
type UnwrapRecordRequest struct {
	WalletAddress      string  `json:"wallet_address"`
	TxHash             string  `json:"tx_hash,omitempty"`
	AmountWei          string  `json:"amount_wei,omitempty"`
	DestinationAddress string  `json:"destination_address,omitempty"`
	LocationID         *uint64 `json:"location_id,omitempty"`
	// WalletRole is "payment" or "tipping"; it is how a tips unwrap is told
	// apart from a till unwrap in the ledger.
	WalletRole string `json:"wallet_role,omitempty"`
}

type UnwrapRecordResponse struct {
	Recorded   bool      `json:"recorded"`
	RecordedAt time.Time `json:"recorded_at"`
	UnwrapID   int64     `json:"unwrap_id,omitempty"`
}
