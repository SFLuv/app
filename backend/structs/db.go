package structs

type Event struct {
	Id          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Codes       uint32 `json:"codes"`
	Amount      uint64 `json:"amount"`
	StartAt     uint64 `json:"start_at"`
	Expiration  uint64 `json:"expiration"`
	Owner       string `json:"owner"`
	// OrganizationId scopes the event to the creating user's organization so any
	// member of that org can manage it.
	OrganizationId int64 `json:"organization_id,omitempty"`
}

type EventsRequest struct {
	Search  string `json:"search"`
	Page    int    `json:"page"`
	Count   int    `json:"count"`
	Expired bool   `json:"expired"`
}

type Code struct {
	Id       string `json:"id"`
	Redeemed bool   `json:"redeemed"`
	Event    string `json:"event"`
	// Number is the stable per-event label printed on the QR sheet. Assigned at
	// mint time and never reused, so a reprint puts the same number on the same
	// QR.
	Number int `json:"number"`
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
