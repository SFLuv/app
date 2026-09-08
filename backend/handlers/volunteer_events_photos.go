package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
)

// stagedPhotoTTL is how long an unclaimed photo survives.
//
// Long enough that nobody loses work to a slow form — someone can pick photos,
// take a call, and still submit — and short enough that a form abandoned in a
// closed tab does not leave its bytes in Postgres indefinitely.
const stagedPhotoTTL = 12 * time.Hour

// StageVolunteerEventPhoto accepts a cover photo before its event exists.
//
// Open to any signed-in user, which sounds broader than it is: staging only
// parks bytes under the uploader's own name. Nothing becomes visible until an
// event claims it, and an event can only claim photos the same user staged, so
// the authorisation that matters is on event creation where it already lives.
func (a *AppService) StageVolunteerEventPhoto(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("the events service is unavailable right now, please try again shortly"))
		return
	}

	if err := r.ParseMultipartForm(maxEventPhotoBytes); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("could not read the upload"))
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("photo")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("a photo file is required"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxEventPhotoBytes+1))
	if err != nil || len(data) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("that photo file was empty"))
		return
	}
	if len(data) > maxEventPhotoBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte(fmt.Sprintf("photos must be %d MB or smaller", maxEventPhotoBytes>>20)))
		return
	}

	contentType := resolvePartnerLogoContentType(data, header.Filename)
	if contentType == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("cover photos must be a PNG, JPEG, GIF, WebP, or SVG image"))
		return
	}

	width, height := partnerLogoDimensions(data, contentType)

	photoId, err := a.bot.db.StageVolunteerEventPhoto(r.Context(), *userDid, data, contentType, header.Filename, width, height)
	if err != nil {
		a.logger.Logf("error staging event cover photo for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("could not save the photo, please try again"))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": photoId})
}

// DeleteStagedVolunteerEventPhoto drops a staged photo the uploader removed
// from the form before submitting.
func (a *AppService) DeleteStagedVolunteerEventPhoto(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.bot == nil || a.bot.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	photoId := strings.TrimSpace(chi.URLParam(r, "photo_id"))
	if photoId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.bot.db.DeleteStagedVolunteerEventPhoto(r.Context(), photoId, *userDid); err != nil {
		a.logger.Logf("error deleting staged event photo %s: %s", photoId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ExpireStagedEventPhotos clears staged photos no event ever claimed. Run from
// the maintenance sweep alongside the other abandonment cleanups.
func (a *AppService) ExpireStagedEventPhotos(ctx context.Context) {
	if a.bot == nil || a.bot.db == nil {
		return
	}

	cutoff := time.Now().Add(-stagedPhotoTTL).Unix()
	removed, err := a.bot.db.ExpireStagedVolunteerEventPhotos(ctx, cutoff)
	if err != nil {
		a.logger.Logf("error expiring staged event photos: %s", err)
		return
	}
	if removed > 0 {
		a.logger.Logf("cleared %d staged event photo(s) that were never attached", removed)
	}
}

// validateStagedPhotoIds rejects a photo list that could never be attached,
// before any event is created from it.
func validateStagedPhotoIds(ids []string) (cleaned []string, errMsg string) {
	cleaned = make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))

	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, duplicate := seen[trimmed]; duplicate {
			// The same photo twice would take two of the six slots and render
			// as a duplicate slide.
			return nil, "the same photo was submitted twice"
		}
		seen[trimmed] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}

	if len(cleaned) > maxEventPhotos {
		return nil, fmt.Sprintf("an event can have at most %d cover photos", maxEventPhotos)
	}

	return cleaned, ""
}
