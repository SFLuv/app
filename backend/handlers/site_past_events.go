package handlers

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// ── Past events ──────────────────────────────────────────────────────────────
//
// Tiles in the site's Past events section, each with a photo gallery page.
// Editing them is its own permission (website_past_events). A past event is
// only a tile: it makes no volunteer event, QR codes or rewards.

const (
	maxPastEventPhotosPerUpload = 50
	maxPastEventPhotos          = 300
)

var pastEventSlugJunk = regexp.MustCompile(`[^a-z0-9]+`)

// Accented letters common in event names, so "Día" becomes "dia" rather than "d-a".
var pastEventSlugFold = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ñ", "n", "ç", "c",
	"’", "", "'", "", "&", " and ", "+", " and ",
)

// pastEventSlug turns a title into the gallery page's address,
// e.g. "Día de los Muertos + Crafts" → "dia-de-los-muertos-and-crafts".
func pastEventSlug(title string) string {
	slug := pastEventSlugFold.Replace(strings.ToLower(title))
	slug = strings.Trim(pastEventSlugJunk.ReplaceAllString(slug, "-"), "-")
	if len(slug) > 60 {
		slug = strings.TrimRight(slug[:60], "-")
	}
	if slug == "" {
		slug = "event"
	}
	return slug
}

// finishSitePhoto fills in the URL of an uploaded photo.
func (a *AppService) finishSitePhoto(ctx context.Context, photo *structs.SitePhoto) {
	if photo == nil || photo.FileId == nil {
		return
	}
	if photo.Filename != "" {
		photo.URL = siteFileURL(*photo.FileId, photo.Filename)
		return
	}
	if file, err := a.db.GetSiteFile(ctx, *photo.FileId); err == nil {
		photo.URL = siteFileURL(file.Id, file.Filename)
	}
}

func (a *AppService) finishPastEvent(ctx context.Context, ev *structs.SitePastEvent) {
	a.finishSitePhoto(ctx, ev.Cover)
	for i := range ev.Photos {
		a.finishSitePhoto(ctx, &ev.Photos[i])
	}
}

func publicSitePhoto(photo structs.SitePhoto, fallbackAlt string) structs.SitePublicImage {
	alt := photo.Alt
	if alt == "" {
		alt = fallbackAlt
	}
	return structs.SitePublicImage{URL: photo.URL, Width: photo.Width, Height: photo.Height, Alt: alt}
}

func toPublicPastEvent(ev *structs.SitePastEvent, withPhotos bool) structs.SitePublicPastEvent {
	out := structs.SitePublicPastEvent{Slug: ev.Slug, Title: ev.Title, Date: ev.Date, PhotoCount: len(ev.Photos)}
	if ev.Cover != nil && ev.Cover.URL != "" && ev.Cover.Width > 0 && ev.Cover.Height > 0 {
		cover := publicSitePhoto(*ev.Cover, ev.Title)
		out.Cover = &cover
	}
	if withPhotos {
		out.Description = ev.Description
		out.Photos = []structs.SitePublicPhoto{}
		for i, photo := range ev.Photos {
			if photo.URL == "" || photo.Width <= 0 || photo.Height <= 0 {
				continue
			}
			// The caption doubles as the description for screen readers.
			if photo.Caption != "" {
				photo.Alt = photo.Caption
			}
			out.Photos = append(out.Photos, structs.SitePublicPhoto{
				SitePublicImage: publicSitePhoto(photo, "Photo "+strconv.Itoa(i+1)+" from "+ev.Title),
				Caption:         photo.Caption,
			})
		}
	}
	return out
}

// GetPublicPastEvents lists the tiles, newest first.
func (a *AppService) GetPublicPastEvents(w http.ResponseWriter, r *http.Request) {
	events, err := a.db.ListPastEvents(r.Context(), false)
	if err != nil {
		a.siteServerError(w, "listing past events", err)
		return
	}
	out := make([]structs.SitePublicPastEvent, 0, len(events))
	for _, ev := range events {
		a.finishSitePhoto(r.Context(), ev.Cover)
		out = append(out, toPublicPastEvent(ev, false))
	}
	writeSiteJSON(w, http.StatusOK, map[string]any{"events": out})
}

// GetPublicPastEvent is one gallery page.
func (a *AppService) GetPublicPastEvent(w http.ResponseWriter, r *http.Request) {
	ev, err := a.db.GetPastEventBySlug(r.Context(), strings.TrimSpace(r.PathValue("slug")))
	if errors.Is(err, db.ErrSiteNotFound) {
		writeSiteError(w, http.StatusNotFound, "That event was not found.")
		return
	}
	if err != nil {
		a.siteServerError(w, "loading a past event", err)
		return
	}
	a.finishPastEvent(r.Context(), ev)
	writeSiteJSON(w, http.StatusOK, toPublicPastEvent(ev, true))
}

func sanitizePastEventRequest(req *structs.SitePastEventRequest) string {
	req.Title = strings.TrimSpace(req.Title)
	req.Date = strings.TrimSpace(req.Date)
	req.Description = strings.TrimSpace(req.Description)
	req.CoverAlt = strings.TrimSpace(req.CoverAlt)
	req.CoverFileId = strings.TrimSpace(req.CoverFileId)
	req.CoverPhotoId = strings.TrimSpace(req.CoverPhotoId)
	switch {
	case req.Title == "":
		return "Give the event a title."
	case tooLong(req.Title, 150):
		return "The title must be 150 characters or fewer."
	case tooLong(req.Description, 2000):
		return "The description must be 2,000 characters or fewer."
	case tooLong(req.CoverAlt, 200):
		return "The photo description must be 200 characters or fewer."
	}
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return "Choose the date the event took place."
	}
	if date.After(time.Now().In(sfluvTimeZone).AddDate(0, 0, 1)) {
		return "A past event's date can't be in the future."
	}
	return ""
}

// siteImage loads an upload that must be an image the site can show.
func (a *AppService) siteImage(ctx context.Context, fileId string) (*structs.SiteFile, bool) {
	file, err := a.db.GetSiteFile(ctx, fileId)
	if err != nil || !strings.HasPrefix(file.ContentType, "image/") || file.Width <= 0 || file.Height <= 0 {
		return nil, false
	}
	return file, true
}

// pastEventCover works out the new tile photo from a request: an upload, one
// of the gallery's photos, or nil to keep what is there.
func (a *AppService) pastEventCover(w http.ResponseWriter, ctx context.Context, req *structs.SitePastEventRequest, existing *structs.SitePastEvent) (*structs.SitePhoto, bool) {
	if req.CoverFileId != "" {
		file, ok := a.siteImage(ctx, req.CoverFileId)
		if !ok {
			writeSiteError(w, http.StatusBadRequest, "The tile's photo must be an image you uploaded. Upload it again.")
			return nil, false
		}
		return &structs.SitePhoto{FileId: &file.Id, Width: file.Width, Height: file.Height}, true
	}
	if req.CoverPhotoId != "" && existing != nil {
		for _, photo := range existing.Photos {
			if photo.Id == req.CoverPhotoId {
				cover := photo
				return &cover, true
			}
		}
		writeSiteError(w, http.StatusBadRequest, "That photo is no longer in the gallery.")
		return nil, false
	}
	return nil, true
}

func (a *AppService) GetAdminPastEvents(w http.ResponseWriter, r *http.Request) {
	events, err := a.db.ListPastEvents(r.Context(), true)
	if err != nil {
		a.siteServerError(w, "listing past events", err)
		return
	}
	for _, ev := range events {
		a.finishPastEvent(r.Context(), ev)
	}
	writeSiteJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (a *AppService) respondPastEvent(w http.ResponseWriter, r *http.Request, id string, status int) {
	ev, err := a.db.GetPastEvent(r.Context(), id)
	if err != nil {
		a.siteServerError(w, "reloading a past event", err)
		return
	}
	a.finishPastEvent(r.Context(), ev)
	writeSiteJSON(w, status, ev)
}

func (a *AppService) PostAdminPastEvent(w http.ResponseWriter, r *http.Request) {
	var req structs.SitePastEventRequest
	if !decodeSiteBody(w, r, &req) {
		return
	}
	if problem := sanitizePastEventRequest(&req); problem != "" {
		writeSiteError(w, http.StatusBadRequest, problem)
		return
	}
	req.CoverPhotoId = "" // a new event has no gallery yet
	cover, ok := a.pastEventCover(w, r.Context(), &req, nil)
	if !ok {
		return
	}
	id, err := a.db.CreatePastEvent(r.Context(), pastEventSlug(req.Title), &req, cover, utils.GetDid(r))
	if err != nil {
		a.siteServerError(w, "creating a past event", err)
		return
	}
	a.siteActivity(r, structs.SiteCapabilityPastEvents, "add", "past_event", id, "Added the past event “"+req.Title+"”")
	a.respondPastEvent(w, r, id, http.StatusCreated)
}

// loadAdminPastEvent fetches the event named in the path, answering 404 itself.
func (a *AppService) loadAdminPastEvent(w http.ResponseWriter, r *http.Request) (*structs.SitePastEvent, bool) {
	ev, err := a.db.GetPastEvent(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, db.ErrSiteNotFound) {
		writeSiteError(w, http.StatusNotFound, "That event no longer exists.")
		return nil, false
	}
	if err != nil {
		a.siteServerError(w, "loading a past event", err)
		return nil, false
	}
	return ev, true
}

func (a *AppService) PutAdminPastEvent(w http.ResponseWriter, r *http.Request) {
	ev, ok := a.loadAdminPastEvent(w, r)
	if !ok {
		return
	}
	var req structs.SitePastEventRequest
	if !decodeSiteBody(w, r, &req) {
		return
	}
	if problem := sanitizePastEventRequest(&req); problem != "" {
		writeSiteError(w, http.StatusBadRequest, problem)
		return
	}
	cover, ok := a.pastEventCover(w, r.Context(), &req, ev)
	if !ok {
		return
	}
	if err := a.db.UpdatePastEvent(r.Context(), ev.Id, &req, cover); err != nil {
		a.siteServerError(w, "updating a past event", err)
		return
	}
	a.siteActivity(r, structs.SiteCapabilityPastEvents, "edit", "past_event", ev.Id, "Edited the past event “"+req.Title+"”")
	a.respondPastEvent(w, r, ev.Id, http.StatusOK)
}

func (a *AppService) setPastEventRemoved(w http.ResponseWriter, r *http.Request, removed bool) {
	ev, ok := a.loadAdminPastEvent(w, r)
	if !ok {
		return
	}
	if err := a.db.SetPastEventRemoved(r.Context(), ev.Id, removed); err != nil {
		a.siteServerError(w, "updating a past event", err)
		return
	}
	verb, action := "Removed", "remove"
	if !removed {
		verb, action = "Restored", "restore"
	}
	a.siteActivity(r, structs.SiteCapabilityPastEvents, action, "past_event", ev.Id, verb+" the past event “"+ev.Title+"”")
	a.respondPastEvent(w, r, ev.Id, http.StatusOK)
}

func (a *AppService) DeleteAdminPastEvent(w http.ResponseWriter, r *http.Request) {
	a.setPastEventRemoved(w, r, true)
}

func (a *AppService) RestoreAdminPastEvent(w http.ResponseWriter, r *http.Request) {
	a.setPastEventRemoved(w, r, false)
}

func (a *AppService) PostAdminPastEventPhotos(w http.ResponseWriter, r *http.Request) {
	ev, ok := a.loadAdminPastEvent(w, r)
	if !ok {
		return
	}
	var req structs.SitePastEventPhotosRequest
	if !decodeSiteBody(w, r, &req) {
		return
	}
	if len(req.FileIds) == 0 {
		writeSiteError(w, http.StatusBadRequest, "Choose some photos to add.")
		return
	}
	if len(req.FileIds) > maxPastEventPhotosPerUpload {
		writeSiteError(w, http.StatusBadRequest, "Add up to 50 photos at a time.")
		return
	}
	if len(ev.Photos)+len(req.FileIds) > maxPastEventPhotos {
		writeSiteError(w, http.StatusBadRequest, "A gallery can hold up to 300 photos.")
		return
	}
	files := make([]*structs.SiteFile, 0, len(req.FileIds))
	for _, id := range req.FileIds {
		file, ok := a.siteImage(r.Context(), strings.TrimSpace(id))
		if !ok {
			writeSiteError(w, http.StatusBadRequest, "Every gallery photo must be an image you uploaded. Upload them again.")
			return
		}
		files = append(files, file)
	}
	if err := a.db.AddPastEventPhotos(r.Context(), ev.Id, files); err != nil {
		a.siteServerError(w, "adding past event photos", err)
		return
	}
	noun := "photos"
	if len(files) == 1 {
		noun = "photo"
	}
	a.siteActivity(r, structs.SiteCapabilityPastEvents, "add", "past_event", ev.Id,
		"Added "+strconv.Itoa(len(files))+" "+noun+" to “"+ev.Title+"”")
	a.respondPastEvent(w, r, ev.Id, http.StatusOK)
}

func (a *AppService) PutAdminPastEventPhotoOrder(w http.ResponseWriter, r *http.Request) {
	ev, ok := a.loadAdminPastEvent(w, r)
	if !ok {
		return
	}
	var req structs.SitePastEventOrderRequest
	if !decodeSiteBody(w, r, &req) {
		return
	}
	if err := a.db.ReorderPastEventPhotos(r.Context(), ev.Id, req.Order); err != nil {
		a.siteServerError(w, "reordering past event photos", err)
		return
	}
	a.siteActivity(r, structs.SiteCapabilityPastEvents, "edit", "past_event", ev.Id, "Reordered the photos of “"+ev.Title+"”")
	a.respondPastEvent(w, r, ev.Id, http.StatusOK)
}

func (a *AppService) PutAdminPastEventPhoto(w http.ResponseWriter, r *http.Request) {
	ev, ok := a.loadAdminPastEvent(w, r)
	if !ok {
		return
	}
	var req structs.SitePastEventPhotoRequest
	if !decodeSiteBody(w, r, &req) {
		return
	}
	req.Caption = strings.TrimSpace(req.Caption)
	if tooLong(req.Caption, 300) {
		writeSiteError(w, http.StatusBadRequest, "A caption must be 300 characters or fewer.")
		return
	}
	if err := a.db.UpdatePastEventPhotoCaption(r.Context(), ev.Id, strings.TrimSpace(r.PathValue("photoId")), req.Caption); err != nil {
		if errors.Is(err, db.ErrSiteNotFound) {
			writeSiteError(w, http.StatusNotFound, "That photo is no longer in the gallery.")
			return
		}
		a.siteServerError(w, "updating a past event photo", err)
		return
	}
	verb := "Changed a caption in"
	if req.Caption == "" {
		verb = "Removed a caption from"
	}
	a.siteActivity(r, structs.SiteCapabilityPastEvents, "edit", "past_event", ev.Id, verb+" “"+ev.Title+"”")
	a.respondPastEvent(w, r, ev.Id, http.StatusOK)
}

func (a *AppService) DeleteAdminPastEventPhoto(w http.ResponseWriter, r *http.Request) {
	ev, ok := a.loadAdminPastEvent(w, r)
	if !ok {
		return
	}
	if err := a.db.DeletePastEventPhoto(r.Context(), ev.Id, strings.TrimSpace(r.PathValue("photoId"))); err != nil {
		if errors.Is(err, db.ErrSiteNotFound) {
			writeSiteError(w, http.StatusNotFound, "That photo is no longer in the gallery.")
			return
		}
		a.siteServerError(w, "removing a past event photo", err)
		return
	}
	a.siteActivity(r, structs.SiteCapabilityPastEvents, "remove", "past_event", ev.Id, "Removed a photo from “"+ev.Title+"”")
	a.respondPastEvent(w, r, ev.Id, http.StatusOK)
}
