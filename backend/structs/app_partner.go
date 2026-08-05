package structs

// Partner is an organization shown in the public site's partner carousel.
//
// LogoURL points at a public image endpoint rather than carrying bytes, so the
// carousel can cache logos and the list payload stays small. Width/Height are
// captured at upload so the carousel can reserve layout space before the image
// loads (the strip sizes every logo into a fixed box).
type Partner struct {
	Id         string  `json:"id"`
	Name       string  `json:"name"`
	LinkURL    string  `json:"link_url"`
	LogoURL    *string `json:"logo_url"`
	LogoWidth  int     `json:"logo_width"`
	LogoHeight int     `json:"logo_height"`
	Position   int     `json:"position"`
	Active     bool    `json:"active"`
	CreatedAt  int64   `json:"created_at"`
	UpdatedAt  int64   `json:"updated_at"`
}

// PartnerRequest creates or updates a partner. Logo bytes are uploaded
// separately (multipart) so this stays a plain JSON body.
type PartnerRequest struct {
	Name     string `json:"name"`
	LinkURL  string `json:"link_url"`
	Active   *bool  `json:"active,omitempty"`
	Position *int   `json:"position,omitempty"`
}

// PartnerReorderRequest replaces the display order in one call, so dragging
// several rows does not produce a burst of individual updates that could
// interleave.
type PartnerReorderRequest struct {
	OrderedIds []string `json:"ordered_ids"`
}
