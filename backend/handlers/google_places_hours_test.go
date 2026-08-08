package handlers

import "testing"

func point(day, hour, minute int) *googlePlacesTimePoint {
	return &googlePlacesTimePoint{Day: day, Hour: hour, Minute: minute}
}

// Google numbers days from Sunday, our storage from Monday. Getting this wrong
// shifts every merchant's week by one day, which reads as plausible data.
func TestGoogleDayToWeekday(t *testing.T) {
	cases := map[int]int{0: 6, 1: 0, 2: 1, 3: 2, 4: 3, 5: 4, 6: 5}
	for googleDay, want := range cases {
		if got := googleDayToWeekday(googleDay); got != want {
			t.Errorf("google day %d = %d, want %d", googleDay, got, want)
		}
	}
}

func TestStructuredHoursMapsSimpleWeek(t *testing.T) {
	periods := []googlePlacesPeriod{
		{Open: point(1, 9, 0), Close: point(1, 17, 30)},  // Monday
		{Open: point(0, 10, 0), Close: point(0, 16, 0)},  // Sunday
	}
	days, ok := structuredHoursFromPeriods(periods)
	if !ok {
		t.Fatal("expected usable hours")
	}
	if len(days[0].Intervals) != 1 || days[0].Intervals[0].OpenMinute != 9*60 || days[0].Intervals[0].CloseMinute != 17*60+30 {
		t.Fatalf("monday wrong: %+v", days[0])
	}
	if len(days[6].Intervals) != 1 || days[6].Intervals[0].OpenMinute != 10*60 {
		t.Fatalf("sunday wrong: %+v", days[6])
	}
	// A day Google lists no period for is shut.
	if !days[2].IsClosed {
		t.Fatalf("wednesday should be closed: %+v", days[2])
	}
}

// A kitchen that shuts between lunch and dinner keeps both stretches. Widening
// them to 11:00–21:00 would tell customers the shop is open while it is shut.
func TestStructuredHoursKeepsSplitDays(t *testing.T) {
	periods := []googlePlacesPeriod{
		{Open: point(1, 17, 0), Close: point(1, 21, 0)},
		{Open: point(1, 11, 0), Close: point(1, 14, 0)},
	}
	days, ok := structuredHoursFromPeriods(periods)
	if !ok {
		t.Fatal("expected a result")
	}
	if len(days[0].Intervals) != 2 {
		t.Fatalf("both stretches must survive, got %+v", days[0].Intervals)
	}
	// Sorted regardless of the order Google listed them.
	if days[0].Intervals[0].OpenMinute != 11*60 || days[0].Intervals[1].OpenMinute != 17*60 {
		t.Fatalf("stretches out of order: %+v", days[0].Intervals)
	}
	if days[0].IsClosed {
		t.Fatal("a split day is not closed")
	}
}

// Google signals "open 24 hours" with an open and no close. That is not a
// zero-length day and must not be recorded as one.
func TestStructuredHoursHandlesAlwaysOpen(t *testing.T) {
	days, ok := structuredHoursFromPeriods([]googlePlacesPeriod{{Open: point(2, 0, 0)}})
	if !ok {
		t.Fatal("expected a result")
	}
	if days[1].HasTimes() {
		t.Fatal("always-open must not become a zero-length span")
	}
}

// No periods at all means Google told us nothing — the caller must be able to
// tell that apart from "closed all week" so the nightly sync can skip.
func TestStructuredHoursReportsNothingUsable(t *testing.T) {
	if _, ok := structuredHoursFromPeriods(nil); ok {
		t.Fatal("no periods must not report usable hours")
	}
}

func TestStructuredHoursKeepsOvernightClose(t *testing.T) {
	days, ok := structuredHoursFromPeriods([]googlePlacesPeriod{
		{Open: point(5, 20, 0), Close: point(6, 2, 0)},
	})
	if !ok {
		t.Fatal("expected usable hours")
	}
	if len(days[4].Intervals) != 1 || days[4].Intervals[0].CloseMinute != 2*60 {
		t.Fatalf("friday overnight wrong: %+v", days[4])
	}
}
