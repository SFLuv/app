package handlers

import (
	"reflect"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func TestUtf16Len(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"hello", 5},
		{"café", 4}, // é is one BMP code unit, two UTF-8 bytes
		{"👋", 2},    // outside the BMP: a surrogate pair
		{"👋🏽", 4},   // emoji plus skin-tone modifier, both astral
		{"日本語", 3},  // BMP, three bytes each in UTF-8
		{"a👋b", 4},
	}
	for _, c := range cases {
		if got := utf16Len(c.in); got != c.want {
			t.Errorf("utf16Len(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestNormalizeMentions(t *testing.T) {
	members := map[string]struct{}{"maria": {}, "jo": {}, "sam": {}}
	m := func(user string, offset, length int) structs.MessageMention {
		return structs.MessageMention{UserId: user, Offset: offset, Length: length}
	}

	cases := []struct {
		name string
		body string
		in   []structs.MessageMention
		want []structs.MessageMention
	}{
		{
			name: "nothing requested",
			body: "hello",
			in:   nil,
			want: nil,
		},
		{
			name: "plain mention kept",
			body: "@Maria are you coming?",
			in:   []structs.MessageMention{m("maria", 0, 6)},
			want: []structs.MessageMention{m("maria", 0, 6)},
		},
		{
			// "👋 " is three UTF-16 units, so @Maria starts at 3 — at 2 in
			// runes and 5 in bytes, both of which would be wrong.
			name: "offset after an emoji counts UTF-16 units",
			body: "👋 @Maria",
			in:   []structs.MessageMention{m("maria", 3, 6)},
			want: []structs.MessageMention{m("maria", 3, 6)},
		},
		{
			name: "ending exactly at the end of a body with emoji is in range",
			body: "👋👋 @Jo",
			in:   []structs.MessageMention{m("jo", 5, 3)},
			want: []structs.MessageMention{m("jo", 5, 3)},
		},
		{
			name: "one unit past the end is dropped",
			body: "👋👋 @Jo",
			in:   []structs.MessageMention{m("jo", 5, 4)},
			want: nil,
		},
		{
			name: "non-member dropped, member kept",
			body: "@Stranger and @Jo",
			in:   []structs.MessageMention{m("stranger", 0, 9), m("jo", 14, 3)},
			want: []structs.MessageMention{m("jo", 14, 3)},
		},
		{
			name: "negative offset, zero and negative length dropped",
			body: "@Maria @Jo",
			in:   []structs.MessageMention{m("maria", -1, 6), m("jo", 7, 0), m("sam", 0, -3)},
			want: nil,
		},
		{
			name: "overlap with an earlier kept entry dropped",
			body: "@Maria @Jo",
			in:   []structs.MessageMention{m("maria", 0, 6), m("jo", 3, 5), m("jo", 7, 3)},
			want: []structs.MessageMention{m("maria", 0, 6), m("jo", 7, 3)},
		},
		{
			name: "touching but not overlapping is fine",
			body: "@Maria@Jo",
			in:   []structs.MessageMention{m("maria", 0, 6), m("jo", 6, 3)},
			want: []structs.MessageMention{m("maria", 0, 6), m("jo", 6, 3)},
		},
		{
			name: "exact duplicate dropped",
			body: "@Maria hi",
			in:   []structs.MessageMention{m("maria", 0, 6), m("maria", 0, 6)},
			want: []structs.MessageMention{m("maria", 0, 6)},
		},
		{
			name: "result sorted by offset",
			body: "@Maria @Jo",
			in:   []structs.MessageMention{m("jo", 7, 3), m("maria", 0, 6)},
			want: []structs.MessageMention{m("maria", 0, 6), m("jo", 7, 3)},
		},
		{
			// The client counted against what it sent; the server stores the
			// trimmed body, so offsets move left by the trimmed prefix.
			name: "leading whitespace the server trims shifts offsets",
			body: "\n  @Maria hi  ",
			in:   []structs.MessageMention{m("maria", 3, 6)},
			want: []structs.MessageMention{m("maria", 0, 6)},
		},
		{
			name: "an offset into the trimmed prefix is dropped",
			body: "  @Maria",
			in:   []structs.MessageMention{m("maria", 0, 3)},
			want: nil,
		},
		{
			name: "user id whitespace trimmed before the member check",
			body: "@Maria",
			in:   []structs.MessageMention{m(" maria ", 0, 6)},
			want: []structs.MessageMention{m("maria", 0, 6)},
		},
	}
	for _, c := range cases {
		got := normalizeMentions(c.in, c.body, members)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestNormalizeMentionsCap(t *testing.T) {
	members := map[string]struct{}{"maria": {}}
	body := ""
	requested := []structs.MessageMention{}
	for i := 0; i < messageMaxMentions+10; i++ {
		requested = append(requested, structs.MessageMention{UserId: "maria", Offset: len(body), Length: 2})
		body += "@M "
	}
	got := normalizeMentions(requested, body, members)
	if len(got) != messageMaxMentions {
		t.Fatalf("kept %d mentions, want the cap of %d", len(got), messageMaxMentions)
	}
	if got[len(got)-1].Offset != (messageMaxMentions-1)*3 {
		t.Errorf("the cap should keep the first %d in order; last kept offset %d", messageMaxMentions, got[len(got)-1].Offset)
	}
}

func TestMentionedUserIds(t *testing.T) {
	if got := mentionedUserIds(&structs.Message{}); got != nil {
		t.Errorf("no mentions: got %v, want nil", got)
	}
	got := mentionedUserIds(&structs.Message{Mentions: []structs.MessageMention{
		{UserId: "maria", Offset: 0, Length: 6},
		{UserId: "jo", Offset: 7, Length: 3},
		{UserId: "maria", Offset: 11, Length: 6},
	}})
	want := map[string]struct{}{"maria": {}, "jo": {}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
