package structs

import "time"

type UserOAuthCredential struct {
	UserID                string     `json:"user_id"`
	Provider              string     `json:"provider"`
	ProviderSubject       string     `json:"provider_subject,omitempty"`
	ProviderEmail         string     `json:"provider_email,omitempty"`
	IsPrivateRelay        bool       `json:"is_private_relay"`
	AccessTokenEncrypted  string     `json:"-"`
	RefreshTokenEncrypted string     `json:"-"`
	AccessTokenExpiresAt  *time.Time `json:"access_token_expires_at,omitempty"`
	RefreshTokenExpiresAt *time.Time `json:"refresh_token_expires_at,omitempty"`
	Scopes                []string   `json:"scopes,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	RevokedAt             *time.Time `json:"revoked_at,omitempty"`
}

type AppleOAuthCredentialUpsertRequest struct {
	AccessToken                  string   `json:"access_token"`
	RefreshToken                 string   `json:"refresh_token"`
	AccessTokenExpiresInSeconds  int      `json:"access_token_expires_in_seconds"`
	RefreshTokenExpiresInSeconds int      `json:"refresh_token_expires_in_seconds"`
	Scopes                       []string `json:"scopes"`
	ProviderSubject              string   `json:"provider_subject"`
	ProviderEmail                string   `json:"provider_email"`
	IsPrivateRelay               bool     `json:"is_private_relay"`
}

type AppleRecoveryResolution string

const (
	AppleRecoveryResolutionCurrentAccountExists AppleRecoveryResolution = "current_account_exists"
	AppleRecoveryResolutionRecoverySuggested    AppleRecoveryResolution = "recovery_suggested"
	AppleRecoveryResolutionNoMatch              AppleRecoveryResolution = "no_match"
	AppleRecoveryResolutionAmbiguousMatch       AppleRecoveryResolution = "ambiguous_match"
	AppleRecoveryResolutionNoAppleAccount       AppleRecoveryResolution = "no_apple_account"
)

type AppleRecoverySuggestedAccount struct {
	UserID               string `json:"user_id"`
	ContactName          string `json:"contact_name,omitempty"`
	VerifiedEmail        string `json:"verified_email,omitempty"`
	PrimaryWalletAddress string `json:"primary_wallet_address,omitempty"`
}

type AppleRecoveryResponse struct {
	CurrentUserID            string                         `json:"current_user_id"`
	CurrentUserExists        bool                           `json:"current_user_exists"`
	AppleLinked              bool                           `json:"apple_linked"`
	AppleEmail               string                         `json:"apple_email,omitempty"`
	IsPrivateRelay           bool                           `json:"is_private_relay"`
	Resolution               AppleRecoveryResolution        `json:"resolution"`
	SuggestedExistingAccount *AppleRecoverySuggestedAccount `json:"suggested_existing_account,omitempty"`
}

type AppleRecoveryRequest struct {
	ProviderSubject string `json:"provider_subject"`
	ProviderEmail   string `json:"provider_email"`
	IsPrivateRelay  bool   `json:"is_private_relay"`
}
