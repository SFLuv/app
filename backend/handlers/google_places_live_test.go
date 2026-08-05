package handlers

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Live checks against the real Places API. They cost a request each and need a
// working key, so they are opt-in:
//
//	GOOGLE_PLACES_LIVE_TEST=1 GOOGLE_MAPS_SERVER_API_KEY=... \
//	  go test -vet=off ./handlers -run Live -v
//
// The stubbed tests in google_places_test.go cover the same logic against fixed
// payloads; these exist to catch Google changing the shape or the field mask
// being rejected, which a stub can never notice.
func requireLivePlaces(t *testing.T) {
	t.Helper()

	if os.Getenv("GOOGLE_PLACES_LIVE_TEST") == "" {
		t.Skip("GOOGLE_PLACES_LIVE_TEST not set; skipping live Places API tests")
	}
	if !GooglePlacesVerificationEnabled() {
		t.Skip("no Google Places API key configured; skipping live Places API tests")
	}
}

// A real establishment must verify and come back with a business name that is
// not its street address.
func TestLiveVerifyGooglePlaceAcceptsRealBusiness(t *testing.T) {
	requireLivePlaces(t)

	// Sheba Jazz Lounge, San Francisco.
	verified, err := VerifyGooglePlace(context.Background(), "ChIJZQLenLmAhYAR6NpCrelhWCk")
	if err != nil {
		t.Fatalf("VerifyGooglePlace() error = %v; want a verified business", err)
	}

	if verified.Name == "" {
		t.Fatal("Name is empty; want the business name")
	}
	if strings.EqualFold(verified.Name, verified.Street) {
		t.Fatalf("Name %q equals Street %q; the listing resolved to its own address", verified.Name, verified.Street)
	}
	if verified.Lat == 0 || verified.Lng == 0 {
		t.Fatalf("coords = %v,%v; want real coordinates", verified.Lat, verified.Lng)
	}
	if verified.City == "" {
		t.Fatal("City is empty; want the address components to have been parsed")
	}

	t.Logf("verified %q at %q (%v, %v), type %q, %d hours entries",
		verified.Name, verified.Street, verified.Lat, verified.Lng, verified.Type, len(verified.OpeningHours))
}

// The Shiba regression against live data: a geocoded street address must be
// rejected, not accepted as a business named after the address.
func TestLiveVerifyGooglePlaceRejectsRealStreetAddress(t *testing.T) {
	requireLivePlaces(t)

	// Geocoding API place id for "517 Balboa St, San Francisco" — types: [street_address]
	const streetAddressPlaceID = "Eis1MTcgQmFsYm9hIFN0LCBTYW4gRnJhbmNpc2NvLCBDQSA5NDExOCwgVVNBIjESLwoUChIJ0ZCqWD-HhYAR9-gvugBs5uoQhQQqFAoSCcHnr6IOh4WAEaAlTz9_kdm3"

	verified, err := VerifyGooglePlace(context.Background(), streetAddressPlaceID)
	if err == nil {
		t.Fatalf("VerifyGooglePlace() accepted a street address as %q; want a rejection", verified.Name)
	}
	if !IsPlaceVerificationError(err) {
		t.Fatalf("IsPlaceVerificationError(%v) = false; want true so the caller returns 400, not a soft fallback", err)
	}

	t.Logf("correctly rejected: %v", err)
}
