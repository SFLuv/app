package handlers

import (
	"context"
	"time"

	"github.com/SFLuv/app/backend/db"

	"github.com/SFLuv/app/backend/structs"
)

// Pacing for the nightly run. Places calls are billed per request and the whole
// merchant list is polled at once, so the job trickles rather than bursts.
const (
	locationHoursSyncSpacing = 250 * time.Millisecond
	locationHoursSyncTimeout = 20 * time.Second
)

// LocationHoursSyncReport summarises one nightly pass.
type LocationHoursSyncReport struct {
	Considered    int
	Updated       int
	SkippedNoData int
	SkippedManual int
	Failed        int
}

// LocationHoursScheduler refreshes merchant opening hours from Google overnight.
//
// It runs at local midnight rather than on a fixed interval because opening
// hours are a local-calendar fact: a merchant who changes their Sunday hours
// expects the change to land before Sunday, not 24 hours after whenever the
// process last booted.
type LocationHoursScheduler struct {
	app *AppService
}

func NewLocationHoursScheduler(app *AppService) *LocationHoursScheduler {
	return &LocationHoursScheduler{app: app}
}

// hoursSyncZone is the clock the nightly run follows.
//
// Pinned to Pacific rather than the server's local zone: these are San
// Francisco merchants, so "midnight" has to mean midnight where they are, not
// wherever the process happens to be deployed. Naming the zone rather than a
// fixed -08:00 offset is what keeps it at local midnight across the PST/PDT
// change instead of drifting an hour for half the year.
const hoursSyncZoneName = "America/Los_Angeles"

func hoursSyncLocation() *time.Location {
	zone, err := time.LoadLocation(hoursSyncZoneName)
	if err != nil {
		// A container without tzdata would otherwise silently fall back to UTC
		// and run the job at 4pm Pacific. Fixed offset is wrong for half the
		// year, but it is wrong by an hour rather than by eight.
		return time.FixedZone("PST", -8*60*60)
	}
	return zone
}

// nextMidnight is the next 00:00 Pacific strictly after now.
func nextMidnight(now time.Time) time.Time {
	pacific := now.In(hoursSyncLocation())
	year, month, day := pacific.Date()
	midnight := time.Date(year, month, day, 0, 0, 0, 0, pacific.Location())
	if !midnight.After(pacific) {
		midnight = midnight.AddDate(0, 0, 1)
	}
	return midnight
}

func (s *LocationHoursScheduler) Start(ctx context.Context) {
	if s == nil || s.app == nil {
		return
	}

	go func() {
		for {
			// Recomputed each pass rather than using a fixed 24h ticker, so the
			// run stays pinned to midnight across daylight-saving changes.
			wait := time.Until(nextMidnight(time.Now()))
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				timer.Stop()
				s.RunOnce(ctx)
			}
		}
	}()
}

// RunOnce polls Google for every eligible listing and applies what it finds.
//
// Two things it deliberately will not do:
//
//   - It never writes a listing in manual mode. Those are filtered out of the
//     query and re-checked inside the write, so a merchant switching to manual
//     mid-run wins the race.
//   - It never clears hours it failed to read. Google returning nothing usable —
//     an unreachable API, a listing with no published hours, a week of split
//     shifts we refuse to flatten — leaves the existing hours alone. Wiping a
//     merchant's hours because a poll came back thin is worse than serving hours
//     that are a day stale.
func (s *LocationHoursScheduler) RunOnce(ctx context.Context) LocationHoursSyncReport {
	report := LocationHoursSyncReport{}
	if s == nil || s.app == nil {
		return report
	}

	if !GooglePlacesVerificationEnabled() {
		s.app.logger.Logf("location hours sync: skipped, google places is not configured")
		return report
	}

	targets, err := s.app.db.GetLocationsForHoursSync(ctx)
	if err != nil {
		s.app.logger.Logf("location hours sync: could not list locations: %s", err)
		return report
	}
	report.Considered = len(targets)

	for _, target := range targets {
		select {
		case <-ctx.Done():
			return report
		default:
		}

		days, ok := s.fetchHours(ctx, target)
		if !ok {
			report.SkippedNoData++
			continue
		}

		applied, err := s.app.db.ApplySyncedLocationHours(ctx, target.ID, days)
		if err != nil {
			report.Failed++
			s.app.logger.Logf("location hours sync: could not update %s (%d): %s", target.Name, target.ID, err)
			continue
		}
		if !applied {
			report.SkippedManual++
			continue
		}
		report.Updated++

		time.Sleep(locationHoursSyncSpacing)
	}

	s.app.logger.Logf(
		"location hours sync: %d considered, %d updated, %d skipped for no usable hours, %d skipped as manual, %d failed",
		report.Considered, report.Updated, report.SkippedNoData, report.SkippedManual, report.Failed,
	)

	return report
}

// fetchHours returns a full week only when Google gave us something usable.
func (s *LocationHoursScheduler) fetchHours(ctx context.Context, target db.LocationHoursSyncTarget) ([]structs.LocationDayHours, bool) {
	callCtx, cancel := context.WithTimeout(ctx, locationHoursSyncTimeout)
	defer cancel()

	verified, err := VerifyGooglePlace(callCtx, target.GoogleID)
	if err != nil {
		s.app.logger.Logf("location hours sync: could not verify %s (%d): %s", target.Name, target.ID, err)
		return nil, false
	}
	if verified == nil || len(verified.StructuredHours) != len(structs.WeekdayNames) {
		return nil, false
	}

	// A week where nothing could be read is not a week of closures. Requiring at
	// least one day with real times is what stops a thin response from erasing a
	// merchant's hours.
	for _, day := range verified.StructuredHours {
		if day.HasTimes() {
			return verified.StructuredHours, true
		}
	}

	return nil, false
}
