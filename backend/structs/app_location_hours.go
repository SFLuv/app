package structs

import (
	"fmt"
	"sort"
	"strings"
)

// WeekdayNames is the storage order for location hours: index 0 is Monday,
// matching location_hours.weekday. Google's own day numbering starts on Sunday,
// so anything crossing that boundary has to convert.
var WeekdayNames = [7]string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

const MinutesPerDay = 24 * 60

// LocationDayHours is one day's opening times.
//
// Minutes from midnight rather than a clock string: it sorts, compares and
// validates without parsing, and it is what a time input round-trips cleanly.
// Closed and "no times recorded" are distinct states — a merchant who is shut
// on Sunday is not the same as one whose Sunday we simply never learned, and
// collapsing them would let a sync gap read as a closure to customers.
type LocationDayHours struct {
	Weekday  int  `json:"weekday"`
	IsClosed bool `json:"is_closed"`
	// A day can open more than once — a kitchen serving lunch and dinner is the
	// ordinary case, not an edge case. Empty means no times are recorded, which
	// is distinct from IsClosed.
	Intervals []LocationHoursInterval `json:"intervals"`
}

// LocationHoursInterval is one continuous stretch a location is open.
type LocationHoursInterval struct {
	OpenMinute  int `json:"open_minute"`
	CloseMinute int `json:"close_minute"`
}

// HasTimes reports whether the day carries usable opening times.
func (d LocationDayHours) HasTimes() bool {
	return len(d.Intervals) > 0
}

// Display renders the day the way it has always been stored in the `hours`
// column, so map pins, the merchant modal and anything else reading that text
// keep working unchanged.
func (d LocationDayHours) Display() string {
	name := "Unknown"
	if d.Weekday >= 0 && d.Weekday < len(WeekdayNames) {
		name = WeekdayNames[d.Weekday]
	}
	if d.IsClosed {
		return fmt.Sprintf("%s: Closed", name)
	}
	if !d.HasTimes() {
		return fmt.Sprintf("%s: Hours not available", name)
	}
	parts := make([]string, 0, len(d.Intervals))
	for _, interval := range d.Intervals {
		parts = append(parts, fmt.Sprintf("%s – %s",
			FormatClockMinute(interval.OpenMinute), FormatClockMinute(interval.CloseMinute)))
	}
	// Comma-separated, the same way Google writes a split day.
	return fmt.Sprintf("%s: %s", name, strings.Join(parts, ", "))
}

// Validate rejects times that cannot be rendered or ordered.
//
// A close before an open is allowed: it means the day runs past midnight, which
// is ordinary for bars and late kitchens. Equal times are not — that is either a
// mistake or a 24-hour day, and the two need different handling.
func (d LocationDayHours) Validate() error {
	if d.Weekday < 0 || d.Weekday >= len(WeekdayNames) {
		return fmt.Errorf("weekday must be 0 (Monday) through 6 (Sunday)")
	}
	if d.IsClosed {
		if len(d.Intervals) > 0 {
			return fmt.Errorf("%s cannot be closed and have opening times", WeekdayNames[d.Weekday])
		}
		return nil
	}

	for _, interval := range d.Intervals {
		for _, minute := range []int{interval.OpenMinute, interval.CloseMinute} {
			if minute < 0 || minute >= MinutesPerDay {
				return fmt.Errorf("%s has a time outside the day", WeekdayNames[d.Weekday])
			}
		}
		if interval.OpenMinute == interval.CloseMinute {
			return fmt.Errorf("%s opens and closes at the same time", WeekdayNames[d.Weekday])
		}
	}

	// Overlaps are rejected because they cannot be rendered honestly and almost
	// always mean a mistyped second shift. Only same-day spans are compared: a
	// stretch closing after midnight runs into the next day and cannot overlap
	// a later one on this day.
	for i, outer := range d.Intervals {
		if outer.CloseMinute < outer.OpenMinute {
			continue
		}
		for j, inner := range d.Intervals {
			if i == j || inner.CloseMinute < inner.OpenMinute {
				continue
			}
			if outer.OpenMinute < inner.CloseMinute && inner.OpenMinute < outer.CloseMinute {
				return fmt.Errorf("%s has overlapping opening times", WeekdayNames[d.Weekday])
			}
		}
	}

	return nil
}

// SortIntervals puts a day's stretches in clock order so storage, display and
// comparison do not depend on the order a client happened to send.
func (d *LocationDayHours) SortIntervals() {
	sort.SliceStable(d.Intervals, func(i, j int) bool {
		return d.Intervals[i].OpenMinute < d.Intervals[j].OpenMinute
	})
}

// FormatClockMinute renders minutes-from-midnight as a 12-hour clock time,
// matching the format Google returns and the format already stored.
func FormatClockMinute(minute int) string {
	minute = ((minute % MinutesPerDay) + MinutesPerDay) % MinutesPerDay
	hour := minute / 60
	suffix := "AM"
	if hour >= 12 {
		suffix = "PM"
	}
	display := hour % 12
	if display == 0 {
		display = 12
	}
	return fmt.Sprintf("%d:%02d %s", display, minute%60, suffix)
}

// ParseClockMinute reads "7:00 AM", "7:00 PM" or 24-hour "07:00" into minutes
// from midnight. Used to lift the historical free-text hours into structured
// values, and to accept either shape from a client.
func ParseClockMinute(value string) (int, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, false
	}
	// Google uses a narrow no-break space before AM/PM in some locales.
	text = strings.ReplaceAll(text, " ", " ")
	text = strings.ReplaceAll(text, " ", " ")
	upper := strings.ToUpper(text)

	suffix := ""
	for _, candidate := range []string{"AM", "PM"} {
		if strings.HasSuffix(upper, candidate) {
			suffix = candidate
			upper = strings.TrimSpace(strings.TrimSuffix(upper, candidate))
			break
		}
	}

	parts := strings.Split(upper, ":")
	if len(parts) == 0 || len(parts) > 2 {
		return 0, false
	}
	var hour, minute int
	if _, err := fmt.Sscanf(parts[0], "%d", &hour); err != nil {
		return 0, false
	}
	if len(parts) == 2 {
		if _, err := fmt.Sscanf(parts[1], "%d", &minute); err != nil {
			return 0, false
		}
	}
	if minute < 0 || minute > 59 {
		return 0, false
	}

	switch suffix {
	case "AM":
		if hour < 1 || hour > 12 {
			return 0, false
		}
		if hour == 12 {
			hour = 0
		}
	case "PM":
		if hour < 1 || hour > 12 {
			return 0, false
		}
		if hour != 12 {
			hour += 12
		}
	default:
		if hour < 0 || hour > 23 {
			return 0, false
		}
	}

	return hour*60 + minute, true
}

// ParseDisplayHours lifts a stored display string back into structured times.
//
// Only used to backfill rows written before hours were structured, and to
// salvage anything a caller still sends as text. Anything it cannot read comes
// back as "no times recorded" rather than a guess.
func ParseDisplayHours(weekday int, value string) LocationDayHours {
	day := LocationDayHours{Weekday: weekday}
	text := strings.TrimSpace(value)
	if text == "" {
		return day
	}
	if index := strings.Index(text, ":"); index != -1 {
		// Strip the "Monday: " prefix, but only when it really is the weekday
		// label rather than the first colon of a time.
		prefix := strings.TrimSpace(text[:index])
		for _, name := range WeekdayNames {
			if strings.EqualFold(prefix, name) {
				text = strings.TrimSpace(text[index+1:])
				break
			}
		}
	}
	if strings.EqualFold(text, "closed") {
		day.IsClosed = true
		return day
	}

	// Google writes a split day as "11:00 AM – 2:00 PM, 5:00 – 9:00 PM".
	for _, chunk := range strings.Split(text, ",") {
		interval, ok := parseInterval(chunk)
		if !ok {
			// One unreadable stretch makes the whole day untrustworthy: keeping
			// the readable half would publish a day that closes earlier than it
			// really does.
			return LocationDayHours{Weekday: weekday}
		}
		day.Intervals = append(day.Intervals, interval)
	}
	day.SortIntervals()

	return day
}

func parseInterval(text string) (LocationHoursInterval, bool) {
	for _, separator := range []string{"–", "—", " - ", "-", " to ", "至"} {
		open, close, found := strings.Cut(text, separator)
		if !found {
			continue
		}
		openMinute, openOK := ParseClockMinute(open)
		closeMinute, closeOK := ParseClockMinute(close)
		if openOK && closeOK && openMinute != closeMinute {
			return LocationHoursInterval{OpenMinute: openMinute, CloseMinute: closeMinute}, true
		}
	}
	return LocationHoursInterval{}, false
}
