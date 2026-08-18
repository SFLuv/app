package handlers

import (
	"testing"
	"time"
)

func TestMerchantDayIsMidnightToMidnightLocal(t *testing.T) {
	// 11pm Pacific on the 11th is still the 11th's takings, even though it is
	// already the 12th in UTC.
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	now := time.Date(2026, time.August, 11, 23, 0, 0, 0, pacific)

	start, end, date := merchantDayBounds(now, "America/Los_Angeles")

	if date != "2026-08-11" {
		t.Fatalf("business date = %s; want 2026-08-11", date)
	}
	if got := time.Unix(start, 0).In(pacific); got.Hour() != 0 || got.Day() != 11 {
		t.Fatalf("start = %s; want local midnight on the 11th", got)
	}
	if got := time.Unix(end, 0).In(pacific); got.Hour() != 0 || got.Day() != 12 {
		t.Fatalf("end = %s; want local midnight on the 12th", got)
	}
}

// A day is not always 86400 seconds. Adding 24h across the spring-forward
// boundary would end the business day an hour early and drop the last hour of
// trade from the total.
func TestMerchantDaySurvivesDaylightSaving(t *testing.T) {
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	// 8 March 2026 is a spring-forward day in the US: 23 hours long.
	now := time.Date(2026, time.March, 8, 12, 0, 0, 0, pacific)

	start, end, date := merchantDayBounds(now, "America/Los_Angeles")

	if date != "2026-03-08" {
		t.Fatalf("business date = %s; want 2026-03-08", date)
	}
	if span := end - start; span != 23*3600 {
		t.Fatalf("day span = %ds; want 82800s (23h) on a spring-forward day", span)
	}
	if got := time.Unix(end, 0).In(pacific); got.Day() != 9 || got.Hour() != 0 {
		t.Fatalf("end = %s; want local midnight on the 9th", got)
	}
}

// An unknown zone must not silently produce a day in the wrong place; falling
// back to UTC is at least explicit and reported back in the payload.
func TestMerchantDayFallsBackToUTCOnAnUnknownZone(t *testing.T) {
	now := time.Date(2026, time.August, 11, 23, 0, 0, 0, time.UTC)

	start, end, date := merchantDayBounds(now, "Not/AZone")

	if date != "2026-08-11" {
		t.Fatalf("business date = %s; want the UTC day", date)
	}
	if end-start != 24*3600 {
		t.Fatalf("day span = %ds; want a plain 24h UTC day", end-start)
	}
}

func TestMerchantTokenDecimalsCountsTheMultiplier(t *testing.T) {
	t.Setenv("TOKEN_DECIMALS", "1000000")
	if got := merchantTokenDecimals(); got != 6 {
		t.Fatalf("decimals = %d; want 6 for a multiplier of 1000000", got)
	}

	// Unset or nonsense must not yield 0, which would show 25000000 SFLUV
	// instead of 25 on every row.
	t.Setenv("TOKEN_DECIMALS", "")
	if got := merchantTokenDecimals(); got != 6 {
		t.Fatalf("decimals = %d; want the 6 default when unset", got)
	}
}
