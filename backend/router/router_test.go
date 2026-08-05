package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// newTestRouter builds the real route table with nil services. No handler body
// runs for the assertions below: guarded routes reject before touching a
// service, and the route-table walk never invokes handlers.
func newTestRouter(t *testing.T) *chi.Mux {
	t.Helper()
	t.Setenv("IN_PRODUCTION", "true") // skip dev-only prank middleware
	t.Setenv("ADMIN_KEY", "")         // no X-Admin-Key bypass
	return New(nil, nil, nil, nil)
}

// The volunteer portal's public reads must not live under /events, which is an
// admin-guarded subtree. Keeping them on a separate prefix is what stops a
// future r.Route("/events", ...) refactor from silently flipping the
// public/private boundary in either direction.
func TestVolunteerRoutesAreNotUnderAdminEventsPrefix(t *testing.T) {
	router := newTestRouter(t)

	registered := map[string]bool{}
	err := chi.Walk(router, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		registered[method+" "+strings.TrimSuffix(route, "/")] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walking routes: %s", err)
	}

	expected := []string{
		"GET /volunteer-events",
		"GET /volunteer-events/organizers",
		"GET /volunteer-events/photos/{photo_id}",
		"GET /volunteer-events/{id}",
		"GET /organizers/{id}/logo",
	}
	for _, route := range expected {
		if !registered[route] {
			t.Errorf("expected public volunteer route %q to be registered", route)
		}
	}

	for route := range registered {
		_, path, _ := strings.Cut(route, " ")
		if strings.HasPrefix(path, "/events/volunteer") {
			t.Errorf("public volunteer route %q must not live under the admin-guarded /events prefix", route)
		}
	}
}

// The other half of the boundary: the admin event routes must stay guarded.
// An unauthenticated caller is rejected before any handler or service is
// touched, which is why this passes with nil services.
func TestAdminEventRoutesRejectUnauthenticated(t *testing.T) {
	router := newTestRouter(t)

	for _, target := range []string{"/events", "/events/some-event-id"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusForbidden {
			t.Errorf("GET %s without credentials = %d, want %d", target, recorder.Code, http.StatusForbidden)
		}
	}
}

// The public reads must NOT be credential-gated. A guarded route short-circuits
// with 403 before reaching its handler; an ungated one reaches the handler,
// which panics here only because the service is nil. So "not 403" is the
// assertion, and reaching the handler is the proof the request was let through.
func TestVolunteerReadsAreNotCredentialGated(t *testing.T) {
	router := newTestRouter(t)

	for _, target := range []string{"/volunteer-events", "/volunteer-events/some-event-id"} {
		t.Run(target, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, target, nil)
			recorder := httptest.NewRecorder()

			reached := false
			func() {
				defer func() {
					// A panic means the nil service was dereferenced, i.e. the
					// handler ran and no guard blocked the anonymous request.
					if recover() != nil {
						reached = true
					}
				}()
				router.ServeHTTP(recorder, request)
			}()

			if !reached && recorder.Code == http.StatusForbidden {
				t.Fatalf("GET %s is credential-gated; public volunteer reads must be anonymous", target)
			}
		})
	}
}
