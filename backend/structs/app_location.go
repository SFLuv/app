package structs

// VerifiedGooglePlace is the Google-derived subset of a location as returned by
// the Places API and re-fetched server-side. Everything in here is authoritative
// and overwrites whatever the client submitted for the same fields.
type VerifiedGooglePlace struct {
	GoogleID     string   `json:"google_id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Street       string   `json:"street"`
	City         string   `json:"city"`
	State        string   `json:"state"`
	ZIP          string   `json:"zip"`
	Lat          float64  `json:"lat"`
	Lng          float64  `json:"lng"`
	Phone        string   `json:"phone"`
	Website      string   `json:"website"`
	Rating       float64  `json:"rating"`
	MapsPage     string   `json:"maps_page"`
	OpeningHours []string `json:"opening_hours"`
}

// ApplyTo overwrites the Google-derived fields of a location with the verified
// values. Merchant-authored fields (description, contact details, POS answers)
// are left untouched.
func (v *VerifiedGooglePlace) ApplyTo(location *Location) {
	if v == nil || location == nil {
		return
	}

	location.GoogleID = v.GoogleID
	location.Name = v.Name
	location.Type = v.Type
	location.Street = v.Street
	location.City = v.City
	location.State = v.State
	location.ZIP = v.ZIP
	location.Lat = v.Lat
	location.Lng = v.Lng
	location.Website = v.Website
	location.Rating = v.Rating
	location.MapsPage = v.MapsPage
	location.OpeningHours = v.OpeningHours

	// The merchant may publish a different customer-facing number than the one
	// on the Google listing, so Google's phone is only a fallback.
	if location.Phone == "" {
		location.Phone = v.Phone
	}
}

// TODO SANCHEZ: Define the Location struct with appropriate fields, this is the serializer

/*type Location struct {
	ID           uint         `json:"id"`
	GoogleID     string       `json:"google_id"`
	OwnerID      string       `json:"owner_id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Type         string       `json:"type"`
	Approval     bool         `json:"approval"`
	Street       string       `json:"street"`
	City         string       `json:"city"`
	State        string       `json:"state"`
	ZIP          string       `json:"zip"`
	Lat          float64      `json:"lat"`
	Lng          float64      `json:"lng"`
	Phone        string       `json:"phone"`
	Email        string       `json:"email"`
	Website      string       `json:"website"`
	ImageURL     string       `json:"image_url"`
	Rating       float64      `json:"rating"`
	MapsPage     string       `json:"maps_page"`
	OpeningHours [][2]float64 `json:"opening_hours"`
}*/

type Location struct {
	ID                 uint                    `json:"id"`
	GoogleID           string                  `json:"google_id"`
	OwnerID            string                  `json:"owner_id"`
	Name               string                  `json:"name"`
	Description        string                  `json:"description"`
	Type               string                  `json:"type"`
	Approval           *bool                   `json:"approval"`
	Street             string                  `json:"street"`
	City               string                  `json:"city"`
	State              string                  `json:"state"`
	ZIP                string                  `json:"zip"`
	Lat                float64                 `json:"lat"`
	Lng                float64                 `json:"lng"`
	Phone              string                  `json:"phone"`
	Email              string                  `json:"email"`
	AdminPhone         string                  `json:"admin_phone"`
	AdminEmail         string                  `json:"admin_email"`
	Website            string                  `json:"website"`
	ImageURL           string                  `json:"image_url"`
	Rating             float64                 `json:"rating"`
	MapsPage           string                  `json:"maps_page"`
	OpeningHours       []string                `json:"opening_hours"`
	ContactFirstName   string                  `json:"contact_firstname"`
	ContactLastName    string                  `json:"contact_lastname"`
	ContactPhone       string                  `json:"contact_phone"`
	PosSystem          string                  `json:"pos_system"`
	SoleProprietorship string                  `json:"sole_proprietorship"`
	TippingPolicy      string                  `json:"tipping_policy"`
	TippingDivision    string                  `json:"tipping_division"`
	TableCoverage      string                  `json:"table_coverage"`
	ServiceStations    int                     `json:"service_stations"`
	TabletModel        string                  `json:"tablet_model"`
	MessagingService   string                  `json:"messaging_service"`
	PayToAddress       string                  `json:"pay_to_address"`
	TipToAddress       string                  `json:"tip_to_address"`
	PaymentWallets     []LocationPaymentWallet `json:"payment_wallets"`
	Reference          string                  `json:"reference"`
}

type LocationPaymentWallet struct {
	ID            int    `json:"id"`
	LocationID    uint   `json:"location_id"`
	WalletAddress string `json:"wallet_address"`
	IsDefault     bool   `json:"is_default"`
}

type LocationWalletSettingsUpdateRequest struct {
	PaymentWalletAddresses      []string `json:"payment_wallet_addresses"`
	DefaultPaymentWalletAddress string   `json:"default_payment_wallet_address"`
	TippingWalletAddress        string   `json:"tipping_wallet_address"`
}

type PublicLocation struct {
	ID           uint     `json:"id"`
	GoogleID     string   `json:"google_id"`
	Name         string   `json:"name"`
	Approval     bool     `json:"approval"`
	PayToAddress string   `json:"pay_to_address"`
	TipToAddress string   `json:"tip_to_address"`
	Description  string   `json:"description"`
	Type         string   `json:"type"`
	Street       string   `json:"street"`
	City         string   `json:"city"`
	State        string   `json:"state"`
	ZIP          string   `json:"zip"`
	Lat          float64  `json:"lat"`
	Lng          float64  `json:"lng"`
	Phone        string   `json:"phone"`
	Email        string   `json:"email"`
	Website      string   `json:"website"`
	ImageURL     string   `json:"image_url"`
	Rating       float64  `json:"rating"`
	MapsPage     string   `json:"maps_page"`
	OpeningHours []string `json:"opening_hours"`
}

type LocationsPageRequest struct {
	Page  uint
	Count uint
}

type AuthedLocationResponse struct {
}

type LocationResponse struct {
	Name    string          `json:"name"`
	Address LocationAddress `json:"address"`
}

type LocationAddress struct {
	Street string `json:"street"`
}
