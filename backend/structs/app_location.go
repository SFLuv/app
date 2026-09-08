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
	// Parsed from Google's periods. Empty when Google published none we can
	// use, which is what lets a nightly sync skip rather than wipe.
	StructuredHours []LocationDayHours `json:"structured_hours"`
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

	// Google's category only when Google has one. Not every place carries a
	// primary type, and business type is required at submission — overwriting
	// unconditionally meant a merchant whose listing has no category had their
	// own answer wiped on the way in and the submission refused for a field
	// they had in fact filled.
	if v.Type != "" {
		location.Type = v.Type
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

// How a merchant location entered the system. Google is still the primary and
// preferred path; manual exists because a business without a Google Business
// Profile has no place id to select and was previously unable to onboard at all.
const (
	ListingSourceGooglePlace = "google_place"
	ListingSourceManual      = "manual"
)

// EffectiveListingSource resolves an unset source to the Google path. Validation
// must not depend on NormalizeForSubmission having run first, and defaulting the
// other way would mean an omitted field silently opts a submission out of
// server-side place verification.
func (l *Location) EffectiveListingSource() string {
	if l == nil || l.ListingSource == "" {
		return ListingSourceGooglePlace
	}
	return l.ListingSource
}

type Location struct {
	ID uint `json:"id"`
	// GoogleID is empty for manual listings. It is written to the database as
	// NULL in that case so the partial unique index on google_id does not treat
	// every manual row as a duplicate of the last.
	GoogleID string `json:"google_id"`
	// ListingSource defaults to google_place when a client omits it, so an older
	// client cannot silently downgrade into the manual path and skip the
	// server-side Places verification that google_place submissions get.
	ListingSource string  `json:"listing_source"`
	OwnerID       string  `json:"owner_id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Type          string  `json:"type"`
	Approval      *bool   `json:"approval"`
	Street        string  `json:"street"`
	City          string  `json:"city"`
	State         string  `json:"state"`
	ZIP           string  `json:"zip"`
	Lat           float64 `json:"lat"`
	Lng           float64 `json:"lng"`
	Phone         string  `json:"phone"`
	Email         string  `json:"email"`
	AdminPhone    string  `json:"admin_phone"`
	AdminEmail    string  `json:"admin_email"`
	Website       string  `json:"website"`
	ImageURL      string  `json:"image_url"`
	// IconURL is the merchant's uploaded map icon, empty when they have not
	// uploaded one. Version-stamped so a replacement is picked up despite the
	// long cache lifetime the bytes themselves are served with.
	IconURL string `json:"icon_url"`
	// PhotoURL is the merchant's uploaded storefront photo, empty when they have
	// not uploaded one. Distinct from ImageURL, which is a link to a Google Maps
	// page captured at creation and is not an image address at all.
	PhotoURL     string   `json:"photo_url"`
	Rating       float64  `json:"rating"`
	MapsPage     string   `json:"maps_page"`
	OpeningHours []string `json:"opening_hours"`
	// ContactName is the Location Approval Form's single Contact field, and
	// supersedes the first/last pair below. Those are still read so a listing
	// filled in under the old form does not lose the name it already holds.
	ContactName      string `json:"contact_name"`
	ContactFirstName string `json:"contact_firstname"`
	ContactLastName  string `json:"contact_lastname"`
	// ContactPhone and the Admin* pair below are the same two answers stored
	// twice, a split the single-sheet form left behind. The form writes both,
	// and Admin* stays the column every existing reader — the approval email,
	// the MCP merchant report, the admin panel — already looks at.
	ContactPhone string `json:"contact_phone"`
	// ReferralSource answers "How did you hear about SFLuv". An "Other" answer
	// is flattened to its write-in text before it is stored, the same way every
	// other Other-bearing dropdown on the form is.
	ReferralSource string `json:"referral_source"`
	PosSystem      string `json:"pos_system"`
	// AcceptsTips decides whether approval mints the location a tipping wallet,
	// so it is a pointer: nil is "never asked" — every listing that predates the
	// Location Approval Form — and must not be read as a no.
	AcceptsTips *bool `json:"accepts_tips"`
	// HasStaffTablet records whether staff have a tablet or phone to hand. It is
	// collected for the admin walking the merchant through setup, and is a
	// pointer for the same reason AcceptsTips is.
	HasStaffTablet *bool `json:"has_staff_tablet"`
	// Retired with the single-sheet form. Still read and written on the update
	// path so an existing listing's answers survive an edit, but the Location
	// Approval Form no longer asks any of them.
	SoleProprietorship string `json:"sole_proprietorship"`
	TippingPolicy      string `json:"tipping_policy"`
	TippingDivision    string `json:"tipping_division"`
	TableCoverage      string `json:"table_coverage"`
	ServiceStations    int    `json:"service_stations"`
	TabletModel        string `json:"tablet_model"`
	MessagingService   string `json:"messaging_service"`
	// PayToAddress is the location's till, read straight from
	// locations.payment_wallet_address. location_payment_wallets is still the
	// write model and still holds the soft-delete history, but no reader
	// re-derives the address from it: the column is the single answer, kept in
	// step by syncLocationPaymentWalletAddress inside whichever transaction
	// moved the wallets.
	PayToAddress   string                  `json:"pay_to_address"`
	TipToAddress   string                  `json:"tip_to_address"`
	PaymentWallets []LocationPaymentWallet `json:"payment_wallets"`
	Reference      string                  `json:"reference"`
	// Structured week, Monday first. Backs the time pickers; OpeningHours is
	// its rendering and stays for existing readers.
	Hours []LocationDayHours `json:"hours"`
	// When true the nightly Google sync leaves this listing's hours alone.
	HoursManual bool `json:"hours_manual"`
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

// AssignableWallet is one of the merchant's wallets, as offered when they swap
// the address filling a role at a location. Unavailable wallets are listed too,
// with the reason, because "in use by Shop B" explains far more than an address
// quietly missing from the list.
type AssignableWallet struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	// InUseBy names whatever already holds this address, or is empty when free.
	InUseBy string `json:"in_use_by"`
	// IsCurrent marks the address filling this role at this location right now.
	IsCurrent bool `json:"is_current"`
	// Available is true only when the wallet can be selected.
	Available bool `json:"available"`
}

// LocationWalletReplaceRequest swaps the wallet filling one role at a location.
//
// There is no "detach" mode for payments on purpose: a location must always have
// somewhere for money to land, so unhooking is always a replacement. Mode "new"
// derives a fresh address from the account factory; mode "existing" points at a
// wallet the merchant already owns and no other location is using.
type LocationWalletReplaceRequest struct {
	Mode    string `json:"mode"`
	Address string `json:"address"`
}

type AssignableWalletsResponse struct {
	Wallets []AssignableWallet `json:"wallets"`
}

type PublicLocation struct {
	ID       uint   `json:"id"`
	GoogleID string `json:"google_id"`
	Name     string `json:"name"`
	Approval bool   `json:"approval"`
	// The public face of locations.payment_wallet_address, and the address the
	// map's Pay button sends to; see Location.PayToAddress.
	PayToAddress string             `json:"pay_to_address"`
	TipToAddress string             `json:"tip_to_address"`
	Description  string             `json:"description"`
	Type         string             `json:"type"`
	Street       string             `json:"street"`
	City         string             `json:"city"`
	State        string             `json:"state"`
	ZIP          string             `json:"zip"`
	Lat          float64            `json:"lat"`
	Lng          float64            `json:"lng"`
	Phone        string             `json:"phone"`
	Email        string             `json:"email"`
	Website      string             `json:"website"`
	ImageURL     string             `json:"image_url"`
	IconURL      string             `json:"icon_url"`
	PhotoURL     string             `json:"photo_url"`
	Rating       float64            `json:"rating"`
	MapsPage     string             `json:"maps_page"`
	OpeningHours []string           `json:"opening_hours"`
	Hours        []LocationDayHours `json:"hours"`
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
