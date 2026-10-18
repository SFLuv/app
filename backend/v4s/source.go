package v4s

import (
	"net/url"
	"strings"
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
		Site:    site,
		JobId:   jobId,
		ShiftId: strings.TrimSpace(q.Get("volunteerShiftId")),
	}, true
}
