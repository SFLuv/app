package structs

type Event struct {
	Id          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Codes       uint32 `json:"codes"`
	Amount      uint64 `json:"amount"`
	Expiration  uint64 `json:"expiration"`
	Creator     string
}

type Code struct {
	Id       string `json:"id"`
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
