package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsBusinessPlace(t *testing.T) {
	tests := []struct {
		name  string
		types []string
		want  bool
	}{
		{
			name:  "restaurant is a business",
			types: []string{"restaurant", "food", "point_of_interest", "establishment"},
			want:  true,
		},
		{
			name:  "street address is not a business",
			types: []string{"street_address"},
			want:  false,
		},
		{
			name:  "premise with political parts is not a business",
			types: []string{"premise", "subpremise", "geocode", "political"},
			want:  false,
		},
		{
			name:  "no types at all is not a business",
			types: nil,
			want:  false,
		},
		{
			name:  "one business type among address types is enough",
			types: []string{"geocode", "premise", "cafe"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBusinessPlace(tt.types); got != tt.want {
				t.Fatalf("isBusinessPlace(%v) = %v; want %v", tt.types, got, tt.want)
			}
		})
	}
}

func TestAddressComponent(t *testing.T) {
	components := []googlePlacesAddressComponent{
		{LongText: "1234", ShortText: "1234", Types: []string{"street_number"}},
		{LongText: "Market Street", ShortText: "Market St", Types: []string{"route"}},
		{LongText: "San Francisco", Types: []string{"locality", "political"}},
	}

	if got := addressComponent(components, "route"); got != "Market Street" {
		t.Fatalf("addressComponent(route) = %q; want %q", got, "Market Street")
	}
	if got := addressComponent(components, "locality"); got != "San Francisco" {
		t.Fatalf("addressComponent(locality) = %q; want %q", got, "San Francisco")
	}
	if got := addressComponent(components, "postal_code"); got != "" {
		t.Fatalf("addressComponent(postal_code) = %q; want empty", got)
	}
}

// newStubPlacesServer points the Places client at a local server returning body.
func newStubPlacesServer(t *testing.T, status int, body any) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-Api-Key") == "" {
			t.Errorf("places request missing X-Goog-Api-Key header")
		}
		if r.Header.Get("X-Goog-FieldMask") == "" {
			t.Errorf("places request missing X-Goog-FieldMask header")
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)

	original := googlePlacesDetailsBaseURL
	googlePlacesDetailsBaseURL = server.URL + "/v1/places/"
	t.Cleanup(func() { googlePlacesDetailsBaseURL = original })

	t.Setenv("GOOGLE_MAPS_SERVER_API_KEY", "test-key")
}

func TestVerifyGooglePlaceAcceptsBusiness(t *testing.T) {
	newStubPlacesServer(t, http.StatusOK, map[string]any{
		"id":                     "ChIJshiba",
		"types":                  []string{"restaurant", "food", "establishment"},
		"displayName":            map[string]string{"text": "Shiba SF"},
		"primaryTypeDisplayName": map[string]string{"text": "Restaurant"},
		"shortFormattedAddress":  "517 Balboa St, San Francisco",
		"addressComponents": []map[string]any{
			{"longText": "517", "types": []string{"street_number"}},
			{"longText": "Balboa Street", "types": []string{"route"}},
			{"longText": "San Francisco", "types": []string{"locality"}},
			{"longText": "California", "types": []string{"administrative_area_level_1"}},
			{"longText": "94118", "types": []string{"postal_code"}},
		},
		"location":            map[string]float64{"latitude": 37.7766, "longitude": -122.4646},
		"rating":              4.6,
		"nationalPhoneNumber": "(415) 555-0134",
		"websiteUri":          "https://shiba.example",
		"googleMapsUri":       "https://maps.google.com/?cid=1",
		"regularOpeningHours": map[string]any{
			"weekdayDescriptions": []string{"Monday: 5:00 – 10:00 PM", "Tuesday: Closed"},
		},
		"businessStatus": "OPERATIONAL",
	})

	verified, err := VerifyGooglePlace(context.Background(), "ChIJshiba")
	if err != nil {
		t.Fatalf("VerifyGooglePlace() error = %v", err)
	}

	if verified.Name != "Shiba SF" {
		t.Fatalf("Name = %q; want %q", verified.Name, "Shiba SF")
	}
	if verified.Street != "517 Balboa Street" {
		t.Fatalf("Street = %q; want %q", verified.Street, "517 Balboa Street")
	}
	if verified.City != "San Francisco" || verified.State != "California" || verified.ZIP != "94118" {
		t.Fatalf("address = %q/%q/%q; want San Francisco/California/94118", verified.City, verified.State, verified.ZIP)
	}
	if verified.Lat != 37.7766 || verified.Lng != -122.4646 {
		t.Fatalf("coords = %v,%v; want 37.7766,-122.4646", verified.Lat, verified.Lng)
	}
	if len(verified.OpeningHours) != 2 {
		t.Fatalf("OpeningHours length = %d; want 2", len(verified.OpeningHours))
	}
}

// The Shiba regression: Google returns an address-shaped place whose display
// name IS the street address. Verification must reject it rather than let a
// merchant be created named "517 Balboa St".
func TestVerifyGooglePlaceRejectsStreetAddress(t *testing.T) {
	newStubPlacesServer(t, http.StatusOK, map[string]any{
		"id":          "ChIJaddress",
		"types":       []string{"street_address", "geocode"},
		"displayName": map[string]string{"text": "517 Balboa St"},
		"addressComponents": []map[string]any{
			{"longText": "517", "types": []string{"street_number"}},
			{"longText": "Balboa Street", "types": []string{"route"}},
		},
		"location": map[string]float64{"latitude": 37.7766, "longitude": -122.4646},
	})

	_, err := VerifyGooglePlace(context.Background(), "ChIJaddress")
	if err == nil {
		t.Fatal("VerifyGooglePlace() error = nil; want a rejection for a street address")
	}
	if !IsPlaceVerificationError(err) {
		t.Fatalf("IsPlaceVerificationError(%v) = false; want true", err)
	}
	if !strings.Contains(err.Error(), "street address") {
		t.Fatalf("error = %q; want it to mention a street address", err.Error())
	}
}

func TestVerifyGooglePlaceRejectsMissingCoordinates(t *testing.T) {
	newStubPlacesServer(t, http.StatusOK, map[string]any{
		"id":          "ChIJnocoords",
		"types":       []string{"restaurant"},
		"displayName": map[string]string{"text": "Shiba SF"},
	})

	_, err := VerifyGooglePlace(context.Background(), "ChIJnocoords")
	if !IsPlaceVerificationError(err) {
		t.Fatalf("IsPlaceVerificationError(%v) = false; want true", err)
	}
}

func TestVerifyGooglePlaceRejectsPermanentlyClosed(t *testing.T) {
	newStubPlacesServer(t, http.StatusOK, map[string]any{
		"id":             "ChIJclosed",
		"types":          []string{"restaurant"},
		"displayName":    map[string]string{"text": "Closed Cafe"},
		"location":       map[string]float64{"latitude": 37.7766, "longitude": -122.4646},
		"businessStatus": "CLOSED_PERMANENTLY",
	})

	_, err := VerifyGooglePlace(context.Background(), "ChIJclosed")
	if !IsPlaceVerificationError(err) {
		t.Fatalf("IsPlaceVerificationError(%v) = false; want true", err)
	}
}

func TestVerifyGooglePlaceUnknownIDIsRejection(t *testing.T) {
	newStubPlacesServer(t, http.StatusNotFound, map[string]any{"error": "not found"})

	_, err := VerifyGooglePlace(context.Background(), "ChIJgone")
	if !IsPlaceVerificationError(err) {
		t.Fatalf("IsPlaceVerificationError(%v) = false; want true", err)
	}
}

// A Places outage must be a soft failure so onboarding is not blocked by it.
func TestVerifyGooglePlaceServerErrorIsNotARejection(t *testing.T) {
	newStubPlacesServer(t, http.StatusInternalServerError, map[string]any{"error": "boom"})

	_, err := VerifyGooglePlace(context.Background(), "ChIJboom")
	if err == nil {
		t.Fatal("VerifyGooglePlace() error = nil; want a transport error")
	}
	if IsPlaceVerificationError(err) {
		t.Fatalf("IsPlaceVerificationError(%v) = true; want false so the caller falls back", err)
	}
}

func TestVerifyGooglePlaceRequiresPlaceID(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_SERVER_API_KEY", "test-key")

	_, err := VerifyGooglePlace(context.Background(), "   ")
	if !IsPlaceVerificationError(err) {
		t.Fatalf("IsPlaceVerificationError(%v) = false; want true", err)
	}
}

func TestGooglePlacesVerificationEnabled(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_SERVER_API_KEY", "")
	t.Setenv("GOOGLE_MAPS_API_KEY", "")
	t.Setenv("NEXT_PUBLIC_GOOGLE_MAPS_API_KEY", "")
	if GooglePlacesVerificationEnabled() {
		t.Fatal("GooglePlacesVerificationEnabled() = true with no key configured; want false")
	}

	t.Setenv("GOOGLE_MAPS_API_KEY", "fallback-key")
	if !GooglePlacesVerificationEnabled() {
		t.Fatal("GooglePlacesVerificationEnabled() = false with the public key set; want true")
	}
}
