package structs

import "time"

type UnwrapEligibilityRequest struct {
	WalletAddress string `json:"wallet_address"`
	AmountWei     string `json:"amount_wei"`
}

type UnwrapEligibilityResponse struct {
	Allowed                  bool       `json:"allowed"`
	Reason                   string     `json:"reason,omitempty"`
	LastUnwrapAt             *time.Time `json:"last_unwrap_at,omitempty"`
	MinimumFollowupAmountWei string     `json:"minimum_followup_amount_wei"`
}

type UnwrapRecordRequest struct {
	WalletAddress      string  `json:"wallet_address"`
	TxHash             string  `json:"tx_hash,omitempty"`
	AmountWei          string  `json:"amount_wei,omitempty"`
	DestinationAddress string  `json:"destination_address,omitempty"`
	LocationID         *uint64 `json:"location_id,omitempty"`
}

type UnwrapRecordResponse struct {
	Recorded   bool      `json:"recorded"`
	RecordedAt time.Time `json:"recorded_at"`
}

// UnwrapLedgerEntry is one row of the unwraps audit ledger: a withdrawTo
// cash-out of SFLUV to a location's Bridge liquidation address.
type UnwrapLedgerEntry struct {
	ID                 int       `json:"id"`
	UserID             string    `json:"user_id"`
	WalletID           int       `json:"wallet_id"`
	WalletAddress      string    `json:"wallet_address"`
	LocationID         *uint64   `json:"location_id,omitempty"`
	DestinationAddress string    `json:"destination_address"`
	AmountWei          string    `json:"amount_wei"`
	TxHash             string    `json:"tx_hash"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
}
