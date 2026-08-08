package handlers

import (
	"testing"
	"time"
)

// The job is pinned to midnight Pacific rather than a 24h ticker. A ticker
// drifts to whenever the process last booted, and the server's own zone is not
// the merchants' zone.
func TestNextMidnightIsTheNextLocalMidnight(t *testing.T) {
	now := time.Date(2026, 8, 8, 15, 4, 5, 0, hoursSyncLocation())
	got := nextMidnight(now)
	want := time.Date(2026, 8, 9, 0, 0, 0, 0, hoursSyncLocation())
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// Exactly midnight must schedule the NEXT one, not fire in a zero-length loop.
func TestNextMidnightAtMidnightAdvancesADay(t *testing.T) {
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, hoursSyncLocation())
	got := nextMidnight(now)
	want := time.Date(2026, 8, 9, 0, 0, 0, 0, hoursSyncLocation())
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
	if !got.After(now) {
		t.Fatal("next midnight must be strictly in the future")
	}
}

func TestNextMidnightIsAlwaysWithinADay(t *testing.T) {
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, hoursSyncLocation())
	for offset := 0; offset < 72; offset++ {
		now := start.Add(time.Duration(offset) * time.Hour).Add(37 * time.Minute)
		wait := time.Until(now)
		_ = wait
		next := nextMidnight(now)
		if !next.After(now) {
			t.Fatalf("at %s next midnight %s is not in the future", now, next)
		}
		if next.Sub(now) > 24*time.Hour {
			t.Fatalf("at %s waited %s, which is more than a day", now, next.Sub(now))
		}
	}
}

// A server in another zone must still fire at midnight Pacific, not at its own
// midnight — that is the whole point of naming the zone.
func TestNextMidnightIsPacificRegardlessOfServerZone(t *testing.T) {
	// 09:00 UTC on 8 Aug 2026 is 02:00 Pacific, so the next Pacific midnight is
	// the start of the 9th, Pacific.
	now := time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)
	got := nextMidnight(now)

	if got.Hour() != 0 || got.Minute() != 0 {
		t.Fatalf("got %s, which is not midnight in its own zone", got)
	}
	inPacific := got.In(hoursSyncLocation())
	if inPacific.Hour() != 0 || inPacific.Day() != 9 || inPacific.Month() != time.August {
		t.Fatalf("got %s (%s Pacific), want 9 Aug 00:00 Pacific", got, inPacific)
	}
}

// Across the PST/PDT boundary the gap between runs is 23 or 25 hours, and each
// must still land on midnight. A fixed offset would drift by an hour instead.
func TestNextMidnightHoldsAcrossDaylightSaving(t *testing.T) {
	zone := hoursSyncLocation()
	// US DST begins 8 March 2026 and ends 1 November 2026.
	for _, day := range []time.Time{
		time.Date(2026, 3, 7, 12, 0, 0, 0, zone),
		time.Date(2026, 3, 8, 12, 0, 0, 0, zone),
		time.Date(2026, 10, 31, 12, 0, 0, 0, zone),
		time.Date(2026, 11, 1, 12, 0, 0, 0, zone),
	} {
		next := nextMidnight(day)
		if next.Hour() != 0 || next.Minute() != 0 {
			t.Fatalf("from %s got %s, which is not midnight Pacific", day, next)
		}
		if !next.After(day) {
			t.Fatalf("from %s got %s, which is not in the future", day, next)
		}
	}
}
