package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SFLuv/app/backend/db"
)

// ── Pictures for a notification ─────────────────────────────────────────────
//
// iOS draws a chat push the way Messages does — the sender's picture, or the
// group's, with SFLuv's icon in the corner — only if the app's Notification
// Service Extension hands it the picture. That extension runs in its own
// process, with no access to the app's session, so it cannot use the
// authenticated /users/{id}/avatar or attachment routes.
//
// So the push carries short-lived signed URLs instead, and this route serves
// them without a session. A URL grants exactly one picture, until it expires:
//
//   - a person's profile picture, which any signed-in member can already see;
//   - a group's picture, re-checked against the RECIPIENT's membership when it
//     is fetched — the signature names who the push was for, so somebody who
//     left the group between the push and opening it gets nothing.
//
// Nothing else is reachable through it, and a forged or expired URL is a 404.

// pushImageTTL is long enough for a push that sat undelivered on a phone that
// was off over a weekend, and no longer.
const pushImageTTL = 72 * time.Hour

var (
	pushImageKeyOnce sync.Once
	pushImageKey     []byte
)

// pushImageSecret is PUSH_IMAGE_SECRET if set. Otherwise it is derived from
// PONDER_KEY, which every instance of this server already shares, under a
// label of its own so the two can never be confused for each other. Failing
// both it is random per process: URLs then stop working across a restart or
// between instances, and the extension falls back to initials — degraded, not
// broken.
func pushImageSecret() []byte {
	pushImageKeyOnce.Do(func() {
		if v := strings.TrimSpace(os.Getenv("PUSH_IMAGE_SECRET")); v != "" {
			pushImageKey = []byte(v)
			return
		}
		if v := strings.TrimSpace(os.Getenv("PONDER_KEY")); v != "" {
			mac := hmac.New(sha256.New, []byte(v))
			mac.Write([]byte("sfluv push images v1"))
			pushImageKey = mac.Sum(nil)
			return
		}
		pushImageKey = make([]byte, 32)
		_, _ = rand.Read(pushImageKey)
	})
	return pushImageKey
}

func signPushImage(parts ...string) string {
	mac := hmac.New(sha256.New, pushImageSecret())
	mac.Write([]byte(strings.Join(parts, "\x00")))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// publicBackendBase is where this server is reachable from a phone, resolved
// the same way as the W-9 return URL. Empty when unconfigured, and then no
// picture URLs are sent at all: a relative URL is useless to the extension.
func publicBackendBase() string {
	base := strings.TrimSpace(envString("PUBLIC_BACKEND_URL", ""))
	if base == "" {
		base = strings.TrimSpace(envString("NEXT_PUBLIC_BACKEND_URL", ""))
	}
	return strings.TrimSuffix(base, "/")
}

// pushAvatarURL is a signed URL for a person's profile picture, or "".
func pushAvatarURL(userId string) string {
	base := publicBackendBase()
	if base == "" || userId == "" {
		return ""
	}
	exp := strconv.FormatInt(time.Now().Add(pushImageTTL).Unix(), 10)
	q := url.Values{}
	q.Set("k", "u")
	q.Set("id", userId)
	q.Set("exp", exp)
	q.Set("sig", signPushImage("u", userId, exp))
	return base + "/push-image?" + q.Encode()
}

// pushGroupIconURL is a signed URL for a group's picture, valid only while
// recipientId is a member, or "".
func pushGroupIconURL(attachmentId int64, recipientId string) string {
	base := publicBackendBase()
	if base == "" || attachmentId <= 0 || recipientId == "" {
		return ""
	}
	id := strconv.FormatInt(attachmentId, 10)
	exp := strconv.FormatInt(time.Now().Add(pushImageTTL).Unix(), 10)
	q := url.Values{}
	q.Set("k", "g")
	q.Set("id", id)
	q.Set("r", recipientId)
	q.Set("exp", exp)
	q.Set("sig", signPushImage("g", id, recipientId, exp))
	return base + "/push-image?" + q.Encode()
}

// GetPushImage serves a signed notification picture. Unauthenticated by
// design — see the note at the top of this file — so every refusal is the
// same bare 404, and nothing distinguishes "bad signature" from "no picture".
func (a *AppService) GetPushImage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind, id, recipient, exp, sig := q.Get("k"), q.Get("id"), q.Get("r"), q.Get("exp"), q.Get("sig")

	expires, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > expires || id == "" || sig == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var want string
	switch kind {
	case "u":
		want = signPushImage("u", id, exp)
	case "g":
		want = signPushImage("g", id, recipient, exp)
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if kind == "u" {
		contentType, data, err := a.db.GetProfileAvatar(r.Context(), id)
		if err != nil {
			if !errors.Is(err, db.ErrAvatarNotFound) {
				a.logger.Logf("error loading a push avatar for %s: %s", id, err)
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writePushImage(w, contentType, data)
		return
	}

	attachmentId, err := strconv.ParseInt(id, 10, 64)
	if err != nil || attachmentId <= 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	blob, err := a.db.GetAttachmentBlobForMember(r.Context(), attachmentId, recipient)
	if err != nil || !strings.HasPrefix(blob.ContentType, "image/") {
		if err != nil && !errors.Is(err, db.ErrAttachmentNotFound) {
			a.logger.Logf("error loading a push group icon %d: %s", attachmentId, err)
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writePushImage(w, blob.ContentType, blob.Data)
}

// writePushImage is writeImage with a short cache. The URL is only fetched by
// the extension that received it, and writeImage's day-long max-age would let a
// device keep showing a group's picture for a day after leaving it, without
// the membership re-check ever running again.
func writePushImage(w http.ResponseWriter, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
