package handlers

import (
	"testing"
	"time"
)

// Reminder copy reads as a human would say it, not as a raw duration.
func TestHumanizeLeadTime(t *testing.T) {
	cases := []struct {
		remaining time.Duration
		want      string
	}{
		{-time.Hour, "now"},
		{0, "now"},
		{30 * time.Minute, "in 30 minutes"},
		{90 * time.Minute, "in about an hour"},
		{5 * time.Hour, "in 5 hours"},
		{30 * time.Hour, "tomorrow"},
		{72 * time.Hour, "in 3 days"},
	}

	for _, testCase := range cases {
		if got := humanizeLeadTime(testCase.remaining); got != testCase.want {
			t.Errorf("humanizeLeadTime(%s) = %q, want %q", testCase.remaining, got, testCase.want)
		}
	}
}

// The scan window must exceed the largest configurable lead time, or a user who
// asks for a week's notice is never found by the sweep.
func TestReminderLookaheadCoversMaxLeadTime(t *testing.T) {
	maxLead := 168 * time.Hour
	if volunteerReminderLookahead <= maxLead {
		t.Fatalf("lookahead %s must exceed the maximum lead time %s", volunteerReminderLookahead, maxLead)
	}
}
