package handlers

import "testing"

func TestShortCreatorName(t *testing.T) {
	cases := map[string]string{
		"Ada Lovelace":      "Ada L.",
		"Ada King Lovelace": "Ada L.",
		"Ada":               "Ada",
		"  Ada  Lovelace  ": "Ada L.",
		"ada lovelace":      "ada L.",
		"":                  "",
		"   ":               "",
	}

	for input, want := range cases {
		if got := shortCreatorName(input); got != want {
			t.Errorf("shortCreatorName(%q) = %q, want %q", input, got, want)
		}
	}
}

// A middle name must not change the initial — "Ada King Lovelace" and
// "Ada Lovelace" are the same person's short form and should collide, not
// silently disambiguate on a middle name the roster may or may not record.
func TestShortCreatorNameUsesLastWordAsSurname(t *testing.T) {
	if shortCreatorName("Ada King Lovelace") != shortCreatorName("Ada Lovelace") {
		t.Error("a recorded middle name must not change the short form")
	}
}
