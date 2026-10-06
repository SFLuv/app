package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// Admin routes for the editable site and Forms and Waivers. See
// docs/features/website-editing-and-forms.md.

const spotlightKey = "spotlight"

func writeSiteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeSiteError sends a message a non-technical editor can act on. Unlike most
// of the API this carries text, because the person reading it is staff using a
// form, not a developer reading a status code.
func writeSiteError(w http.ResponseWriter, status int, message string) {
	writeSiteJSON(w, status, map[string]string{"message": message})
}

func siteFileURL(id string, filename string) string {
	return publicURL("/site/files/" + id + "/" + url.PathEscape(filename))
}

func decodeSiteBody(w http.ResponseWriter, r *http.Request, into any) bool {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || json.Unmarshal(body, into) != nil {
		writeSiteError(w, http.StatusBadRequest, "That request could not be read.")
		return false
	}
	return true
}

func (a *AppService) siteActivity(r *http.Request, capability string, action string, entity string, entityId string, summary string) {
	a.db.RecordSiteActivity(r.Context(), utils.GetDid(r), capability, action, entity, entityId, summary)
}

func (a *AppService) siteServerError(w http.ResponseWriter, what string, err error) {
	if isClientGone(err) {
		return
	}
	a.logger.Logf("error %s: %s", what, err)
	writeSiteError(w, http.StatusInternalServerError, "Something went wrong on our end. Please try again.")
}

// ── Capabilities ─────────────────────────────────────────────────────────────

// SiteCapabilitiesFor is what a user may edit: everything for an admin,
// otherwise whatever private credentials they hold.
func (a *AppService) SiteCapabilitiesFor(ctx context.Context, userId string) structs.SiteCapabilities {
	if a.IsAdmin(ctx, userId) {
		return structs.SiteCapabilities{Banner: true, Financials: true, Forms: true}
	}
	has := func(credential string) bool {
		held, err := a.db.UserHasActiveCredential(ctx, userId, credential)
		if err != nil {
			a.logger.Logf("error checking site capability %s for %s: %s", credential, userId, err)
			return false
		}
		return held
	}
	return structs.SiteCapabilities{
		Banner:     has(structs.SiteCapabilityBanner),
		Financials: has(structs.SiteCapabilityFinancials),
		Forms:      has(structs.SiteCapabilityForms),
	}
}

// CanEditSite reports whether a user holds a capability. An empty capability
// means "any of them", for the routes every editor shares (uploads, activity).
func (a *AppService) CanEditSite(ctx context.Context, userId string, capability string) bool {
	caps := a.SiteCapabilitiesFor(ctx, userId)
	switch capability {
	case structs.SiteCapabilityBanner:
		return caps.Banner
	case structs.SiteCapabilityFinancials:
		return caps.Financials
	case structs.SiteCapabilityForms:
		return caps.Forms
	default:
		return caps.Banner || caps.Financials || caps.Forms
	}
}

func (a *AppService) GetSiteCapabilities(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	writeSiteJSON(w, http.StatusOK, a.SiteCapabilitiesFor(r.Context(), *userDid))
}

func (a *AppService) GetSiteActivity(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	caps := a.SiteCapabilitiesFor(r.Context(), *userDid)
	allowed := []string{}
	if caps.Banner {
		allowed = append(allowed, structs.SiteCapabilityBanner)
	}
	if caps.Financials {
		allowed = append(allowed, structs.SiteCapabilityFinancials)
	}
	if caps.Forms {
		allowed = append(allowed, structs.SiteCapabilityForms)
	}

	items, err := a.db.ListSiteActivity(r.Context(), allowed, 50)
	if err != nil {
		a.siteServerError(w, "listing site activity", err)
		return
	}
	writeSiteJSON(w, http.StatusOK, map[string]any{"activity": items})
}

// ── Files ────────────────────────────────────────────────────────────────────

// UploadSiteFile stores a PDF or image for use on the site. The bytes are
// sniffed rather than trusted: the browser's idea of the type is easy to get
// wrong, and whatever is stored here is served to every visitor.
func (a *AppService) UploadSiteFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxSiteFileBytes); err != nil {
		writeSiteError(w, http.StatusBadRequest, "That upload could not be read. Files can be up to 25 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeSiteError(w, http.StatusBadRequest, "Choose a file to upload.")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxSiteFileBytes+1))
	if err != nil {
		a.siteServerError(w, "reading a site file upload", err)
		return
	}
	if len(data) == 0 {
		writeSiteError(w, http.StatusBadRequest, "That file is empty.")
		return
	}
	if len(data) > maxSiteFileBytes {
		writeSiteError(w, http.StatusRequestEntityTooLarge, "That file is too large. Files can be up to 25 MB.")
		return
	}

	contentType, isImage := sniffSiteFile(data)
	if contentType == "" {
		writeSiteError(w, http.StatusBadRequest, "Upload a PDF, or a PNG, JPEG, WebP or GIF image.")
		return
	}
	width, height := 0, 0
	if isImage {
		width, height = siteImageDimensions(data)
	}

	stored, err := a.db.CreateSiteFile(r.Context(), cleanSiteFilename(header.Filename), contentType, width, height, data, utils.GetDid(r))
	if err != nil {
		a.siteServerError(w, "storing a site file", err)
		return
	}
	stored.URL = siteFileURL(stored.Id, stored.Filename)
	writeSiteJSON(w, http.StatusCreated, stored)

	// Uploads are the moment new orphans can appear, so tidy up then. A day's
	// grace keeps a file someone uploaded but has not saved yet.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if n, err := a.db.DeleteUnreferencedSiteFiles(ctx, time.Now().Add(-24*time.Hour).Unix()); err != nil {
			a.logger.Logf("error cleaning up site files: %s", err)
		} else if n > 0 {
			a.logger.Logf("removed %d unused site files", n)
		}
	}()
}

// ── Homepage highlights (the Spotlight carousel) ─────────────────────────────

// fillSpotlightPreviews tells the admin panel where each slide's picture can be
// shown. Output only; never stored.
func (a *AppService) fillSpotlightPreviews(ctx context.Context, sp *structs.SiteSpotlight) {
	for i := range sp.Slides {
		sl := &sp.Slides[i]
		sl.ImagePreviewURL = nil
		if sl.ImageFileId != nil {
			if file, err := a.db.GetSiteFile(ctx, *sl.ImageFileId); err == nil {
				u := siteFileURL(file.Id, file.Filename)
				sl.ImagePreviewURL = &u
			}
		} else if sl.ImageURL != nil {
			sl.ImagePreviewURL = sl.ImageURL
		}
	}
}

func (a *AppService) GetAdminSpotlight(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := &structs.SiteSpotlightAdmin{
		Current:  structs.SiteSpotlight{Slides: []structs.SiteSpotlightSlide{}},
		Versions: []*structs.SiteContentVersion{},
	}

	row, err := a.db.GetSiteContent(ctx, spotlightKey)
	if err != nil {
		a.siteServerError(w, "loading the highlights", err)
		return
	}
	if row != nil {
		_ = json.Unmarshal(row.Value, &out.Current)
		out.Version = row.Version
		out.UpdatedAt = row.UpdatedAt
	}
	if out.Current.Slides == nil {
		out.Current.Slides = []structs.SiteSpotlightSlide{}
	}
	a.fillSpotlightPreviews(ctx, &out.Current)

	versions, err := a.db.ListSiteContentVersions(ctx, spotlightKey, 30)
	if err != nil {
		a.siteServerError(w, "listing highlight versions", err)
		return
	}
	for _, v := range versions {
		item := &structs.SiteContentVersion{
			Version:   v.Version,
			SavedAt:   v.SavedAt,
			SavedBy:   v.SavedBy,
			Note:      v.Note,
			IsCurrent: row != nil && v.Version == row.Version,
		}
		_ = json.Unmarshal(v.Value, &item.Value)
		if item.Value.Slides == nil {
			item.Value.Slides = []structs.SiteSpotlightSlide{}
		}
		a.fillSpotlightPreviews(ctx, &item.Value)
		out.Versions = append(out.Versions, item)
	}
	writeSiteJSON(w, http.StatusOK, out)
}

func countEnabledSlides(sp *structs.SiteSpotlight) int {
	n := 0
	for _, sl := range sp.Slides {
		if sl.Enabled {
			n++
		}
	}
	return n
}

func (a *AppService) saveSpotlight(w http.ResponseWriter, r *http.Request, sp *structs.SiteSpotlight, note string, expectedVersion int) {
	ctx := r.Context()

	for _, sl := range sp.Slides {
		if sl.ImageFileId != nil {
			file, err := a.db.GetSiteFile(ctx, *sl.ImageFileId)
			if err != nil || !strings.HasPrefix(file.ContentType, "image/") {
				writeSiteError(w, http.StatusBadRequest, "A highlight’s photo must be an image you uploaded. Upload it again.")
				return
			}
		}
		if sl.Action.AlsoOpenFileId != nil {
			if _, err := a.db.GetSiteFile(ctx, *sl.Action.AlsoOpenFileId); err != nil {
				writeSiteError(w, http.StatusBadRequest, "A button’s document could not be found. Upload it again.")
				return
			}
		}
	}

	value, err := json.Marshal(sp)
	if err != nil {
		a.siteServerError(w, "encoding the highlights", err)
		return
	}
	version, err := a.db.SaveSiteContent(ctx, spotlightKey, value, utils.GetDid(r), note, expectedVersion)
	if errors.Is(err, db.ErrSiteVersionConflict) {
		writeSiteError(w, http.StatusConflict, "Someone else saved the highlights while you were editing. Copy anything you want to keep, then reload to see their changes.")
		return
	}
	if err != nil {
		a.siteServerError(w, "saving the highlights", err)
		return
	}

	a.siteActivity(r, structs.SiteCapabilityBanner, "save", "spotlight", fmt.Sprint(version),
		fmt.Sprintf("%s (%d showing)", note, countEnabledSlides(sp)))
	a.GetAdminSpotlight(w, r)
}

func (a *AppService) PutAdminSpotlight(w http.ResponseWriter, r *http.Request) {
	var req structs.SiteSpotlightSaveRequest
	if !decodeSiteBody(w, r, &req) {
		return
	}
	sp := structs.SiteSpotlight{Slides: req.Slides}
	expected := -1
	if req.BaseVersion != nil {
		expected = *req.BaseVersion
	}
	if err := sanitizeSpotlight(&sp); err != nil {
		writeSiteError(w, http.StatusBadRequest, capitalizeFirst(err.Error())+".")
		return
	}

	note := "Edited the homepage highlights"
	if row, _ := a.db.GetSiteContent(r.Context(), spotlightKey); row != nil {
		var previous structs.SiteSpotlight
		if json.Unmarshal(row.Value, &previous) == nil {
			switch {
			case countEnabledSlides(&sp) == 0 && countEnabledSlides(&previous) > 0:
				note = "Turned all the highlights off"
			case len(sp.Slides) > len(previous.Slides):
				note = "Added a highlight"
			case len(sp.Slides) < len(previous.Slides):
				note = "Removed a highlight"
			}
		}
	}
	a.saveSpotlight(w, r, &sp, note, expected)
}

// RestoreAdminSpotlight copies an old version forward as a new one, so the
// history only ever grows and an undo can itself be undone.
func (a *AppService) RestoreAdminSpotlight(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version int `json:"version"`
	}
	if !decodeSiteBody(w, r, &req) {
		return
	}
	value, err := a.db.GetSiteContentVersion(r.Context(), spotlightKey, req.Version)
	if errors.Is(err, db.ErrSiteNotFound) {
		writeSiteError(w, http.StatusNotFound, "That version no longer exists.")
		return
	}
	if err != nil {
		a.siteServerError(w, "loading a highlights version", err)
		return
	}
	var sp structs.SiteSpotlight
	if err := json.Unmarshal(value, &sp); err != nil {
		a.siteServerError(w, "reading a highlights version", err)
		return
	}
	a.saveSpotlight(w, r, &sp, fmt.Sprintf("Restored version %d", req.Version), -1)
}

// ── Financial documents ──────────────────────────────────────────────────────

func (a *AppService) finishFinancialDocument(doc *structs.FinancialDocument) {
	if doc.FileId != nil && doc.Filename != nil {
		doc.Href = siteFileURL(*doc.FileId, *doc.Filename)
	}
}

func (a *AppService) requireSitePDF(w http.ResponseWriter, ctx context.Context, fileId string) bool {
	file, err := a.db.GetSiteFile(ctx, fileId)
	if err != nil || file.ContentType != "application/pdf" {
		writeSiteError(w, http.StatusBadRequest, "Upload the document as a PDF.")
		return false
	}
	return true
}

func (a *AppService) GetAdminFinancials(w http.ResponseWriter, r *http.Request) {
	docs, err := a.db.ListFinancialDocuments(r.Context(), true)
	if err != nil {
		a.siteServerError(w, "listing financial documents", err)
		return
	}
	for _, doc := range docs {
		a.finishFinancialDocument(doc)
	}
	writeSiteJSON(w, http.StatusOK, map[string]any{"documents": docs})
}

func (a *AppService) PostAdminFinancial(w http.ResponseWriter, r *http.Request) {
	var req structs.FinancialDocumentRequest
	if !decodeSiteBody(w, r, &req) {
		return
	}
	if err := sanitizeFinancialRequest(&req); err != nil {
		writeSiteError(w, http.StatusBadRequest, capitalizeFirst(err.Error())+".")
		return
	}
	if req.FileId == "" {
		writeSiteError(w, http.StatusBadRequest, "Upload the document first.")
		return
	}
	if !a.requireSitePDF(w, r.Context(), req.FileId) {
		return
	}

	label := financialLabel(&req)
	id, err := a.db.CreateFinancialDocument(r.Context(), &req, label, utils.GetDid(r))
	if err != nil {
		a.siteServerError(w, "creating a financial document", err)
		return
	}
	doc, err := a.db.GetFinancialDocument(r.Context(), id)
	if err != nil {
		a.siteServerError(w, "reloading a financial document", err)
		return
	}
	a.finishFinancialDocument(doc)
	a.siteActivity(r, structs.SiteCapabilityFinancials, "add", "financial_document", id, "Added "+label)
	writeSiteJSON(w, http.StatusCreated, doc)
}

func (a *AppService) PutAdminFinancial(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req structs.FinancialDocumentRequest
	if id == "" || !decodeSiteBody(w, r, &req) {
		return
	}
	if err := sanitizeFinancialRequest(&req); err != nil {
		writeSiteError(w, http.StatusBadRequest, capitalizeFirst(err.Error())+".")
		return
	}
	if req.FileId != "" && !a.requireSitePDF(w, r.Context(), req.FileId) {
		return
	}

	label := financialLabel(&req)
	if err := a.db.UpdateFinancialDocument(r.Context(), id, &req, label); err != nil {
		if errors.Is(err, db.ErrSiteNotFound) {
			writeSiteError(w, http.StatusNotFound, "That document no longer exists.")
			return
		}
		a.siteServerError(w, "updating a financial document", err)
		return
	}
	doc, err := a.db.GetFinancialDocument(r.Context(), id)
	if err != nil {
		a.siteServerError(w, "reloading a financial document", err)
		return
	}
	a.finishFinancialDocument(doc)
	a.siteActivity(r, structs.SiteCapabilityFinancials, "edit", "financial_document", id, "Edited "+label)
	writeSiteJSON(w, http.StatusOK, doc)
}

func (a *AppService) setFinancialRemoved(w http.ResponseWriter, r *http.Request, removed bool) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := a.db.SetFinancialDocumentRemoved(r.Context(), id, removed); err != nil {
		if errors.Is(err, db.ErrSiteNotFound) {
			writeSiteError(w, http.StatusNotFound, "That document no longer exists.")
			return
		}
		a.siteServerError(w, "updating a financial document", err)
		return
	}
	doc, err := a.db.GetFinancialDocument(r.Context(), id)
	if err != nil {
		a.siteServerError(w, "reloading a financial document", err)
		return
	}
	a.finishFinancialDocument(doc)

	verb, action := "Removed", "remove"
	if !removed {
		verb, action = "Restored", "restore"
	}
	a.siteActivity(r, structs.SiteCapabilityFinancials, action, "financial_document", id, verb+" "+doc.Label)
	writeSiteJSON(w, http.StatusOK, doc)
}

func (a *AppService) DeleteAdminFinancial(w http.ResponseWriter, r *http.Request) {
	a.setFinancialRemoved(w, r, true)
}

func (a *AppService) RestoreAdminFinancial(w http.ResponseWriter, r *http.Request) {
	a.setFinancialRemoved(w, r, false)
}

// ── Forms ────────────────────────────────────────────────────────────────────

func (a *AppService) loadSiteFormDetail(ctx context.Context, id string) (*structs.SiteFormDetail, error) {
	summary, err := a.db.GetSiteFormSummaryById(ctx, id)
	if err != nil {
		return nil, err
	}
	versions, err := a.db.ListSiteFormVersions(ctx, id)
	if err != nil {
		return nil, err
	}
	detail := &structs.SiteFormDetail{SiteFormSummary: *summary, Versions: versions}
	for _, v := range versions {
		if v.Version == summary.CurrentVersion {
			detail.Current = *v
		}
	}
	return detail, nil
}

func (a *AppService) writeSiteFormDetail(w http.ResponseWriter, r *http.Request, id string, status int) {
	detail, err := a.loadSiteFormDetail(r.Context(), id)
	if errors.Is(err, db.ErrSiteNotFound) {
		writeSiteError(w, http.StatusNotFound, "That form no longer exists.")
		return
	}
	if err != nil {
		a.siteServerError(w, "loading a form", err)
		return
	}
	writeSiteJSON(w, status, detail)
}

func (a *AppService) GetAdminForms(w http.ResponseWriter, r *http.Request) {
	forms, err := a.db.ListSiteForms(r.Context(), false)
	if err != nil {
		a.siteServerError(w, "listing forms", err)
		return
	}
	writeSiteJSON(w, http.StatusOK, map[string]any{"forms": forms})
}

func (a *AppService) GetAdminForm(w http.ResponseWriter, r *http.Request) {
	a.writeSiteFormDetail(w, r, strings.TrimSpace(r.PathValue("id")), http.StatusOK)
}

func (a *AppService) PostAdminForm(w http.ResponseWriter, r *http.Request) {
	var req structs.SiteFormRequest
	if !decodeSiteBody(w, r, &req) {
		return
	}
	if err := sanitizeFormRequest(&req, true); err != nil {
		writeSiteError(w, http.StatusBadRequest, capitalizeFirst(err.Error())+".")
		return
	}
	id, err := a.db.CreateSiteForm(r.Context(), &req, utils.GetDid(r))
	if errors.Is(err, db.ErrSiteSlugTaken) {
		writeSiteError(w, http.StatusConflict, "That web address is already used by another form. Choose a different one.")
		return
	}
	if err != nil {
		a.siteServerError(w, "creating a form", err)
		return
	}
	a.siteActivity(r, structs.SiteCapabilityForms, "create", "form", id, "Created the form “"+req.Title+"” (closed until opened)")
	a.writeSiteFormDetail(w, r, id, http.StatusCreated)
}

// PutAdminForm saves an edit as a new version. The form's web address and kind
// are fixed once created: the address is what a printed QR code points at.
func (a *AppService) PutAdminForm(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req structs.SiteFormRequest
	if id == "" || !decodeSiteBody(w, r, &req) {
		return
	}
	if err := sanitizeFormRequest(&req, false); err != nil {
		writeSiteError(w, http.StatusBadRequest, capitalizeFirst(err.Error())+".")
		return
	}
	version, changed, err := a.db.AddSiteFormVersion(r.Context(), id, &req, utils.GetDid(r))
	if errors.Is(err, db.ErrSiteNotFound) {
		writeSiteError(w, http.StatusNotFound, "That form no longer exists.")
		return
	}
	if err != nil {
		a.siteServerError(w, "saving a form", err)
		return
	}
	if changed {
		a.siteActivity(r, structs.SiteCapabilityForms, "edit", "form", id,
			fmt.Sprintf("Edited “%s” (now version %d)", req.Title, version))
	}
	a.writeSiteFormDetail(w, r, id, http.StatusOK)
}

func (a *AppService) PostAdminFormOpen(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req structs.SiteFormOpenRequest
	if r.ContentLength != 0 && !decodeSiteBody(w, r, &req) {
		return
	}
	if req.ClosesAt != nil && *req.ClosesAt <= time.Now().Unix() {
		writeSiteError(w, http.StatusBadRequest, "The closing date must be in the future.")
		return
	}
	if err := a.db.SetSiteFormOpen(r.Context(), id, true, req.ClosesAt); err != nil {
		if errors.Is(err, db.ErrSiteNotFound) {
			writeSiteError(w, http.StatusNotFound, "That form no longer exists.")
			return
		}
		a.siteServerError(w, "opening a form", err)
		return
	}
	summary := "Opened the form"
	if req.ClosesAt != nil {
		summary += " until " + time.Unix(*req.ClosesAt, 0).UTC().Format("2006-01-02 15:04 UTC")
	}
	a.siteActivity(r, structs.SiteCapabilityForms, "open", "form", id, summary)
	a.writeSiteFormDetail(w, r, id, http.StatusOK)
}

func (a *AppService) PostAdminFormClose(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := a.db.SetSiteFormOpen(r.Context(), id, false, nil); err != nil {
		if errors.Is(err, db.ErrSiteNotFound) {
			writeSiteError(w, http.StatusNotFound, "That form no longer exists.")
			return
		}
		a.siteServerError(w, "closing a form", err)
		return
	}
	a.siteActivity(r, structs.SiteCapabilityForms, "close", "form", id, "Closed the form")
	a.writeSiteFormDetail(w, r, id, http.StatusOK)
}

// ── Signatures ───────────────────────────────────────────────────────────────

func (a *AppService) GetAdminFormSignatures(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if _, err := a.db.GetSiteFormSummaryById(r.Context(), id); err != nil {
		writeSiteError(w, http.StatusNotFound, "That form no longer exists.")
		return
	}
	signatures, err := a.db.ListSiteFormSignatures(r.Context(), id)
	if err != nil {
		a.siteServerError(w, "listing signatures", err)
		return
	}
	writeSiteJSON(w, http.StatusOK, map[string]any{"signatures": signatures})
}

func (a *AppService) GetAdminSignature(w http.ResponseWriter, r *http.Request) {
	sig, err := a.db.GetSiteFormSignature(r.Context(), strings.TrimSpace(r.PathValue("sid")))
	if errors.Is(err, db.ErrSiteNotFound) {
		writeSiteError(w, http.StatusNotFound, "That signature no longer exists.")
		return
	}
	if err != nil {
		a.siteServerError(w, "loading a signature", err)
		return
	}
	// The exact words this person agreed to, not whatever the form says today.
	if signed, err := a.db.GetSiteFormVersion(r.Context(), sig.FormId, sig.Version); err == nil {
		sig.Signed = signed
	}
	writeSiteJSON(w, http.StatusOK, sig)
}

func (a *AppService) PostAdminSignatureWithdraw(w http.ResponseWriter, r *http.Request) {
	sid := strings.TrimSpace(r.PathValue("sid"))
	req := structs.SiteWithdrawRequest{Withdrawn: true}
	if r.ContentLength != 0 && !decodeSiteBody(w, r, &req) {
		return
	}
	if err := a.db.SetSiteSignatureWithdrawn(r.Context(), sid, req.Withdrawn); err != nil {
		if errors.Is(err, db.ErrSiteNotFound) {
			writeSiteError(w, http.StatusNotFound, "That signature no longer exists.")
			return
		}
		a.siteServerError(w, "updating a signature", err)
		return
	}
	verb := "Marked a signature as withdrawn"
	if !req.Withdrawn {
		verb = "Cleared the withdrawn mark on a signature"
	}
	a.siteActivity(r, structs.SiteCapabilityForms, "withdraw", "signature", sid, verb)
	a.GetAdminSignature(w, r)
}

// csvSafe stops a spreadsheet treating a signer's text as a formula. These
// cells hold whatever a stranger typed into a public form.
func csvSafe(value string) string {
	if value != "" && strings.ContainsAny(value[:1], "=+-@\t\r") {
		return "'" + value
	}
	return value
}

func (a *AppService) GetAdminFormSignaturesCSV(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	form, err := a.db.GetSiteFormSummaryById(r.Context(), id)
	if err != nil {
		writeSiteError(w, http.StatusNotFound, "That form no longer exists.")
		return
	}
	signatures, err := a.db.ListSiteFormSignatures(r.Context(), id)
	if err != nil {
		a.siteServerError(w, "exporting signatures", err)
		return
	}
	versions, err := a.db.ListSiteFormVersions(r.Context(), id)
	if err != nil {
		a.siteServerError(w, "exporting signatures", err)
		return
	}
	choiceLabels := map[int]map[string]string{}
	for _, v := range versions {
		labels := map[string]string{}
		for _, c := range v.Config.Choices {
			labels[c.Id] = c.Label
		}
		choiceLabels[v.Version] = labels
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-signatures.csv"`, form.Slug))
	out := csv.NewWriter(w)
	_ = out.Write([]string{
		"Signed (UTC)", "Form", "Version", "Name", "Preferred name", "Contact", "Event / project",
		"Event date", "Scope chosen", "Under 18", "Guardian name", "Guardian relationship",
		"Agreed to e-sign", "Withdrawn (UTC)", "Record id",
	})
	for _, s := range signatures {
		var answers struct {
			PreferredName string `json:"preferred_name"`
			Event         string `json:"event"`
			EventDate     string `json:"event_date"`
			Choice        string `json:"choice"`
		}
		_ = json.Unmarshal(s.Answers, &answers)
		scope := choiceLabels[s.Version][answers.Choice]
		if scope == "" {
			scope = answers.Choice
		}
		withdrawn := ""
		if s.WithdrawnAt != nil {
			withdrawn = time.Unix(*s.WithdrawnAt, 0).UTC().Format(time.RFC3339)
		}
		_ = out.Write([]string{
			time.Unix(s.SignedAt, 0).UTC().Format(time.RFC3339),
			csvSafe(s.FormTitle), fmt.Sprint(s.Version),
			csvSafe(s.SignerName), csvSafe(answers.PreferredName), csvSafe(s.Contact),
			csvSafe(answers.Event), csvSafe(answers.EventDate), csvSafe(scope),
			fmt.Sprint(s.IsMinor), csvSafe(s.GuardianName), csvSafe(s.GuardianRelationship),
			fmt.Sprint(s.EsignConsent), withdrawn, s.Id,
		})
	}
	out.Flush()
}

func capitalizeFirst(value string) string {
	// By character, not byte: messages can open with a multi-byte one, like “.
	first, size := utf8.DecodeRuneInString(value)
	if first == utf8.RuneError {
		return value
	}
	return string(unicode.ToUpper(first)) + value[size:]
}
