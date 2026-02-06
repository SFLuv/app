package structs

import "time"

type W9WalletEarning struct {
	WalletAddress string     `json:"wallet_address"`
	Year          int        `json:"year"`
	AmountReceived string    `json:"amount_received"`
	UserId        *string    `json:"user_id,omitempty"`
	W9Required    bool       `json:"w9_required"`
	W9RequiredAt  *time.Time `json:"w9_required_at,omitempty"`
	LastTxHash    *string    `json:"last_tx_hash,omitempty"`
	LastTxTimestamp *int     `json:"last_tx_timestamp,omitempty"`
}

type W9Submission struct {
	Id               int        `json:"id"`
	WalletAddress    string     `json:"wallet_address"`
	Year             int        `json:"year"`
	Email            string     `json:"email"`
	SubmittedAt      time.Time  `json:"submitted_at"`
	PendingApproval  bool       `json:"pending_approval"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`
	ApprovedByUserId *string    `json:"approved_by_user_id,omitempty"`
	W9URL            *string    `json:"-"`
}

type W9SubmitRequest struct {
	WalletAddress string `json:"wallet_address"`
	Email         string `json:"email"`
	Year          *int   `json:"year,omitempty"`
}

type W9ApprovalRequest struct {
	Id int `json:"id"`
}

type W9CheckRequest struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      string `json:"amount"`
}

type W9CheckResponse struct {
	Allowed      bool   `json:"allowed"`
	Reason       string `json:"reason,omitempty"`
	W9URL        string `json:"w9_url,omitempty"`
	Email        string `json:"email,omitempty"`
	CurrentTotal string `json:"current_total,omitempty"`
	NewTotal     string `json:"new_total,omitempty"`
	Limit        string `json:"limit,omitempty"`
	Year         int    `json:"year,omitempty"`
}

type W9PendingResponse struct {
	Submissions []*W9Submission `json:"submissions"`
}

type W9TransactionRequest struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      string `json:"amount"`
	Hash        string `json:"hash"`
	Timestamp   int64  `json:"timestamp"`
}
