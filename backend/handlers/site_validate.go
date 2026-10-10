package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
)

// Validation and small pure helpers for the admin-editable site and Forms and
// Waivers. Nothing here touches the database.

const (
	maxSiteFileBytes      = 25 << 20 // 25 MiB
	maxSiteSignatureBytes = 400 << 10
	maxSiteSignBodyBytes  = 2 << 20
)

var (
	siteSlugPattern   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	siteChoiceIDRegex = regexp.MustCompile(`^[a-z0-9_]+$`)
	sitePhoneChars    = regexp.MustCompile(`^[0-9+()\-. ]+$`)
)

var financialKindLabels = map[string]string{
	"activity":            "Statement of Activity",
	"cash_flows":          "Statement of Cash Flows",
	"financial_position":  "Statement of Financial Position",
	"activity_comparison": "Statement of Activity Comparison",
}

func tooLong(value string, max int) bool {
	return utf8.RuneCountInString(value) > max
}

func validDate(value string) bool {
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

// validSiteLink allows an on-site path or an http(s)/mailto URL, and nothing
// else: a javascript: or data: URL pasted into a button would run on the site.
func validSiteLink(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || tooLong(value, 500) {
		return false
	}
	// Browsers treat "/\evil.com" like "//evil.com": another site. Neither a
	// backslash nor whitespace belongs in a link an editor types.
	if strings.ContainsAny(value, "\\ \t\r\n") {
		return false
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "mailto:")
}

// ── Homepage banner ──────────────────────────────────────────────────────

const maxSpotlightSlides = 12

// A CSS object-position: "center 30%", "50% 20%", "left top"…
var spotlightPositionPattern = regexp.MustCompile(`^(left|center|right|\d{1,3}%) (top|center|bottom|\d{1,3}%)$`)

// sanitizeSpotlight normalises and validates the whole list. A slide that is
// switched off may be unfinished (a draft); one that is switched on must be
// complete, because it is what the public sees.
func sanitizeSpotlight(sp *structs.SiteSpotlight) error {
	if sp.Slides == nil {
		sp.Slides = []structs.SiteSpotlightSlide{}
	}
	if len(sp.Slides) > maxSpotlightSlides {
		return fmt.Errorf("there can be at most %d banner items", maxSpotlightSlides)
	}

	seen := map[string]bool{}
	for i := range sp.Slides {
		sl := &sp.Slides[i]
		n := i + 1

		sl.Id = strings.TrimSpace(sl.Id)
		sl.Label = strings.TrimSpace(sl.Label)
		sl.Title = strings.TrimSpace(sl.Title)
		sl.Body = strings.TrimSpace(sl.Body)
		sl.ImageAlt = strings.TrimSpace(sl.ImageAlt)
		sl.ImagePosition = strings.ToLower(strings.TrimSpace(sl.ImagePosition))
		sl.EventMatch = strings.TrimSpace(sl.EventMatch)
		sl.ImagePreviewURL = nil

		if sl.Id == "" {
			sl.Id = "slide-" + uuid.NewString()[:8]
		}
		if seen[sl.Id] {
			return fmt.Errorf("banner item %d repeats another banner item's id", n)
		}
		seen[sl.Id] = true

		if tooLong(sl.Label, 40) {
			return fmt.Errorf("banner item %d: the small label must be 40 characters or fewer", n)
		}
		if tooLong(sl.Title, 150) {
			return fmt.Errorf("banner item %d: the title must be 150 characters or fewer", n)
		}
		if tooLong(sl.Body, 600) {
			return fmt.Errorf("banner item %d: the text must be 600 characters or fewer", n)
		}
		if tooLong(sl.ImageAlt, 200) {
			return fmt.Errorf("banner item %d: the picture description must be 200 characters or fewer", n)
		}
		if tooLong(sl.EventMatch, 100) {
			return fmt.Errorf("banner item %d: the event words must be 100 characters or fewer", n)
		}
		if sl.ImagePosition != "" && !spotlightPositionPattern.MatchString(sl.ImagePosition) {
			return fmt.Errorf("banner item %d: the picture position is not valid", n)
		}

		if sl.ImageFileId != nil && strings.TrimSpace(*sl.ImageFileId) == "" {
			sl.ImageFileId = nil
		}
		if sl.ImageFileId != nil {
			// An uploaded picture replaces any legacy one.
			sl.ImageURL, sl.ImageWidth, sl.ImageHeight = nil, 0, 0
		}

		act := &sl.Action
		act.Label = strings.TrimSpace(act.Label)
		act.Href = strings.TrimSpace(act.Href)
		if tooLong(act.Label, 40) {
			return fmt.Errorf("banner item %d: the button words must be 40 characters or fewer", n)
		}
		// A button is optional, but half of one (words with nowhere to go, or
		// the other way round) is almost certainly a mistake.
		if (act.Label == "") != (act.Href == "") {
			if act.Label == "" {
				return fmt.Errorf("banner item %d: the button needs words, or clear its link to have no button", n)
			}
			return fmt.Errorf("banner item %d: choose where the button goes, or clear its words to have no button", n)
		}
		if act.Href != "" && !validSiteLink(act.Href) {
			return fmt.Errorf("banner item %d: the button link must start with / (a page on this site), https://, or mailto:", n)
		}
		if act.AlsoOpenURL != nil {
			trimmed := strings.TrimSpace(*act.AlsoOpenURL)
			switch {
			case trimmed == "":
				act.AlsoOpenURL = nil
			case !validSiteLink(trimmed):
				return fmt.Errorf("banner item %d: the second link must start with / , https://, or mailto:", n)
			default:
				act.AlsoOpenURL = &trimmed
			}
		}
		if act.AlsoOpenFileId != nil && strings.TrimSpace(*act.AlsoOpenFileId) == "" {
			act.AlsoOpenFileId = nil
		}
		// An uploaded document wins over a typed link.
		if act.AlsoOpenFileId != nil {
			act.AlsoOpenURL = nil
		}

		if sl.Enabled {
			label := sl.Title
			if label == "" {
				label = fmt.Sprintf("#%d", n)
			}
			if sl.Title == "" {
				return fmt.Errorf("banner item %d needs a title before it can be turned on", n)
			}
			if sl.ImageFileId == nil && (sl.ImageURL == nil || sl.ImageWidth <= 0 || sl.ImageHeight <= 0) {
				return fmt.Errorf("“%s” needs a photo before it can be turned on", label)
			}
		}
		if sl.ImageAlt == "" {
			sl.ImageAlt = sl.Title
		}
	}
	return nil
}

// ── Financial documents ──────────────────────────────────────────────────────

func sanitizeFinancialRequest(req *structs.FinancialDocumentRequest) error {
	req.Label = strings.TrimSpace(req.Label)
	req.FileId = strings.TrimSpace(req.FileId)

	switch req.Kind {
	case "activity", "cash_flows", "financial_position", "activity_comparison",
		"form_990n", "form_199n", "impact_report", "other":
	default:
		return fmt.Errorf("choose what kind of document this is")
	}
	if req.FiscalYear < 2000 || req.FiscalYear > 2100 {
		return fmt.Errorf("choose the fiscal year")
	}
	if req.Kind == "impact_report" {
		req.Period = "FULL"
	}
	switch req.Period {
	case "Q1", "Q2", "Q3", "Q4", "FULL":
	default:
		return fmt.Errorf("choose the quarter, or full year")
	}
	if tooLong(req.Label, 200) {
		return fmt.Errorf("the label must be 200 characters or fewer")
	}

	if req.AsOf != nil {
		trimmed := strings.TrimSpace(*req.AsOf)
		if trimmed == "" {
			req.AsOf = nil
		} else {
			req.AsOf = &trimmed
		}
	}
	if _, isStatement := financialKindLabels[req.Kind]; isStatement {
		if req.AsOf == nil || !validDate(*req.AsOf) {
			return fmt.Errorf("enter the statement date (the day the period ends)")
		}
	} else {
		req.AsOf = nil
	}
	if req.Kind == "other" && req.Label == "" {
		return fmt.Errorf("give this document a label")
	}
	return nil
}

// financialLabel is what the site shows for a document: the editor's own label
// when they typed one, otherwise one built from its kind and date so labels
// stay uniform ("2026-06-30 Statement of Cash Flows").
func financialLabel(req *structs.FinancialDocumentRequest) string {
	if req.Label != "" {
		return req.Label
	}
	switch req.Kind {
	case "form_990n":
		return fmt.Sprintf("%d 990N", req.FiscalYear)
	case "form_199n":
		return fmt.Sprintf("%d 199N Confirmation", req.FiscalYear)
	case "impact_report":
		return fmt.Sprintf("%d–%d Annual Impact Report", req.FiscalYear-1, req.FiscalYear)
	}
	if name, ok := financialKindLabels[req.Kind]; ok && req.AsOf != nil {
		return *req.AsOf + " " + name
	}
	return req.Label
}

// ── Forms ────────────────────────────────────────────────────────────────────

func sanitizeFormRequest(req *structs.SiteFormRequest, creating bool) error {
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	req.Title = strings.TrimSpace(req.Title)
	req.Summary = strings.TrimSpace(req.Summary)
	req.Body = strings.TrimSpace(strings.ReplaceAll(req.Body, "\r\n", "\n"))

	if creating {
		if len(req.Slug) < 3 || len(req.Slug) > 60 || !siteSlugPattern.MatchString(req.Slug) {
			return fmt.Errorf("the web address must be 3–60 characters: lowercase letters, numbers and dashes (like boundless-video)")
		}
		switch req.Kind {
		case "waiver", "withdrawal":
		default:
			return fmt.Errorf("choose whether this is a waiver or a withdrawal form")
		}
	}
	if req.Title == "" || tooLong(req.Title, 150) {
		return fmt.Errorf("the title is required and must be 150 characters or fewer")
	}
	if tooLong(req.Summary, 300) {
		return fmt.Errorf("the one-line summary must be 300 characters or fewer")
	}
	if req.Body == "" {
		return fmt.Errorf("the form text is required")
	}
	if tooLong(req.Body, 30000) {
		return fmt.Errorf("the form text is too long")
	}
	return sanitizeFormConfig(&req.Config)
}

func sanitizeFormConfig(c *structs.SiteFormConfig) error {
	switch c.Contact {
	case "off", "optional", "required":
	case "":
		c.Contact = "optional"
	default:
		return fmt.Errorf("invalid contact setting")
	}
	switch c.Event {
	case "off", "ask", "fixed":
	case "":
		c.Event = "off"
	default:
		return fmt.Errorf("invalid event setting")
	}

	c.EventName = strings.TrimSpace(c.EventName)
	if c.Event == "fixed" && c.EventName == "" {
		return fmt.Errorf("enter the event or project name, or choose a different event setting")
	}
	if c.Event != "fixed" {
		c.EventName = ""
	}
	if tooLong(c.EventName, 200) {
		return fmt.Errorf("the event name must be 200 characters or fewer")
	}

	c.ChoicesPrompt = strings.TrimSpace(c.ChoicesPrompt)
	c.ConfirmationMessage = strings.TrimSpace(c.ConfirmationMessage)
	if tooLong(c.ChoicesPrompt, 200) || tooLong(c.ConfirmationMessage, 500) {
		return fmt.Errorf("a prompt or message is too long")
	}
	if c.ConfirmationMessage == "" {
		c.ConfirmationMessage = "Thank you. Your signed form has been recorded."
	}

	if c.Choices == nil {
		c.Choices = []structs.SiteFormChoice{}
	}
	if len(c.Choices) > 8 {
		return fmt.Errorf("a form can have at most 8 choices")
	}
	// Existing choices keep their ids (signatures refer to them); new ones get
	// the lowest free choice_N. Numbering by list position would collide as
	// soon as a choice was deleted and another added.
	seen := map[string]bool{}
	for i := range c.Choices {
		choice := &c.Choices[i]
		choice.Label = strings.TrimSpace(choice.Label)
		choice.Description = strings.TrimSpace(choice.Description)
		choice.Id = strings.TrimSpace(choice.Id)
		if choice.Label == "" || tooLong(choice.Label, 200) {
			return fmt.Errorf("every choice needs a label of 200 characters or fewer")
		}
		if tooLong(choice.Description, 500) {
			return fmt.Errorf("a choice description is too long")
		}
		if choice.Id == "" {
			continue
		}
		if !siteChoiceIDRegex.MatchString(choice.Id) || seen[choice.Id] {
			return fmt.Errorf("two choices ended up with the same internal id; reload the page and try again")
		}
		seen[choice.Id] = true
	}
	next := 1
	for i := range c.Choices {
		if c.Choices[i].Id != "" {
			continue
		}
		for seen[fmt.Sprintf("choice_%d", next)] {
			next++
		}
		c.Choices[i].Id = fmt.Sprintf("choice_%d", next)
		seen[c.Choices[i].Id] = true
	}
	return nil
}

// siteFormTextHash fingerprints exactly what a signer was shown, so a
// signature can later be matched to the words it was given against.
func siteFormTextHash(v *structs.SiteFormVersion) string {
	payload, _ := json.Marshal(struct {
		Title    string                   `json:"title"`
		Body     string                   `json:"body"`
		Choices  []structs.SiteFormChoice `json:"choices"`
		Guardian bool                     `json:"guardian"`
	}{v.Title, v.Body, v.Config.Choices, v.Config.GuardianSection})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// ── Signatures ───────────────────────────────────────────────────────────────

// decodeSignaturePNG accepts a canvas data URL and returns clean PNG bytes.
//
// The image is decoded and re-encoded rather than stored as sent, which drops
// anything smuggled into the file, and it must actually contain ink — a blank
// canvas is not a signature.
func decodeSignaturePNG(dataURL string) ([]byte, error) {
	const prefix = "data:image/png;base64,"
	dataURL = strings.TrimSpace(dataURL)
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, fmt.Errorf("the signature could not be read")
	}
	raw, err := base64.StdEncoding.DecodeString(dataURL[len(prefix):])
	if err != nil || len(raw) == 0 || len(raw) > maxSiteSignatureBytes {
		return nil, fmt.Errorf("the signature could not be read")
	}

	// Size first, from the header alone. The decoder allocates width x height x
	// 4 bytes before reading a single pixel, so a few bytes claiming to be a
	// 60000x60000 image would otherwise exhaust memory and take the whole
	// backend down. This is a public, unauthenticated endpoint.
	header, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || header.Width < 40 || header.Height < 20 || header.Width > 2400 || header.Height > 1200 {
		return nil, fmt.Errorf("the signature could not be read")
	}

	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("the signature could not be read")
	}
	b := img.Bounds()

	ink := 0
	for y := b.Min.Y; y < b.Max.Y && ink < 40; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a > 0x2000 && !(r > 0xf000 && g > 0xf000 && bl > 0xf000) {
				ink++
				if ink >= 40 {
					break
				}
			}
		}
	}
	if ink < 40 {
		return nil, fmt.Errorf("please sign in the box")
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, fmt.Errorf("the signature could not be read")
	}
	return out.Bytes(), nil
}

// validContact accepts an email address or a phone number. Deliberately loose
// for phones: this is for reaching someone, not for verifying them.
func validContact(value string) bool {
	if strings.Contains(value, "@") {
		_, err := mail.ParseAddress(value)
		return err == nil
	}
	if !sitePhoneChars.MatchString(value) {
		return false
	}
	digits := 0
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return digits >= 7 && digits <= 15
}

// ── Files ────────────────────────────────────────────────────────────────────

// sniffSiteFile decides what an upload really is from its bytes. SVG and HTML
// are refused outright: files here are served from the API's own origin.
func sniffSiteFile(data []byte) (contentType string, isImage bool) {
	if len(data) >= 5 && string(data[:5]) == "%PDF-" {
		return "application/pdf", false
	}
	detected := http.DetectContentType(data)
	switch detected {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return detected, true
	}
	return "", false
}

func siteImageDimensions(data []byte) (int, int) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		return config.Width, config.Height
	}
	// No WebP decoder is linked in, so read its size from the header.
	return webpDimensions(data)
}

// webpDimensions reads a WebP's size from its first chunk header. Returns 0x0
// if it cannot, which callers treat as unknown.
func webpDimensions(d []byte) (int, int) {
	if len(d) < 30 || string(d[0:4]) != "RIFF" || string(d[8:12]) != "WEBP" {
		return 0, 0
	}
	le24 := func(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }
	switch string(d[12:16]) {
	case "VP8X": // extended: canvas size, each minus one, 24-bit
		return le24(d[24:27]) + 1, le24(d[27:30]) + 1
	case "VP8 ": // lossy: after a 3-byte frame tag and the 9d 01 2a start code
		if d[23] != 0x9d || d[24] != 0x01 || d[25] != 0x2a {
			return 0, 0
		}
		return (int(d[26]) | int(d[27])<<8) & 0x3fff, (int(d[28]) | int(d[29])<<8) & 0x3fff
	case "VP8L": // lossless: 0x2f, then 14 bits each of width-1 and height-1
		if d[20] != 0x2f {
			return 0, 0
		}
		bits := uint32(d[21]) | uint32(d[22])<<8 | uint32(d[23])<<16 | uint32(d[24])<<24
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1
	}
	return 0, 0
}

var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._ \-()]+`)

// cleanSiteFilename keeps a recognisable name but nothing that could break out
// of a header or a URL.
func cleanSiteFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(unsafeFilenameChars.ReplaceAllString(name, "_"))
	name = strings.Trim(name, ". ")
	if name == "" {
		name = "file"
	}
	if utf8.RuneCountInString(name) > 120 {
		runes := []rune(name)
		name = string(runes[len(runes)-120:])
	}
	return name
}
