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
	ID                 int64      `json:"id"`
	UserID             string     `json:"user_id"`
	TaxYear            int        `json:"tax_year"`
	Status             string     `json:"status"`
	Provider           string     `json:"provider,omitempty"`
	ProviderRequestID  string     `json:"-"`
	FormURL            string     `json:"form_url,omitempty"`
	FormURLExpiresAt   *time.Time `json:"form_url_expires_at,omitempty"`
	ThresholdCrossedAt *time.Time `json:"threshold_crossed_at,omitempty"`
	RequestedAt        *time.Time `json:"requested_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	TINType            string     `json:"tin_type,omitempty"`
	// TINMatch is the vendor's asynchronous verification: pending, matched or
	// rejected. Independent of whether the form is signed.
	TINMatch            string     `json:"tin_match,omitempty"`
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
	// Raw base units for the progress bar, so a client never has to parse a
	// formatted amount back into a number to draw it.
	ThresholdBase string `json:"threshold_base"`
	EarnedBase    string `json:"earned_base"`

	// Tier is the warning the person has not yet dismissed, if any, and drives
	// which modal shows. Blocked means a payout was actually refused.
	Tier             string `json:"tier,omitempty"`
	TierAcknowledged bool   `json:"tier_acknowledged"`
	Blocked          bool   `json:"blocked"`

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

// RedeemRefusedResponse is returned when a scan is turned away before the code
// is touched. It mirrors the escrow body's status/reason/message keys, because
// mobile special-cases `reason` on /redeem, but carries no amount: nothing was
// ever reserved, and the code is still there to be scanned again.
type RedeemRefusedResponse struct {
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// Form1099Row is one payee's position for a tax year: what we actually paid
// them, and whether we can file for them.
//
// The figure that matters is what was PAID in the year, not what was earned in
// it. A 1099-NEC is cash-basis, so a reward escrowed in December and released in
// February belongs to the later year. That is why the ledger keeps tax_year and
// paid_tax_year apart, and why this reads the latter.
type Form1099Row struct {
	UserID       string `json:"user_id"`
	ContactName  string `json:"contact_name,omitempty"`
	ContactEmail string `json:"contact_email,omitempty"`
	TaxYear      int    `json:"tax_year"`

	// PaidSfluv is box 1, nonemployee compensation: the sum of everything that
	// actually reached them during the year.
	PaidSfluv   string `json:"paid_sfluv"`
	PayoutCount int    `json:"payout_count"`

	// Reportable is true once the year's payments reach the filing threshold.
	Reportable bool `json:"reportable"`

	// FilingStatus and TINType describe whether a form can be produced at all.
	// A legacy_approved filing clears someone to be paid but has no TIN behind
	// it, so it cannot support a 1099 — that person has to be asked for one.
	FilingStatus string `json:"filing_status"`
	TINType      string `json:"tin_type,omitempty"`
	Fileable     bool   `json:"fileable"`
	// BlockedReason says why a reportable payee cannot yet be filed for.
	BlockedReason string `json:"blocked_reason,omitempty"`
}

// Form1099Report is the year-end view: everyone at or over the threshold, plus
// the ones we cannot file for yet.
type Form1099Report struct {
	TaxYear         int           `json:"tax_year"`
	ThresholdSfluv  string        `json:"threshold_sfluv"`
	ReportableCount int           `json:"reportable_count"`
	FileableCount   int           `json:"fileable_count"`
	BlockedCount    int           `json:"blocked_count"`
	TotalPaidSfluv  string        `json:"total_paid_sfluv"`
	Rows            []Form1099Row `json:"rows"`
}
