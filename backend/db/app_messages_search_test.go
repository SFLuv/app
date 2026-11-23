package db

import (
	"reflect"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func TestEscapeLikePattern(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello", "hello"},
		{"50%", `50\%`},
		{"a_b", `a\_b`},
		{`back\slash`, `back\\slash`},
		// The backslash is escaped first, so the escapes added for % and _ are
		// not themselves doubled.
		{`\%_`, `\\\%\_`},
		{"%%", `\%\%`},
		{"café 👋", "café 👋"},
	}
	for _, c := range cases {
		if got := escapeLikePattern(c.in); got != c.want {
			t.Errorf("escapeLikePattern(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMentionsRoundTrip(t *testing.T) {
	if got := encodeMentions(nil); got != nil {
		t.Errorf("nil mentions should store NULL, got %q", *got)
	}
	if got := encodeMentions([]structs.MessageMention{}); got != nil {
		t.Errorf("empty mentions should store NULL, got %q", *got)
	}
	in := []structs.MessageMention{{UserId: "maria", Offset: 3, Length: 6}}
	encoded := encodeMentions(in)
	if encoded == nil || *encoded != `[{"user_id":"maria","offset":3,"length":6}]` {
		t.Fatalf("unexpected encoding: %v", encoded)
	}
	if got := decodeMentions(encoded); !reflect.DeepEqual(got, in) {
		t.Errorf("round trip: got %+v, want %+v", got, in)
	}
	if got := decodeMentions(nil); got != nil {
		t.Errorf("NULL should decode to nil, got %+v", got)
	}
	bad := "not json"
	if got := decodeMentions(&bad); got != nil {
		t.Errorf("unparseable column should decode to nil, got %+v", got)
	}
	empty := "[]"
	if got := decodeMentions(&empty); got != nil {
		t.Errorf("[] should decode to nil, got %+v", got)
	}
}
