package handlers

import (
	"testing"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
)

// The verification WEB asked for in comms: an admin picking 1:00 PM local must
// store 20:00Z in summer, not 13:00Z. Stamping the wall clock straight in as
// UTC would look correct in the database and in every client — all three render
// whatever instant they are given — while shifting every event by the UTC
// offset.
func TestParseLocalWallClockConvertsFromEventTimezone(t *testing.T) {
	// 2026-08-06 is PDT (UTC-7).
	got, err := parseLocalWallClock("2026-08-06T13:00:00", "America/Los_Angeles")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	want := time.Date(2026, 8, 6, 20, 0, 0, 0, time.UTC).Unix()
	if got != want {
		t.Fatalf("1pm PDT = %s, want %s", time.Unix(got, 0).UTC(), time.Unix(want, 0).UTC())
	}
}

// The same wall clock in winter is an hour further from UTC (PST, UTC-8). A
// fixed offset would get exactly one of these two cases wrong.
func TestParseLocalWallClockHandlesDSTBothDirections(t *testing.T) {
	summer, err := parseLocalWallClock("2026-08-06T09:00:00", "America/Los_Angeles")
	if err != nil {
		t.Fatalf("summer parse failed: %s", err)
	}
	winter, err := parseLocalWallClock("2026-12-03T09:00:00", "America/Los_Angeles")
	if err != nil {
		t.Fatalf("winter parse failed: %s", err)
	}

	if hour := time.Unix(summer, 0).UTC().Hour(); hour != 16 {
		t.Errorf("9am PDT should be 16:00Z, got %02d:00Z", hour)
	}
	if hour := time.Unix(winter, 0).UTC().Hour(); hour != 17 {
		t.Errorf("9am PST should be 17:00Z, got %02d:00Z", hour)
	}
}

func TestParseLocalWallClockAcceptedForms(t *testing.T) {
	for _, value := range []string{
		"2026-08-06T13:00:00",
		"2026-08-06T13:00",
		"2026-08-06 13:00:00",
		"2026-08-06 13:00",
	} {
		if _, err := parseLocalWallClock(value, "America/Los_Angeles"); err != nil {
			t.Errorf("expected %q to parse, got %s", value, err)
		}
	}
}

// A value carrying an offset means the caller already did the conversion, which
// is precisely the mistake this endpoint exists to prevent. Rejecting it is
// better than silently accepting a second interpretation of the same field.
func TestParseLocalWallClockRejectsOffsets(t *testing.T) {
	for _, value := range []string{
		"2026-08-06T13:00:00Z",
		"2026-08-06T13:00:00+07:00",
	} {
		if _, err := parseLocalWallClock(value, "America/Los_Angeles"); err == nil {
			t.Errorf("expected %q to be rejected as pre-converted", value)
		}
	}

	if _, err := parseLocalWallClock("2026-08-06T13:00:00", "Not/AZone"); err == nil {
		t.Error("expected an unknown timezone to be rejected")
	}
	if _, err := parseLocalWallClock("", "America/Los_Angeles"); err == nil {
		t.Error("expected an empty time to be rejected")
	}
}

func TestValidateVolunteerEventRequest(t *testing.T) {
	base := func() *structs.VolunteerEventCreateRequest {
		return &structs.VolunteerEventCreateRequest{
			Title:             "Ocean Beach Cleanup",
			StartAtLocal:      "2026-08-06T09:00:00",
			EndAtLocal:        "2026-08-06T12:00:00",
			Timezone:          "America/Los_Angeles",
			MaxParticipants:   40,
			RewardAmountSfluv: 15,
			SignupMode:        structs.SignupModeInternal,
		}
	}

	t.Run("valid request passes", func(t *testing.T) {
		if _, _, _, errMsg := validateVolunteerEventRequest(base()); errMsg != "" {
			t.Fatalf("unexpected rejection: %s", errMsg)
		}
	})

	t.Run("end must be after start", func(t *testing.T) {
		req := base()
		req.EndAtLocal = "2026-08-06T08:00:00"
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg == "" {
			t.Fatal("expected an end-before-start rejection")
		}
	})

	// max_participants is the number of QR codes minted, so an unbounded value
	// is unbounded faucet exposure.
	t.Run("participants must be bounded", func(t *testing.T) {
		req := base()
		req.MaxParticipants = 0
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg == "" {
			t.Error("expected zero participants to be rejected")
		}
		req.MaxParticipants = 999999
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg == "" {
			t.Error("expected an absurd participant count to be rejected")
		}
	})

	t.Run("external signup requires a safe link", func(t *testing.T) {
		req := base()
		req.SignupMode = structs.SignupModeExternal
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg == "" {
			t.Error("expected a missing external link to be rejected")
		}

		req.SignupURL = "javascript:alert(1)"
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg == "" {
			t.Error("expected a javascript: signup link to be rejected")
		}

		req.SignupURL = "https://partner.org/signup"
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg != "" {
			t.Errorf("expected a valid external link to pass, got %s", errMsg)
		}
	})

	// A signup URL on a non-external event would be dead data that clients
	// might still render, so it is cleared rather than stored.
	t.Run("signup url cleared for non-external modes", func(t *testing.T) {
		req := base()
		req.SignupURL = "https://partner.org/signup"
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg != "" {
			t.Fatalf("unexpected rejection: %s", errMsg)
		}
		if req.SignupURL != "" {
			t.Errorf("expected signup url to be cleared for internal mode, got %q", req.SignupURL)
		}
	})

	t.Run("unknown signup mode rejected", func(t *testing.T) {
		req := base()
		req.SignupMode = "carrier-pigeon"
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg == "" {
			t.Error("expected an unknown signup mode to be rejected")
		}
	})

	t.Run("recurrence none is normalized away", func(t *testing.T) {
		req := base()
		req.Recurrence = &structs.VolunteerEventRecurrenceInput{Frequency: structs.RecurrenceNone}
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg != "" {
			t.Fatalf("unexpected rejection: %s", errMsg)
		}
		if req.Recurrence != nil {
			t.Error("expected a 'none' recurrence to be normalized to nil")
		}
	})

	t.Run("repeat-until must follow the first event", func(t *testing.T) {
		req := base()
		earlier := "2026-08-01T09:00:00"
		req.Recurrence = &structs.VolunteerEventRecurrenceInput{
			Frequency:  structs.RecurrenceWeekly,
			UntilLocal: &earlier,
		}
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg == "" {
			t.Error("expected a repeat-until before the start to be rejected")
		}
	})

	t.Run("monthly defaults to day_of_month", func(t *testing.T) {
		req := base()
		req.Recurrence = &structs.VolunteerEventRecurrenceInput{Frequency: structs.RecurrenceMonthly}
		if _, _, _, errMsg := validateVolunteerEventRequest(req); errMsg != "" {
			t.Fatalf("unexpected rejection: %s", errMsg)
		}
		if req.Recurrence.MonthlyMode != structs.MonthlyModeDayOfMonth {
			t.Errorf("monthly mode = %q, want %q", req.Recurrence.MonthlyMode, structs.MonthlyModeDayOfMonth)
		}
	})
}

// Recurrence must re-anchor to the same LOCAL wall-clock time. Advancing in UTC
// would shift every occurrence by an hour once a series crosses a DST boundary.
func TestNextVolunteerOccurrenceHoldsLocalTimeAcrossDST(t *testing.T) {
	location, _ := time.LoadLocation("America/Los_Angeles")

	// Weekly 9am, one week before the autumn DST change (2026-11-01 in the US).
	start := time.Date(2026, 10, 29, 9, 0, 0, 0, location)
	row := &db.VolunteerEventRow{
		Timezone:            "America/Los_Angeles",
		StartAt:             start.Unix(),
		Expiration:          start.Add(3 * time.Hour).Unix(),
		RecurrenceFrequency: structs.RecurrenceWeekly,
	}

	nextStart, nextEnd, ok := nextVolunteerOccurrence(row)
	if !ok {
		t.Fatal("expected a next occurrence")
	}

	got := time.Unix(nextStart, 0).In(location)
	if got.Hour() != 9 {
		t.Errorf("next occurrence is at %02d:00 local, want 09:00 — DST shifted the series", got.Hour())
	}
	if got.Day() != 5 || got.Month() != time.November {
		t.Errorf("next occurrence = %s, want Nov 5", got.Format("Jan 2"))
	}
	// Duration must survive the transition too.
	if nextEnd-nextStart != int64((3 * time.Hour).Seconds()) {
		t.Errorf("occurrence length changed across DST: %ds", nextEnd-nextStart)
	}
}

// A day-of-month that does not exist in the target month must clamp to the last
// day rather than rolling into the following month, which is what a naive
// AddDate(0,1,0) does (Jan 31 + 1 month = Mar 3).
func TestNextMonthlyDateClampsShortMonths(t *testing.T) {
	location, _ := time.LoadLocation("America/Los_Angeles")
	start := time.Date(2026, 1, 31, 10, 0, 0, 0, location)
	day := 31
	row := &db.VolunteerEventRow{
		Timezone:              "America/Los_Angeles",
		StartAt:               start.Unix(),
		Expiration:            start.Add(time.Hour).Unix(),
		RecurrenceFrequency:   structs.RecurrenceMonthly,
		RecurrenceMonthlyMode: structs.MonthlyModeDayOfMonth,
		RecurrenceDayOfMonth:  &day,
	}

	nextStart, _, ok := nextVolunteerOccurrence(row)
	if !ok {
		t.Fatal("expected a next occurrence")
	}

	got := time.Unix(nextStart, 0).In(location)
	if got.Month() != time.February || got.Day() != 28 {
		t.Fatalf("Jan 31 + 1 month = %s, want Feb 28", got.Format("Jan 2"))
	}
}

// "First Thursday of every month" must land on the first Thursday, not on the
// same date, and not on a day in the wrong month.
func TestNextMonthlyWeekdayOrdinals(t *testing.T) {
	location, _ := time.LoadLocation("America/Los_Angeles")
	// 2026-08-06 is the first Thursday of August 2026.
	start := time.Date(2026, 8, 6, 9, 0, 0, 0, location)
	thursday := int(time.Thursday)

	first := 1
	row := &db.VolunteerEventRow{
		Timezone:              "America/Los_Angeles",
		StartAt:               start.Unix(),
		Expiration:            start.Add(time.Hour).Unix(),
		RecurrenceFrequency:   structs.RecurrenceMonthly,
		RecurrenceMonthlyMode: structs.MonthlyModeDayOfWeek,
		RecurrenceWeekday:     &thursday,
		RecurrenceWeekOfMonth: &first,
	}

	nextStart, _, ok := nextVolunteerOccurrence(row)
	if !ok {
		t.Fatal("expected a next occurrence")
	}
	got := time.Unix(nextStart, 0).In(location)
	if got.Weekday() != time.Thursday || got.Month() != time.September || got.Day() != 3 {
		t.Fatalf("first Thursday after Aug 6 = %s, want Thu Sep 3", got.Format("Mon Jan 2"))
	}

	last := -1
	row.RecurrenceWeekOfMonth = &last
	nextStart, _, ok = nextVolunteerOccurrence(row)
	if !ok {
		t.Fatal("expected a next occurrence for 'last'")
	}
	got = time.Unix(nextStart, 0).In(location)
	if got.Weekday() != time.Thursday || got.Month() != time.September || got.Day() != 24 {
		t.Fatalf("last Thursday of September = %s, want Thu Sep 24", got.Format("Mon Jan 2"))
	}
}

// A series past its end date must stop generating.
func TestNextVolunteerOccurrenceRespectsUntil(t *testing.T) {
	location, _ := time.LoadLocation("America/Los_Angeles")
	start := time.Date(2026, 8, 6, 9, 0, 0, 0, location)
	until := start.AddDate(0, 0, 3).Unix()

	row := &db.VolunteerEventRow{
		Timezone:            "America/Los_Angeles",
		StartAt:             start.Unix(),
		Expiration:          start.Add(time.Hour).Unix(),
		RecurrenceFrequency: structs.RecurrenceWeekly,
		RecurrenceUntil:     &until,
	}

	if _, _, ok := nextVolunteerOccurrence(row); ok {
		t.Fatal("expected generation to stop past recurrence_until")
	}
}

func TestNextVolunteerOccurrenceIgnoresNonRecurring(t *testing.T) {
	row := &db.VolunteerEventRow{Timezone: "UTC", RecurrenceFrequency: structs.RecurrenceNone}
	if _, _, ok := nextVolunteerOccurrence(row); ok {
		t.Fatal("a non-recurring event must not generate a successor")
	}
}
