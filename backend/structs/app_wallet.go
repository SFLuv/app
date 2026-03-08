package structs

import "time"

type Wallet struct {
	Id           *int       `json:"id"`
	Owner        string     `json:"owner"`
	Name         string     `json:"name"`
	IsEoa        bool       `json:"is_eoa"`
	IsRedeemer   bool       `json:"is_redeemer"`
	IsMinter     bool       `json:"is_minter"`
	EoaAddress   string     `json:"eoa_address"`
	SmartAddress *string    `json:"smart_address"`
	SmartIndex   *int       `json:"smart_index"`
	LastUnwrapAt *time.Time `json:"last_unwrap_at,omitempty"`
}

type WalletAddressOwnerLookup struct {
	UserID         string `json:"user_id"`
	IsMerchant     bool   `json:"is_merchant"`
	MerchantName   string `json:"merchant_name"`
	WalletName     string `json:"wallet_name"`
	MatchedAddress string `json:"matched_address"`
}
