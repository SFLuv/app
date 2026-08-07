package structs

// Signup modes for a volunteer event. "internal" is the only mode that keeps
// signup records; "external" hands off to the organizer's own flow and "none"
// means the event is drop-in.
const (
	SignupModeNone     = "none"
	SignupModeExternal = "external"
	SignupModeInternal = "internal"
)

// Moderation lifecycle. Never exposed on public payloads — the public list is
// approved-only by construction, so a moderation state is meaningless there.
const (
	EventReviewPending   = "pending"
	EventReviewApproved  = "approved"
	EventReviewRejected  = "rejected"
	EventReviewCancelled = "cancelled"
)

// Occurrence lifecycle. This is the status public clients consume: it describes
// where the event is in real time, which is the only thing a volunteer can act
// on.
const (
	EventStatusScheduled = "scheduled"
	EventStatusLive      = "live"
	EventStatusEnded     = "ended"
	EventStatusCancelled = "cancelled"
)

const (
	RecurrenceNone    = "none"
	RecurrenceDaily   = "daily"
	RecurrenceWeekly  = "weekly"
	RecurrenceMonthly = "monthly"
)

const (
	MonthlyModeDayOfMonth = "day_of_month"
	MonthlyModeDayOfWeek  = "day_of_week"
)

const (
	FundingStatusFunded    = "funded"
	FundingStatusAwaiting  = "awaiting_funding"
	OrganizerTypeSFLuv     = "sfluv"
	OrganizerTypeAffiliate = "affiliate"
)

// Reasons a signup is closed. Mirrors the values both clients switch on.
const (
	SignupClosedFull       = "full"
	SignupClosedEnded      = "ended"
	SignupClosedCancelled  = "cancelled"
	SignupClosedNotOpenYet = "not_open_yet"
)

type VolunteerEventOrganizer struct {
	Type           string  `json:"type"`
	OrganizationId *int64  `json:"organization_id"`
	Name           string  `json:"name"`
	LogoURL        *string `json:"logo_url"`
}

type VolunteerEventPhoto struct {
	Id       string `json:"id"`
	URL      string `json:"url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Position int    `json:"position"`
}

// VolunteerEventLocation is the structured location shape agreed in comms [12]:
// volunteer locations are real rows in the shared locations table, so clients
// get consistent address parts rather than free text.
type VolunteerEventLocation struct {
	Id     int64    `json:"id"`
	Name   string   `json:"name"`
	Street string   `json:"street"`
	City   string   `json:"city"`
	State  string   `json:"state"`
	Zip    string   `json:"zip"`
	Lat    *float64 `json:"lat"`
	Lng    *float64 `json:"lng"`
}

// VolunteerEventRecurrence carries the machine-readable rule plus a
// server-rendered Summary. The summary exists so the web, mobile, and admin
// clients cannot each invent a different phrasing of "first Thursday".
type VolunteerEventRecurrence struct {
	Frequency   string   `json:"frequency"`
	Interval    int      `json:"interval"`
	Weekdays    []string `json:"weekdays,omitempty"`
	MonthlyMode string   `json:"monthly_mode,omitempty"`
	DayOfMonth  *int     `json:"day_of_month,omitempty"`
	WeekOfMonth *int     `json:"week_of_month,omitempty"`
	Weekday     *string  `json:"weekday,omitempty"`
	Until       *string  `json:"until"`
	Summary     string   `json:"summary"`
}

type VolunteerEventSignupInfo struct {
	Mode         string  `json:"mode"`
	URL          *string `json:"url"`
	Open         bool    `json:"open"`
	ClosedReason *string `json:"closed_reason"`
}

// VolunteerEventQR is admin/affiliate only. Codes are downloadable as soon as
// the event exists but only spendable from LiveAt (start - 24h).
type VolunteerEventQR struct {
	Live           bool    `json:"live"`
	LiveAt         *string `json:"live_at"`
	CodesGenerated bool    `json:"codes_generated"`
}

// VolunteerEventViewer is present whenever the request is authenticated,
// regardless of signup mode (comms [13], Q-M2).
type VolunteerEventViewer struct {
	SignedUp bool    `json:"signed_up"`
	SignupId *string `json:"signup_id"`
	Redeemed bool    `json:"redeemed"`
}

type VolunteerEvent struct {
	Id          string                    `json:"id"`
	SeriesId    *string                   `json:"series_id"`
	Slug        string                    `json:"slug"`
	Title       string                    `json:"title"`
	Description string                    `json:"description"`
	CoverPhotos []VolunteerEventPhoto     `json:"cover_photos"`
	Organizer   VolunteerEventOrganizer   `json:"organizer"`
	StartAt     string                    `json:"start_at"`
	EndAt       string                    `json:"end_at"`
	Timezone    string                    `json:"timezone"`
	Recurrence  *VolunteerEventRecurrence `json:"recurrence"`

	MaxParticipants int  `json:"max_participants"`
	SignupCount     *int `json:"signup_count"`
	SpotsRemaining  *int `json:"spots_remaining"`

	RewardAmountSfluv uint64                   `json:"reward_amount_sfluv"`
	Signup            VolunteerEventSignupInfo `json:"signup"`
	Status            string                   `json:"status"`
	Location          *VolunteerEventLocation  `json:"location"`
	Viewer            *VolunteerEventViewer    `json:"viewer"`

	// Creator is who made the event. Management-only: the public portal shows
	// the organization, never the individual.
	Creator *VolunteerEventCreator `json:"creator,omitempty"`

	// Management-only fields. Omitted entirely from public responses.
	ReviewStatus  string            `json:"review_status,omitempty"`
	QR            *VolunteerEventQR `json:"qr,omitempty"`
	FundingStatus string            `json:"funding_status,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// VolunteerEventCreator identifies who created an event, as "first name, last
// initial". Email is populated ONLY when that short form is ambiguous within
// the same organization — showing it always would expose addresses with no
// benefit.
type VolunteerEventCreator struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type VolunteerEventsResponse struct {
	Events  []*VolunteerEvent `json:"events"`
	Page    int               `json:"page"`
	Count   int               `json:"count"`
	HasMore bool              `json:"has_more"`
	Total   int               `json:"total"`
}

// VolunteerEventOrganizerFacet powers the organizer filter chips. Served from
// its own cacheable endpoint rather than inlined per page, because events and
// organizations live in different databases and the join happens in Go.
type VolunteerEventOrganizerFacet struct {
	Type           string  `json:"type"`
	OrganizationId *int64  `json:"organization_id"`
	Name           string  `json:"name"`
	LogoURL        *string `json:"logo_url"`
	EventCount     int     `json:"event_count"`
}

// VolunteerEventsFilter is the parsed query for the public list.
type VolunteerEventsFilter struct {
	Search         string
	OrganizerType  string
	OrganizationId *int64
	When           string // upcoming | past | all
	From           *int64
	To             *int64
	OpenSignups    bool
	Sort           string // start_at | newest
	Page           int
	Count          int
}

// VolunteerEventRecurrenceInput is the recurrence rule as an admin specifies it.
type VolunteerEventRecurrenceInput struct {
	Frequency   string  `json:"frequency"`
	MonthlyMode string  `json:"monthly_mode,omitempty"`
	DayOfMonth  *int    `json:"day_of_month,omitempty"`
	WeekOfMonth *int    `json:"week_of_month,omitempty"`
	UntilLocal  *string `json:"until_local,omitempty"`
}

// VolunteerEventCreateRequest creates a volunteer event.
//
// Times arrive as WALL CLOCK in the event's own timezone ("2026-08-06T13:00:00",
// no offset, no Z) and the server converts them to the stored UTC instant.
// Clients never do the conversion: doing it here means one implementation with
// tests instead of every client re-deriving it, and it is what makes recurring
// events re-anchor correctly across DST — a series regenerates at the same
// local wall-clock time, which cannot be recovered if the wrong instant was
// captured at ingestion.
type VolunteerEventCreateRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`

	StartAtLocal string `json:"start_at_local"`
	EndAtLocal   string `json:"end_at_local"`
	Timezone     string `json:"timezone"`

	MaxParticipants   int    `json:"max_participants"`
	RewardAmountSfluv uint64 `json:"reward_amount_sfluv"`

	SignupMode string `json:"signup_mode"`
	SignupURL  string `json:"signup_url,omitempty"`

	LocationId *int64 `json:"location_id,omitempty"`

	Recurrence *VolunteerEventRecurrenceInput `json:"recurrence,omitempty"`

	// QRCutoffLocal is an explicit redemption deadline as wall clock in the
	// event's timezone. Empty means the default: 24 hours after the event ends.
	QRCutoffLocal string `json:"qr_cutoff_local,omitempty"`
}
