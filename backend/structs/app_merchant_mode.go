package structs

import "time"

type MerchantModeDevice struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	LocationID   uint   `json:"location_id"`
	LocationName string `json:"location_name"`
	// WalletAddress is the location's payment wallet as it stands right now, not
	// as it stood when this device was enrolled. A merchant who swaps the wallet
	// behind a shop expects the till to follow on its next poll, without anyone
	// walking over to re-enrol the tablet.
	WalletAddress string `json:"wallet_address"`
	// LocationActive and LocationApproved describe the shop this device is bound
	// to. A device at a shop that has been deactivated or had its approval pulled
	// must stop behaving like a till, so the app needs to see why.
	LocationActive      bool       `json:"location_active"`
	LocationApproved    bool       `json:"location_approved"`
	DisplayName         string     `json:"display_name"`
	Platform            string     `json:"platform"`
	AppVersion          string     `json:"app_version"`
	MerchantModeEnabled bool       `json:"merchant_mode_enabled"`
	EnabledAt           *time.Time `json:"enabled_at,omitempty"`
	DisabledAt          *time.Time `json:"disabled_at,omitempty"`
	LastSeenAt          time.Time  `json:"last_seen_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type MerchantModeStatusResponse struct {
	UserID      string              `json:"user_id"`
	IsMerchant  bool                `json:"is_merchant"`
	PasscodeSet bool                `json:"passcode_set"`
	Device      *MerchantModeDevice `json:"device,omitempty"`
	// ForcedExitReason is set when the server has just turned merchant mode off
	// for this device on its own. The app shows it once and returns to the normal
	// wallet, rather than silently dropping out of till mode mid-shift.
	ForcedExitReason string `json:"forced_exit_reason,omitempty"`
}

// MerchantModeLocation is one shop a device can be put to work at.
type MerchantModeLocation struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	// Street disambiguates two branches sharing a name, which is the whole
	// reason a picker exists.
	Street string `json:"street"`
	City   string `json:"city"`
	// WalletAddress is where payments to this shop land today.
	WalletAddress string `json:"wallet_address"`
	// TippingWalletAddress is empty when the shop takes no tips.
	TippingWalletAddress string `json:"tipping_wallet_address"`
}

type MerchantModeLocationsResponse struct {
	Locations []MerchantModeLocation `json:"locations"`
}

type MerchantModeForgotPINRequest struct {
	ContactEmail string `json:"contact_email,omitempty"`
}

type MerchantModeDevicesResponse struct {
	Devices []*MerchantModeDevice `json:"devices"`
}

type MerchantModeSetPINRequest struct {
	PIN        string `json:"pin"`
	CurrentPIN string `json:"current_pin,omitempty"`
}

type MerchantModeEnableRequest struct {
	InstallationID string `json:"installation_id"`
	LocationID     uint64 `json:"location_id"`
	WalletAddress  string `json:"wallet_address"`
	DisplayName    string `json:"display_name"`
	Platform       string `json:"platform"`
	AppVersion     string `json:"app_version"`
}

type MerchantModeDisableRequest struct {
	InstallationID string `json:"installation_id"`
	PIN            string `json:"pin"`
}

type MerchantModeDeviceUpdateRequest struct {
	MerchantModeEnabled bool `json:"merchant_mode_enabled"`
}

type MerchantModeDeviceUpdateResponse struct {
	Device *MerchantModeDevice `json:"device"`
}
