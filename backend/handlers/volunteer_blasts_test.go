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
		nil,
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
	html := buildEventBlastEmail("Cleanup", "Update", "Line one\nLine two", nil)

	if !strings.Contains(html, "Line one<br />Line two") {
		t.Error("expected newlines to render as <br />")
	}

	// A literal <br> typed by the organizer must be escaped, not honoured.
	injected := buildEventBlastEmail("Cleanup", "Update", "before<br>after", nil)
	if strings.Contains(injected, "before<br>after") {
		t.Error("a literal <br> in the message must be escaped, not rendered")
	}
}

// Every volunteer email must go through the shared SFLuv shell rather than
// being hand-rolled, so branding and layout stay consistent.
func TestBuildEventBlastEmailUsesStyledShell(t *testing.T) {
	html := buildEventBlastEmail("Cleanup", "Update", "Hello", nil)

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

// Formatting is a closed set of markers rendered server-side. The client never
// sends HTML, so an organizer typing a tag gets literal text while the markers
// they were offered produce real emphasis.
func TestRenderBlastBodyAppliesOnlySafeFormatting(t *testing.T) {
	html := renderBlastBody("**bold** and _italic_ and <b>literal</b>")

	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Error("expected ** ** to render as bold")
	}
	if !strings.Contains(html, "<em>italic</em>") {
		t.Error("expected _ _ to render as italic")
	}
	if strings.Contains(html, "<b>literal</b>") {
		t.Error("a typed HTML tag must stay literal text, not become markup")
	}
	if !strings.Contains(html, "&lt;b&gt;literal&lt;/b&gt;") {
		t.Error("expected the typed tag to appear escaped")
	}
}

// An organizer-supplied link target is unvalidated input heading into an email
// under our brand, so it gets the same http(s) allowlist as partner links.
func TestRenderBlastBodyRejectsUnsafeLinkTargets(t *testing.T) {
	safe := renderBlastBody("[our page](https://sfluv.org/volunteers)")
	if !strings.Contains(safe, `href="https://sfluv.org/volunteers"`) {
		t.Error("expected an http(s) link to render as an anchor")
	}

	for _, dangerous := range []string{
		"[click](javascript:alert(1))",
		"[click](data:text/html;base64,PHNjcmlwdD4=)",
	} {
		html := renderBlastBody(dangerous)
		if strings.Contains(html, "href=") {
			t.Errorf("%q produced an anchor; unsafe schemes must degrade to plain text", dangerous)
		}
		if !strings.Contains(html, "click") {
			t.Errorf("%q should keep its label as text", dangerous)
		}
	}
}

// Blank lines separate paragraphs; single newlines stay line breaks.
func TestRenderBlastBodyParagraphs(t *testing.T) {
	html := renderBlastBody("First para\nsecond line\n\nSecond para")
	if strings.Count(html, "<p ") != 2 {
		t.Errorf("expected two paragraphs, got %d", strings.Count(html, "<p "))
	}
	if !strings.Contains(html, "First para<br />second line") {
		t.Error("a single newline should stay a line break inside the paragraph")
	}
}

// Attachments are rendered from server-built URLs, never from client HTML.
func TestBlastEmailEmbedsAttachments(t *testing.T) {
	html := buildEventBlastEmail("Cleanup", "Update", "See you there", []string{
		"https://api.sfluv.org/volunteer-events/blast-images/abc",
	})
	if !strings.Contains(html, `src="https://api.sfluv.org/volunteer-events/blast-images/abc"`) {
		t.Error("expected the attachment to be embedded")
	}
}
