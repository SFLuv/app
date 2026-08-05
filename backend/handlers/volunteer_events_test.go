package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
)

func TestSlugifyTitle(t *testing.T) {
	cases := map[string]string{
		"Ocean Beach Cleanup":            "ocean-beach-cleanup",
		"  Tenderloin  Weekly  Cleanup ": "tenderloin-weekly-cleanup",
		"GLIDE Beautification Day 2026!": "glide-beautification-day-2026",
		"Café / Night of Ideas":          "caf-night-of-ideas",
		"!!!":                            "",
	}

	for title, expected := range cases {
		if got := slugifyTitle(title); got != expected {
			t.Errorf("slugifyTitle(%q) = %q, want %q", title, got, expected)
		}
	}
}

func TestSlugifyTitleIsBounded(t *testing.T) {
	long := ""
	for range 40 {
		long += "volunteer "
	}

	slug := slugifyTitle(long)
	if len(slug) > 80 {
		t.Fatalf("slug length %d exceeds 80", len(slug))
	}
	if slug[len(slug)-1] == '-' {
		t.Fatalf("slug %q must not end in a dash", slug)
	}
}

// The recurrence summary is rendered server-side precisely so the three clients
// cannot each invent their own phrasing, so its exact output is contractual.
func TestBuildRecurrenceSummary(t *testing.T) {
	// 2026-08-06 is the first Thursday of August 2026.
	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	weekOfMonth := 1
	dayOfMonth := 14

	cases := []struct {
		name     string
		row      *db.VolunteerEventRow
		expected string
	}{
		{
			name:     "daily",
			row:      &db.VolunteerEventRow{RecurrenceFrequency: structs.RecurrenceDaily},
			expected: "Every day",
		},
		{
			name:     "weekly uses the start weekday",
			row:      &db.VolunteerEventRow{RecurrenceFrequency: structs.RecurrenceWeekly},
			expected: "Every Thursday",
		},
		{
			name: "monthly by weekday",
			row: &db.VolunteerEventRow{
				RecurrenceFrequency:   structs.RecurrenceMonthly,
				RecurrenceMonthlyMode: structs.MonthlyModeDayOfWeek,
				RecurrenceWeekOfMonth: &weekOfMonth,
			},
			expected: "First Thursday of every month",
		},
		{
			name: "monthly by date",
			row: &db.VolunteerEventRow{
				RecurrenceFrequency:   structs.RecurrenceMonthly,
				RecurrenceMonthlyMode: structs.MonthlyModeDayOfMonth,
				RecurrenceDayOfMonth:  &dayOfMonth,
			},
			expected: "The 14th of every month",
		},
		{
			name:     "no recurrence",
			row:      &db.VolunteerEventRow{RecurrenceFrequency: structs.RecurrenceNone},
			expected: "",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := buildRecurrenceSummary(testCase.row, start); got != testCase.expected {
				t.Fatalf("got %q, want %q", got, testCase.expected)
			}
		})
	}
}

func TestOrdinalSuffix(t *testing.T) {
	cases := map[int]string{1: "st", 2: "nd", 3: "rd", 4: "th", 11: "th", 12: "th", 13: "th", 21: "st", 22: "nd", 23: "rd", 31: "st"}
	for day, expected := range cases {
		if got := ordinalSuffix(day); got != expected {
			t.Errorf("ordinalSuffix(%d) = %q, want %q", day, got, expected)
		}
	}
}

func TestOccurrenceStatus(t *testing.T) {
	now := int64(1_000_000)
	cancelledAt := now - 500

	cases := []struct {
		name     string
		row      *db.VolunteerEventRow
		expected string
	}{
		{
			name:     "future event is scheduled",
			row:      &db.VolunteerEventRow{StartAt: now + 100, Expiration: now + 200, ReviewStatus: structs.EventReviewApproved},
			expected: structs.EventStatusScheduled,
		},
		{
			name:     "in-window event is live",
			row:      &db.VolunteerEventRow{StartAt: now - 100, Expiration: now + 100, ReviewStatus: structs.EventReviewApproved},
			expected: structs.EventStatusLive,
		},
		{
			name:     "past event has ended",
			row:      &db.VolunteerEventRow{StartAt: now - 200, Expiration: now - 100, ReviewStatus: structs.EventReviewApproved},
			expected: structs.EventStatusEnded,
		},
		{
			name:     "cancellation outranks timing",
			row:      &db.VolunteerEventRow{StartAt: now - 100, Expiration: now + 100, ReviewStatus: structs.EventReviewCancelled},
			expected: structs.EventStatusCancelled,
		},
		{
			name:     "cancelled_at alone is enough",
			row:      &db.VolunteerEventRow{StartAt: now + 100, Expiration: now + 200, ReviewStatus: structs.EventReviewApproved, CancelledAt: &cancelledAt},
			expected: structs.EventStatusCancelled,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := occurrenceStatus(testCase.row, now); got != testCase.expected {
				t.Fatalf("got %q, want %q", got, testCase.expected)
			}
		})
	}
}

func TestBuildSignupInfo(t *testing.T) {
	t.Run("internal event with spots is open", func(t *testing.T) {
		info := buildSignupInfo(&db.VolunteerEventRow{
			SignupMode: structs.SignupModeInternal, MaxParticipants: 40, SignupCount: 12,
		}, structs.EventStatusScheduled)

		if !info.Open || info.ClosedReason != nil {
			t.Fatalf("expected open signup, got %+v", info)
		}
	})

	t.Run("full internal event closes with reason full", func(t *testing.T) {
		info := buildSignupInfo(&db.VolunteerEventRow{
			SignupMode: structs.SignupModeInternal, MaxParticipants: 40, SignupCount: 40,
		}, structs.EventStatusScheduled)

		if info.Open || info.ClosedReason == nil || *info.ClosedReason != structs.SignupClosedFull {
			t.Fatalf("expected closed/full, got %+v", info)
		}
	})

	t.Run("cancelled event is never signup-able", func(t *testing.T) {
		info := buildSignupInfo(&db.VolunteerEventRow{
			SignupMode: structs.SignupModeInternal, MaxParticipants: 40,
		}, structs.EventStatusCancelled)

		if info.Open || *info.ClosedReason != structs.SignupClosedCancelled {
			t.Fatalf("expected closed/cancelled, got %+v", info)
		}
	})

	t.Run("ended event is closed", func(t *testing.T) {
		info := buildSignupInfo(&db.VolunteerEventRow{
			SignupMode: structs.SignupModeExternal, SignupURL: "https://example.org",
		}, structs.EventStatusEnded)

		if info.Open || *info.ClosedReason != structs.SignupClosedEnded {
			t.Fatalf("expected closed/ended, got %+v", info)
		}
	})

	t.Run("external event exposes its url", func(t *testing.T) {
		info := buildSignupInfo(&db.VolunteerEventRow{
			SignupMode: structs.SignupModeExternal, SignupURL: "https://example.org",
		}, structs.EventStatusScheduled)

		if info.URL == nil || *info.URL != "https://example.org" {
			t.Fatalf("expected external url, got %+v", info.URL)
		}
	})

	// A "none" event has nothing to sign up for, so it must not advertise an
	// open signup that clients would render a button for.
	t.Run("mode none is never open", func(t *testing.T) {
		info := buildSignupInfo(&db.VolunteerEventRow{SignupMode: structs.SignupModeNone}, structs.EventStatusScheduled)
		if info.Open {
			t.Fatalf("expected mode none to be closed, got %+v", info)
		}
	})
}

func TestDecodeDataURL(t *testing.T) {
	// 1x1 transparent GIF.
	encoded := "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"

	data, contentType, ok := decodeDataURL(encoded)
	if !ok {
		t.Fatal("expected data url to decode")
	}
	if contentType != "image/gif" {
		t.Fatalf("content type = %q, want image/gif", contentType)
	}
	if len(data) == 0 {
		t.Fatal("expected decoded bytes")
	}

	for _, invalid := range []string{"", "https://example.org/logo.png", "data:image/png,notbase64", "data:image/png;base64,!!!"} {
		if _, _, ok := decodeDataURL(invalid); ok {
			t.Errorf("expected %q to be rejected", invalid)
		}
	}
}

func TestParseTimeParamAcceptsBothWireFormats(t *testing.T) {
	rfc, ok := parseTimeParam("2026-08-06T09:00:00Z")
	if !ok || rfc != time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC).Unix() {
		t.Fatalf("rfc3339 parse failed: %d %v", rfc, ok)
	}

	unix, ok := parseTimeParam("1754470800")
	if !ok || unix != 1754470800 {
		t.Fatalf("unix parse failed: %d %v", unix, ok)
	}

	if _, ok := parseTimeParam("not-a-time"); ok {
		t.Fatal("expected garbage to be rejected")
	}
	if _, ok := parseTimeParam(""); ok {
		t.Fatal("expected empty to be rejected")
	}
}

// The volunteer flag must default to false when unset: an old client build or a
// backend that has not deployed the endpoints yet must never light up a tab
// that would 404.
func TestServerFeatureFlagsDefaultOff(t *testing.T) {
	t.Setenv("VOLUNTEER_EVENTS_ENABLED", "")

	merged := withServerFeatureFlags([]byte(`{"community":{"alias":"sfluv"},"features":{"sends_enabled":true}}`))

	var config map[string]any
	if err := json.Unmarshal(merged, &config); err != nil {
		t.Fatalf("merged config is not valid json: %s", err)
	}

	features, ok := config["features"].(map[string]any)
	if !ok {
		t.Fatal("features block missing")
	}
	if features["volunteer_events_enabled"] != false {
		t.Fatalf("expected flag to default false, got %v", features["volunteer_events_enabled"])
	}
	// The overlay must preserve, not replace, upstream feature flags.
	if features["sends_enabled"] != true {
		t.Fatal("overlay clobbered an upstream feature flag")
	}
	if config["community"] == nil {
		t.Fatal("overlay dropped non-feature config")
	}
}

func TestServerFeatureFlagsEnabled(t *testing.T) {
	t.Setenv("VOLUNTEER_EVENTS_ENABLED", "true")

	merged := withServerFeatureFlags([]byte(`{"features":{}}`))

	var config map[string]any
	_ = json.Unmarshal(merged, &config)
	features := config["features"].(map[string]any)
	if features["volunteer_events_enabled"] != true {
		t.Fatalf("expected flag true, got %v", features["volunteer_events_enabled"])
	}
}

// A malformed config must be passed through untouched rather than breaking
// /config, which every client polls at startup.
func TestServerFeatureFlagsPassesThroughInvalidJSON(t *testing.T) {
	raw := []byte(`not json at all`)
	if got := string(withServerFeatureFlags(raw)); got != string(raw) {
		t.Fatalf("expected raw passthrough, got %q", got)
	}
}
