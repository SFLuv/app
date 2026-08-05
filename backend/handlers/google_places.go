package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

// Google Places (New) place-details endpoint. The place id is appended
// directly. A var rather than a const so tests can point it at a stub server.
var googlePlacesDetailsBaseURL = "https://places.googleapis.com/v1/places/"

// Only the fields the merchant record actually stores. Places bills per field
// mask, so this stays as narrow as the location columns allow.
const googlePlacesFieldMask = "id,types,primaryTypeDisplayName,displayName,shortFormattedAddress," +
	"addressComponents,location,rating,regularOpeningHours,websiteUri,nationalPhoneNumber," +
	"googleMapsUri,businessStatus"

var googlePlacesHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

// addressOnlyPlaceTypes are the Places types that describe a postal address
// rather than a business. A place whose types are drawn only from this set has
// no business identity, so its displayName is the street address itself — which
// is how a merchant record can end up named "1234 Main St". Submissions that
// resolve to one of these are rejected before they reach the database.
var addressOnlyPlaceTypes = map[string]struct{}{
	"street_address":              {},
	"street_number":               {},
	"route":                       {},
	"intersection":                {},
	"premise":                     {},
	"subpremise":                  {},
	"plus_code":                   {},
	"postal_code":                 {},
	"postal_code_prefix":          {},
	"postal_code_suffix":          {},
	"geocode":                     {},
	"locality":                    {},
	"sublocality":                 {},
	"sublocality_level_1":         {},
	"sublocality_level_2":         {},
	"neighborhood":                {},
	"administrative_area_level_1": {},
	"administrative_area_level_2": {},
	"administrative_area_level_3": {},
	"country":                     {},
	"political":                   {},
	"floor":                       {},
	"room":                        {},
	"post_box":                    {},
}

type googlePlacesLocalizedText struct {
	Text string `json:"text"`
}

type googlePlacesAddressComponent struct {
	LongText  string   `json:"longText"`
	ShortText string   `json:"shortText"`
	Types     []string `json:"types"`
}

type googlePlacesLatLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type googlePlacesOpeningHours struct {
	WeekdayDescriptions []string `json:"weekdayDescriptions"`
}

type googlePlacesDetails struct {
	ID                     string                         `json:"id"`
	Types                  []string                       `json:"types"`
	PrimaryTypeDisplayName googlePlacesLocalizedText      `json:"primaryTypeDisplayName"`
	DisplayName            googlePlacesLocalizedText      `json:"displayName"`
	ShortFormattedAddress  string                         `json:"shortFormattedAddress"`
	AddressComponents      []googlePlacesAddressComponent `json:"addressComponents"`
	Location               *googlePlacesLatLng            `json:"location"`
	Rating                 float64                        `json:"rating"`
	RegularOpeningHours    *googlePlacesOpeningHours      `json:"regularOpeningHours"`
	WebsiteURI             string                         `json:"websiteUri"`
	NationalPhoneNumber    string                         `json:"nationalPhoneNumber"`
	GoogleMapsURI          string                         `json:"googleMapsUri"`
	BusinessStatus         string                         `json:"businessStatus"`
}

// googlePlacesAPIKey returns the server-side Places key. A dedicated
// server key is preferred because the browser key is referrer-restricted and
// cannot be used from the backend; the public key is accepted as a fallback so
// a single-key deployment still verifies.
func googlePlacesAPIKey() string {
	for _, key := range []string{"GOOGLE_MAPS_SERVER_API_KEY", "GOOGLE_MAPS_API_KEY", "NEXT_PUBLIC_GOOGLE_MAPS_API_KEY"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

// GooglePlacesVerificationEnabled reports whether the backend can reach the
// Places API. When it cannot, submissions fall back to client-supplied values
// and the local validation rules still apply.
func GooglePlacesVerificationEnabled() bool {
	return googlePlacesAPIKey() != ""
}

// errPlaceNotABusiness is returned when a place id resolves to a postal address
// instead of an establishment.
type placeVerificationError struct {
	message string
}

func (e *placeVerificationError) Error() string {
	return e.message
}

func newPlaceVerificationError(format string, args ...any) error {
	return &placeVerificationError{message: fmt.Sprintf(format, args...)}
}

// IsPlaceVerificationError reports whether err is a rejection of the submitted
// place (bad id, address-only result, closed business) rather than a transport
// failure. Callers surface these to the user as a 400 and treat everything else
// as a soft failure.
func IsPlaceVerificationError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*placeVerificationError)
	return ok
}

func addressComponent(components []googlePlacesAddressComponent, wanted string) string {
	for _, component := range components {
		for _, componentType := range component.Types {
			if componentType == wanted {
				return component.LongText
			}
		}
	}
	return ""
}

// isBusinessPlace reports whether any of the place's types describe something
// other than a postal address.
func isBusinessPlace(types []string) bool {
	for _, placeType := range types {
		if _, addressOnly := addressOnlyPlaceTypes[placeType]; !addressOnly {
			return true
		}
	}
	return false
}

// VerifyGooglePlace fetches the canonical Google record for placeID and maps it
// onto the Google-derived subset of a location. It is the server-side guard
// against a client submitting a place that is really a street address, and
// against a client submitting Google fields that do not match the place id.
func VerifyGooglePlace(ctx context.Context, placeID string) (*structs.VerifiedGooglePlace, error) {
	placeID = strings.TrimSpace(placeID)
	if placeID == "" {
		return nil, newPlaceVerificationError("a Google place must be selected")
	}

	apiKey := googlePlacesAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("google places api key is not configured")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, googlePlacesDetailsBaseURL+placeID, nil)
	if err != nil {
		return nil, fmt.Errorf("error building places request: %w", err)
	}
	request.Header.Set("X-Goog-Api-Key", apiKey)
	request.Header.Set("X-Goog-FieldMask", googlePlacesFieldMask)

	response, err := googlePlacesHTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("error calling places api: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("error reading places response: %w", err)
	}

	if response.StatusCode == http.StatusNotFound {
		return nil, newPlaceVerificationError("Google no longer recognizes this place. Search for your business again.")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("places api returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var details googlePlacesDetails
	if err := json.Unmarshal(body, &details); err != nil {
		return nil, fmt.Errorf("error decoding places response: %w", err)
	}

	if details.Location == nil {
		return nil, newPlaceVerificationError("Google returned no coordinates for this place. Search for your business again.")
	}
	if !isBusinessPlace(details.Types) {
		return nil, newPlaceVerificationError("That result is a street address, not a business listing. Search for your business by name.")
	}
	if details.BusinessStatus == "CLOSED_PERMANENTLY" {
		return nil, newPlaceVerificationError("Google lists this business as permanently closed.")
	}

	name := strings.TrimSpace(details.DisplayName.Text)
	if name == "" {
		return nil, newPlaceVerificationError("Google returned no business name for this place. Search for your business by name.")
	}

	street := strings.TrimSpace(strings.Join([]string{
		addressComponent(details.AddressComponents, "street_number"),
		addressComponent(details.AddressComponents, "route"),
	}, " "))
	if street == "" {
		street = strings.TrimSpace(details.ShortFormattedAddress)
	}

	verified := &structs.VerifiedGooglePlace{
		GoogleID:     details.ID,
		Name:         name,
		Type:         strings.TrimSpace(details.PrimaryTypeDisplayName.Text),
		Street:       street,
		City:         addressComponent(details.AddressComponents, "locality"),
		State:        addressComponent(details.AddressComponents, "administrative_area_level_1"),
		ZIP:          addressComponent(details.AddressComponents, "postal_code"),
		Lat:          details.Location.Latitude,
		Lng:          details.Location.Longitude,
		Phone:        strings.TrimSpace(details.NationalPhoneNumber),
		Website:      strings.TrimSpace(details.WebsiteURI),
		Rating:       details.Rating,
		MapsPage:     strings.TrimSpace(details.GoogleMapsURI),
		OpeningHours: []string{},
	}
	if verified.GoogleID == "" {
		verified.GoogleID = placeID
	}
	if details.RegularOpeningHours != nil {
		verified.OpeningHours = details.RegularOpeningHours.WeekdayDescriptions
	}

	return verified, nil
}
