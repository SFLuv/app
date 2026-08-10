package db

import (
	"context"
	"fmt"
	"time"

	"github.com/SFLuv/app/backend/utils"
)

// LocationIcon is a merchant's uploaded map icon as stored.
type LocationIcon struct {
	Data        []byte
	ContentType string
	UpdatedAt   time.Time
}

// LocationIconURL is the address clients fetch a merchant's icon from.
//
// The version stamp matters: the bytes are served with a long cache lifetime
// (a map redraws these constantly and they almost never change), so a merchant
// replacing their logo needs a different URL or they would keep seeing the old
// one. A zero timestamp means no icon, and callers get an empty string rather
// than a URL that 404s.
func LocationIconURL(locationID uint, updatedAt time.Time) string {
	if updatedAt.IsZero() {
		return ""
	}
	return utils.PublicBackendURL(fmt.Sprintf("/locations/%d/icon?v=%d", locationID, updatedAt.Unix()))
}

// SetLocationIcon stores (or replaces) a merchant's icon and returns the
// version stamp the new bytes are published under.
//
// The stamp is written to `locations` in the same transaction as the bytes, so
// a listing can never advertise a version that is not yet fetchable.
func (a *AppDB) SetLocationIcon(ctx context.Context, locationID uint, data []byte, contentType string) (time.Time, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("error starting icon transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var updatedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO location_icons (location_id, content_type, image_data, size_bytes, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (location_id) DO UPDATE
		SET content_type = EXCLUDED.content_type,
			image_data = EXCLUDED.image_data,
			size_bytes = EXCLUDED.size_bytes,
			updated_at = NOW()
		RETURNING updated_at;
	`, locationID, contentType, data, len(data)).Scan(&updatedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("error storing location icon: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE locations SET icon_updated_at = $2 WHERE id = $1;
	`, locationID, updatedAt); err != nil {
		return time.Time{}, fmt.Errorf("error stamping location icon: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return time.Time{}, fmt.Errorf("error committing location icon: %w", err)
	}

	return updatedAt, nil
}

// DeleteLocationIcon removes a merchant's icon, returning them to the
// generated default. Deleting one that was never there is not an error — the
// caller's intent (no icon) is satisfied either way.
func (a *AppDB) DeleteLocationIcon(ctx context.Context, locationID uint) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting icon delete: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM location_icons WHERE location_id = $1;`, locationID); err != nil {
		return fmt.Errorf("error deleting location icon: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE locations SET icon_updated_at = NULL WHERE id = $1;`, locationID); err != nil {
		return fmt.Errorf("error clearing location icon stamp: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing location icon delete: %w", err)
	}

	return nil
}

// GetLocationIcon reads the stored bytes. Returns pgx.ErrNoRows when the
// merchant has not uploaded one.
func (a *AppDB) GetLocationIcon(ctx context.Context, locationID uint) (*LocationIcon, error) {
	icon := LocationIcon{}
	err := a.db.QueryRow(ctx, `
		SELECT image_data, content_type, updated_at
		FROM location_icons
		WHERE location_id = $1;
	`, locationID).Scan(&icon.Data, &icon.ContentType, &icon.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &icon, nil
}

// LocationExists reports whether an active location id is real, so an icon
// upload for a nonexistent listing fails before any bytes are read.
func (a *AppDB) LocationExists(ctx context.Context, locationID uint) (bool, error) {
	var exists bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM locations WHERE id = $1 AND active = TRUE);
	`, locationID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking location existence: %w", err)
	}

	return exists, nil
}
