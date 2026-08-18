package test

import (
	"net/http"
	"testing"
)

func GroupW9Handlers(t *testing.T) {
	t.Run("w9 endpoints require a session", ModuleW9RequiresAuth)
	t.Run("the old public submit endpoint is gone", ModuleW9SubmitIsGone)
}

// The replacement for a test that used to assert the opposite.
//
// POST /w9/submit was unauthenticated, so anyone could file a submission for
// any wallet with any email — and that email then received the approval notice.
// The new endpoints are session-scoped: a person can only ever act on their own
// filing.
func ModuleW9RequiresAuth(t *testing.T) {
	Spoofer.SetValue("userDid", nil)
	defer Spoofer.SetValue("userDid", TEST_USER_1.Id)

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/w9/status"},
		{http.MethodPost, "/w9/start"},
	} {
		req, err := http.NewRequest(route.method, TestServer.URL+route.path, nil)
		if err != nil {
			t.Fatalf("error building %s %s: %s", route.method, route.path, err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("error calling %s %s: %s", route.method, route.path, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s returned %d without a session; want 401 or 403",
				route.method, route.path, res.StatusCode)
		}
	}
}

func ModuleW9SubmitIsGone(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, TestServer.URL+"/w9/submit", nil)
	if err != nil {
		t.Fatalf("error building the request: %s", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error calling /w9/submit: %s", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("/w9/submit still answers with %d; the unauthenticated submission route should be gone", res.StatusCode)
	}
}
