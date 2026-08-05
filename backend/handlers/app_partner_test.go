package handlers

import "testing"

// Partner links become hrefs on the public marketing site, so anything other
// than an absolute http(s) URL must be rejected at the API boundary.
func TestIsSafePartnerLink(t *testing.T) {
	allowed := []string{
		"https://citizenwallet.xyz",
		"http://example.org/partners",
		"https://sub.domain.example.com/path?q=1#frag",
		"HTTPS://Example.com",
	}
	for _, link := range allowed {
		if !isSafePartnerLink(link) {
			t.Errorf("expected %q to be accepted", link)
		}
	}

	rejected := []string{
		"javascript:alert(1)",
		"JavaScript:alert(1)",
		"data:text/html;base64,PHNjcmlwdD4=",
		"vbscript:msgbox(1)",
		"file:///etc/passwd",
		"/relative/path",
		"example.com",     // no scheme — would resolve relative on the site
		"https://",        // no host
		"",                // empty is handled by the caller, but must not pass
		"   ",
	}
	for _, link := range rejected {
		if isSafePartnerLink(link) {
			t.Errorf("expected %q to be rejected", link)
		}
	}
}

// The stored content type is echoed back on a public endpoint, so it is
// resolved by sniffing the bytes rather than trusting the upload's own claim.
func TestResolvePartnerLogoContentType(t *testing.T) {
	pngBytes := []byte("\x89PNG\r\n\x1a\n" + "0000000000000000")
	if got := resolvePartnerLogoContentType(pngBytes, "logo.png"); got != "image/png" {
		t.Errorf("png = %q, want image/png", got)
	}

	gifBytes := []byte("GIF89a" + "00000000000000")
	if got := resolvePartnerLogoContentType(gifBytes, "logo.gif"); got != "image/gif" {
		t.Errorf("gif = %q, want image/gif", got)
	}

	svgBytes := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="120" height="40"></svg>`)
	if got := resolvePartnerLogoContentType(svgBytes, "logo.svg"); got != "image/svg+xml" {
		t.Errorf("svg = %q, want image/svg+xml", got)
	}

	// A non-image must not be storable, even with an image-looking filename —
	// these bytes are served back to every visitor of the public site.
	for _, payload := range []struct {
		data     []byte
		filename string
	}{
		{[]byte("#!/bin/sh\nrm -rf /"), "logo.png"},
		{[]byte("%PDF-1.4 ..............."), "logo.png"},
		{[]byte("just some text, not an image at all"), "logo.jpg"},
	} {
		if got := resolvePartnerLogoContentType(payload.data, payload.filename); got != "" {
			t.Errorf("expected %q upload to be rejected, got %q", payload.filename, got)
		}
	}
}

// The carousel reserves layout space from intrinsic dimensions, so SVG logos —
// which the standard decoders cannot read — need their size recovered from
// attributes or viewBox.
func TestPartnerLogoDimensions(t *testing.T) {
	withAttrs := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="421" height="66"></svg>`)
	if width, height := partnerLogoDimensions(withAttrs, "image/svg+xml"); width != 421 || height != 66 {
		t.Errorf("svg width/height attrs = %dx%d, want 421x66", width, height)
	}

	viewBoxOnly := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 515 134"></svg>`)
	if width, height := partnerLogoDimensions(viewBoxOnly, "image/svg+xml"); width != 515 || height != 134 {
		t.Errorf("svg viewBox = %dx%d, want 515x134", width, height)
	}

	fractional := []byte(`<svg viewBox="0 0 100.6 50.2"></svg>`)
	if width, height := partnerLogoDimensions(fractional, "image/svg+xml"); width != 101 || height != 50 {
		t.Errorf("fractional viewBox = %dx%d, want 101x50", width, height)
	}

	// Unknown size must degrade to 0x0 rather than erroring — a logo the server
	// cannot measure is still a usable logo.
	if width, height := partnerLogoDimensions([]byte(`<svg></svg>`), "image/svg+xml"); width != 0 || height != 0 {
		t.Errorf("sizeless svg = %dx%d, want 0x0", width, height)
	}
	if width, height := partnerLogoDimensions([]byte("not an image"), "image/png"); width != 0 || height != 0 {
		t.Errorf("undecodable raster = %dx%d, want 0x0", width, height)
	}
}
