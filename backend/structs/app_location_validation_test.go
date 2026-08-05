package structs

import (
	"strings"
	"testing"
)

// validLocation is a submission that should pass every rule, so each test can
// break exactly one thing.
func validLocation() *Location {
	return &Location{
		GoogleID:         "ChIJshiba",
		Name:             "Shiba SF",
		Description:      "Neighborhood restaurant.",
		Type:             "Restaurant",
		Street:           "517 Balboa Street",
		City:             "San Francisco",
		State:            "California",
		ZIP:              "94118",
		Lat:              37.7766,
		Lng:              -122.4646,
		AdminEmail:       "owner@shiba.example",
		AdminPhone:       "(415) 555-0134",
		ContactFirstName: "Sam",
		ContactLastName:  "Rivera",
		ServiceStations:  4,
		Rating:           4.6,
		OpeningHours:     []string{"Monday: 5:00 – 10:00 PM"},
	}
}

func TestValidateForSubmissionAcceptsValidLocation(t *testing.T) {
	if err := validLocation().ValidateForSubmission(); err != nil {
		t.Fatalf("ValidateForSubmission() error = %v; want nil", err)
	}
}

// The Shiba regression, at the layer that runs even when Places verification is
// unavailable: a merchant whose name is its own street address is not a real
// business record.
func TestValidateForSubmissionRejectsNameEqualToStreet(t *testing.T) {
	location := validLocation()
	location.Name = "517 Balboa Street"

	err := location.ValidateForSubmission()
	if err == nil {
		t.Fatal("ValidateForSubmission() error = nil; want a rejection")
	}
	if !IsLocationValidationError(err) {
		t.Fatalf("IsLocationValidationError(%v) = false; want true", err)
	}
	if !strings.Contains(err.Error(), "street address") {
		t.Fatalf("error = %q; want it to mention a street address", err.Error())
	}
}

func TestValidateForSubmissionIsCaseInsensitiveOnNameEqualToStreet(t *testing.T) {
	location := validLocation()
	location.Name = "517 balboa STREET"

	if err := location.ValidateForSubmission(); err == nil {
		t.Fatal("ValidateForSubmission() error = nil; want a rejection regardless of case")
	}
}

func TestValidateForSubmissionRequiresFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Location)
	}{
		{"missing google id", func(l *Location) { l.GoogleID = "" }},
		{"missing name", func(l *Location) { l.Name = "" }},
		{"missing street", func(l *Location) { l.Street = "" }},
		{"missing city", func(l *Location) { l.City = "" }},
		{"missing description", func(l *Location) { l.Description = "" }},
		{"missing contact first name", func(l *Location) { l.ContactFirstName = "" }},
		{"missing contact last name", func(l *Location) { l.ContactLastName = "" }},
		{"missing admin email", func(l *Location) { l.AdminEmail = "" }},
		{"missing admin phone", func(l *Location) { l.AdminPhone = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			location := validLocation()
			tt.mutate(location)

			err := location.ValidateForSubmission()
			if err == nil {
				t.Fatalf("ValidateForSubmission() error = nil; want a rejection for %s", tt.name)
			}
			if !IsLocationValidationError(err) {
				t.Fatalf("IsLocationValidationError(%v) = false; want true", err)
			}
		})
	}
}

func TestValidateForSubmissionRejectsBadCoordinates(t *testing.T) {
	tests := []struct {
		name string
		lat  float64
		lng  float64
	}{
		{"null island", 0, 0},
		{"latitude out of range", 200, -122.4646},
		{"longitude out of range", 37.7766, 400},
		{"outside the service area", 40.7128, -74.0060},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			location := validLocation()
			location.Lat = tt.lat
			location.Lng = tt.lng

			if err := location.ValidateForSubmission(); err == nil {
				t.Fatalf("ValidateForSubmission() error = nil; want a rejection for %s", tt.name)
			}
		})
	}
}

func TestServiceAreaBoundsRespectEnvOverrides(t *testing.T) {
	t.Setenv("SERVICE_AREA_CENTER_LAT", "40.7128")
	t.Setenv("SERVICE_AREA_CENTER_LNG", "-74.0060")
	t.Setenv("SERVICE_AREA_LAT_RADIUS", "1")
	t.Setenv("SERVICE_AREA_LNG_RADIUS", "1")

	location := validLocation()
	location.Lat = 40.7128
	location.Lng = -74.0060

	if err := location.ValidateForSubmission(); err != nil {
		t.Fatalf("ValidateForSubmission() error = %v; want the overridden service area to accept it", err)
	}
}

func TestValidateForSubmissionRejectsOutOfRangeNumbers(t *testing.T) {
	location := validLocation()
	location.ServiceStations = -1
	if err := location.ValidateForSubmission(); err == nil {
		t.Fatal("ValidateForSubmission() error = nil; want a rejection for negative service stations")
	}

	location = validLocation()
	location.Rating = 9
	if err := location.ValidateForSubmission(); err == nil {
		t.Fatal("ValidateForSubmission() error = nil; want a rejection for an out-of-range rating")
	}
}

func TestNormalizeForSubmissionTrimsWhitespaceOnlyInputToEmpty(t *testing.T) {
	location := validLocation()
	location.Description = "   \t  "
	location.Name = "  Shiba SF  "
	location.OpeningHours = []string{"  Monday: 5:00 – 10:00 PM  "}

	location.NormalizeForSubmission()

	if location.Name != "Shiba SF" {
		t.Fatalf("Name = %q; want %q", location.Name, "Shiba SF")
	}
	if location.OpeningHours[0] != "Monday: 5:00 – 10:00 PM" {
		t.Fatalf("OpeningHours[0] = %q; want it trimmed", location.OpeningHours[0])
	}
	if err := location.ValidateForSubmission(); err == nil {
		t.Fatal("ValidateForSubmission() error = nil; want whitespace-only description to be treated as missing")
	}
}

// An update carries no coordinates or place id — those are written only by the
// verified Google path — so their absence must not fail validation.
func TestValidateForUpdateIgnoresPlaceIdentityFields(t *testing.T) {
	location := &Location{
		ID:          7,
		Name:        "Shiba SF",
		Description: "Neighborhood restaurant.",
		Street:      "517 Balboa Street",
	}

	if err := location.ValidateForUpdate(); err != nil {
		t.Fatalf("ValidateForUpdate() error = %v; want nil", err)
	}
}

func TestValidateForUpdateRequiresEditableFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Location)
	}{
		{"missing name", func(l *Location) { l.Name = "" }},
		{"missing description", func(l *Location) { l.Description = "" }},
		{"missing street", func(l *Location) { l.Street = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			location := &Location{
				ID:          7,
				Name:        "Shiba SF",
				Description: "Neighborhood restaurant.",
				Street:      "517 Balboa Street",
			}
			tt.mutate(location)

			err := location.ValidateForUpdate()
			if err == nil {
				t.Fatalf("ValidateForUpdate() error = nil; want a rejection for %s", tt.name)
			}
			if !IsLocationValidationError(err) {
				t.Fatalf("IsLocationValidationError(%v) = false; want true", err)
			}
		})
	}
}

// An edit must not be able to recreate the Shiba failure mode.
func TestValidateForUpdateRejectsNameEqualToStreet(t *testing.T) {
	location := &Location{
		ID:          7,
		Name:        "517 Balboa Street",
		Description: "Neighborhood restaurant.",
		Street:      "517 Balboa Street",
	}

	if err := location.ValidateForUpdate(); err == nil {
		t.Fatal("ValidateForUpdate() error = nil; want a rejection")
	}
}

// A legacy record with no contact details must still be editable, otherwise the
// merchant is locked out of their own description by data they cannot reach
// from this screen.
func TestValidateForUpdateAllowsEmptyLegacyContactFields(t *testing.T) {
	location := &Location{
		ID:          7,
		Name:        "Shiba SF",
		Description: "Neighborhood restaurant.",
		Street:      "517 Balboa Street",
	}

	if err := location.ValidateForUpdate(); err != nil {
		t.Fatalf("ValidateForUpdate() error = %v; want nil", err)
	}
}

func TestApplyToOverwritesGoogleFieldsAndKeepsMerchantFields(t *testing.T) {
	location := &Location{
		Name:        "Whatever The Client Sent",
		Street:      "999 Fake St",
		Lat:         1,
		Lng:         1,
		Description: "Merchant-authored copy.",
		AdminEmail:  "owner@shiba.example",
		Phone:       "(415) 555-9999",
	}

	verified := &VerifiedGooglePlace{
		GoogleID:     "ChIJshiba",
		Name:         "Shiba SF",
		Type:         "Restaurant",
		Street:       "517 Balboa Street",
		City:         "San Francisco",
		State:        "California",
		ZIP:          "94118",
		Lat:          37.7766,
		Lng:          -122.4646,
		Phone:        "(415) 555-0134",
		Website:      "https://shiba.example",
		Rating:       4.6,
		MapsPage:     "https://maps.google.com/?cid=1",
		OpeningHours: []string{"Monday: 5:00 – 10:00 PM"},
	}

	verified.ApplyTo(location)

	if location.Name != "Shiba SF" || location.Street != "517 Balboa Street" {
		t.Fatalf("google fields were not overwritten: name=%q street=%q", location.Name, location.Street)
	}
	if location.Lat != 37.7766 || location.Lng != -122.4646 {
		t.Fatalf("coordinates were not overwritten: %v,%v", location.Lat, location.Lng)
	}
	if location.Description != "Merchant-authored copy." {
		t.Fatalf("Description = %q; want the merchant's own copy to survive", location.Description)
	}
	// A merchant's published phone can differ from the Google listing's.
	if location.Phone != "(415) 555-9999" {
		t.Fatalf("Phone = %q; want the merchant-supplied number to win", location.Phone)
	}
}

func TestApplyToFillsPhoneWhenMerchantLeftItBlank(t *testing.T) {
	location := &Location{}
	verified := &VerifiedGooglePlace{Phone: "(415) 555-0134"}

	verified.ApplyTo(location)

	if location.Phone != "(415) 555-0134" {
		t.Fatalf("Phone = %q; want Google's number as the fallback", location.Phone)
	}
}
