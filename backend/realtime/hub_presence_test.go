package realtime

import "testing"

type fakeSink struct{ id int }

func (fakeSink) send([]byte) {}
func (fakeSink) close()      {}

func TestPresenceAmongIsPerDevice(t *testing.T) {
	h := NewHub()
	phone, simulator, oldApp := &fakeSink{1}, &fakeSink{2}, &fakeSink{3}

	// One person with the app open on a simulator: their phone is not open.
	h.add("sanchez", "sim-install", simulator)
	p := h.PresenceAmong([]string{"sanchez", "nobody"})["sanchez"]
	if p.Unidentified || len(p.Installations) != 1 {
		t.Fatalf("want just the simulator, got %+v", p)
	}
	if _, ok := p.Installations["sim-install"]; !ok {
		t.Fatalf("simulator missing: %+v", p)
	}
	if _, ok := h.PresenceAmong([]string{"nobody"})["nobody"]; ok {
		t.Fatal("a user with no socket must be left out")
	}

	// Both open.
	h.add("sanchez", "phone-install", phone)
	if got := len(h.PresenceAmong([]string{"sanchez"})["sanchez"].Installations); got != 2 {
		t.Fatalf("want 2 open devices, got %d", got)
	}

	// An app older than 2.5.0 sends no id: the whole account counts as open.
	h.add("sanchez", "  ", oldApp)
	if !h.PresenceAmong([]string{"sanchez"})["sanchez"].Unidentified {
		t.Fatal("a socket without an installation id must mark the person unidentified")
	}

	// Closing them takes them out again.
	h.remove("sanchez", oldApp)
	h.remove("sanchez", simulator)
	p = h.PresenceAmong([]string{"sanchez"})["sanchez"]
	if p.Unidentified || len(p.Installations) != 1 || !p.Open() {
		t.Fatalf("want only the phone, got %+v", p)
	}
	h.remove("sanchez", phone)
	if (Presence{}).Open() || len(h.PresenceAmong([]string{"sanchez"})) != 0 {
		t.Fatal("no sockets should mean no presence")
	}
}
