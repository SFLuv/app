package structs

import "testing"

func TestParseDisplayHoursLiftsStoredGoogleText(t *testing.T) {
	day := ParseDisplayHours(0, "Monday: 7:00 AM – 8:00 PM")
	if day.IsClosed {
		t.Fatal("a day with times is not closed")
	}
	if len(day.Intervals) != 1 || day.Intervals[0].OpenMinute != 7*60 || day.Intervals[0].CloseMinute != 20*60 {
		t.Fatalf("got %+v", day.Intervals)
	}
}

func TestParseDisplayHoursRecognisesClosed(t *testing.T) {
	day := ParseDisplayHours(6, "Sunday: Closed")
	if !day.IsClosed {
		t.Fatal("expected closed")
	}
	if day.HasTimes() {
		t.Fatal("a closed day carries no times")
	}
}

// Unreadable text must not become a guess: "no times recorded" is a distinct,
// honest state, and inventing 12:00–12:00 would publish a wrong opening time.
func TestParseDisplayHoursLeavesUnreadableTextEmpty(t *testing.T) {
	for _, input := range []string{"Monday: by appointment", "Tuesday: 9ish till late", ""} {
		day := ParseDisplayHours(0, input)
		if day.HasTimes() || day.IsClosed {
			t.Fatalf("%q should have produced no times, got %+v", input, day)
		}
	}
}

func TestClockMinuteRoundTrips(t *testing.T) {
	for _, input := range []string{"12:00 AM", "7:30 AM", "12:00 PM", "11:45 PM"} {
		minute, ok := ParseClockMinute(input)
		if !ok {
			t.Fatalf("could not parse %q", input)
		}
		if got := FormatClockMinute(minute); got != input {
			t.Fatalf("%q round-tripped to %q", input, got)
		}
	}
}

func TestParseClockMinuteAccepts24Hour(t *testing.T) {
	minute, ok := ParseClockMinute("19:15")
	if !ok || minute != 19*60+15 {
		t.Fatalf("got %d ok=%v", minute, ok)
	}
}

func TestParseClockMinuteRejectsNonsense(t *testing.T) {
	for _, input := range []string{"25:00", "7:99 AM", "13:00 PM", "noon", ""} {
		if _, ok := ParseClockMinute(input); ok {
			t.Fatalf("%q should not parse", input)
		}
	}
}

// Closing before opening is a day that runs past midnight, which is ordinary
// for bars — it must not be rejected as invalid.
func TestValidateAllowsOvernightHours(t *testing.T) {
	day := LocationDayHours{Weekday: 4, Intervals: []LocationHoursInterval{{OpenMinute: 20 * 60, CloseMinute: 2 * 60}}}
	if err := day.Validate(); err != nil {
		t.Fatalf("overnight hours should be valid: %s", err)
	}
}

// Split hours are the point of the model, and the parser must read the shape
// Google writes them in.
func TestParseDisplayHoursReadsSplitDays(t *testing.T) {
	day := ParseDisplayHours(1, "Tuesday: 11:00 AM – 2:00 PM, 5:00 PM – 9:00 PM")
	if len(day.Intervals) != 2 {
		t.Fatalf("expected two stretches, got %+v", day.Intervals)
	}
	if day.Intervals[0].OpenMinute != 11*60 || day.Intervals[1].CloseMinute != 21*60 {
		t.Fatalf("wrong stretches: %+v", day.Intervals)
	}
	if got := day.Display(); got != "Tuesday: 11:00 AM – 2:00 PM, 5:00 PM – 9:00 PM" {
		t.Fatalf("split day did not round-trip: %q", got)
	}
}

// Half a readable day is worse than none: keeping only the lunch stretch would
// publish a shop as closing at 2pm when it reopens at 5pm.
func TestParseDisplayHoursDropsPartiallyUnreadableDays(t *testing.T) {
	day := ParseDisplayHours(1, "Tuesday: 11:00 AM – 2:00 PM, evenings by arrangement")
	if len(day.Intervals) != 0 {
		t.Fatalf("expected no stretches, got %+v", day.Intervals)
	}
}

func TestValidateRejectsOverlappingStretches(t *testing.T) {
	day := LocationDayHours{Weekday: 1, Intervals: []LocationHoursInterval{
		{OpenMinute: 11 * 60, CloseMinute: 15 * 60},
		{OpenMinute: 14 * 60, CloseMinute: 21 * 60},
	}}
	if err := day.Validate(); err == nil {
		t.Fatal("overlapping stretches must be rejected")
	}
}

func TestValidateAllowsBackToBackStretches(t *testing.T) {
	day := LocationDayHours{Weekday: 1, Intervals: []LocationHoursInterval{
		{OpenMinute: 11 * 60, CloseMinute: 14 * 60},
		{OpenMinute: 17 * 60, CloseMinute: 21 * 60},
	}}
	if err := day.Validate(); err != nil {
		t.Fatalf("a normal split day must be valid: %s", err)
	}
}

// A closed day carrying times is contradictory and would render two ways.
func TestValidateRejectsClosedDayWithTimes(t *testing.T) {
	day := LocationDayHours{Weekday: 1, IsClosed: true, Intervals: []LocationHoursInterval{
		{OpenMinute: 9 * 60, CloseMinute: 17 * 60},
	}}
	if err := day.Validate(); err == nil {
		t.Fatal("a closed day must not carry opening times")
	}
}

func TestDisplayDistinguishesClosedFromUnknown(t *testing.T) {
	closed := LocationDayHours{Weekday: 6, IsClosed: true}.Display()
	unknown := LocationDayHours{Weekday: 6}.Display()
	if closed == unknown {
		t.Fatal("closed and unknown must not render identically")
	}
	if closed != "Sunday: Closed" {
		t.Fatalf("got %q", closed)
	}
}
