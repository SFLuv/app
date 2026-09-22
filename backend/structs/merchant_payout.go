package structs

import "time"

// MerchantPayoutProfile is a business's standing with Bridge, keyed by owner.
type MerchantPayoutProfile struct {
	OwnerID          string     `json:"owner_id"`
	BridgeCustomerID string     `json:"bridge_customer_id"`
	BridgeKYCLinkID  string     `json:"bridge_kyc_link_id"`
	KYBStatus        string     `json:"kyb_status"`
	TOSStatus        string     `json:"tos_status"`
	KYBLinkURL       string     `json:"kyb_link_url,omitempty"`
	KYBRequestedAt   *time.Time `json:"kyb_requested_at,omitempty"`
	KYBApprovedAt    *time.Time `json:"kyb_approved_at,omitempty"`
	LastSyncedAt     *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// MerchantBankAccount is the recognisable part of a Bridge external account.
type MerchantBankAccount struct {
	ID                      int64     `json:"id"`
	OwnerID                 string    `json:"owner_id"`
	BridgeExternalAccountID string    `json:"bridge_external_account_id"`
	BankName                string    `json:"bank_name"`
	Last4                   string    `json:"last_4"`
	AccountOwnerName        string    `json:"account_owner_name"`
	Currency                string    `json:"currency"`
	Active                  bool      `json:"active"`
	CreatedAt               time.Time `json:"created_at"`
}

// LocationLiquidationAddress is where one location's unwrapped USDC goes.
type LocationLiquidationAddress struct {
	LocationID                 uint64    `json:"location_id"`
	OwnerID                    string    `json:"owner_id"`
	BridgeLiquidationAddressID string    `json:"bridge_liquidation_address_id"`
	Address                    string    `json:"address"`
	Chain                      string    `json:"chain"`
	Currency                   string    `json:"currency"`
	DestinationPaymentRail     string    `json:"destination_payment_rail"`
	DestinationCurrency        string    `json:"destination_currency"`
	BridgeExternalAccountID    string    `json:"bridge_external_account_id"`
	Source                     string    `json:"source"`
	SetByUserID                string    `json:"set_by_user_id,omitempty"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

// Unwrap is one ledger row: a withdrawTo and its journey to the bank.
type Unwrap struct {
	ID                 int64      `json:"id"`
	OwnerID            string     `json:"owner_id"`
	LocationID         *int64     `json:"location_id,omitempty"`
	WalletID           *int64     `json:"wallet_id,omitempty"`
	WalletAddress      string     `json:"wallet_address"`
	WalletRole         string     `json:"wallet_role"`
	DestinationAddress string     `json:"destination_address"`
	AmountWei          string     `json:"amount_wei"`
	TxHash             string     `json:"tx_hash"`
	Status             string     `json:"status"`
	BridgeDrainID      string     `json:"bridge_drain_id,omitempty"`
	BridgeState        string     `json:"bridge_state,omitempty"`
	BankReference      string     `json:"bank_reference,omitempty"`
	LastSyncedAt       *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// --- API shapes -------------------------------------------------------------

// MerchantPayoutStatusResponse is everything the settings page needs to draw
// a location's bank & unwrap section.
type MerchantPayoutStatusResponse struct {
	Enabled      bool                         `json:"enabled"`
	Production   bool                         `json:"production"`
	Profile      *MerchantPayoutProfile       `json:"profile"`
	BankAccounts []MerchantBankAccount        `json:"bank_accounts"`
	Locations    []LocationLiquidationAddress `json:"locations"`
	Unwraps      []*Unwrap                    `json:"unwraps"`
}

type PlaidLinkTokenResponse struct {
	LinkToken string `json:"link_token"`
	ExpiresAt string `json:"link_token_expires_at"`
}

type PlaidExchangeRequest struct {
	LinkToken   string `json:"link_token"`
	PublicToken string `json:"public_token"`
	// LocationID is the shop whose card started the flow. Payouts attach per
	// location, and this is the location the merchant was looking at, so it is
	// the one the new bank connects. Other locations still have to be attached
	// deliberately. Optional: an older client omits it and gets a bank with
	// nothing attached, which is the pre-existing behaviour.
	LocationID *uint64 `json:"location_id,omitempty"`
}

// PlaidExchangeResponse says what actually happened, which the provisioning
// fields alone cannot: Bridge creates the bank record asynchronously, so the
// exchange can succeed while the account is still seconds away from existing.
// BankConnected is what the client should believe, not the 200.
type PlaidExchangeResponse struct {
	ProvisionLiquidationAddressesResponse
	BankConnected bool `json:"bank_connected"`
}

type ProvisionLiquidationAddressesResponse struct {
	Provisioned []LocationLiquidationAddress `json:"provisioned"`
	Skipped     []uint64                     `json:"skipped_location_ids"`
	Message     string                       `json:"message,omitempty"`
}

type SetLocationPayoutBankRequest struct {
	BridgeExternalAccountID string `json:"bridge_external_account_id"`
}

type KYBLinkResponse struct {
	KYBStatus string `json:"kyb_status"`
	URL       string `json:"url"`
}

type AdminSetLiquidationAddressRequest struct {
	Address string `json:"address"`
}

// AdminAttachBridgeCustomerRequest identifies the business either by the
// owner's contact email (the normal case: an admin knows the merchant's
// email, not their Privy id) or by owner id (the fallback, and how a
// duplicate-email pick is resubmitted).
type AdminAttachBridgeCustomerRequest struct {
	OwnerID          string `json:"owner_id"`
	OwnerEmail       string `json:"owner_email"`
	BridgeCustomerID string `json:"bridge_customer_id"`
}

// MerchantOwnerCandidate is one account matching an email lookup, with
// enough context (name, locations) for an admin to pick the right one when
// an email is shared by several accounts.
type MerchantOwnerCandidate struct {
	OwnerID       string   `json:"owner_id"`
	ContactName   string   `json:"contact_name"`
	ContactEmail  string   `json:"contact_email"`
	LocationNames []string `json:"location_names"`
}

// AdminPayoutLocation is one approved location as the admin panel sees it:
// where its unwraps go, or nil when nothing has been provisioned yet.
type AdminPayoutLocation struct {
	LocationID  uint64                      `json:"location_id"`
	Name        string                      `json:"name"`
	Liquidation *LocationLiquidationAddress `json:"liquidation_address"`
}

type AdminMerchantPayoutBusiness struct {
	Profile      *MerchantPayoutProfile `json:"profile"`
	BankAccounts []MerchantBankAccount  `json:"bank_accounts"`
	Locations    []AdminPayoutLocation  `json:"locations"`
}

type AdminMerchantPayoutsResponse struct {
	Businesses []AdminMerchantPayoutBusiness `json:"businesses"`
	Unwraps    []*Unwrap                     `json:"unwraps"`
}
