package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SitePastEventsDDL creates the tables behind the site's Past events section:
// one row per tile, and its gallery photos in order. Applied by migration 1.64.
const SitePastEventsDDL = `
	CREATE TABLE IF NOT EXISTS site_past_events(
		id TEXT PRIMARY KEY,
		-- The gallery page's address; fixed once created, so links keep working.
		slug TEXT NOT NULL UNIQUE,
		title TEXT NOT NULL,
		event_date DATE NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		-- The tile's photo: an upload, or for tiles that predate editing, an
		-- image hosted by the site itself. Neither means a placeholder.
		cover_file_id TEXT REFERENCES site_files(id) ON DELETE RESTRICT,
		cover_url TEXT,
		cover_width INTEGER NOT NULL DEFAULT 0,
		cover_height INTEGER NOT NULL DEFAULT 0,
		cover_alt TEXT NOT NULL DEFAULT '',
		removed_at BIGINT,
		created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
		created_at BIGINT NOT NULL DEFAULT unix_now(),
		updated_at BIGINT NOT NULL DEFAULT unix_now(),
		CHECK (cover_file_id IS NULL OR cover_url IS NULL)
	);

	CREATE INDEX IF NOT EXISTS site_past_events_date_idx ON site_past_events(event_date DESC);

	CREATE TABLE IF NOT EXISTS site_past_event_photos(
		id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES site_past_events(id) ON DELETE CASCADE,
		file_id TEXT REFERENCES site_files(id) ON DELETE RESTRICT,
		url TEXT,
		width INTEGER NOT NULL DEFAULT 0,
		height INTEGER NOT NULL DEFAULT 0,
		alt TEXT NOT NULL DEFAULT '',
		-- Shown under the photo on the gallery page. Optional.
		caption TEXT NOT NULL DEFAULT '',
		position INTEGER NOT NULL,
		created_at BIGINT NOT NULL DEFAULT unix_now(),
		CHECK ((file_id IS NOT NULL) <> (url IS NOT NULL))
	);

	CREATE INDEX IF NOT EXISTS site_past_event_photos_event_idx ON site_past_event_photos(event_id, position);
`

const pastEventColumns = `
	e.id, e.slug, e.title, to_char(e.event_date, 'YYYY-MM-DD'), e.description,
	e.cover_file_id, e.cover_url, e.cover_width, e.cover_height, e.cover_alt, COALESCE(cf.filename, ''),
	e.removed_at, e.created_at, e.updated_at`

const pastEventFrom = `site_past_events e LEFT JOIN site_files cf ON cf.id = e.cover_file_id`

func scanPastEvent(row pgx.Row) (*structs.SitePastEvent, error) {
	ev := &structs.SitePastEvent{Photos: []structs.SitePhoto{}}
	var coverFileId, coverURL *string
	var cover structs.SitePhoto
	if err := row.Scan(
		&ev.Id, &ev.Slug, &ev.Title, &ev.Date, &ev.Description,
		&coverFileId, &coverURL, &cover.Width, &cover.Height, &cover.Alt, &cover.Filename,
		&ev.RemovedAt, &ev.CreatedAt, &ev.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if coverFileId != nil || coverURL != nil {
		cover.FileId = coverFileId
		if coverURL != nil {
			cover.URL = *coverURL
		}
		ev.Cover = &cover
	}
	return ev, nil
}

// loadPastEventPhotos fills in each event's gallery, in order. File-backed
// photos are left without a URL; the handler builds it.
func (a *AppDB) loadPastEventPhotos(ctx context.Context, events []*structs.SitePastEvent) error {
	if len(events) == 0 {
		return nil
	}
	byId := map[string]*structs.SitePastEvent{}
	ids := make([]string, 0, len(events))
	for _, ev := range events {
		byId[ev.Id] = ev
		ids = append(ids, ev.Id)
	}
	rows, err := a.db.Query(ctx, `
		SELECT p.event_id, p.id, p.file_id, COALESCE(p.url, ''), p.width, p.height, p.alt, p.caption, COALESCE(f.filename, '')
		FROM site_past_event_photos p
		LEFT JOIN site_files f ON f.id = p.file_id
		WHERE p.event_id = ANY($1)
		ORDER BY p.event_id, p.position, p.created_at, p.id;
	`, ids)
	if err != nil {
		return fmt.Errorf("error listing past event photos: %s", err)
	}
	defer rows.Close()
	for rows.Next() {
		var eventId string
		var photo structs.SitePhoto
		if err := rows.Scan(&eventId, &photo.Id, &photo.FileId, &photo.URL, &photo.Width, &photo.Height, &photo.Alt, &photo.Caption, &photo.Filename); err != nil {
			return fmt.Errorf("error scanning past event photo: %s", err)
		}
		if ev := byId[eventId]; ev != nil {
			ev.Photos = append(ev.Photos, photo)
		}
	}
	return rows.Err()
}

// ListPastEvents returns events newest first, each with its photos.
func (a *AppDB) ListPastEvents(ctx context.Context, includeRemoved bool) ([]*structs.SitePastEvent, error) {
	rows, err := a.db.Query(ctx, `
		SELECT `+pastEventColumns+`
		FROM `+pastEventFrom+`
		WHERE $1 OR e.removed_at IS NULL
		ORDER BY e.event_date DESC, e.created_at DESC, e.id;
	`, includeRemoved)
	if err != nil {
		return nil, fmt.Errorf("error listing past events: %s", err)
	}
	events := []*structs.SitePastEvent{}
	for rows.Next() {
		ev, err := scanPastEvent(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("error scanning past event: %s", err)
		}
		events = append(events, ev)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := a.loadPastEventPhotos(ctx, events); err != nil {
		return nil, err
	}
	return events, nil
}

func (a *AppDB) getPastEvent(ctx context.Context, where string, arg string) (*structs.SitePastEvent, error) {
	ev, err := scanPastEvent(a.db.QueryRow(ctx, `
		SELECT `+pastEventColumns+` FROM `+pastEventFrom+` WHERE `+where+`;
	`, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSiteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error loading past event: %s", err)
	}
	if err := a.loadPastEventPhotos(ctx, []*structs.SitePastEvent{ev}); err != nil {
		return nil, err
	}
	return ev, nil
}

func (a *AppDB) GetPastEvent(ctx context.Context, id string) (*structs.SitePastEvent, error) {
	return a.getPastEvent(ctx, "e.id = $1", id)
}

// GetPastEventBySlug returns a shown (not removed) event by its address.
func (a *AppDB) GetPastEventBySlug(ctx context.Context, slug string) (*structs.SitePastEvent, error) {
	return a.getPastEvent(ctx, "e.slug = $1 AND e.removed_at IS NULL", slug)
}

// CreatePastEvent adds a tile. The slug must already be unique-ish; on a clash
// a short suffix is added.
func (a *AppDB) CreatePastEvent(ctx context.Context, slug string, req *structs.SitePastEventRequest, cover *structs.SitePhoto, userId *string) (string, error) {
	id := uuid.NewString()
	var coverFileId any
	coverW, coverH, coverAlt := 0, 0, req.CoverAlt
	if cover != nil {
		coverFileId, coverW, coverH = *cover.FileId, cover.Width, cover.Height
	}
	for attempt := 0; attempt < 5; attempt++ {
		candidate := slug
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%s", slug, uuid.NewString()[:4])
		}
		_, err := a.db.Exec(ctx, `
			INSERT INTO site_past_events (id, slug, title, event_date, description, cover_file_id, cover_width, cover_height, cover_alt, created_by)
			VALUES ($1, $2, $3, $4::date, $5, $6, $7, $8, $9, $10);
		`, id, candidate, req.Title, req.Date, req.Description, coverFileId, coverW, coverH, coverAlt, userId)
		if err == nil {
			return id, nil
		}
		if !isUniqueViolation(err) {
			return "", fmt.Errorf("error creating past event: %s", err)
		}
	}
	return "", fmt.Errorf("error creating past event: could not find a free address")
}

// UpdatePastEvent edits the words and date, and the cover when one is given.
func (a *AppDB) UpdatePastEvent(ctx context.Context, id string, req *structs.SitePastEventRequest, cover *structs.SitePhoto) error {
	setCover := cover != nil
	var coverFileId, coverURL any
	coverW, coverH := 0, 0
	if cover != nil {
		if cover.FileId != nil {
			coverFileId = *cover.FileId
		} else {
			coverURL = cover.URL
		}
		coverW, coverH = cover.Width, cover.Height
	}
	tag, err := a.db.Exec(ctx, `
		UPDATE site_past_events SET
			title = $2, event_date = $3::date, description = $4, cover_alt = $5,
			cover_file_id = CASE WHEN $6 THEN $7 ELSE cover_file_id END,
			cover_url = CASE WHEN $6 THEN $8 ELSE cover_url END,
			cover_width = CASE WHEN $6 THEN $9 ELSE cover_width END,
			cover_height = CASE WHEN $6 THEN $10 ELSE cover_height END,
			updated_at = unix_now()
		WHERE id = $1;
	`, id, req.Title, req.Date, req.Description, req.CoverAlt, setCover, coverFileId, coverURL, coverW, coverH)
	if err != nil {
		return fmt.Errorf("error updating past event: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSiteNotFound
	}
	return nil
}

func (a *AppDB) SetPastEventRemoved(ctx context.Context, id string, removed bool) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE site_past_events
		SET removed_at = CASE WHEN $2 THEN unix_now() ELSE NULL END, updated_at = unix_now()
		WHERE id = $1;
	`, id, removed)
	if err != nil {
		return fmt.Errorf("error updating past event: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSiteNotFound
	}
	return nil
}

// AddPastEventPhotos appends uploaded images to the end of a gallery.
func (a *AppDB) AddPastEventPhotos(ctx context.Context, eventId string, files []*structs.SiteFile) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var next int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(position), -1) + 1 FROM site_past_event_photos WHERE event_id = $1;
	`, eventId).Scan(&next); err != nil {
		return fmt.Errorf("error adding past event photos: %s", err)
	}
	for i, file := range files {
		if _, err := tx.Exec(ctx, `
			INSERT INTO site_past_event_photos (id, event_id, file_id, width, height, position)
			VALUES ($1, $2, $3, $4, $5, $6);
		`, uuid.NewString(), eventId, file.Id, file.Width, file.Height, next+i); err != nil {
			return fmt.Errorf("error adding past event photos: %s", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE site_past_events SET updated_at = unix_now() WHERE id = $1;`, eventId); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReorderPastEventPhotos puts a gallery in the given order. Photos missing from
// the list keep their relative order after the listed ones.
func (a *AppDB) ReorderPastEventPhotos(ctx context.Context, eventId string, order []string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE site_past_event_photos p
		SET position = COALESCE(array_position($2::text[], p.id), 100000 + p.position)
		WHERE p.event_id = $1;
	`, eventId, order); err != nil {
		return fmt.Errorf("error reordering past event photos: %s", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE site_past_events SET updated_at = unix_now() WHERE id = $1;`, eventId); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *AppDB) UpdatePastEventPhotoCaption(ctx context.Context, eventId string, photoId string, caption string) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE site_past_event_photos SET caption = $3 WHERE event_id = $1 AND id = $2;
	`, eventId, photoId, caption)
	if err != nil {
		return fmt.Errorf("error updating past event photo: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSiteNotFound
	}
	return nil
}

// DeletePastEventPhoto takes a photo out of a gallery. The uploaded file
// itself is cleared away later, once nothing refers to it.
func (a *AppDB) DeletePastEventPhoto(ctx context.Context, eventId string, photoId string) error {
	tag, err := a.db.Exec(ctx, `
		DELETE FROM site_past_event_photos WHERE event_id = $1 AND id = $2;
	`, eventId, photoId)
	if err != nil {
		return fmt.Errorf("error removing past event photo: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSiteNotFound
	}
	return nil
}
