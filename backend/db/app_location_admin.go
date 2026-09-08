package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// AdminUpdateLocationDetails rewrites the parts of a listing an admin is
// allowed to correct, for any location rather than only their own.
//
// This exists because the owner-facing update deliberately cannot reach the
// fields most often wrong: type, hours and the city/state/zip half of the
// address are written only by the Google sync, so until now a listing Google
// had wrong (or never had) could not be corrected at all. Admins are the
// backstop for that, so they get a direct write.
//
// google_id, lat, lng, rating and maps_page are still not writable here. Those
// identify the place rather than describe it, and hand-editing them would let a
// listing drift away from the Google record it claims to be without ever
// failing the duplicate check that the re-point path enforces. Correcting the
// place identity is what AdminUpdateLocationGooglePlace is for.
func (a *AppDB) AdminUpdateLocationDetails(ctx context.Context, location *structs.Location) error {
	if location == nil {
		return fmt.Errorf("a location is required")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error opening transaction for admin location update: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE locations
		SET
			name = $1,
			description = $2,
			type = $3,
			street = $4,
			city = $5,
			state = $6,
			zip = $7,
			phone = $8,
			email = $9,
			website = $10
		WHERE (id = $11 AND active = TRUE);
	`,
		location.Name,
		location.Description,
		location.Type,
		location.Street,
		location.City,
		location.State,
		location.ZIP,
		location.Phone,
		location.Email,
		location.Website,
		location.ID,
	)
	if err != nil {
		return fmt.Errorf("error updating location %d as admin: %w", location.ID, err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	// Hours are replaced wholesale, in the same transaction as the rest. A
	// nil slice means "leave them alone"; an empty one would silently wipe a
	// merchant's opening times, so the handler only passes hours through when
	// the request actually carried them.
	if location.OpeningHours != nil {
		if err := replaceLocationHours(ctx, tx, location.ID, location.OpeningHours); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing admin location update: %w", err)
	}

	return nil
}

// AdminUpdateLocationGooglePlace re-points any location at a Google place.
//
// Same write as the owner-facing path, including the one-active-location-per
// place check; it simply is not restricted to locations the caller owns.
func (a *AppDB) AdminUpdateLocationGooglePlace(ctx context.Context, locationID uint, place *structs.VerifiedGooglePlace) error {
	return a.updateLocationGooglePlace(ctx, "", locationID, place)
}
