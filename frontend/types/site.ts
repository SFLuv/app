// Mirrors backend/structs/site.go. See docs/features/website-editing-and-forms.md.

export interface SiteCapabilities {
  banner: boolean
  financials: boolean
  forms: boolean
  past_events: boolean
  live_events: boolean
  resources: boolean
}

/** One video in the Resources page's How to section. `video_id` is read from `video_url` by the server. */
export interface SiteHowToVideo {
  id: string
  enabled: boolean
  title: string
  description: string
  video_url: string
  video_id: string
}

/** A book in the reading list. Its cover is an upload (`cover_file_id`) or an image already on the site (`cover_url`). */
export interface SiteResourceBook {
  id: string
  enabled: boolean
  title: string
  author: string
  href: string
  cover_file_id: string | null
  cover_url: string | null
  cover_width: number
  cover_height: number
  cover_alt: string
  /** For display only; never saved. */
  cover_preview_url: string | null
}

export interface SiteResourcePodcast {
  id: string
  enabled: boolean
  name: string
  href: string
  video_url: string
  video_id: string
}

export interface SiteResourceLink {
  id: string
  enabled: boolean
  label: string
  href: string
}

export interface SiteResources {
  how_to: SiteHowToVideo[]
  books: SiteResourceBook[]
  podcasts: SiteResourcePodcast[]
  technical_links: SiteResourceLink[]
}

export interface SiteResourcesVersion {
  version: number
  saved_at: number
  saved_by: string
  note: string
  value: SiteResources
  is_current: boolean
}

export interface SiteResourcesAdmin {
  current: SiteResources
  version: number
  updated_at: number
  versions: SiteResourcesVersion[]
}

/** An uploaded image (`file_id`) or one already hosted by the site (`url` only). */
export interface SitePhoto {
  id?: string
  file_id: string | null
  url: string
  width: number
  height: number
  alt: string
  /** Shown under a gallery photo on the website. Optional. */
  caption?: string
}

/** A tile in the site's Past events section, with its gallery. Only a tile: no event, QR codes or rewards. */
export interface SitePastEvent {
  id: string
  slug: string
  title: string
  /** YYYY-MM-DD */
  date: string
  description: string
  cover: SitePhoto | null
  photos: SitePhoto[]
  removed_at: number | null
  created_at: number
  updated_at: number
}

/** A volunteer event as the Website tools edit it. `gallery` is null until one is started. */
export interface SiteLiveEvent {
  id: string
  title: string
  slug: string
  start_at: string
  timezone: string
  recurrence_summary: string
  organizer_name: string
  cover_photos: { id: string; url: string; width: number; height: number; position: number }[]
  gallery: SitePastEvent | null
}

export interface SitePastEventRequest {
  title: string
  date: string
  description: string
  cover_file_id?: string
  cover_photo_id?: string
  cover_alt: string
}

export interface SiteFile {
  id: string
  filename: string
  content_type: string
  size_bytes: number
  width: number
  height: number
  url: string
}

export interface SiteSpotlightAction {
  label: string
  href: string
  new_tab: boolean
  also_open_file_id: string | null
  also_open_url: string | null
}

export interface SiteSpotlightSlide {
  id: string
  enabled: boolean
  label: string
  title: string
  body: string
  image_file_id: string | null
  image_url: string | null
  image_width: number
  image_height: number
  image_alt: string
  image_position: string
  /** Output only: where the admin panel can show the picture. */
  image_preview_url?: string | null
  action: SiteSpotlightAction
  event_match: string
}

export interface SiteSpotlight {
  slides: SiteSpotlightSlide[]
}

export interface SiteSpotlightVersion {
  version: number
  saved_at: number
  saved_by: string
  note: string
  value: SiteSpotlight
  is_current: boolean
}

export interface SiteSpotlightAdmin {
  current: SiteSpotlight
  version: number
  updated_at: number
  versions: SiteSpotlightVersion[]
}

export type FinancialKind =
  | "activity"
  | "cash_flows"
  | "financial_position"
  | "activity_comparison"
  | "form_990n"
  | "form_199n"
  | "impact_report"
  | "other"

export type FinancialPeriod = "Q1" | "Q2" | "Q3" | "Q4" | "FULL"

export interface FinancialDocument {
  id: string
  kind: FinancialKind
  fiscal_year: number
  period: FinancialPeriod
  as_of: string | null
  label: string
  href: string
  source: "upload" | "legacy"
  file_id: string | null
  filename: string | null
  removed_at: number | null
  created_at: number
  updated_at: number
}

export interface FinancialDocumentRequest {
  kind: FinancialKind
  fiscal_year: number
  period: FinancialPeriod
  as_of: string | null
  label: string
  file_id: string
}

export interface SiteFormChoice {
  id: string
  label: string
  description: string
}

export interface SiteFormConfig {
  contact: "off" | "optional" | "required"
  preferred_name: boolean
  event: "off" | "ask" | "fixed"
  event_name: string
  choices_prompt: string
  choices: SiteFormChoice[]
  guardian_section: boolean
  confirmation_message: string
}

export interface SiteFormVersion {
  version: number
  title: string
  summary: string
  body: string
  config: SiteFormConfig
  saved_at: number
  saved_by: string
}

export interface SiteFormSummary {
  id: string
  slug: string
  kind: "waiver" | "withdrawal"
  status: "draft" | "open" | "closed"
  is_open: boolean
  closes_at: number | null
  current_version: number
  title: string
  summary: string
  signature_count: number
  updated_at: number
}

export interface SiteFormDetail extends SiteFormSummary {
  current: SiteFormVersion
  versions: SiteFormVersion[]
}

export interface SiteFormRequest {
  slug: string
  kind: "waiver" | "withdrawal"
  title: string
  summary: string
  body: string
  config: SiteFormConfig
}

export interface SiteSignatureRecord {
  id: string
  form_id: string
  form_title: string
  version: number
  signer_name: string
  contact: string
  answers: { preferred_name?: string; event?: string; event_date?: string; choice?: string }
  is_minor: boolean
  guardian_name: string
  guardian_relationship: string
  has_signature: boolean
  has_guardian_signature: boolean
  esign_consent: boolean
  text_sha256: string
  signed_at: number
  client_ip: string
  user_agent: string
  withdrawn_at: number | null
  signature_image?: string
  guardian_signature_image?: string
  signed?: SiteFormVersion
}

export interface SiteActivity {
  id: number
  actor: string
  capability: string
  action: string
  entity: string
  entity_id: string
  summary: string
  at: number
}
