package structs

import "time"

// PayoutLedgerRow is one platform-originated payout, whatever state it is in.
//
// Amounts are token base units held as a string: they pass through Postgres
// NUMERIC(78,0) and Go big.Int, and a float anywhere in that path would quietly
// round somebody's money.
type PayoutLedgerRow struct {
	ID                    int64      `json:"id"`
	IdempotencyKey        string     `json:"idempotency_key"`
	UserID                string     `json:"user_id"`
	RecipientAddress      string     `json:"recipient_address"`
	ChainID               int64      `json:"chain_id"`
	TaxYear               int        `json:"tax_year"`
	PaidTaxYear           int        `json:"paid_tax_year,omitempty"`
	Source                string     `json:"source"`
	SourceRef             string     `json:"source_ref"`
	AmountBase            string     `json:"amount_base"`
	State                 string     `json:"state"`
	EscrowedAt            *time.Time `json:"escrowed_at,omitempty"`
	ExpiredAt             *time.Time `json:"expired_at,omitempty"`
	ReleasedAt            *time.Time `json:"released_at,omitempty"`
	PaidAt                *time.Time `json:"paid_at,omitempty"`
	TxHash                string     `json:"tx_hash,omitempty"`
	Attempts              int        `json:"attempts"`
	LastError             string     `json:"last_error,omitempty"`
	CountsTowardThreshold bool       `json:"counts_toward_threshold"`
	CreatedAt             time.Time  `json:"created_at"`
}

// W9Filing is one person's tax form state for one year.
type W9Filing struct {
	ID                  int64      `json:"id"`
	UserID              string     `json:"user_id"`
	TaxYear             int        `json:"tax_year"`
	Status              string     `json:"status"`
	Provider            string     `json:"provider,omitempty"`
	ProviderRequestID   string     `json:"-"`
	FormURL             string     `json:"form_url,omitempty"`
	FormURLExpiresAt    *time.Time `json:"form_url_expires_at,omitempty"`
	ThresholdCrossedAt  *time.Time `json:"threshold_crossed_at,omitempty"`
	RequestedAt         *time.Time `json:"requested_at,omitempty"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	TINType             string     `json:"tin_type,omitempty"`
	ClearedByUserID     string     `json:"cleared_by_user_id,omitempty"`
	ClearReason         string     `json:"clear_reason,omitempty"`
	LastProviderStatus  string     `json:"last_provider_status,omitempty"`
	LastProviderEventAt *time.Time `json:"last_provider_event_at,omitempty"`
}

// W9StatusResponse is what a person sees: whether they owe a form, how much is
// waiting on it, and how long the automatic window has left.
type W9StatusResponse struct {
	TaxYear        int    `json:"tax_year"`
	Required       bool   `json:"required"`
	FilingStatus   string `json:"filing_status"`
	Cleared        bool   `json:"cleared"`
	ThresholdSfluv string `json:"threshold_sfluv"`
	EarnedSfluv    string `json:"earned_sfluv"`

	EscrowedSfluv string `json:"escrowed_sfluv"`
	EscrowedCount int    `json:"escrowed_count"`
	// EscrowExpiresAt is when the oldest held payout leaves the automatic
	// window. Surfaced so the deadline can be shown before it passes rather
	// than explained afterwards.
	EscrowExpiresAt *time.Time `json:"escrow_expires_at,omitempty"`

	// BackPay is money that was held too long. It is still owed, but an admin
	// has to send it.
	BackPaySfluv string `json:"back_pay_sfluv"`
	BackPayCount int    `json:"back_pay_count"`

	FormURL          string      `json:"form_url,omitempty"`
	FormURLExpiresAt *time.Time  `json:"form_url_expires_at,omitempty"`
	Items            []W9DayItem `json:"items"`
}

// W9DayItem is one held payout, for the list in the escrow panel.
type W9DayItem struct {
	Source      string     `json:"source"`
	SourceLabel string     `json:"source_label"`
	AmountSfluv string     `json:"amount_sfluv"`
	State       string     `json:"state"`
	EscrowedAt  *time.Time `json:"escrowed_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// W9AdminRow is one person's tax position in the admin panel.
type W9AdminRow struct {
	UserID          string     `json:"user_id"`
	ContactName     string     `json:"contact_name,omitempty"`
	ContactEmail    string     `json:"contact_email,omitempty"`
	TaxYear         int        `json:"tax_year"`
	FilingStatus    string     `json:"filing_status"`
	EarnedSfluv     string     `json:"earned_sfluv"`
	EscrowedSfluv   string     `json:"escrowed_sfluv"`
	BackPaySfluv    string     `json:"back_pay_sfluv"`
	BackPayCount    int        `json:"back_pay_count"`
	OldestEscrowAt  *time.Time `json:"oldest_escrow_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	NeedsBackPayNow bool       `json:"needs_back_pay_now"`
}

// W9AdminOverview is the faucet coverage panel: what is reserved, what is owed,
// and whether the faucet can currently cover it.
type W9AdminOverview struct {
	FaucetSfluv    string `json:"faucet_sfluv"`
	AllocatedSfluv string `json:"allocated_sfluv"`
	// EscrowedSfluv is reserved and must not be spent on anything else.
	EscrowedSfluv string `json:"escrowed_sfluv"`
	// AvailableSfluv is what may actually be allocated: faucet − allocated − escrowed.
	AvailableSfluv string `json:"available_sfluv"`
	// BackPaySfluv is owed but deliberately not reserved.
	BackPaySfluv    string       `json:"back_pay_sfluv"`
	BackPayCovered  bool         `json:"back_pay_covered"`
	BackPayShortBy  string       `json:"back_pay_short_by"`
	PeopleWithHolds int          `json:"people_with_holds"`
	OldestEscrowAge int          `json:"oldest_escrow_age_days"`
	Rows            []W9AdminRow `json:"rows"`
}

// RedeemEscrowedResponse is returned when a scanned reward is held rather than
// sent. A 202 with this body, not a 200 — the reward exists but has not moved,
// and a client that renders it as a completed redemption would be lying to
// whoever is standing there.
type RedeemEscrowedResponse struct {
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	AmountSfluv string `json:"amount_sfluv"`
	TaxYear     int    `json:"tax_year"`
	Message     string `json:"message"`
}
