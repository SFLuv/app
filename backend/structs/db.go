package structs

import (
	"gorm.io/gorm"
)

type Redemption struct {
	gorm.Model
	UUID    string `json:"id"`
	Account string `json:"account"`
	Code    string `json:"code"`
}

type Account struct {
	gorm.Model
	UUID    string `json:"id"`
	Address string `json:"address"`
}

type Event struct {
	gorm.Model
	UUID        string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Codes       uint32 `json:"codes"`
	Amount      uint64 `json:"amount"`
	Expiration  uint64 `json:"expiration"`
}

type Code struct {
	gorm.Model
	UUID     string `json:"id"`
	Redeemed bool   `json:"redeemed"`
	Event    string `json:"event"`
}

type CodesPageRequest struct {
	Event string `json:"event"`
	Count uint32 `json:"count"`
	Page  uint32 `json:"page"`
}

type RedeemRequest struct {
	Code    string `json:"code"`
	Address string `json:"address"`
}

type NewCodesRequest struct {
	Event string `json:"event"`
	Count uint32 `json:"count"`
}
