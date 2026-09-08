package handlers

import (
	"fmt"
	"io"
	"net/http"

	"github.com/SFLuv/app/backend/db"
	"github.com/jackc/pgx/v5"
)

// A storefront photo is shown at card width rather than inside a map pin, so
// the cap is well above the icon's. It matches the event cover photo limit:
// enough for a photo straight off a phone, and the web uploader re-encodes its
// crop to a JPEG far below it.
const maxLocationPhotoBytes = 8 << 20 // 8 MiB

// UploadLocationPhoto stores the picture shown on this merchant's listing.
//
// Aspect is enforced by the client, which crops before uploading. Nothing here
// decodes the image to check: the renderers all cover-fit the result, so an
// unusual aspect is cropped for display rather than distorted, and decoding
// every upload to reject something no client produces buys nothing.
func (a *AppService) UploadLocationPhoto(w http.ResponseWriter, r *http.Request) {
	locationID, ok := locationIDFromPath(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A valid location id is required."})
		return
	}

	allowed, err := a.canEditLocationImage(r, locationID)
	if err != nil {
		a.logger.Logf("error checking photo permission for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !allowed {
		// 404 rather than 403, matching the icon and hours endpoints: probing
		// ids should not reveal which locations exist.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "That location could not be found."})
		return
	}

	if err := r.ParseMultipartForm(maxLocationPhotoBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Could not read the upload."})
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, _, err := r.FormFile("photo")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A photo file is required."})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxLocationPhotoBytes+1))
	if err != nil || len(data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That photo file was empty."})
		return
	}
	if len(data) > maxLocationPhotoBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": fmt.Sprintf("Photos must be %d MB or smaller.", maxLocationPhotoBytes>>20),
		})
		return
	}

	// Same raster-only sniff as the icon, and for the same reason: an SVG is an
	// active document, and this one is served to three public clients from an
	// untrusted merchant upload.
	contentType := locationIconContentType(data)
	if contentType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Photos must be a PNG, JPEG, or WebP image."})
		return
	}

	updatedAt, err := a.db.SetLocationPhoto(r.Context(), locationID, data, contentType)
	if err != nil {
		a.logger.Logf("error storing photo for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"photo_url": db.LocationPhotoURL(locationID, updatedAt),
	})
}

// DeleteLocationPhoto drops the upload, returning the listing to showing no
// picture.
func (a *AppService) DeleteLocationPhoto(w http.ResponseWriter, r *http.Request) {
	locationID, ok := locationIDFromPath(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A valid location id is required."})
		return
	}

	allowed, err := a.canEditLocationImage(r, locationID)
	if err != nil {
		a.logger.Logf("error checking photo permission for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !allowed {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "That location could not be found."})
		return
	}

	if err := a.db.DeleteLocationPhoto(r.Context(), locationID); err != nil {
		a.logger.Logf("error deleting photo for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetLocationPhoto serves the stored bytes.
//
// Public for the same reason the icon is: the merchant listing is public on
// three surfaces including the marketing site, and a picture of the shopfront
// is exactly as public as the shopfront. The version stamp in the URL is what
// makes a replacement visible despite the long cache lifetime.
func (a *AppService) GetLocationPhoto(w http.ResponseWriter, r *http.Request) {
	locationID, ok := locationIDFromPath(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	photo, err := a.db.GetLocationPhoto(r.Context(), locationID)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading photo for location %d: %s", locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", photo.ContentType)
	if r.URL.Query().Get("v") != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(photo.Data)
}
