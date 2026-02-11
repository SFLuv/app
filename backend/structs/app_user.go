package structs

type User struct {
	Id             string  `json:"id"`
	Exists         bool    `json:"exists"`
	IsAdmin        bool    `json:"is_admin"`
	IsMerchant     bool    `json:"is_merchant"`
	IsOrganizer    bool    `json:"is_organizer"`
	IsImprover     bool    `json:"is_improver"`
	IsAffiliate    bool    `json:"is_affiliate"`
	Email          *string `json:"contact_email"`
	Phone          *string `json:"contact_phone"`
	Name           *string `json:"contact_name"`
	PayPalEth      string  `json:"paypal_eth"`
	LastRedemption int     `json:"last_redemption"`
}

type AuthedUserResponse struct {
	User      User        `json:"user"`
	Wallets   []*Wallet   `json:"wallets"`
	Locations []*Location `json:"locations"`
	Contacts  []*Contact  `json:"contacts"`
	Affiliate *Affiliate  `json:"affiliate,omitempty"`
}
