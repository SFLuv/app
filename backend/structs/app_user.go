package structs

import "time"

type User struct {
	Id                       string     `json:"id"`
	Exists                   bool       `json:"exists"`
	IsAdmin                  bool       `json:"is_admin"`
	IsMerchant               bool       `json:"is_merchant"`
	IsOrganizer              bool       `json:"is_organizer"`
	IsImprover               bool       `json:"is_improver"`
	IsProposer               bool       `json:"is_proposer"`
	IsVoter                  bool       `json:"is_voter"`
	IsIssuer                 bool       `json:"is_issuer"`
	IsSupervisor             bool       `json:"is_supervisor"`
	IsAffiliate              bool       `json:"is_affiliate"`
	Email                    *string    `json:"contact_email"`
	Phone                    *string    `json:"contact_phone"`
	Name                     *string    `json:"contact_name"`
	PrimaryWalletAddress     string     `json:"primary_wallet_address"`
	PayPalEth                string     `json:"paypal_eth"`
	LastRedemption           int        `json:"last_redemption"`
	AcceptedPrivacyPolicy    bool       `json:"accepted_privacy_policy"`
	AcceptedPrivacyPolicyAt  *time.Time `json:"accepted_privacy_policy_at,omitempty"`
	PrivacyPolicyVersion     string     `json:"privacy_policy_version"`
	MailingListOptIn         bool       `json:"mailing_list_opt_in"`
	MailingListOptInAt       *time.Time `json:"mailing_list_opt_in_at,omitempty"`
	MailingListPolicyVersion string     `json:"mailing_list_policy_version"`
	// AccountType is what the person chose at signup. IsMerchant is not a
	// substitute: it is recomputed from approved listings, so it says a shop of
	// theirs is live, not which app they thought they were signing up for.
	AccountType                   string                 `json:"account_type"`
	MerchantOnboardingCompletedAt *time.Time             `json:"merchant_onboarding_completed_at,omitempty"`
	ClientDevices                 []*ClientVersionDevice `json:"client_devices,omitempty"`
}

type ClientVersionDevice struct {
	Id             int64     `json:"id"`
	UserId         string    `json:"user_id,omitempty"`
	Platform       string    `json:"platform"`
	Version        string    `json:"version"`
	Build          string    `json:"build"`
	VersionLabel   string    `json:"version_label"`
	Source         string    `json:"source"`
	LegacyInferred bool      `json:"legacy_inferred"`
	FirstSeenAt    time.Time `json:"first_seen_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
}

type ClientVersionUserCount struct {
	VersionLabel   string `json:"version_label"`
	Version        string `json:"version"`
	Build          string `json:"build"`
	UserCount      int    `json:"user_count"`
	LegacyInferred bool   `json:"legacy_inferred"`
	Unknown        bool   `json:"unknown"`
}

type ClientVersionObservation struct {
	UserId         string
	ClientKey      string
	Platform       string
	Version        string
	Build          string
	BuildNumber    int
	UserAgent      string
	Source         string
	LegacyInferred bool
	SeenAt         time.Time
}

// ClientPhoneHome is an anonymous, aggregate record of an app "phone home"
// (an unauthenticated /config or /client-version fetch). It carries no user
// identity and is rolled up per UTC day + platform/version/build so it can be
// used as a real-app-usage metric independent of authenticated traffic.
type ClientPhoneHome struct {
	Endpoint string
	Platform string
	Version  string
	Build    string
}

const (
	CurrentPrivacyPolicyVersion     = "2026-04-15"
	CurrentMailingListPolicyVersion = "2026-04-15"
	AuthReasonPrivacyPolicyRequired = "privacy-policy-required"
)

// The account types a signup may choose between.
const (
	AccountTypeRegular  = "regular"
	AccountTypeMerchant = "merchant"
)

// IsValidAccountType reports whether a client-supplied account type is one we
// store. The empty string is not valid here and is handled by the caller: an
// older client omits the field entirely, which means "leave it alone", not
// "write an empty account type".
func IsValidAccountType(accountType string) bool {
	return accountType == AccountTypeRegular || accountType == AccountTypeMerchant
}

type AccountDeletionStatus string

const (
	AccountDeletionStatusActive              AccountDeletionStatus = "active"
	AccountDeletionStatusScheduled           AccountDeletionStatus = "scheduled_for_deletion"
	AccountDeletionStatusReadyForManualPurge AccountDeletionStatus = "ready_for_manual_purge"
)

type AccountDeletionPreview struct {
	UserId               string                `json:"user_id"`
	Status               AccountDeletionStatus `json:"status"`
	DeleteDate           *time.Time            `json:"delete_date,omitempty"`
	RequestedAt          *time.Time            `json:"requested_at,omitempty"`
	CanCancel            bool                  `json:"can_cancel"`
	PrimaryWalletAddress string                `json:"primary_wallet_address"`
	WalletAddresses      []string              `json:"wallet_addresses"`
	Counts               AccountDeletionCounts `json:"counts"`
	PurgeEnabled         bool                  `json:"purge_enabled"`
}

type AccountDeletionCounts struct {
	Wallets             int `json:"wallets"`
	Contacts            int `json:"contacts"`
	Locations           int `json:"locations"`
	LocationHours       int `json:"location_hours"`
	LocationWallets     int `json:"location_wallets"`
	PonderSubscriptions int `json:"ponder_subscriptions"`
	VerifiedEmails      int `json:"verified_emails"`
	Memos               int `json:"memos"`
}

type AccountDeletionStatusResponse struct {
	UserId         string                `json:"user_id"`
	Status         AccountDeletionStatus `json:"status"`
	DeleteDate     *time.Time            `json:"delete_date,omitempty"`
	RequestedAt    *time.Time            `json:"requested_at,omitempty"`
	CanceledAt     *time.Time            `json:"canceled_at,omitempty"`
	CompletedAt    *time.Time            `json:"completed_at,omitempty"`
	CanCancel      bool                  `json:"can_cancel"`
	PurgeEnabled   bool                  `json:"purge_enabled"`
	PurgeEnabledBy string                `json:"purge_enabled_by,omitempty"`
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

type UserPolicyStatusResponse struct {
	UserId                   string     `json:"user_id"`
	Active                   bool       `json:"active"`
	AcceptedPrivacyPolicy    bool       `json:"accepted_privacy_policy"`
	AcceptedPrivacyPolicyAt  *time.Time `json:"accepted_privacy_policy_at,omitempty"`
	PrivacyPolicyVersion     string     `json:"privacy_policy_version"`
	MailingListOptIn         bool       `json:"mailing_list_opt_in"`
	MailingListOptInAt       *time.Time `json:"mailing_list_opt_in_at,omitempty"`
	MailingListPolicyVersion string     `json:"mailing_list_policy_version"`
	// Carried here rather than only on the profile because policy status is the
	// first thing a client gets back at startup, and the profile is withheld
	// until the policy is accepted. A client that has to know whether to send
	// somebody into merchant onboarding would otherwise have nothing to go on.
	AccountType                   string     `json:"account_type"`
	MerchantOnboardingCompletedAt *time.Time `json:"merchant_onboarding_completed_at,omitempty"`
}

// AdminUpdateUserAccountTypeRequest is the support-side repair for an account
// type that cannot be corrected any other way: the signup answer is written
// once, so a wrong choice — or a client old enough that it never asked the
// question — otherwise stands forever.
type AdminUpdateUserAccountTypeRequest struct {
	UserId string `json:"user_id"`
	// Required here, unlike on the signup path, where an empty value means
	// "leave it alone". Stating the type is the only reason to call this.
	AccountType string `json:"account_type"`
}

// AdminUserAccountTypeResponse carries the previous type back so the caller can
// see what the repair actually changed, and the onboarding stamp so an admin
// can tell whether the person will now be sent through merchant onboarding.
type AdminUserAccountTypeResponse struct {
	UserId                        string     `json:"user_id"`
	PreviousAccountType           string     `json:"previous_account_type"`
	AccountType                   string     `json:"account_type"`
	MerchantOnboardingCompletedAt *time.Time `json:"merchant_onboarding_completed_at,omitempty"`
}

type UserPolicyAcceptanceRequest struct {
	AcceptedPrivacyPolicy bool `json:"accepted_privacy_policy"`
	MailingListOptIn      bool `json:"mailing_list_opt_in"`
	// AccountType is the signup choice, and is only honoured on a first
	// acceptance. Omitted by clients that predate the question, and by every
	// re-acceptance a policy version bump forces.
	AccountType string `json:"account_type"`
}
