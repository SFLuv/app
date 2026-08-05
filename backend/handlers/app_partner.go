package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const maxPartnerLogoBytes = 5 << 20 // 5 MiB

func partnerLogoURL(id string) string {
	return publicURL("/partners/" + id + "/logo")
}

func mapPartner(row *db.PartnerRow) *structs.Partner {
	partner := &structs.Partner{
		Id:         row.Id,
		Name:       row.Name,
		LinkURL:    row.LinkURL,
		LogoWidth:  row.LogoWidth,
		LogoHeight: row.LogoHeight,
		Position:   row.Position,
		Active:     row.Active,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
	if row.HasLogo {
		url := partnerLogoURL(row.Id)
		partner.LogoURL = &url
	}
	return partner
}

func mapPartners(rows []*db.PartnerRow) []*structs.Partner {
	partners := make([]*structs.Partner, 0, len(rows))
	for _, row := range rows {
		partners = append(partners, mapPartner(row))
	}
	return partners
}

var (
	svgWidthAttrPattern   = regexp.MustCompile(`(?i)\bwidth\s*=\s*"([0-9.]+)`)
	svgHeightAttrPattern  = regexp.MustCompile(`(?i)\bheight\s*=\s*"([0-9.]+)`)
	svgViewBoxAttrPattern = regexp.MustCompile(`(?i)\bviewBox\s*=\s*"\s*[-0-9.]+[ ,]+[-0-9.]+[ ,]+([0-9.]+)[ ,]+([0-9.]+)`)
)

// partnerLogoDimensions returns the intrinsic size of an uploaded logo.
//
// The carousel sizes every logo into a fixed box but still needs intrinsic
// dimensions to reserve layout space before the image loads. Raster formats are
// decoded directly; SVG is handled separately because logos are very often SVG
// and the standard image decoders cannot read it — width/height attributes
// first, then viewBox, which is what scalable logos usually carry instead.
// Returns 0x0 when the size cannot be determined, which callers treat as
// "unknown" rather than as an error.
func partnerLogoDimensions(data []byte, contentType string) (int, int) {
	if strings.Contains(strings.ToLower(contentType), "svg") {
		head := data
		if len(head) > 4096 {
			head = head[:4096]
		}
		text := string(head)

		widthMatch := svgWidthAttrPattern.FindStringSubmatch(text)
		heightMatch := svgHeightAttrPattern.FindStringSubmatch(text)
		if len(widthMatch) == 2 && len(heightMatch) == 2 {
			width, widthErr := strconv.ParseFloat(widthMatch[1], 64)
			height, heightErr := strconv.ParseFloat(heightMatch[1], 64)
			if widthErr == nil && heightErr == nil && width > 0 && height > 0 {
				return int(math.Round(width)), int(math.Round(height))
			}
		}

		if viewBox := svgViewBoxAttrPattern.FindStringSubmatch(text); len(viewBox) == 3 {
			width, widthErr := strconv.ParseFloat(viewBox[1], 64)
			height, heightErr := strconv.ParseFloat(viewBox[2], 64)
			if widthErr == nil && heightErr == nil && width > 0 && height > 0 {
				return int(math.Round(width)), int(math.Round(height))
			}
		}
		return 0, 0
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

// GetPublicPartners serves the partner carousel for the public site. No auth:
// it is the same data any visitor sees rendered on the marketing page.
func (a *AppService) GetPublicPartners(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.GetPartners(r.Context(), true)
	if err != nil {
		if !isClientGone(err) {
			a.logger.Logf("error listing public partners: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300, stale-while-revalidate=3600")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"partners": mapPartners(rows)})
}

func (a *AppService) GetPartnerLogo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logo, err := a.db.GetPartnerLogo(r.Context(), id)
	if err != nil {
		if err == pgx.ErrNoRows || strings.Contains(err.Error(), "no logo") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading partner logo %s: %s", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	contentType := logo.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Short cache: a logo can be replaced in place under the same id, and a
	// stale partner logo on the public site is worse than an extra request.
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(logo.Data)
}

func (a *AppService) AdminGetPartners(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.GetPartners(r.Context(), false)
	if err != nil {
		a.logger.Logf("error listing partners: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"partners": mapPartners(rows)})
}

func (a *AppService) AdminCreatePartner(w http.ResponseWriter, r *http.Request) {
	req, ok := decodePartnerRequest(w, r)
	if !ok {
		return
	}

	row, err := a.db.CreatePartner(r.Context(), req)
	if err != nil {
		a.logger.Logf("error creating partner: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(mapPartner(row))
}

func (a *AppService) AdminUpdatePartner(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	req, ok := decodePartnerRequest(w, r)
	if !ok {
		return
	}

	row, err := a.db.UpdatePartner(r.Context(), id, req)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error updating partner %s: %s", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(mapPartner(row))
}

func (a *AppService) AdminDeletePartner(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.db.DeletePartner(r.Context(), id); err != nil {
		a.logger.Logf("error deleting partner %s: %s", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) AdminReorderPartners(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.PartnerReorderRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.db.ReorderPartners(r.Context(), req.OrderedIds); err != nil {
		a.logger.Logf("error reordering partners: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	rows, err := a.db.GetPartners(r.Context(), false)
	if err != nil {
		a.logger.Logf("error listing partners after reorder: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"partners": mapPartners(rows)})
}

// AdminUploadPartnerLogo accepts a multipart logo upload. The uploaded bytes
// are sniffed rather than trusted: a browser-supplied content type is easy to
// get wrong and this response is served back to every visitor of the public
// site.
func (a *AppService) AdminUploadPartnerLogo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := r.ParseMultipartForm(maxPartnerLogoBytes); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("could not read upload"))
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("logo")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("logo file is required"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxPartnerLogoBytes+1))
	if err != nil {
		a.logger.Logf("error reading partner logo upload for %s: %s", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if len(data) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("logo file is empty"))
		return
	}
	if len(data) > maxPartnerLogoBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte(fmt.Sprintf("logo must be %d MB or smaller", maxPartnerLogoBytes>>20)))
		return
	}

	contentType := resolvePartnerLogoContentType(data, header.Filename)
	if contentType == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("logo must be a PNG, JPEG, GIF, WebP, or SVG image"))
		return
	}

	width, height := partnerLogoDimensions(data, contentType)

	if err := a.db.SetPartnerLogo(r.Context(), id, data, contentType, width, height); err != nil {
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error storing partner logo for %s: %s", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	row, err := a.db.GetPartner(r.Context(), id)
	if err != nil {
		a.logger.Logf("error reloading partner %s after logo upload: %s", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(mapPartner(row))
}

// resolvePartnerLogoContentType sniffs the real type of the uploaded bytes and
// returns "" for anything that is not an allowed image format. SVG is detected
// by content because http.DetectContentType reports it as plain text or XML.
func resolvePartnerLogoContentType(data []byte, filename string) string {
	sniffed := http.DetectContentType(data)

	switch {
	case strings.HasPrefix(sniffed, "image/png"):
		return "image/png"
	case strings.HasPrefix(sniffed, "image/jpeg"):
		return "image/jpeg"
	case strings.HasPrefix(sniffed, "image/gif"):
		return "image/gif"
	case strings.HasPrefix(sniffed, "image/webp"):
		return "image/webp"
	}

	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	lowered := strings.ToLower(string(head))
	if strings.Contains(lowered, "<svg") &&
		(strings.HasSuffix(strings.ToLower(filename), ".svg") || strings.Contains(sniffed, "text") || strings.Contains(sniffed, "xml")) {
		return "image/svg+xml"
	}

	return ""
}

func decodePartnerRequest(w http.ResponseWriter, r *http.Request) (*structs.PartnerRequest, bool) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}

	var req structs.PartnerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("name is required"))
		return nil, false
	}

	req.LinkURL = strings.TrimSpace(req.LinkURL)
	if req.LinkURL != "" && !isSafePartnerLink(req.LinkURL) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("link must be an http(s) URL"))
		return nil, false
	}

	return &req, true
}

// isSafePartnerLink restricts partner links to absolute http(s) URLs.
//
// This value is rendered as an href on the public marketing site, so an
// admin-supplied "javascript:" or "data:" URL would become script execution for
// every visitor. Allow-listing the two schemes we actually want is the check
// that cannot be worked around by encoding tricks.
func isSafePartnerLink(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if parsed.Host == "" {
		return false
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	}
	return false
}
