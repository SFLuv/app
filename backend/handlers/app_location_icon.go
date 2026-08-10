package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// A map icon is a small square mark. The cap is generous for a 512px PNG and
// still far below anything that would make the map slow to paint.
const maxLocationIconBytes = 2 << 20 // 2 MiB

// locationIconContentType sniffs the real type of the uploaded bytes.
//
// Raster only, unlike the partner-logo uploader: these are drawn inside a map
// pin on three clients, and an SVG is an active document that one of those
// clients would be rendering from an untrusted merchant upload. The web
// uploader re-encodes its crop to PNG, so this is a floor rather than a
// restriction anyone should hit.
func locationIconContentType(data []byte) string {
	switch sniffed := http.DetectContentType(data); {
	case strings.HasPrefix(sniffed, "image/png"):
		return "image/png"
	case strings.HasPrefix(sniffed, "image/jpeg"):
		return "image/jpeg"
	case strings.HasPrefix(sniffed, "image/webp"):
		return "image/webp"
	}
	return ""
}

// canEditLocationIcon reports whether the caller may change this listing's
// icon. Admins can, because they field the support requests when a merchant
// uploads something wrong and cannot fix it themselves.
func (a *AppService) canEditLocationIcon(r *http.Request, locationID uint) (bool, error) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		return false, nil
	}

	owned, err := a.db.UserOwnsLocation(r.Context(), *userDid, locationID)
	if err != nil {
		return false, err
	}
	if owned {
		return true, nil
	}

	return a.IsAdmin(r.Context(), *userDid), nil
}

func locationIDFromPath(r *http.Request) (uint, bool) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}

// UploadLocationIcon stores the mark shown on this merchant's map pin.
//
// Squareness is enforced by the client, which crops before uploading — the pin
// draws the image inside a circle, so a non-square upload is centre-cropped by
// the renderer rather than distorted. Validating dimensions here would mean
// decoding every upload to reject something no client can produce and no
// viewer would see.
func (a *AppService) UploadLocationIcon(w http.ResponseWriter, r *http.Request) {
	locationID, ok := locationIDFromPath(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A valid location id is required."})
		return
	}

	allowed, err := a.canEditLocationIcon(r, locationID)
	if err != nil {
		a.logger.Logf("error checking icon permission for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !allowed {
		// 404 rather than 403, matching the hours endpoint: probing ids should
		// not reveal which locations exist.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "That location could not be found."})
		return
	}

	if err := r.ParseMultipartForm(maxLocationIconBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Could not read the upload."})
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, _, err := r.FormFile("icon")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "An icon file is required."})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxLocationIconBytes+1))
	if err != nil || len(data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That icon file was empty."})
		return
	}
	if len(data) > maxLocationIconBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": fmt.Sprintf("Icons must be %d MB or smaller.", maxLocationIconBytes>>20),
		})
		return
	}

	contentType := locationIconContentType(data)
	if contentType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Icons must be a PNG, JPEG, or WebP image."})
		return
	}

	updatedAt, err := a.db.SetLocationIcon(r.Context(), locationID, data, contentType)
	if err != nil {
		a.logger.Logf("error storing icon for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"icon_url": db.LocationIconURL(locationID, updatedAt),
	})
}

// DeleteLocationIcon drops the upload, returning the pin to the generated
// initials mark.
func (a *AppService) DeleteLocationIcon(w http.ResponseWriter, r *http.Request) {
	locationID, ok := locationIDFromPath(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A valid location id is required."})
		return
	}

	allowed, err := a.canEditLocationIcon(r, locationID)
	if err != nil {
		a.logger.Logf("error checking icon permission for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !allowed {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "That location could not be found."})
		return
	}

	if err := a.db.DeleteLocationIcon(r.Context(), locationID); err != nil {
		a.logger.Logf("error deleting icon for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetLocationIcon serves the stored bytes.
//
// Public, because the merchant map is public on three surfaces including the
// marketing site, and because the icon is exactly as public as the shopfront
// it represents. Cached hard: a map repaints these on every pan, and the
// version stamp in the URL is what makes a replacement visible immediately.
func (a *AppService) GetLocationIcon(w http.ResponseWriter, r *http.Request) {
	locationID, ok := locationIDFromPath(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	icon, err := a.db.GetLocationIcon(r.Context(), locationID)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading icon for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", icon.ContentType)
	if r.URL.Query().Get("v") != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		// Without a version stamp the URL cannot distinguish revisions, so it
		// must be revalidated rather than pinned.
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(icon.Data)
}
