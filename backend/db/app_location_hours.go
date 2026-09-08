package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// GetLocationDayHours returns a location's structured week, Monday first.
func (a *AppDB) GetLocationDayHours(ctx context.Context, locationID uint) ([]structs.LocationDayHours, error) {
	rows, err := a.db.Query(ctx, `
		SELECT weekday, is_closed, intervals
		FROM location_hours
		WHERE location_id = $1 AND active = TRUE
		ORDER BY weekday;
	`, locationID)
	if err != nil {
		return nil, fmt.Errorf("error getting structured location hours: %w", err)
	}
	defer rows.Close()

	days := make([]structs.LocationDayHours, 0, len(structs.WeekdayNames))
	for rows.Next() {
		var day structs.LocationDayHours
		var intervals []byte
		if err := rows.Scan(&day.Weekday, &day.IsClosed, &intervals); err != nil {
			return nil, fmt.Errorf("error scanning structured location hours: %w", err)
		}
		if len(intervals) > 0 {
			if err := json.Unmarshal(intervals, &day.Intervals); err != nil {
				return nil, fmt.Errorf("error decoding structured location hours: %w", err)
			}
		}
		days = append(days, day)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating structured location hours: %w", err)
	}

	return days, nil
}

// SetLocationHours replaces a location's week.
func (a *AppDB) SetLocationHours(ctx context.Context, locationID uint, days []structs.LocationDayHours) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error opening transaction for location hours: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := replaceLocationDayHours(ctx, tx, locationID, days); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing location hours: %w", err)
	}

	return nil
}

// SetLocationHoursManual toggles a listing out of (or back into) the nightly
// Google sync.
func (a *AppDB) SetLocationHoursManual(ctx context.Context, locationID uint, manual bool) error {
	result, err := a.db.Exec(ctx, `
		UPDATE locations SET hours_manual = $1 WHERE id = $2 AND active = TRUE;
	`, manual, locationID)
	if err != nil {
		return fmt.Errorf("error setting manual hours mode for location %d: %w", locationID, err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// LocationHoursSyncTarget is one listing the nightly job may refresh.
type LocationHoursSyncTarget struct {
	ID       uint
	GoogleID string
	Name     string
}

// GetLocationsForHoursSync lists the active listings eligible for the nightly
// refresh: they have a Google listing to poll, and have not been switched to
// manual. Manual listings are filtered out here rather than skipped later, so
// the job cannot spend a Places call on a listing it is forbidden to write.
func (a *AppDB) GetLocationsForHoursSync(ctx context.Context) ([]LocationHoursSyncTarget, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, google_id, name
		FROM locations
		WHERE active = TRUE
		AND hours_manual = FALSE
		AND google_id IS NOT NULL
		AND google_id <> ''
		ORDER BY id;
	`)
	if err != nil {
		return nil, fmt.Errorf("error listing locations for hours sync: %w", err)
	}
	defer rows.Close()

	targets := make([]LocationHoursSyncTarget, 0)
	for rows.Next() {
		var target LocationHoursSyncTarget
		if err := rows.Scan(&target.ID, &target.GoogleID, &target.Name); err != nil {
			return nil, fmt.Errorf("error scanning hours sync target: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating hours sync targets: %w", err)
	}

	return targets, nil
}

// ApplySyncedLocationHours writes refreshed hours and stamps the sync.
//
// The manual check is repeated inside the write, not just in the listing query:
// a merchant can switch to manual while the job is midway through its run, and
// the update must lose that race rather than overwrite the choice they just made.
func (a *AppDB) ApplySyncedLocationHours(ctx context.Context, locationID uint, days []structs.LocationDayHours) (bool, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("error opening transaction for synced hours: %w", err)
	}
	defer tx.Rollback(ctx)

	var manual bool
	if err := tx.QueryRow(ctx, `
		SELECT hours_manual FROM locations WHERE id = $1 AND active = TRUE FOR UPDATE;
	`, locationID).Scan(&manual); err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("error locking location %d for hours sync: %w", locationID, err)
	}
	if manual {
		return false, nil
	}

	if err := replaceLocationDayHours(ctx, tx, locationID, days); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE locations SET hours_synced_at = NOW() WHERE id = $1;
	`, locationID); err != nil {
		return false, fmt.Errorf("error stamping hours sync for location %d: %w", locationID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("error committing synced hours: %w", err)
	}

	return true, nil
}

// UserOwnsLocation reports whether an active location belongs to a user.
func (a *AppDB) UserOwnsLocation(ctx context.Context, ownerID string, locationID uint) (bool, error) {
	var owns bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM locations
			WHERE id = $1 AND owner_id = $2 AND active = TRUE
		);
	`, locationID, ownerID).Scan(&owns)
	if err != nil {
		return false, fmt.Errorf("error checking location ownership: %w", err)
	}

	return owns, nil
}
