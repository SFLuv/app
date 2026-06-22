package structs

import "time"

// Recovery claim statuses for recovery_balances.claim_status.
const (
	RecoveryStatusUnclaimed = "unclaimed"
	RecoveryStatusClaiming  = "claiming"
	RecoveryStatusClaimed   = "claimed"
)

// RecoveryBalance is a non-auto-migrated (mostly Citizen Wallet) holder's last
// known Berachain balance, decimal-adjusted for the Celo migration, recorded so
// the holder can claim it after the migration.
type RecoveryBalance struct {
	Address     string     `json:"address"`
	ChainID     int64      `json:"chain_id"`
	Amount      string     `json:"amount"` // exact base units (NUMERIC), as a string
	ClaimStatus string     `json:"claim_status"`
	ClaimedBy   string     `json:"claimed_by,omitempty"`
	ClaimTxHash string     `json:"claim_tx_hash,omitempty"`
	ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
}

// RecoveryClaimRequest is the authenticated claim payload: the full Citizen
// Wallet sigAuth parameter set proving control of the previous account.
type RecoveryClaimRequest struct {
	SigAuthAccount   string `json:"sigAuthAccount"`
	SigAuthExpiry    string `json:"sigAuthExpiry"`
	SigAuthSignature string `json:"sigAuthSignature"`
	SigAuthRedirect  string `json:"sigAuthRedirect,omitempty"`
}

// RecoveryClaimResponse reports the outcome of a claim.
type RecoveryClaimResponse struct {
	Claimed   bool   `json:"claimed"`
	Amount    string `json:"amount,omitempty"`
	TxHash    string `json:"tx_hash,omitempty"`
	Recipient string `json:"recipient,omitempty"`
	Message   string `json:"message,omitempty"`
}

// RecoveryPreviewResponse reports a sigAuth-verified claimable balance without
// claiming it. Used by the logged-out recovery page to show the previous
// account and amount before the user logs in.
type RecoveryPreviewResponse struct {
	Account string `json:"account"`
	Amount  string `json:"amount"`
	Claimed bool   `json:"claimed"`
	Message string `json:"message,omitempty"`
}
