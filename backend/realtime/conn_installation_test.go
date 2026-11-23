package realtime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The auth frame the app sends from 2.5.0 (src/services/realtime.ts) registers
// the socket under its installation id; an older app's frame, without one,
// still connects and counts as an unidentified device.
func TestAuthFrameCarriesInstallationId(t *testing.T) {
	hub := NewHub()
	server := NewServer(hub, func(token string) (string, error) { return "user-" + token, nil }, nil, func(string, ...any) {}, false)
	ts := httptest.NewServer(http.HandlerFunc(server.Serve))
	defer ts.Close()
	url := "ws" + strings.TrimPrefix(ts.URL, "http")

	dial := func(frame string) *websocket.Conn {
		ws, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := ws.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
			t.Fatal(err)
		}
		_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, msg, err := ws.ReadMessage(); err != nil || !strings.Contains(string(msg), `"ready"`) {
			t.Fatalf("no ready frame: %v %s", err, msg)
		}
		return ws
	}

	phone := dial(`{"kind":"auth","token":"a","installation_id":"install-123"}`)
	defer phone.Close()
	p := hub.PresenceAmong([]string{"user-a"})["user-a"]
	if _, ok := p.Installations["install-123"]; !ok || p.Unidentified {
		t.Fatalf("want install-123 registered, got %+v", p)
	}

	old := dial(`{"kind":"auth","token":"b"}`)
	defer old.Close()
	if !hub.PresenceAmong([]string{"user-b"})["user-b"].Unidentified {
		t.Fatal("an auth frame without an installation id must count as unidentified")
	}
}
