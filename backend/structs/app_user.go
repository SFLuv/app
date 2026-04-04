package structs

import "time"

type User struct {
	Id                   string  `json:"id"`
	Exists               bool    `json:"exists"`
	IsAdmin              bool    `json:"is_admin"`
	IsMerchant           bool    `json:"is_merchant"`
	IsOrganizer          bool    `json:"is_organizer"`
	IsImprover           bool    `json:"is_improver"`
	IsProposer           bool    `json:"is_proposer"`
	IsVoter              bool    `json:"is_voter"`
	IsIssuer             bool    `json:"is_issuer"`
	IsSupervisor         bool    `json:"is_supervisor"`
	IsAffiliate          bool    `json:"is_affiliate"`
	Email                *string `json:"contact_email"`
	Phone                *string `json:"contact_phone"`
	Name                 *string `json:"contact_name"`
	PrimaryWalletAddress string  `json:"primary_wallet_address"`
	PayPalEth            string  `json:"paypal_eth"`
	LastRedemption       int     `json:"last_redemption"`
}

type AuthedUserResponse struct {
	User       User        `json:"user"`
	Wallets    []*Wallet   `json:"wallets"`
	Locations  []*Location `json:"locations"`
	Contacts   []*Contact  `json:"contacts"`
	Affiliate  *Affiliate  `json:"affiliate,omitempty"`
	Proposer   *Proposer   `json:"proposer,omitempty"`
	Improver   *Improver   `json:"improver,omitempty"`
	Issuer     *Issuer     `json:"issuer,omitempty"`
	Supervisor *Supervisor `json:"supervisor,omitempty"`
}

type UserVerifiedEmailStatus string

const (
	UserVerifiedEmailStatusVerified UserVerifiedEmailStatus = "verified"
	UserVerifiedEmailStatusPending  UserVerifiedEmailStatus = "pending"
	UserVerifiedEmailStatusExpired  UserVerifiedEmailStatus = "expired"
)

type UserVerifiedEmail struct {
	Id                         string                  `json:"id"`
	UserId                     string                  `json:"user_id"`
	Email                      string                  `json:"email"`
	Status                     UserVerifiedEmailStatus `json:"status"`
	VerifiedAt                 *time.Time              `json:"verified_at,omitempty"`
	VerificationSentAt         *time.Time              `json:"verification_sent_at,omitempty"`
	VerificationTokenExpiresAt *time.Time              `json:"verification_token_expires_at,omitempty"`
	CreatedAt                  time.Time               `json:"created_at"`
	UpdatedAt                  time.Time               `json:"updated_at"`
}

type UserVerifiedEmailRequest struct {
	Email string `json:"email"`
}

type UserEmailVerificationTokenRequest struct {
	Token string `json:"token"`
}

type UserPrimaryWalletUpdateRequest struct {
	PrimaryWalletAddress string `json:"primary_wallet_address"`
}
