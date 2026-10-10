package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// Public routes for the editable site and Forms and Waivers. No auth: this is
// the same content any visitor of sfluv.org sees. See
// docs/features/website-editing-and-forms.md.

// ── Banner items ───────────────────────────────────────────────────────────

// GetPublicSpotlight returns the slides that are switched on and complete. A
// slide whose picture has gone missing is left out rather than shown broken.
func (a *AppService) GetPublicSpotlight(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "public, max-age=15, stale-while-revalidate=60")

	out := structs.SitePublicSpotlight{Slides: []structs.SitePublicSlide{}}
	row, err := a.db.GetSiteContent(ctx, spotlightKey)
	if err != nil {
		a.siteServerError(w, "loading the public banner items", err)
		return
	}
	var sp structs.SiteSpotlight
	if row == nil || json.Unmarshal(row.Value, &sp) != nil {
		writeSiteJSON(w, http.StatusOK, out)
		return
	}

	for _, sl := range sp.Slides {
		// The button is optional; the website draws one only when it has words and a link.
		if !sl.Enabled || sl.Title == "" {
			continue
		}

		var image structs.SitePublicImage
		switch {
		case sl.ImageFileId != nil:
			file, err := a.db.GetSiteFile(ctx, *sl.ImageFileId)
			if err != nil {
				continue
			}
			image = structs.SitePublicImage{URL: siteFileURL(file.Id, file.Filename), Width: file.Width, Height: file.Height}
		case sl.ImageURL != nil && sl.ImageWidth > 0 && sl.ImageHeight > 0:
			image = structs.SitePublicImage{URL: *sl.ImageURL, Width: sl.ImageWidth, Height: sl.ImageHeight}
		default:
			continue
		}
		image.Alt = sl.ImageAlt
		if image.Alt == "" {
			image.Alt = sl.Title
		}

		action := structs.SitePublicAction{Label: sl.Action.Label, Href: sl.Action.Href, NewTab: sl.Action.NewTab}
		if sl.Action.AlsoOpenFileId != nil {
			if file, err := a.db.GetSiteFile(ctx, *sl.Action.AlsoOpenFileId); err == nil {
				action.AlsoOpen = siteFileURL(file.Id, file.Filename)
			}
		} else if sl.Action.AlsoOpenURL != nil {
			action.AlsoOpen = *sl.Action.AlsoOpenURL
		}

		out.Slides = append(out.Slides, structs.SitePublicSlide{
			Id: sl.Id, Label: sl.Label, Title: sl.Title, Body: sl.Body, Image: image,
			ImagePosition: sl.ImagePosition, Action: action, EventMatch: sl.EventMatch,
		})
	}
	writeSiteJSON(w, http.StatusOK, out)
}

// ── Financials ───────────────────────────────────────────────────────────────

func financialPeriodLabel(period string) string {
	switch period {
	case "Q4":
		return "Q4 / FYE"
	case "FULL":
		return "Full year"
	default:
		return period
	}
}

// buildPublicFinancials groups documents the way the page shows them: fiscal
// years newest first, quarters within a year, and impact reports in their own
// list. The input is already in display order.
func buildPublicFinancials(docs []*structs.FinancialDocument) structs.PublicFinancials {
	out := structs.PublicFinancials{
		Years:         []structs.PublicFinancialYear{},
		ImpactReports: []structs.PublicFinancialDocument{},
	}

	for _, doc := range docs {
		item := structs.PublicFinancialDocument{Id: doc.Id, Label: doc.Label, Href: doc.Href, Kind: doc.Kind}
		if doc.Kind == "impact_report" {
			out.ImpactReports = append(out.ImpactReports, item)
			continue
		}

		if len(out.Years) == 0 || out.Years[len(out.Years)-1].FiscalYear != doc.FiscalYear {
			out.Years = append(out.Years, structs.PublicFinancialYear{
				Label:      fmt.Sprintf("FYE June 30, %d", doc.FiscalYear),
				FiscalYear: doc.FiscalYear,
				Periods:    []structs.PublicFinancialPeriod{},
			})
		}
		year := &out.Years[len(out.Years)-1]

		label := financialPeriodLabel(doc.Period)
		if len(year.Periods) == 0 || year.Periods[len(year.Periods)-1].Label != label {
			year.Periods = append(year.Periods, structs.PublicFinancialPeriod{
				Label:     label,
				Documents: []structs.PublicFinancialDocument{},
			})
		}
		period := &year.Periods[len(year.Periods)-1]
		period.Documents = append(period.Documents, item)
	}
	return out
}

func (a *AppService) GetPublicFinancials(w http.ResponseWriter, r *http.Request) {
	docs, err := a.db.ListFinancialDocuments(r.Context(), false)
	if err != nil {
		a.siteServerError(w, "listing public financials", err)
		return
	}
	for _, doc := range docs {
		a.finishFinancialDocument(doc)
	}
	w.Header().Set("Cache-Control", "public, max-age=15, stale-while-revalidate=60")
	writeSiteJSON(w, http.StatusOK, buildPublicFinancials(docs))
}

// ── Files ────────────────────────────────────────────────────────────────────

// GetSiteFile serves an uploaded PDF or image. The filename in the URL is only
// for people: the id is what is looked up. A file never changes once stored, so
// it can be cached hard.
func (a *AppService) GetSiteFile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	file, cached := siteFileCache.get(id)
	var err error
	if !cached {
		file, err = a.db.GetSiteFileData(r.Context(), id)
		if err == nil {
			siteFileCache.put(file)
		}
	}
	if err != nil {
		if errors.Is(err, db.ErrSiteNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !isClientGone(err) {
			a.logger.Logf("error serving site file: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, cleanSiteFilename(file.Filename)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Data)
}

// fileCache keeps recently served uploads in memory, so a popular PDF is not
// read out of Postgres on every view. Uploads never change once stored, so an
// entry cannot go stale.
type fileCache struct {
	mu    sync.Mutex
	items map[string]*structs.SiteFileData
	size  int
	max   int
}

var siteFileCache = &fileCache{items: map[string]*structs.SiteFileData{}, max: 64 << 20}

func (c *fileCache) get(id string) (*structs.SiteFileData, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	file, ok := c.items[id]
	return file, ok
}

func (c *fileCache) put(file *structs.SiteFileData) {
	if len(file.Data) > c.max/4 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[file.Id]; ok {
		return
	}
	for id, old := range c.items {
		if c.size+len(file.Data) <= c.max {
			break
		}
		delete(c.items, id)
		c.size -= len(old.Data)
	}
	c.items[file.Id] = file
	c.size += len(file.Data)
}

// ── Forms ────────────────────────────────────────────────────────────────────

func (a *AppService) GetPublicForms(w http.ResponseWriter, r *http.Request) {
	forms, err := a.db.ListSiteForms(r.Context(), true)
	if err != nil {
		a.siteServerError(w, "listing public forms", err)
		return
	}
	items := make([]structs.SitePublicForm, 0, len(forms))
	for _, f := range forms {
		items = append(items, structs.SitePublicForm{
			Slug: f.Slug, Kind: f.Kind, Status: "open", Title: f.Title, Summary: f.Summary,
		})
	}
	w.Header().Set("Cache-Control", "public, max-age=10, stale-while-revalidate=30")
	writeSiteJSON(w, http.StatusOK, map[string]any{"forms": items})
}

// GetPublicForm returns an open form in full. A closed one returns only its
// title, so an old QR code can say it is no longer collecting signatures. A form
// that was never opened does not exist as far as the public is concerned.
func (a *AppService) GetPublicForm(w http.ResponseWriter, r *http.Request) {
	summary, err := a.db.GetSiteFormSummaryBySlug(r.Context(), strings.TrimSpace(r.PathValue("slug")))
	if err != nil || summary.Status == "draft" {
		if err != nil && !errors.Is(err, db.ErrSiteNotFound) {
			a.siteServerError(w, "loading a public form", err)
			return
		}
		writeSiteError(w, http.StatusNotFound, "That form was not found.")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=10, stale-while-revalidate=30")
	if summary.Status != "open" {
		writeSiteJSON(w, http.StatusOK, structs.SitePublicForm{Slug: summary.Slug, Status: "closed", Title: summary.Title})
		return
	}

	version, err := a.db.GetSiteFormVersion(r.Context(), summary.Id, summary.CurrentVersion)
	if err != nil {
		a.siteServerError(w, "loading a public form version", err)
		return
	}
	writeSiteJSON(w, http.StatusOK, structs.SitePublicForm{
		Slug: summary.Slug, Kind: summary.Kind, Status: "open", Title: version.Title, Summary: version.Summary,
		Version: version.Version, Body: version.Body, Config: &version.Config,
	})
}

// ── Signing ──────────────────────────────────────────────────────────────────

// slidingLimiter allows at most max events per key within window. In-process
// is enough: there is one backend, and a restart forgetting a few minutes of
// history costs nothing.
type slidingLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	max    int
	window time.Duration
}

func newSlidingLimiter(max int, window time.Duration) *slidingLimiter {
	return &slidingLimiter{hits: map[string][]time.Time{}, max: max, window: window}
}

// allow records an event for key and reports whether it was within the limit.
func (l *slidingLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.max {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)

	// Keep the map from growing without bound.
	if len(l.hits) > 5000 {
		for k, times := range l.hits {
			if len(times) == 0 || !times[len(times)-1].After(cutoff) {
				delete(l.hits, k)
			}
		}
	}
	return true
}

var (
	// Signatures recorded per address. Generous because a whole room signs
	// from one venue network at a shoot. Only successful signatures count, so
	// someone fixing typos is never locked out.
	siteSignRateLimiter = newSlidingLimiter(60, 10*time.Minute)
	// Every attempt, valid or not: a looser ceiling on the work a single
	// address can make the server do.
	siteSignAttemptLimiter = newSlidingLimiter(300, 10*time.Minute)
	// Confirmation emails per recipient. The address is typed by an anonymous
	// visitor, so without this anyone could point SFLuv's mail at a stranger.
	siteSignEmailLimiter = newSlidingLimiter(3, 24*time.Hour)
	// Staff alerts for withdrawals, in total. Anyone can submit the withdrawal
	// form, so this keeps a flood of fake ones from flooding the inbox too; the
	// submissions themselves are all still recorded.
	siteWithdrawalAlertLimiter = newSlidingLimiter(20, time.Hour)
)

// PostSignForm records a signature. Everything is validated against the version
// of the form the signer was shown; the server, not the browser, decides the
// time and whether the form is open.
func (a *AppService) PostSignForm(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSiteSignBodyBytes)
	var req structs.SiteSignRequest
	if !decodeSiteBodyLimited(w, r, &req) {
		return
	}

	// Honeypot filled: accept so a bot cannot tell it was rejected, record nothing.
	if strings.TrimSpace(req.Website) != "" {
		writeSiteJSON(w, http.StatusCreated, structs.SiteSignResponse{ConfirmationMessage: "Thank you."})
		return
	}

	ip := clientIPForRateLimit(r)
	if !siteSignAttemptLimiter.allow(ip, time.Now()) {
		writeSiteError(w, http.StatusTooManyRequests, "Too many attempts from this connection just now. Please wait a few minutes and try again.")
		return
	}

	summary, err := a.db.GetSiteFormSummaryBySlug(r.Context(), strings.TrimSpace(r.PathValue("slug")))
	if err != nil || summary.Status == "draft" {
		if err != nil && !errors.Is(err, db.ErrSiteNotFound) {
			a.siteServerError(w, "loading a form to sign", err)
			return
		}
		writeSiteError(w, http.StatusNotFound, "That form was not found.")
		return
	}
	if summary.Status != "open" {
		writeSiteError(w, http.StatusGone, "This form is no longer collecting signatures.")
		return
	}
	if req.Version != summary.CurrentVersion {
		writeSiteError(w, http.StatusConflict, "This form was just updated. Please reload the page, review it, and sign again.")
		return
	}
	version, err := a.db.GetSiteFormVersion(r.Context(), summary.Id, summary.CurrentVersion)
	if err != nil {
		a.siteServerError(w, "loading a form version to sign", err)
		return
	}
	cfg := version.Config

	insert := &db.SiteSignatureInsert{
		FormId:       summary.Id,
		Version:      version.Version,
		EsignConsent: req.EsignConsent,
		TextSHA256:   siteFormTextHash(version),
		ClientIP:     ip,
		UserAgent:    truncateRunes(r.UserAgent(), 300),
	}

	insert.SignerName = strings.TrimSpace(req.SignerName)
	if n := len([]rune(insert.SignerName)); n < 2 || n > 120 {
		writeSiteError(w, http.StatusBadRequest, "Please enter your full name.")
		return
	}
	if !req.EsignConsent {
		writeSiteError(w, http.StatusBadRequest, "Please confirm that you agree to sign electronically.")
		return
	}

	answers := map[string]string{}
	if cfg.PreferredName {
		answers["preferred_name"] = truncateRunes(strings.TrimSpace(req.PreferredName), 80)
	}

	contact := strings.TrimSpace(req.Contact)
	switch {
	case cfg.Contact == "off":
		contact = ""
	case contact == "" && cfg.Contact == "required":
		writeSiteError(w, http.StatusBadRequest, "Please enter an email address or phone number.")
		return
	case contact != "" && (tooLong(contact, 200) || !validContact(contact)):
		writeSiteError(w, http.StatusBadRequest, "That email address or phone number does not look right.")
		return
	}
	insert.Contact = contact

	switch cfg.Event {
	case "fixed":
		answers["event"] = cfg.EventName
	case "ask":
		answers["event"] = truncateRunes(strings.TrimSpace(req.Event), 200)
		date := strings.TrimSpace(req.EventDate)
		if date != "" && !validDate(date) {
			writeSiteError(w, http.StatusBadRequest, "That event date does not look right.")
			return
		}
		answers["event_date"] = date
	}

	chosenLabel := ""
	if len(cfg.Choices) > 0 {
		for _, choice := range cfg.Choices {
			if choice.Id == req.Choice {
				answers["choice"] = choice.Id
				chosenLabel = choice.Label
			}
		}
		if chosenLabel == "" {
			writeSiteError(w, http.StatusBadRequest, "Please choose one of the options.")
			return
		}
	}

	insert.IsMinor = cfg.GuardianSection && req.IsMinor
	if insert.IsMinor {
		insert.GuardianName = strings.TrimSpace(req.GuardianName)
		insert.GuardianRelationship = strings.TrimSpace(req.GuardianRelationship)
		if n := len([]rune(insert.GuardianName)); n < 2 || n > 120 {
			writeSiteError(w, http.StatusBadRequest, "Please enter the parent or guardian's full name.")
			return
		}
		if insert.GuardianRelationship == "" || tooLong(insert.GuardianRelationship, 80) {
			writeSiteError(w, http.StatusBadRequest, "Please enter the parent or guardian's relationship to the participant.")
			return
		}
		guardianPNG, err := decodeSignaturePNG(req.GuardianSignaturePNG)
		if err != nil {
			writeSiteError(w, http.StatusBadRequest, "The parent or guardian needs to sign too. "+capitalizeFirst(err.Error())+".")
			return
		}
		insert.GuardianSignaturePNG = guardianPNG
	}

	// A minor's own signature is optional (a young child may only be able to
	// scribble); an adult's is required.
	if strings.TrimSpace(req.SignaturePNG) != "" || !insert.IsMinor {
		signaturePNG, err := decodeSignaturePNG(req.SignaturePNG)
		if err != nil {
			writeSiteError(w, http.StatusBadRequest, capitalizeFirst(err.Error())+".")
			return
		}
		insert.SignaturePNG = signaturePNG
	}

	encoded, _ := json.Marshal(answers)
	insert.Answers = encoded

	if !siteSignRateLimiter.allow(ip, time.Now()) {
		writeSiteError(w, http.StatusTooManyRequests, "Too many signatures from this connection just now. Please wait a few minutes and try again.")
		return
	}

	id, signedAt, err := a.db.CreateSiteFormSignature(r.Context(), insert)
	if err != nil {
		a.siteServerError(w, "recording a signature", err)
		return
	}

	// Sent before responding, so the page only says "we emailed you a copy"
	// when one actually went out.
	emailed := false
	if address, err := mail.ParseAddress(contact); err == nil && strings.Contains(contact, "@") {
		if siteSignEmailLimiter.allow(strings.ToLower(address.Address), time.Now()) {
			emailed = a.sendSiteSignatureConfirmation(contact, insert.SignerName, version, req.Choice, answers["event"], signedAt)
		}
	}

	// Someone asking to withdraw consent needs a person to act on it, so staff
	// are told rather than left to notice. In the background: the visitor does
	// not wait on it, and a mail failure changes nothing for them.
	if summary.Kind == "withdrawal" && siteWithdrawalAlertLimiter.allow("all", time.Now()) {
		go a.sendSiteWithdrawalAlert(version.Title, insert.SignerName, contact, answers, signedAt)
	}

	writeSiteJSON(w, http.StatusCreated, structs.SiteSignResponse{
		Id: id, SignedAt: signedAt, ConfirmationMessage: cfg.ConfirmationMessage, Emailed: emailed,
	})
}

// siteWithdrawalAlertTo is who hears about withdrawal requests:
// SITE_WITHDRAWAL_ALERT_EMAIL, or admin@sfluv.org when that is unset.
func siteWithdrawalAlertTo() string {
	if to := strings.TrimSpace(os.Getenv("SITE_WITHDRAWAL_ALERT_EMAIL")); to != "" {
		return to
	}
	return "admin@sfluv.org"
}

// sendSiteWithdrawalAlert tells staff that someone submitted a withdrawal form,
// with what they entered and where to act on it. It never includes the
// signature image.
func (a *AppService) sendSiteWithdrawalAlert(formTitle string, name string, contact string, answers map[string]string, signedAt int64) {
	defer func() {
		if rec := recover(); rec != nil {
			a.logger.Logf("recovered while sending a withdrawal alert: %v", rec)
		}
	}()

	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		a.logger.Logf("withdrawal alert not sent: email sender is not configured")
		return
	}

	row := func(label string, value string) string {
		if strings.TrimSpace(value) == "" {
			value = "—"
		}
		return fmt.Sprintf(`<tr><td style="padding:8px 0; font-size:13px; color:#6b7280; width:150px;">%s</td><td style="padding:8px 0; font-size:13px; color:#111827;">%s</td></tr>`,
			utils.EscapeEmailHTML(label), utils.EscapeEmailHTML(value))
	}
	rows := `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">`
	rows += row("Name", name)
	if answers["preferred_name"] != "" {
		rows += row("Preferred name", answers["preferred_name"])
	}
	rows += row("Email or phone", contact)
	if answers["event"] != "" {
		rows += row("Event / project", answers["event"])
	}
	rows += row("Submitted", time.Unix(signedAt, 0).In(sfluvTimeZone).Format("January 2, 2006 at 3:04 PM MST"))
	rows += `</table>`

	next := "Find this person's original signature under Website → Forms &amp; waivers → Signatures, open it, and mark their consent as withdrawn."
	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/"); base != "" {
		next = fmt.Sprintf(`Find this person's original signature in the <a href="%s/website" style="color:#8f2e2e;">Website tools</a> (Forms &amp; waivers → Signatures), open it, and mark their consent as withdrawn.`,
			utils.EscapeEmailHTML(base))
	}
	rows += `<p style="margin:18px 0 4px; font-size:13px; line-height:1.55; color:#111827;">` + next + `</p>`

	title := "Withdrawal request: " + formTitle
	htmlContent := utils.BuildStyledEmail(title, "Someone asked to withdraw their consent on sfluv.org.", rows)
	if err := emailSender.SendEmail(siteWithdrawalAlertTo(), "SFLuv", title, htmlContent, utils.NotificationFromEmail(), "SFLuv website"); err != nil {
		a.logger.Logf("error sending withdrawal alert: %s", err)
	}
}

// decodeSiteBodyLimited reads a JSON body already wrapped in MaxBytesReader.
func decodeSiteBodyLimited(w http.ResponseWriter, r *http.Request, into any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeSiteError(w, http.StatusRequestEntityTooLarge, "That signature was too large to send. Please try again.")
			return false
		}
		writeSiteError(w, http.StatusBadRequest, "That request could not be read.")
		return false
	}
	return true
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

// ── Confirmation email ───────────────────────────────────────────────────────

// renderSiteFormEmailBody turns the form's plain-text body into email HTML,
// following the same convention the website uses: "## " is a heading, a blank
// line separates paragraphs, "- " lines are bullets, and {{choices}} marks where
// the options go.
func renderSiteFormEmailBody(body string, choices []structs.SiteFormChoice, chosen string) string {
	var out strings.Builder
	for _, block := range strings.Split(body, "\n\n") {
		block = strings.TrimSpace(block)
		switch {
		case block == "":
		case block == "{{choices}}":
			for _, choice := range choices {
				mark := "☐"
				if choice.Id == chosen {
					mark = "☑"
				}
				out.WriteString(fmt.Sprintf(`<p style="margin:6px 0; font-size:13px; color:#111827;">%s <strong>%s</strong> %s</p>`,
					mark, utils.EscapeEmailHTML(choice.Label), utils.EscapeEmailHTML(choice.Description)))
			}
		case strings.HasPrefix(block, "## "):
			out.WriteString(fmt.Sprintf(`<h3 style="margin:18px 0 6px; font-size:13px; letter-spacing:.04em; text-transform:uppercase; color:#8f2e2e;">%s</h3>`,
				utils.EscapeEmailHTML(strings.TrimPrefix(block, "## "))))
		case strings.HasPrefix(block, "- "):
			out.WriteString(`<ul style="margin:6px 0; padding-left:20px; font-size:13px; color:#111827;">`)
			for _, line := range strings.Split(block, "\n") {
				out.WriteString("<li>" + utils.EscapeEmailHTML(strings.TrimPrefix(strings.TrimSpace(line), "- ")) + "</li>")
			}
			out.WriteString("</ul>")
		default:
			out.WriteString(fmt.Sprintf(`<p style="margin:8px 0; font-size:13px; line-height:1.55; color:#111827;">%s</p>`,
				strings.ReplaceAll(utils.EscapeEmailHTML(block), "\n", "<br>")))
		}
	}
	return out.String()
}

// sendSiteSignatureConfirmation emails the signer a record of exactly what they
// agreed to. It runs after the signature is already stored, so a mail failure
// is logged and goes no further.
func (a *AppService) sendSiteSignatureConfirmation(to string, name string, version *structs.SiteFormVersion, chosenID string, event string, signedAt int64) (sent bool) {
	defer func() {
		if rec := recover(); rec != nil {
			a.logger.Logf("recovered while sending a signature confirmation: %v", rec)
			sent = false
		}
	}()

	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		a.logger.Logf("signature confirmation not sent: email sender is not configured")
		return false
	}

	chosenLabel := ""
	for _, choice := range version.Config.Choices {
		if choice.Id == chosenID {
			chosenLabel = choice.Label
		}
	}

	rows := fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr><td style="padding:8px 0; font-size:13px; color:#6b7280; width:150px;">Signed by</td><td style="padding:8px 0; font-size:13px; color:#111827;">%s</td></tr>
  <tr><td style="padding:8px 0; font-size:13px; color:#6b7280;">Signed on</td><td style="padding:8px 0; font-size:13px; color:#111827;">%s</td></tr>`,
		utils.EscapeEmailHTML(name), utils.EscapeEmailHTML(time.Unix(signedAt, 0).In(sfluvTimeZone).Format("January 2, 2006 at 3:04 PM MST")))
	if event != "" {
		rows += fmt.Sprintf(`<tr><td style="padding:8px 0; font-size:13px; color:#6b7280;">Event / project</td><td style="padding:8px 0; font-size:13px; color:#111827;">%s</td></tr>`, utils.EscapeEmailHTML(event))
	}
	if chosenLabel != "" {
		rows += fmt.Sprintf(`<tr><td style="padding:8px 0; font-size:13px; color:#6b7280;">Scope</td><td style="padding:8px 0; font-size:13px; color:#111827;">%s</td></tr>`, utils.EscapeEmailHTML(chosenLabel))
	}
	rows += `</table>
<p style="margin:18px 0 4px; font-size:12px; color:#6b7280;">This is the exact text you agreed to:</p>`

	rows += renderSiteFormEmailBody(version.Body, version.Config.Choices, chosenID)
	if len(version.Config.Choices) > 0 && !strings.Contains(version.Body, "{{choices}}") {
		rows += renderSiteFormEmailBody("{{choices}}", version.Config.Choices, chosenID)
	}

	title := "Your signed copy: " + version.Title
	htmlContent := utils.BuildStyledEmail(title, "Thank you. Keep this email for your records.", rows)
	if err := emailSender.SendEmail(to, name, title, htmlContent, utils.NotificationFromEmail(), "SFLuv"); err != nil {
		a.logger.Logf("error sending signature confirmation: %s", err)
		return false
	}
	return true
}

// sfluvTimeZone is where SFLuv is, for times people read. The zone database is
// embedded (time/tzdata), so this works on a server that has none installed.
var sfluvTimeZone = func() *time.Location {
	if loc, err := time.LoadLocation("America/Los_Angeles"); err == nil {
		return loc
	}
	return time.UTC
}()
