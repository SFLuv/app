package router

import (
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5/middleware"
)

// validRequestID is what a client may supply as X-Request-Id: the mobile app
// sends 16 hex characters. Anything else is discarded and chi generates one,
// so a caller cannot put arbitrary text into the request log.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// requestID tags every request with an id that the access log prints as its
// "[id]" prefix and the response returns as X-Request-Id.
//
// The mobile app sends its own id with each request and attaches it to any
// Sentry report about that request, so an app error can be found in the
// Railway logs by searching for the id. Requests without one get chi's
// generated id, which is still printed and returned.
func requestID(next http.Handler) http.Handler {
	assign := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := middleware.GetReqID(r.Context()); id != "" {
			w.Header().Set(middleware.RequestIDHeader, id)
		}
		next.ServeHTTP(w, r)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := r.Header.Get(middleware.RequestIDHeader); id != "" && !validRequestID.MatchString(id) {
			r.Header.Del(middleware.RequestIDHeader)
		}
		assign.ServeHTTP(w, r)
	})
}
