package structs

import "time"

type Wallet struct {
	Id           *int       `json:"id"`
	Owner        string     `json:"owner"`
	Name         string     `json:"name"`
	IsEoa        bool       `json:"is_eoa"`
	IsRedeemer   bool       `json:"is_redeemer"`
	EoaAddress   string     `json:"eoa_address"`
	SmartAddress *string    `json:"smart_address"`
	SmartIndex   *int       `json:"smart_index"`
	LastUnwrapAt *time.Time `json:"last_unwrap_at,omitempty"`
}
