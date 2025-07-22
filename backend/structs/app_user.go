package structs

type User struct {
	Id          string  `json:"id"`
	Exists      bool    `json:"exists"`
	IsAdmin     bool    `json:"is_admin"`
	IsMerchant  bool    `json:"is_merchant"`
	IsOrganizer bool    `json:"is_organizer"`
	IsImprover  bool    `json:"is_improver"`
	Email       *string `json:"contact_email"`
	Phone       *string `json:"contact_phone"`
	Name        *string `json:"contact_name"`
}
