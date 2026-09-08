package structs

import "time"

// The QR redemption window, as a rule rather than a pair of instants.
//
// A recurring series repeats a rule. Storing only the instants meant every
// occurrence after the first inherited the first one's window, so codes went
// live and expired against a date the event no longer happened on. These
// offsets are stored instead, and each occurrence resolves its own window from
// its own start and end.
//
// Both are nil for the default, which is deliberately NOT expressible as an
// offset: midnight local on the day the event starts, until midnight local on
// the day after it ends. How many hours that is depends on the time of day the
// event runs and on whether a DST boundary falls inside the window, so it has
// to be computed per occurrence in the event's own timezone. Nil and zero mean
// different things here — zero is "exactly at the start", nil is "the default".
type QRWindowRule struct {
	// LiveOffsetHours is how many hours BEFORE the start codes become
	// redeemable. Positive opens the window early; negative would open it after
	// the event begins, which validation refuses.
	LiveOffsetHours *int
	// ExpiryOffsetHours is how many hours AFTER the end codes stop being
	// redeemable.
	ExpiryOffsetHours *int
}

// ResolveQRWindow turns the rule into the two instants this occurrence stores.
//
// startAt and endAt are the occurrence's own, so a successor computed from this
// lands on its own dates. An unknown timezone falls back to UTC rather than
// failing: a window an hour or two off is recoverable, and refusing to create
// the event is not.
func (r QRWindowRule) ResolveQRWindow(startAt int64, endAt int64, timezone string) (liveAt int64, expiresAt int64) {
	location, err := time.LoadLocation(timezone)
	if err != nil || location == nil {
		location = time.UTC
	}

	if r.LiveOffsetHours != nil {
		liveAt = startAt - int64(*r.LiveOffsetHours)*3600
	} else {
		liveAt = startOfLocalDay(startAt, location).Unix()
	}

	if r.ExpiryOffsetHours != nil {
		expiresAt = endAt + int64(*r.ExpiryOffsetHours)*3600
	} else {
		// Midnight ending the event's last day, i.e. the start of the next one.
		expiresAt = startOfLocalDay(endAt, location).AddDate(0, 0, 1).Unix()
	}

	// A window that closes before it opens would make every code dead on
	// arrival. Cannot happen on the defaults; reachable by setting a large
	// negative expiry offset on a short event.
	if expiresAt < liveAt {
		expiresAt = liveAt
	}
	return liveAt, expiresAt
}

// startOfLocalDay is midnight at the top of the day the instant falls on, in
// the given zone.
//
// Built from the calendar date rather than by truncating the unix value, so a
// zone whose offset is not a whole number of hours still lands on midnight.
// Returned as a time.Time so callers can AddDate, which is calendar-aware where
// adding 86400 is not — a day across a DST change is 23 or 25 hours long.
func startOfLocalDay(unix int64, location *time.Location) time.Time {
	local := time.Unix(unix, 0).In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}
