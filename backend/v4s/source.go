package v4s

import (
	"net/url"
	"strings"
	"time"
)

// Source is a V4S job identified from an event's external signup link.
//
// We do not ask anyone to configure this. An affiliate pastes the same link
// they would have given a volunteer, and if it is a V4S job listing we can read
// availability from it; if it is anything else, nothing changes and the link
// stays an ordinary external link.
type Source struct {
	// Site is the Salesforce Site origin and path prefix, e.g.
	// https://example.my.salesforce-sites.com/volunteer
	Site string
	// JobId is the GW_Volunteers__Volunteer_Job__c id. Our recurring event maps
	// to the job; each of its occurrences maps to a shift on a date.
	JobId string
	// ShiftId is set when the link already points at one occurrence. It is not
	// used for matching — a recurring event's link would pin every occurrence
	// to one date — but it is kept so a one-off event can be linked exactly.
	ShiftId string
	// LocalDate is the day the link is filtered to, as YYYY-MM-DD in the
	// organization's local time, taken from its dtMonthFilter. Empty when the
	// link names no day. A link pasted from one occurrence carries that
	// occurrence's day, which is right for a one-off event and wrong for every
	// later occurrence of a recurring one.
	LocalDate string
}

// Key identifies the job for caching and rate limiting.
func (s Source) Key() string { return s.Site + "|" + s.JobId }

// ParseSource recognises a V4S job listing link.
//
// It returns ok=false for anything else, which is the common case: most
// external signup links are Eventbrite, a Google Form, or the organization's
// own page. Callers treat that as "no availability to show" rather than an
// error.
func ParseSource(raw string) (Source, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Source{}, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Source{}, false
	}

	// The page name carries the V4S namespace, which is what makes this a V4S
	// link rather than a guess based on the hostname. Organizations can and do
	// put their Site on a custom domain.
	idx := strings.Index(u.Path, "/GW_Volunteers__")
	if idx < 0 {
		return Source{}, false
	}
	page := u.Path[idx+len("/GW_Volunteers__"):]
	if slash := strings.IndexByte(page, '/'); slash >= 0 {
		page = page[:slash]
	}
	switch page {
	case "VolunteersJobListingFS", "VolunteersJobListing",
		"JobCalendar", "PersonalSiteJobCalendar", "VolunteersSignupFS":
	default:
		return Source{}, false
	}

	q := u.Query()
	jobId := strings.TrimSpace(q.Get("jobId"))
	if jobId == "" {
		// Without a job there is nothing to read: a bare calendar link covers
		// every job on the site and cannot be tied to one of our events.
		return Source{}, false
	}

	site := u.Scheme + "://" + u.Host + u.Path[:idx]
	return Source{
		Site:      site,
		JobId:     jobId,
		ShiftId:   strings.TrimSpace(q.Get("volunteerShiftId")),
		LocalDate: filterDate(q.Get("dtMonthFilter")),
	}, true
}

// filterDate reads the day out of a dtMonthFilter value such as
// "2026-10-26 6:30:0". Anything that does not start with a date gives "".
func filterDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) < len("2006-01-02") {
		return ""
	}
	day := raw[:len("2006-01-02")]
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return ""
	}
	return day
}
