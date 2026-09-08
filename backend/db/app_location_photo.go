package db

import (
	"context"
	"fmt"
	"time"

	"github.com/SFLuv/app/backend/utils"
)

// LocationPhoto is a merchant's uploaded storefront picture as stored.
//
// One per listing, keyed by location id and replaced in place, the same shape
// as the map icon. The two are different pictures doing different jobs — the
// icon is a mark drawn a few pixels wide inside a map pin, this is a
// photograph of the place shown at card width — so they are stored apart
// rather than one being derived from the other.
type LocationPhoto struct {
	Data        []byte
	ContentType string
	UpdatedAt   time.Time
}

// LocationPhotoURL is the address clients fetch a merchant's photo from.
//
// Version-stamped for the same reason as the icon: the bytes are served with a
// long cache lifetime, so a merchant replacing their photo needs a different
// URL or they would keep seeing the old one. A zero timestamp means no photo,
// and callers get an empty string rather than a URL that 404s.
func LocationPhotoURL(locationID uint, updatedAt time.Time) string {
	if updatedAt.IsZero() {
		return ""
	}
	return utils.PublicBackendURL(fmt.Sprintf("/locations/%d/photo?v=%d", locationID, updatedAt.Unix()))
}

// SetLocationPhoto stores (or replaces) a merchant's photo and returns the
// version stamp the new bytes are published under.
//
// The stamp is written to `locations` in the same transaction as the bytes, so
// a listing can never advertise a version that is not yet fetchable.
func (a *AppDB) SetLocationPhoto(ctx context.Context, locationID uint, data []byte, contentType string) (time.Time, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("error starting photo transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var updatedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO location_photos (location_id, content_type, image_data, size_bytes, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (location_id) DO UPDATE
		SET content_type = EXCLUDED.content_type,
			image_data = EXCLUDED.image_data,
			size_bytes = EXCLUDED.size_bytes,
			updated_at = NOW()
		RETURNING updated_at;
	`, locationID, contentType, data, len(data)).Scan(&updatedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("error storing location photo: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE locations SET photo_updated_at = $2 WHERE id = $1;
	`, locationID, updatedAt); err != nil {
		return time.Time{}, fmt.Errorf("error stamping location photo: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return time.Time{}, fmt.Errorf("error committing location photo: %w", err)
	}

	return updatedAt, nil
}

// DeleteLocationPhoto removes a merchant's photo. Deleting one that was never
// there is not an error — the caller's intent (no photo) is satisfied either
// way.
func (a *AppDB) DeleteLocationPhoto(ctx context.Context, locationID uint) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting photo delete: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM location_photos WHERE location_id = $1;`, locationID); err != nil {
		return fmt.Errorf("error deleting location photo: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE locations SET photo_updated_at = NULL WHERE id = $1;`, locationID); err != nil {
		return fmt.Errorf("error clearing location photo stamp: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing location photo delete: %w", err)
	}

	return nil
}

// GetLocationPhoto reads the stored bytes. Returns pgx.ErrNoRows when the
// merchant has not uploaded one.
func (a *AppDB) GetLocationPhoto(ctx context.Context, locationID uint) (*LocationPhoto, error) {
	photo := LocationPhoto{}
	err := a.db.QueryRow(ctx, `
		SELECT image_data, content_type, updated_at
		FROM location_photos
		WHERE location_id = $1;
	`, locationID).Scan(&photo.Data, &photo.ContentType, &photo.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &photo, nil
}
