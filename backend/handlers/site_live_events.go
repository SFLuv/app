package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

// ── Live events ──────────────────────────────────────────────────────────────
//
// Lets someone with the Live events permission (website_live_events) keep a
// volunteer event's photos current: its cover photos, and a gallery of what past
// occurrences looked like. Nothing else about the event can be changed from here.
//
// Cover photos belong to one occurrence, and the next occurrence copies them
// when it is generated, so editing today's carries forward. A gallery belongs
// to the whole series, so there is only ever one.

const maxLiveEventsListed = 50

// eventSeriesKey is what a gallery is filed under: the series, or the event
// itself when it stands alone.
func eventSeriesKey(row *db.VolunteerEventRow) string {
	if row.SeriesId != nil && *row.SeriesId != "" {
		return *row.SeriesId
	}
	return row.Id
}

func (a *AppService) toSiteLiveEvent(r *http.Request, row *db.VolunteerEventRow, eventCtx *volunteerEventContext) (*structs.SiteLiveEvent, error) {
	event := a.bot.mapVolunteerEvent(row, eventCtx)
	out := &structs.SiteLiveEvent{
		Id:            event.Id,
		Title:         event.Title,
		Slug:          event.Slug,
		StartAt:       event.StartAt,
		Timezone:      event.Timezone,
		OrganizerName: event.Organizer.Name,
		CoverPhotos:   event.CoverPhotos,
	}
	if out.CoverPhotos == nil {
		out.CoverPhotos = []structs.VolunteerEventPhoto{}
	}
	if event.Recurrence != nil {
		out.RecurrenceSummary = event.Recurrence.Summary
	}
	gallery, err := a.db.GetEventGallery(r.Context(), eventSeriesKey(row))
	if errors.Is(err, db.ErrSiteNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	a.finishPastEvent(r.Context(), gallery)
	out.Gallery = gallery
	return out, nil
}

// GetAdminLiveEvents lists what is coming up, one entry per series: the next
// occurrence is the one whose photos the site is showing.
func (a *AppService) GetAdminLiveEvents(w http.ResponseWriter, r *http.Request) {
	rows, _, err := a.bot.db.GetPublicVolunteerEvents(r.Context(), &structs.VolunteerEventsFilter{
		When:  "upcoming",
		Sort:  "start_at",
		Count: maxLiveEventsListed,
	})
	if err != nil {
		a.siteServerError(w, "listing live events", err)
		return
	}
	eventCtx := a.bot.buildVolunteerEventContext(r, rows)
	seen := map[string]bool{}
	events := []*structs.SiteLiveEvent{}
	for _, row := range rows {
		key := eventSeriesKey(row)
		if seen[key] {
			continue
		}
		seen[key] = true
		event, err := a.toSiteLiveEvent(r, row, eventCtx)
		if err != nil {
			a.siteServerError(w, "listing live events", err)
			return
		}
		events = append(events, event)
	}
	writeSiteJSON(w, http.StatusOK, map[string]any{"events": events})
}

// loadLiveEvent fetches the event named in the path, answering 404 itself.
func (a *AppService) loadLiveEvent(w http.ResponseWriter, r *http.Request) (*db.VolunteerEventRow, bool) {
	row, err := a.bot.db.GetPublicVolunteerEvent(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, pgx.ErrNoRows) {
		writeSiteError(w, http.StatusNotFound, "That event no longer exists.")
		return nil, false
	}
	if err != nil {
		a.siteServerError(w, "loading a live event", err)
		return nil, false
	}
	return row, true
}

func (a *AppService) respondLiveEvent(w http.ResponseWriter, r *http.Request, row *db.VolunteerEventRow, status int) {
	eventCtx := a.bot.buildVolunteerEventContext(r, []*db.VolunteerEventRow{row})
	event, err := a.toSiteLiveEvent(r, row, eventCtx)
	if err != nil {
		a.siteServerError(w, "reloading a live event", err)
		return
	}
	writeSiteJSON(w, status, event)
}

func (a *AppService) PostAdminLiveEventPhoto(w http.ResponseWriter, r *http.Request) {
	row, ok := a.loadLiveEvent(w, r)
	if !ok {
		return
	}
	photo, status, problem := readEventCoverUpload(r)
	if problem != "" {
		writeSiteError(w, status, siteCoverProblem(status))
		return
	}
	existing, err := a.bot.db.GetVolunteerEventPhotos(r.Context(), []string{row.Id})
	if err != nil {
		a.siteServerError(w, "adding a cover photo", err)
		return
	}
	if len(existing[row.Id]) >= maxEventPhotos {
		writeSiteError(w, http.StatusBadRequest, fmt.Sprintf("An event can have up to %d cover photos. Remove one first.", maxEventPhotos))
		return
	}
	width, height := partnerLogoDimensions(photo.data, photo.contentType)
	if _, err := a.bot.db.AddVolunteerEventPhoto(r.Context(), row.Id, photo.data, photo.contentType, photo.fileName, width, height); err != nil {
		a.siteServerError(w, "adding a cover photo", err)
		return
	}
	a.siteActivity(r, structs.SiteCapabilityLiveEvents, "edit", "live_event", row.Id, "Added a cover photo to “"+row.Title+"”")
	a.respondLiveEvent(w, r, row, http.StatusCreated)
}

// siteCoverProblem puts a failed cover upload in words for an editor.
func siteCoverProblem(status int) string {
	if status == http.StatusRequestEntityTooLarge {
		return fmt.Sprintf("That photo is too large. Cover photos can be up to %d MB.", maxEventPhotoBytes>>20)
	}
	return "That file isn’t a photo the site can show. Choose a PNG, JPEG, WebP or GIF."
}

func (a *AppService) PutAdminLiveEventPhotoOrder(w http.ResponseWriter, r *http.Request) {
	row, ok := a.loadLiveEvent(w, r)
	if !ok {
		return
	}
	var req structs.SiteEventPhotoOrderRequest
	if !decodeSiteBody(w, r, &req) {
		return
	}
	if err := a.bot.db.ReorderEventCoverPhotos(r.Context(), row.Id, req.Order); err != nil {
		a.siteServerError(w, "reordering cover photos", err)
		return
	}
	a.siteActivity(r, structs.SiteCapabilityLiveEvents, "edit", "live_event", row.Id, "Changed the cover photo order on “"+row.Title+"”")
	a.respondLiveEvent(w, r, row, http.StatusOK)
}

func (a *AppService) DeleteAdminLiveEventPhoto(w http.ResponseWriter, r *http.Request) {
	row, ok := a.loadLiveEvent(w, r)
	if !ok {
		return
	}
	err := a.bot.db.RemoveEventCoverPhoto(r.Context(), row.Id, strings.TrimSpace(r.PathValue("photoId")))
	if errors.Is(err, pgx.ErrNoRows) {
		writeSiteError(w, http.StatusNotFound, "That photo is no longer on the event.")
		return
	}
	if err != nil {
		a.siteServerError(w, "removing a cover photo", err)
		return
	}
	a.siteActivity(r, structs.SiteCapabilityLiveEvents, "edit", "live_event", row.Id, "Removed a cover photo from “"+row.Title+"”")
	a.respondLiveEvent(w, r, row, http.StatusOK)
}

// PostAdminLiveEventGallery starts the series' gallery. Asking again is harmless.
func (a *AppService) PostAdminLiveEventGallery(w http.ResponseWriter, r *http.Request) {
	row, ok := a.loadLiveEvent(w, r)
	if !ok {
		return
	}
	today := time.Now().In(sfluvTimeZone).Format("2006-01-02")
	if _, err := a.db.EnsureEventGallery(r.Context(), eventSeriesKey(row), pastEventSlug(row.Title), row.Title, today, utils.GetDid(r)); err != nil {
		a.siteServerError(w, "starting an event gallery", err)
		return
	}
	a.siteActivity(r, structs.SiteCapabilityLiveEvents, "add", "live_event", row.Id, "Started a gallery on “"+row.Title+"”")
	a.respondLiveEvent(w, r, row, http.StatusCreated)
}

type liveGalleryRequestKey struct{}

// isLiveGalleryRequest reports whether a request came in through the live event
// routes below, which have already checked the Live events permission.
func isLiveGalleryRequest(r *http.Request) bool {
	allowed, _ := r.Context().Value(liveGalleryRequestKey{}).(bool)
	return allowed
}

// throughLiveGallery runs a gallery photo handler against the gallery of the
// event named in the path. The photo handling is the same as Past events', so a
// gallery behaves the same way everywhere; only the door and the permission differ.
func (a *AppService) throughLiveGallery(w http.ResponseWriter, r *http.Request, handler http.HandlerFunc) {
	row, ok := a.loadLiveEvent(w, r)
	if !ok {
		return
	}
	gallery, err := a.db.GetEventGallery(r.Context(), eventSeriesKey(row))
	if errors.Is(err, db.ErrSiteNotFound) {
		writeSiteError(w, http.StatusNotFound, "This event doesn’t have a gallery yet.")
		return
	}
	if err != nil {
		a.siteServerError(w, "loading an event gallery", err)
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), liveGalleryRequestKey{}, true))
	r.SetPathValue("id", gallery.Id)
	handler(w, r)
}

func (a *AppService) PostAdminLiveEventGalleryPhotos(w http.ResponseWriter, r *http.Request) {
	a.throughLiveGallery(w, r, a.PostAdminPastEventPhotos)
}

func (a *AppService) PutAdminLiveEventGalleryPhotoOrder(w http.ResponseWriter, r *http.Request) {
	a.throughLiveGallery(w, r, a.PutAdminPastEventPhotoOrder)
}

func (a *AppService) PutAdminLiveEventGalleryPhoto(w http.ResponseWriter, r *http.Request) {
	a.throughLiveGallery(w, r, a.PutAdminPastEventPhoto)
}

func (a *AppService) DeleteAdminLiveEventGalleryPhoto(w http.ResponseWriter, r *http.Request) {
	a.throughLiveGallery(w, r, a.DeleteAdminPastEventPhoto)
}

// GetPublicEventGallery is the gallery shown under a live event. Any occurrence
// finds it, since they share a series.
func (a *AppService) GetPublicEventGallery(w http.ResponseWriter, r *http.Request) {
	row, err := a.bot.db.GetPublicVolunteerEvent(r.Context(), strings.TrimSpace(r.PathValue("event_id")))
	if errors.Is(err, pgx.ErrNoRows) {
		writeSiteError(w, http.StatusNotFound, "That event was not found.")
		return
	}
	if err != nil {
		a.siteServerError(w, "loading an event gallery", err)
		return
	}
	gallery, err := a.db.GetEventGallery(r.Context(), eventSeriesKey(row))
	if errors.Is(err, db.ErrSiteNotFound) || (err == nil && gallery.RemovedAt != nil) {
		writeSiteError(w, http.StatusNotFound, "That event has no gallery.")
		return
	}
	if err != nil {
		a.siteServerError(w, "loading an event gallery", err)
		return
	}
	a.finishPastEvent(r.Context(), gallery)
	writeSiteJSON(w, http.StatusOK, toPublicPastEvent(gallery, true))
}
