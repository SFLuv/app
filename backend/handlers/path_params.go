package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

// userIdParam reads the {userId} path parameter, decoded.
//
// A user id is a Privy DID, `did:privy:…`, and the mobile app encodes it into
// the path, so the colons arrive as %3A. chi matches routes against the raw,
// still-encoded path whenever the request has one, which means chi.URLParam
// hands back `did%3Aprivy%3A…`, an id no user has: every lookup by it answered
// 404, and chat headers showed no name or photo.
func userIdParam(r *http.Request) string {
	raw := chi.URLParam(r, "userId")
	if decoded, err := url.PathUnescape(raw); err == nil {
		raw = decoded
	}
	return strings.TrimSpace(raw)
}
