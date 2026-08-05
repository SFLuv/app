package handlers

import (
	"strings"
	"testing"
)

// Blast subject and message are organizer-supplied and land in an HTML email,
// so markup must be escaped rather than rendered. An organizer typing "<b>" is
// far more likely than an attack, but the same escaping covers both.
func TestBuildEventBlastEmailEscapesInput(t *testing.T) {
	html := buildEventBlastEmail(
		`Beach <Cleanup> & "Picnic"`,
		`Update <script>alert(1)</script>`,
		`Bring gloves & water.<img src=x onerror=alert(1)>`,
	)

	// The property that matters is that no attacker-controlled TAG survives —
	// escaped text like "onerror=" sitting inside a paragraph is inert.
	for _, dangerous := range []string{
		"<script>",
		"</script>",
		"<img src=x",
	} {
		if strings.Contains(html, dangerous) {
			t.Errorf("unescaped %q survived into the email body", dangerous)
		}
	}

	// The text should still be present, just escaped.
	for _, expected := range []string{
		"&lt;script&gt;",
		"Bring gloves &amp; water.",
		"Beach &lt;Cleanup&gt;",
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("expected escaped content %q in the email", expected)
		}
	}
}

// Newlines must survive as <br />, and that conversion has to happen AFTER
// escaping — doing it before would let an injected "<br>" through as markup.
func TestBuildEventBlastEmailPreservesLineBreaks(t *testing.T) {
	html := buildEventBlastEmail("Cleanup", "Update", "Line one\nLine two")

	if !strings.Contains(html, "Line one<br />Line two") {
		t.Error("expected newlines to render as <br />")
	}

	// A literal <br> typed by the organizer must be escaped, not honoured.
	injected := buildEventBlastEmail("Cleanup", "Update", "before<br>after")
	if strings.Contains(injected, "before<br>after") {
		t.Error("a literal <br> in the message must be escaped, not rendered")
	}
}

// Every volunteer email must go through the shared SFLuv shell rather than
// being hand-rolled, so branding and layout stay consistent.
func TestBuildEventBlastEmailUsesStyledShell(t *testing.T) {
	html := buildEventBlastEmail("Cleanup", "Update", "Hello")

	for _, marker := range []string{"<!doctype html>", "background-color:#f6f7fb", "max-width:560px"} {
		if !strings.Contains(html, marker) {
			t.Errorf("expected the standard email shell marker %q", marker)
		}
	}
}

func TestFirstNameOrFallback(t *testing.T) {
	if got := firstNameOrFallback("  Ada  "); got != "Ada" {
		t.Errorf("got %q, want Ada", got)
	}
	if got := firstNameOrFallback("   "); got != "Volunteer" {
		t.Errorf("blank name should fall back, got %q", got)
	}
}
