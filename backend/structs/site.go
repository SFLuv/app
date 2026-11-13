package structs

import "encoding/json"

// Types for the admin-editable parts of the public site and for Forms and
// Waivers. See docs/features/website-editing-and-forms.md.

// Non-admins edit the site with private credentials: Website editor covers
// every part, and each part also has its own credential, listed under Website
// editor when granting. An admin holds all of them implicitly.
const (
	SiteEditorCredential     = "website_editor"
	SiteCapabilityBanner     = "website_banner"
	SiteCapabilityFinancials = "website_financials"
	SiteCapabilityForms      = "website_forms"
	SiteCapabilityPastEvents = "website_past_events"
	SiteCapabilityLiveEvents = "website_live_events"
	SiteCapabilityResources  = "website_resources"
)

// ── Files ────────────────────────────────────────────────────────────────────

type SiteFile struct {
	Id          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int    `json:"size_bytes"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	URL         string `json:"url"`
	CreatedAt   int64  `json:"created_at"`
}

// SiteFileData is a stored file with its bytes, for serving.
type SiteFileData struct {
	Id          string
	Filename    string
	ContentType string
	Width       int
	Height      int
	Data        []byte
}

// ── Homepage banner (the Spotlight carousel) ─────────────────────────────

type SiteSpotlightAction struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	// Open Href in a new tab (for off-site links).
	NewTab bool `json:"new_tab"`
	// Opened in a new tab on the same click, while this tab follows Href.
	AlsoOpenFileId *string `json:"also_open_file_id"`
	AlsoOpenURL    *string `json:"also_open_url"`
}

// SiteSpotlightSlide is one card in the carousel, as stored: file references,
// not URLs.
type SiteSpotlightSlide struct {
	// Stable key. Assigned by the server when a slide is first saved.
	Id      string `json:"id"`
	Enabled bool   `json:"enabled"`
	// Short tag above the title, e.g. "Now published" or "Every Sunday".
	Label string `json:"label"`
	Title string `json:"title"`
	Body  string `json:"body"`

	ImageFileId *string `json:"image_file_id"`
	// A picture already hosted by the site itself (a relative path), for slides
	// that existed before editing moved here. An uploaded image wins.
	ImageURL    *string `json:"image_url"`
	ImageWidth  int     `json:"image_width"`
	ImageHeight int     `json:"image_height"`
	ImageAlt    string  `json:"image_alt"`
	// CSS object-position for the strip the photo is cropped to, e.g. "center 30%".
	ImagePosition string `json:"image_position"`
	// Output only: where the admin panel can show the picture. Never stored.
	ImagePreviewURL *string `json:"image_preview_url,omitempty"`

	Action SiteSpotlightAction `json:"action"`

	// Ties the slide to a recurring volunteer event: words from the event's
	// title, in order. The site looks up the next matching event.
	EventMatch string `json:"event_match"`
}

type SiteSpotlight struct {
	Slides []SiteSpotlightSlide `json:"slides"`
}

// SiteSpotlightSaveRequest is an edit, plus the version the editor started
// from so a save cannot silently overwrite someone else's.
type SiteSpotlightSaveRequest struct {
	Slides      []SiteSpotlightSlide `json:"slides"`
	BaseVersion *int                 `json:"base_version"`
}

type SitePublicImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Alt    string `json:"alt"`
}

type SitePublicAction struct {
	Label    string `json:"label"`
	Href     string `json:"href"`
	NewTab   bool   `json:"new_tab"`
	AlsoOpen string `json:"also_open"`
}

// SitePublicSlide is what the public site renders: file ids resolved to URLs.
type SitePublicSlide struct {
	Id            string           `json:"id"`
	Label         string           `json:"label"`
	Title         string           `json:"title"`
	Body          string           `json:"body"`
	Image         SitePublicImage  `json:"image"`
	ImagePosition string           `json:"image_position"`
	Action        SitePublicAction `json:"action"`
	EventMatch    string           `json:"event_match"`
}

type SitePublicSpotlight struct {
	Slides []SitePublicSlide `json:"slides"`
}

type SiteContentVersion struct {
	Version   int           `json:"version"`
	SavedAt   int64         `json:"saved_at"`
	SavedBy   string        `json:"saved_by"`
	Note      string        `json:"note"`
	Value     SiteSpotlight `json:"value"`
	IsCurrent bool          `json:"is_current"`
}

type SiteSpotlightAdmin struct {
	Current   SiteSpotlight         `json:"current"`
	Version   int                   `json:"version"`
	UpdatedAt int64                 `json:"updated_at"`
	Versions  []*SiteContentVersion `json:"versions"`
}

// ── Resources page ───────────────────────────────────────────────────────────
//
// The whole Resources page as one document, edited and versioned as a whole
// like the banner: its How to videos, reading list, podcasts and technical
// links. Each item can be switched off without being deleted.

// SiteResources is the Resources page as stored.
type SiteResources struct {
	HowTo          []SiteHowToVideo      `json:"how_to"`
	Books          []SiteResourceBook    `json:"books"`
	Podcasts       []SiteResourcePodcast `json:"podcasts"`
	TechnicalLinks []SiteResourceLink    `json:"technical_links"`
}

// SiteHowToVideo is one video in the How to section.
type SiteHowToVideo struct {
	Id          string `json:"id"`
	Enabled     bool   `json:"enabled"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// VideoURL is the YouTube link as the editor pasted it. VideoId is read out
	// of it on every save, so it is never taken from the client.
	VideoURL string `json:"video_url"`
	VideoId  string `json:"video_id"`
}

// SiteResourceBook is one book in the reading list. Its cover is an uploaded
// image (CoverFileId) or one already on the site (CoverURL, with its size).
type SiteResourceBook struct {
	Id          string  `json:"id"`
	Enabled     bool    `json:"enabled"`
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Href        string  `json:"href"`
	CoverFileId *string `json:"cover_file_id"`
	CoverURL    *string `json:"cover_url"`
	CoverWidth  int     `json:"cover_width"`
	CoverHeight int     `json:"cover_height"`
	CoverAlt    string  `json:"cover_alt"`
	// For the editor only: never stored.
	CoverPreviewURL *string `json:"cover_preview_url"`
}

// SiteResourcePodcast is one podcast, with a featured episode on YouTube.
type SiteResourcePodcast struct {
	Id       string `json:"id"`
	Enabled  bool   `json:"enabled"`
	Name     string `json:"name"`
	Href     string `json:"href"`
	VideoURL string `json:"video_url"`
	VideoId  string `json:"video_id"`
}

// SiteResourceLink is one entry in Get Technical.
type SiteResourceLink struct {
	Id      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Label   string `json:"label"`
	Href    string `json:"href"`
}

type SiteResourcesSaveRequest struct {
	SiteResources
	BaseVersion *int `json:"base_version"`
}

type SiteResourcesVersion struct {
	Version   int           `json:"version"`
	SavedAt   int64         `json:"saved_at"`
	SavedBy   string        `json:"saved_by"`
	Note      string        `json:"note"`
	Value     SiteResources `json:"value"`
	IsCurrent bool          `json:"is_current"`
}

type SiteResourcesAdmin struct {
	Current   SiteResources           `json:"current"`
	Version   int                     `json:"version"`
	UpdatedAt int64                   `json:"updated_at"`
	Versions  []*SiteResourcesVersion `json:"versions"`
}

// The Resources page as the website gets it: switched-on, complete items only.

type SitePublicVideo struct {
	Id          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	VideoId     string `json:"video_id"`
	EmbedURL    string `json:"embed_url"`
	WatchURL    string `json:"watch_url"`
}

type SitePublicBook struct {
	Id     string          `json:"id"`
	Title  string          `json:"title"`
	Author string          `json:"author"`
	Href   string          `json:"href"`
	Cover  SitePublicImage `json:"cover"`
}

type SitePublicPodcast struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Href     string `json:"href"`
	EmbedURL string `json:"embed_url"`
}

type SitePublicLink struct {
	Id    string `json:"id"`
	Label string `json:"label"`
	Href  string `json:"href"`
}

type SitePublicResources struct {
	HowTo          []SitePublicVideo   `json:"how_to"`
	Books          []SitePublicBook    `json:"books"`
	Podcasts       []SitePublicPodcast `json:"podcasts"`
	TechnicalLinks []SitePublicLink    `json:"technical_links"`
}

// ── Financial documents ──────────────────────────────────────────────────────

type FinancialDocument struct {
	Id         string  `json:"id"`
	Kind       string  `json:"kind"`
	FiscalYear int     `json:"fiscal_year"`
	Period     string  `json:"period"`
	AsOf       *string `json:"as_of"`
	Label      string  `json:"label"`
	Href       string  `json:"href"`
	// "upload" for a file stored here, "legacy" for one already hosted by the
	// site itself.
	Source    string  `json:"source"`
	FileId    *string `json:"file_id"`
	Filename  *string `json:"filename"`
	RemovedAt *int64  `json:"removed_at"`
	CreatedAt int64   `json:"created_at"`
	UpdatedAt int64   `json:"updated_at"`
}

type FinancialDocumentRequest struct {
	Kind       string  `json:"kind"`
	FiscalYear int     `json:"fiscal_year"`
	Period     string  `json:"period"`
	AsOf       *string `json:"as_of"`
	Label      string  `json:"label"`
	FileId     string  `json:"file_id"`
}

type PublicFinancialDocument struct {
	// Stable, so the site can give each document a link of its own.
	Id    string `json:"id"`
	Label string `json:"label"`
	Href  string `json:"href"`
	Kind  string `json:"kind"`
}

type PublicFinancialPeriod struct {
	Label     string                    `json:"label"`
	Documents []PublicFinancialDocument `json:"documents"`
}

type PublicFinancialYear struct {
	Label      string                  `json:"label"`
	FiscalYear int                     `json:"fiscal_year"`
	Periods    []PublicFinancialPeriod `json:"periods"`
}

type PublicFinancials struct {
	Years         []PublicFinancialYear     `json:"years"`
	ImpactReports []PublicFinancialDocument `json:"impact_reports"`
}

// ── Forms ────────────────────────────────────────────────────────────────────

type SiteFormChoice struct {
	Id          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// SiteFormConfig is the set of standard-field switches. Deliberately small:
// this is a waiver tool, not a form builder.
type SiteFormConfig struct {
	// "off" | "optional" | "required"
	Contact       string `json:"contact"`
	PreferredName bool   `json:"preferred_name"`
	// "off" | "ask" (signer types it) | "fixed" (EventName, set by the editor)
	Event               string           `json:"event"`
	EventName           string           `json:"event_name"`
	ChoicesPrompt       string           `json:"choices_prompt"`
	Choices             []SiteFormChoice `json:"choices"`
	GuardianSection     bool             `json:"guardian_section"`
	ConfirmationMessage string           `json:"confirmation_message"`
}

type SiteFormVersion struct {
	Version int            `json:"version"`
	Title   string         `json:"title"`
	Summary string         `json:"summary"`
	Body    string         `json:"body"`
	Config  SiteFormConfig `json:"config"`
	SavedAt int64          `json:"saved_at"`
	SavedBy string         `json:"saved_by"`
}

type SiteFormSummary struct {
	Id   string `json:"id"`
	Slug string `json:"slug"`
	Kind string `json:"kind"`
	// "draft" | "open" | "closed"
	Status         string `json:"status"`
	IsOpen         bool   `json:"is_open"`
	ClosesAt       *int64 `json:"closes_at"`
	CurrentVersion int    `json:"current_version"`
	Title          string `json:"title"`
	Summary        string `json:"summary"`
	SignatureCount int    `json:"signature_count"`
	UpdatedAt      int64  `json:"updated_at"`
}

type SiteFormDetail struct {
	SiteFormSummary
	Current  SiteFormVersion    `json:"current"`
	Versions []*SiteFormVersion `json:"versions"`
}

type SiteFormRequest struct {
	Slug    string         `json:"slug"`
	Kind    string         `json:"kind"`
	Title   string         `json:"title"`
	Summary string         `json:"summary"`
	Body    string         `json:"body"`
	Config  SiteFormConfig `json:"config"`
}

type SiteFormOpenRequest struct {
	ClosesAt *int64 `json:"closes_at"`
}

// SitePublicForm is a form as shown to the public. A closed form carries only
// its identity, so a stale QR code can say "no longer collecting signatures".
type SitePublicForm struct {
	Slug    string          `json:"slug"`
	Kind    string          `json:"kind,omitempty"`
	Status  string          `json:"status"`
	Title   string          `json:"title"`
	Summary string          `json:"summary,omitempty"`
	Version int             `json:"version,omitempty"`
	Body    string          `json:"body,omitempty"`
	Config  *SiteFormConfig `json:"config,omitempty"`
}

// ── Signatures ───────────────────────────────────────────────────────────────

type SiteSignRequest struct {
	// The version the signer was shown. If the form has been edited since, the
	// signature is refused rather than attached to words they never saw.
	Version              int    `json:"version"`
	SignerName           string `json:"signer_name"`
	PreferredName        string `json:"preferred_name"`
	Contact              string `json:"contact"`
	Event                string `json:"event"`
	EventDate            string `json:"event_date"`
	Choice               string `json:"choice"`
	IsMinor              bool   `json:"is_minor"`
	GuardianName         string `json:"guardian_name"`
	GuardianRelationship string `json:"guardian_relationship"`
	SignaturePNG         string `json:"signature_png"`
	GuardianSignaturePNG string `json:"guardian_signature_png"`
	EsignConsent         bool   `json:"esign_consent"`
	// Honeypot: real people never see this field.
	Website string `json:"website"`
}

type SiteSignResponse struct {
	Id                  string `json:"id"`
	SignedAt            int64  `json:"signed_at"`
	ConfirmationMessage string `json:"confirmation_message"`
	Emailed             bool   `json:"emailed"`
}

// SiteSignatureRecord is one signature. Images are only set on the single-record
// view, never in list responses.
type SiteSignatureRecord struct {
	Id                   string          `json:"id"`
	FormId               string          `json:"form_id"`
	FormTitle            string          `json:"form_title"`
	Version              int             `json:"version"`
	SignerName           string          `json:"signer_name"`
	Contact              string          `json:"contact"`
	Answers              json.RawMessage `json:"answers"`
	IsMinor              bool            `json:"is_minor"`
	GuardianName         string          `json:"guardian_name"`
	GuardianRelationship string          `json:"guardian_relationship"`
	HasSignature         bool            `json:"has_signature"`
	HasGuardianSignature bool            `json:"has_guardian_signature"`
	EsignConsent         bool            `json:"esign_consent"`
	TextSHA256           string          `json:"text_sha256"`
	SignedAt             int64           `json:"signed_at"`
	ClientIP             string          `json:"client_ip"`
	UserAgent            string          `json:"user_agent"`
	WithdrawnAt          *int64          `json:"withdrawn_at"`

	// Single-record view only.
	SignatureImage         string           `json:"signature_image,omitempty"`
	GuardianSignatureImage string           `json:"guardian_signature_image,omitempty"`
	Signed                 *SiteFormVersion `json:"signed,omitempty"`
}

type SiteWithdrawRequest struct {
	Withdrawn bool `json:"withdrawn"`
}

// ── Capabilities + activity ──────────────────────────────────────────────────

type SiteCapabilities struct {
	Banner     bool `json:"banner"`
	Financials bool `json:"financials"`
	Forms      bool `json:"forms"`
	PastEvents bool `json:"past_events"`
	LiveEvents bool `json:"live_events"`
	Resources  bool `json:"resources"`
}

// ── Past events (the photo galleries under Volunteers) ───────────────────────
//
// A past event is a tile in the site's Past events section and a page with a
// photo gallery. It is only that: creating one makes no volunteer event, no
// QR codes and no rewards.

// SitePhoto is an uploaded image (FileId) or, for tiles that existed before
// editing moved here, an image already hosted by the site (URL).
type SitePhoto struct {
	Id     string  `json:"id,omitempty"`
	FileId *string `json:"file_id"`
	URL    string  `json:"url"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Alt    string  `json:"alt"`
	// Shown under a gallery photo. Optional.
	Caption string `json:"caption,omitempty"`
	// The uploaded file's name, for building its URL. Not sent.
	Filename string `json:"-"`
}

type SitePastEvent struct {
	Id          string      `json:"id"`
	Slug        string      `json:"slug"`
	Title       string      `json:"title"`
	Date        string      `json:"date"` // YYYY-MM-DD
	Description string      `json:"description"`
	Cover       *SitePhoto  `json:"cover"`
	Photos      []SitePhoto `json:"photos"`
	// Set when this gallery belongs to a live event's series rather than being
	// a Past events tile. Decides which permission may edit it.
	SeriesKey *string `json:"-"`
	RemovedAt *int64  `json:"removed_at"`
	CreatedAt   int64       `json:"created_at"`
	UpdatedAt   int64       `json:"updated_at"`
}

type SitePastEventRequest struct {
	Title       string `json:"title"`
	Date        string `json:"date"`
	Description string `json:"description"`
	// An uploaded image to use as the tile's photo; "" keeps the current one.
	CoverFileId string `json:"cover_file_id"`
	// Or one of the event's gallery photos; "" keeps the current one.
	CoverPhotoId string `json:"cover_photo_id"`
	CoverAlt     string `json:"cover_alt"`
}

type SitePastEventPhotosRequest struct {
	FileIds []string `json:"file_ids"`
}

type SitePastEventOrderRequest struct {
	Order []string `json:"order"`
}

type SitePastEventPhotoRequest struct {
	Caption string `json:"caption"`
}

// SitePublicPastEvent is a tile (no Photos) or a gallery page (with them).
type SitePublicPastEvent struct {
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Date        string            `json:"date"`
	Description string            `json:"description,omitempty"`
	Cover       *SitePublicImage  `json:"cover"`
	PhotoCount  int               `json:"photo_count"`
	Photos      []SitePublicPhoto `json:"photos,omitempty"`
}

// SitePublicPhoto is a gallery photo with its caption, if it has one.
type SitePublicPhoto struct {
	SitePublicImage
	Caption string `json:"caption,omitempty"`
}

// SiteLiveEvent is a volunteer event as the Website tools edit it: its cover
// photos, and the gallery shared by its series (nil until one is started).
type SiteLiveEvent struct {
	Id                string                `json:"id"`
	Title             string                `json:"title"`
	Slug              string                `json:"slug"`
	StartAt           string                `json:"start_at"`
	Timezone          string                `json:"timezone"`
	RecurrenceSummary string                `json:"recurrence_summary"`
	OrganizerName     string                `json:"organizer_name"`
	CoverPhotos       []VolunteerEventPhoto `json:"cover_photos"`
	Gallery           *SitePastEvent        `json:"gallery"`
}

type SiteEventPhotoOrderRequest struct {
	Order []string `json:"order"`
}

type SiteActivity struct {
	Id         int64  `json:"id"`
	Actor      string `json:"actor"`
	Capability string `json:"capability"`
	Action     string `json:"action"`
	Entity     string `json:"entity"`
	EntityId   string `json:"entity_id"`
	Summary    string `json:"summary"`
	At         int64  `json:"at"`
}
