package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
)

const (
	contactNotesMaxRunes  = 2000
	contactAvatarMaxBytes = 4 << 20
)

// The contact card: one contact, its notes, and its picture.
//
// Everything here is private to the contact's owner. A contact is somebody's own
// note about somebody else — the name they chose for them, what they wrote about
// them, the photo they picked — and none of it is visible to the person it
// describes. That is why the picture lives in contact_avatars rather than being
// confused with the profile picture the contact chose for themselves, which is
// theirs and is public.

// GetContactDetail is one contact card.
func (a *AppService) GetContactDetail(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	contactId, ok := contactIdFromPath(r)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	contact, err := a.db.GetContact(r.Context(), contactId, *userDid)
	if errors.Is(err, db.ErrContactNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		a.logger.Logf("error loading contact %d for %s: %s", contactId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, contact)
}

// PatchContactDetail saves part of a contact card.
//
// A patch rather than a put, because the detail screen saves one field at a time
// as it is edited — and a put would mean a half-filled screen could blank the
// notes somebody wrote on another device.
func (a *AppService) PatchContactDetail(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	contactId, ok := contactIdFromPath(r)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var patch structs.ContactPatch
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&patch); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			http.Error(w, "a contact needs a name", http.StatusBadRequest)
			return
		}
		patch.Name = &name
	}
	if patch.Notes != nil {
		notes := strings.TrimSpace(*patch.Notes)
		if utf8.RuneCountInString(notes) > contactNotesMaxRunes {
			http.Error(w, "notes can be up to 2,000 characters", http.StatusBadRequest)
			return
		}
		patch.Notes = &notes
	}
	if patch.Address != nil {
		address := strings.TrimSpace(*patch.Address)
		if address == "" {
			http.Error(w, "a contact needs an address", http.StatusBadRequest)
			return
		}
		patch.Address = &address
	}

	if err := a.db.PatchContact(r.Context(), contactId, *userDid, &patch); err != nil {
		if errors.Is(err, db.ErrContactNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error updating contact %d for %s: %s", contactId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	contact, err := a.db.GetContact(r.Context(), contactId, *userDid)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, contact)
}

// PutContactAvatar stores a contact's picture.
func (a *AppService) PutContactAvatar(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	contactId, ok := contactIdFromPath(r)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	contentType, data, ok := readUploadedImage(w, r, contactAvatarMaxBytes)
	if !ok {
		return
	}

	if err := a.db.SetContactAvatar(r.Context(), contactId, *userDid, contentType, data); err != nil {
		if errors.Is(err, db.ErrContactNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error storing the picture for contact %d: %s", contactId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetContactAvatar serves a contact's picture to its owner.
func (a *AppService) GetContactAvatar(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	contactId, ok := contactIdFromPath(r)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	contentType, data, err := a.db.GetContactAvatar(r.Context(), contactId, *userDid)
	if errors.Is(err, db.ErrContactNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		a.logger.Logf("error loading the picture for contact %d: %s", contactId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeImage(w, contentType, data)
}

// DeleteContactAvatar removes a contact's picture.
func (a *AppService) DeleteContactAvatar(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	contactId, ok := contactIdFromPath(r)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err := a.db.DeleteContactAvatar(r.Context(), contactId, *userDid); err != nil {
		a.logger.Logf("error removing the picture for contact %d: %s", contactId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Profile pictures ─────────────────────────────────────────────────────────

// PutMyProfileAvatar stores the caller's own profile picture.
func (a *AppService) PutMyProfileAvatar(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	contentType, data, ok := readUploadedImage(w, r, contactAvatarMaxBytes)
	if !ok {
		return
	}
	if err := a.db.SetProfileAvatar(r.Context(), userId, contentType, data); err != nil {
		a.logger.Logf("error storing the profile picture for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteMyProfileAvatar removes the caller's profile picture.
func (a *AppService) DeleteMyProfileAvatar(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	if err := a.db.DeleteProfileAvatar(r.Context(), userId); err != nil {
		a.logger.Logf("error removing the profile picture for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetUserAvatar serves somebody's profile picture.
//
// Readable by any signed-in user, matching the public name it sits beside: both
// are what a person chose to show in threads and on posts, and gating the
// picture more tightly than the name would protect nothing while breaking every
// list that shows both.
func (a *AppService) GetUserAvatar(w http.ResponseWriter, r *http.Request) {
	if _, ok := bulletinViewer(w, r); !ok {
		return
	}
	targetId := userIdParam(r)
	if targetId == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	contentType, data, err := a.db.GetProfileAvatar(r.Context(), targetId)
	if errors.Is(err, db.ErrAvatarNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		a.logger.Logf("error loading the profile picture for %s: %s", targetId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeImage(w, contentType, data)
}

// ── Shared plumbing ──────────────────────────────────────────────────────────

func contactIdFromPath(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	return id, err == nil && id > 0
}

// readUploadedImage reads a multipart image upload, writing the refusal itself.
//
// Raster only, sniffed from the bytes rather than trusted from the extension or
// the declared type: these are served back to other devices, and an SVG is an
// active document.
func readUploadedImage(w http.ResponseWriter, r *http.Request, maxBytes int64) (string, []byte, bool) {
	if err := r.ParseMultipartForm(maxBytes + (1 << 20)); err != nil {
		http.Error(w, "could not read the upload", http.StatusBadRequest)
		return "", nil, false
	}
	defer r.MultipartForm.RemoveAll()

	file, _, err := r.FormFile("photo")
	if err != nil {
		file, _, err = r.FormFile("file")
	}
	if err != nil {
		http.Error(w, "an image file is required", http.StatusBadRequest)
		return "", nil, false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || len(data) == 0 {
		http.Error(w, "that image file was empty", http.StatusBadRequest)
		return "", nil, false
	}
	if int64(len(data)) > maxBytes {
		http.Error(w, fmt.Sprintf("that image is too large (limit %d MB)", maxBytes>>20),
			http.StatusRequestEntityTooLarge)
		return "", nil, false
	}
	contentType := locationIconContentType(data)
	if contentType == "" {
		http.Error(w, "images must be a PNG, JPEG, or WebP", http.StatusBadRequest)
		return "", nil, false
	}
	return contentType, data, true
}

// writeImage serves stored image bytes.
func writeImage(w http.ResponseWriter, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
