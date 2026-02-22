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
	WalletAddress string `json:"wallet_address"`
	TxHash        string `json:"tx_hash,omitempty"`
}

type UnwrapRecordResponse struct {
	Recorded   bool      `json:"recorded"`
	RecordedAt time.Time `json:"recorded_at"`
}
