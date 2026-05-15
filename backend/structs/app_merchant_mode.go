package structs

import "time"

type MerchantModeDevice struct {
	ID                  string     `json:"id"`
	UserID              string     `json:"user_id"`
	LocationID          uint       `json:"location_id"`
	LocationName        string     `json:"location_name"`
	WalletAddress       string     `json:"wallet_address"`
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
