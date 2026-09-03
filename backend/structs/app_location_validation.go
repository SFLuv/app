package structs

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// Service-area bounds for a submitted merchant location. The browser restricts
// Places search to a tight box around San Francisco; these defaults are a
// deliberately looser regional box so a listing whose coordinates land just
// outside the search box is still accepted, while junk (0,0 or another
// continent) is rejected. Override per-deployment with SERVICE_AREA_* env vars.
const (
	defaultServiceAreaCenterLat = 37.7749
	defaultServiceAreaCenterLng = -122.4194
	defaultServiceAreaLatRadius = 0.5
	defaultServiceAreaLngRadius = 0.5
)

const (
	maxLocationTextLength        = 512
	maxLocationDescriptionLength = 4000
)

// LocationValidationError is a user-facing rejection of a submitted location.
type LocationValidationError struct {
	Field   string
	Message string
}

func (e *LocationValidationError) Error() string {
	return e.Message
}

func newLocationValidationError(field, format string, args ...any) error {
	return &LocationValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// IsLocationValidationError reports whether err is a user-facing validation
// rejection rather than an internal failure.
func IsLocationValidationError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*LocationValidationError)
	return ok
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}

// ServiceAreaBounds returns the south/north/west/east bounds a merchant
// location must fall inside.
func ServiceAreaBounds() (south, north, west, east float64) {
	centerLat := envFloat("SERVICE_AREA_CENTER_LAT", defaultServiceAreaCenterLat)
	centerLng := envFloat("SERVICE_AREA_CENTER_LNG", defaultServiceAreaCenterLng)
	latRadius := math.Abs(envFloat("SERVICE_AREA_LAT_RADIUS", defaultServiceAreaLatRadius))
	lngRadius := math.Abs(envFloat("SERVICE_AREA_LNG_RADIUS", defaultServiceAreaLngRadius))

	return centerLat - latRadius, centerLat + latRadius, centerLng - lngRadius, centerLng + lngRadius
}

// NormalizeForSubmission trims every free-text field on the location in place.
// It runs before validation so that whitespace-only input is treated as empty.
func (l *Location) NormalizeForSubmission() {
	if l == nil {
		return
	}

	trim := func(target *string) {
		*target = strings.TrimSpace(*target)
	}

	trim(&l.GoogleID)
	trim(&l.ListingSource)
	// Absent means the Google path. Defaulting the other way would let any
	// client skip place verification simply by omitting the field.
	if l.ListingSource == "" {
		l.ListingSource = ListingSourceGooglePlace
	}
	if l.ListingSource == ListingSourceManual {
		l.GoogleID = ""
	}

	trim(&l.Name)
	trim(&l.Description)
	trim(&l.Type)
	trim(&l.Street)
	trim(&l.City)
	trim(&l.State)
	trim(&l.ZIP)
	trim(&l.Phone)
	trim(&l.Email)
	trim(&l.AdminPhone)
	trim(&l.AdminEmail)
	trim(&l.Website)
	trim(&l.ImageURL)
	trim(&l.MapsPage)
	trim(&l.ContactName)
	trim(&l.ContactFirstName)
	trim(&l.ContactLastName)
	trim(&l.ContactPhone)
	trim(&l.ReferralSource)
	trim(&l.PosSystem)
	trim(&l.SoleProprietorship)
	trim(&l.TippingPolicy)
	trim(&l.TippingDivision)
	trim(&l.TableCoverage)
	trim(&l.TabletModel)
	trim(&l.MessagingService)
	trim(&l.Reference)

	// Contact phone lives in two columns — see Location.ContactPhone — and the
	// form only ever fills one of them. Mirroring here rather than at each write
	// site means the admin panel, the approval email and the merchant's own form
	// cannot disagree about which number is the contact's.
	if l.AdminPhone == "" {
		l.AdminPhone = l.ContactPhone
	}
	if l.ContactPhone == "" {
		l.ContactPhone = l.AdminPhone
	}
	// Likewise the name. A listing filled in under the single-sheet form carries
	// the pair and no ContactName; one filled in now carries the reverse.
	if l.ContactName == "" {
		l.ContactName = strings.TrimSpace(strings.Join(filterEmpty(l.ContactFirstName, l.ContactLastName), " "))
	}

	hours := make([]string, 0, len(l.OpeningHours))
	for _, entry := range l.OpeningHours {
		hours = append(hours, strings.TrimSpace(entry))
	}
	l.OpeningHours = hours
}

// formattedStreetLine rebuilds the comma-joined address the way Google and the
// browser autocomplete render it ("1234 Main St, San Francisco, CA 94110").
// Comparing the business name against this as well as the bare street catches
// the merchant who pastes the whole suggestion into the name field, which the
// street-only comparison misses.
func (l *Location) formattedStreetLine() string {
	if l == nil {
		return ""
	}

	cityState := strings.TrimSpace(strings.Join(filterEmpty(l.City, l.State), ", "))
	if l.ZIP != "" {
		cityState = strings.TrimSpace(cityState + " " + l.ZIP)
	}

	return strings.Join(filterEmpty(l.Street, cityState), ", ")
}

func filterEmpty(values ...string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return kept
}

// ValidateForSubmission checks the invariants the map depends on. It runs after
// Google verification has overwritten the Google-derived fields, so a failure
// here means the merchant-authored part of the form is unusable.
func (l *Location) ValidateForSubmission() error {
	if l == nil {
		return newLocationValidationError("location", "location is required")
	}

	listingSource := l.EffectiveListingSource()
	switch listingSource {
	case ListingSourceGooglePlace, ListingSourceManual:
	default:
		return newLocationValidationError("listing_source", "Unknown listing source.")
	}

	required := []struct {
		field string
		label string
		value string
	}{
		{"name", "Business name", l.Name},
		{"street", "Street address", l.Street},
		{"city", "City", l.City},
		// The description is deliberately absent. It is optional on the form —
		// a shop with nothing to add still belongs on the map — and requiring
		// it here refused a submission over a field the merchant was told they
		// could leave blank. Its length is still bounded below.
		{"type", "Business type", l.Type},
		{"contact_name", "Contact name", l.ContactName},
		{"admin_email", "Contact email", l.AdminEmail},
		{"admin_phone", "Contact phone", l.AdminPhone},
		{"referral_source", "How you heard about SFLuv", l.ReferralSource},
		{"pos_system", "POS type", l.PosSystem},
	}
	// A Google listing must carry the place id it was verified against. A manual
	// listing must not carry one at all — accepting both would leave it ambiguous
	// which half of the record Google actually vouched for.
	if listingSource == ListingSourceGooglePlace {
		required = append(required, struct {
			field string
			label string
			value string
		}{"google_id", "Google place", l.GoogleID})
	} else if l.GoogleID != "" {
		return newLocationValidationError("google_id", "A manually entered listing cannot carry a Google place id.")
	}
	for _, entry := range required {
		if entry.value == "" {
			return newLocationValidationError(entry.field, "%s is required.", entry.label)
		}
	}

	// Every question on the Payment System step is required, and both of these
	// are yes/no. Unset is a third state that only a client which skipped the
	// step can produce, and accepts_tips decides whether the shop gets a tipping
	// wallet at approval — guessing it would be a decision made for the merchant.
	if l.AcceptsTips == nil {
		return newLocationValidationError("accepts_tips", "Tell us whether this location accepts tips.")
	}
	if l.HasStaffTablet == nil {
		return newLocationValidationError("has_staff_tablet", "Tell us whether staff have a tablet or phone available.")
	}

	lengths := []struct {
		field string
		label string
		value string
		max   int
	}{
		{"name", "Business name", l.Name, maxLocationTextLength},
		{"description", "Business description", l.Description, maxLocationDescriptionLength},
		{"street", "Street address", l.Street, maxLocationTextLength},
		{"contact_name", "Contact name", l.ContactName, maxLocationTextLength},
		{"referral_source", "How you heard about SFLuv", l.ReferralSource, maxLocationDescriptionLength},
		{"pos_system", "POS type", l.PosSystem, maxLocationTextLength},
	}
	for _, entry := range lengths {
		if len(entry.value) > entry.max {
			return newLocationValidationError(entry.field, "%s must be %d characters or fewer.", entry.label, entry.max)
		}
	}

	// The Shiba failure mode: a merchant ends up on the map named "1234 Main St".
	// It arrives two ways. On the Google path a place that is really a postal
	// address returns its street address as the display name — Places
	// verification catches that upstream and this is the backstop for
	// deployments with no server-side key. On the manual path there is no
	// upstream check at all: the merchant filled the address in via autocomplete
	// and left the name field holding the same text, which is exactly what this
	// whole flow exists to catch.
	if strings.EqualFold(l.Name, l.Street) || strings.EqualFold(l.Name, l.formattedStreetLine()) {
		if listingSource == ListingSourceManual {
			return newLocationValidationError("name", "Enter your business name — that looks like the street address, not a name.")
		}
		return newLocationValidationError("name", "The selected result looks like a street address rather than a business. Search for your business by name.")
	}

	if math.IsNaN(l.Lat) || math.IsNaN(l.Lng) || math.IsInf(l.Lat, 0) || math.IsInf(l.Lng, 0) {
		return newLocationValidationError("lat", "Location coordinates are invalid.")
	}
	if l.Lat == 0 && l.Lng == 0 {
		return newLocationValidationError("lat", "Location coordinates are missing.")
	}
	if l.Lat < -90 || l.Lat > 90 {
		return newLocationValidationError("lat", "Latitude is out of range.")
	}
	if l.Lng < -180 || l.Lng > 180 {
		return newLocationValidationError("lng", "Longitude is out of range.")
	}

	south, north, west, east := ServiceAreaBounds()
	if l.Lat < south || l.Lat > north || l.Lng < west || l.Lng > east {
		return newLocationValidationError("lat", "That location is outside the SFLuv service area.")
	}

	if l.ServiceStations < 0 || l.ServiceStations > 1000 {
		return newLocationValidationError("service_stations", "Number of service stations is out of range.")
	}
	if l.Rating < 0 || l.Rating > 5 {
		return newLocationValidationError("rating", "Rating is out of range.")
	}
	if len(l.OpeningHours) > 14 {
		return newLocationValidationError("opening_hours", "Too many opening-hours entries.")
	}

	return nil
}

// ValidateForUpdate checks the merchant-authored fields an owner is allowed to
// change. Google-derived fields are not writable on that route, so they are not
// re-checked here.
//
// Almost nothing is required. The description is optional, as it is on the
// submission form. The contact fields are required at submission but not here:
// a record predating those columns can hold empty contact details, and blocking
// such a merchant from editing their public description over data they cannot
// reach from this screen is worse than letting the empty value round-trip
// unchanged.
func (l *Location) ValidateForUpdate() error {
	if l == nil {
		return newLocationValidationError("location", "location is required")
	}

	required := []struct {
		field string
		label string
		value string
	}{
		{"name", "Business name", l.Name},
		{"street", "Street address", l.Street},
	}
	for _, entry := range required {
		if entry.value == "" {
			return newLocationValidationError(entry.field, "%s is required.", entry.label)
		}
	}

	// Same guard as submission: a listing named after its own address is the
	// Shiba failure mode, and an edit must not be able to recreate it.
	if strings.EqualFold(l.Name, l.Street) {
		return newLocationValidationError("name", "Business name cannot be the same as the street address.")
	}

	lengths := []struct {
		field string
		label string
		value string
		max   int
	}{
		{"name", "Business name", l.Name, maxLocationTextLength},
		{"street", "Street address", l.Street, maxLocationTextLength},
		{"description", "Business description", l.Description, maxLocationDescriptionLength},
		{"contact_name", "Contact name", l.ContactName, maxLocationTextLength},
		{"referral_source", "How you heard about SFLuv", l.ReferralSource, maxLocationDescriptionLength},
		{"pos_system", "POS type", l.PosSystem, maxLocationTextLength},
	}
	for _, entry := range lengths {
		if len(entry.value) > entry.max {
			return newLocationValidationError(entry.field, "%s must be %d characters or fewer.", entry.label, entry.max)
		}
	}

	if l.ServiceStations < 0 || l.ServiceStations > 1000 {
		return newLocationValidationError("service_stations", "Number of service stations is out of range.")
	}

	return nil
}
