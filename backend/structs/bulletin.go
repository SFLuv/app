package structs

import "time"

// Bulletin post categories. Lost and found is deliberately absent at launch.
const (
	BulletinCategoryFree    = "free"
	BulletinCategoryForSale = "for_sale"
	BulletinCategoryNotice  = "notice"
)

// Bulletin post statuses. "hidden" is set automatically when enough distinct
// people report a post, and holds until an admin reviews it.
const (
	BulletinStatusActive  = "active"
	BulletinStatusSold    = "sold"
	BulletinStatusClaimed = "claimed"
	BulletinStatusRemoved = "removed"
	BulletinStatusHidden  = "hidden"
)

// Badges shown next to a person's public name. Derived from data that already
// exists (roles and credentials); there is nothing to apply for.
const (
	BulletinBadgeMerchant          = "merchant"
	BulletinBadgeImprover          = "improver"
	BulletinBadgeVerifiedVolunteer = "verified_volunteer"
)

// UserProfile is a person's public identity on the Bulletin and in Messages.
type UserProfile struct {
	UserId         string     `json:"user_id"`
	PublicName     string     `json:"public_name"`
	Handle         *string    `json:"handle"`
	Discoverable   bool       `json:"discoverable"`
	SuspendedUntil *time.Time `json:"suspended_until,omitempty"`
	// Set only on GET /me/profile before a profile exists: the "First L."
	// suggestion built from the account's contact name.
	SuggestedPublicName string `json:"suggested_public_name,omitempty"`
	Exists              bool   `json:"exists"`
}

type UserProfileRequest struct {
	PublicName   string  `json:"public_name"`
	Handle       *string `json:"handle"`
	Discoverable bool    `json:"discoverable"`
}

// PublicUser is everything one user may see about another.
type PublicUser struct {
	UserId     string   `json:"user_id"`
	PublicName string   `json:"public_name"`
	Handle     *string  `json:"handle,omitempty"`
	Badges     []string `json:"badges"`
	GoodTrades int      `json:"good_trades"`
}

type BulletinPhoto struct {
	Id       int64 `json:"id"`
	Position int   `json:"position"`
}

type BulletinMeetupLocation struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type BulletinLinkedEvent struct {
	Id      string `json:"id"`
	Title   string `json:"title"`
	StartAt int64  `json:"start_at"`
	EndAt   int64  `json:"end_at"`
}

type BulletinPost struct {
	Id             int64                   `json:"id"`
	Author         PublicUser              `json:"author"`
	Category       string                  `json:"category"`
	Title          string                  `json:"title"`
	Body           string                  `json:"body"`
	PriceSfluv     *string                 `json:"price_sfluv"`
	AcceptsSfluv   bool                    `json:"accepts_sfluv"`
	AcceptsCash    bool                    `json:"accepts_cash"`
	DistanceMiles  *float64                `json:"distance_miles,omitempty"`
	AreaLabel      string                  `json:"area_label"`
	MeetupLocation *BulletinMeetupLocation `json:"meetup_location"`
	LinkedEvent    *BulletinLinkedEvent    `json:"linked_event"`
	Status         string                  `json:"status"`
	ExpiresAt      time.Time               `json:"expires_at"`
	CreatedAt      time.Time               `json:"created_at"`
	Photos         []BulletinPhoto         `json:"photos"`
	IsMine         bool                    `json:"is_mine"`

	// Internal, never serialized: the raw columns the handler resolves.
	LinkedEventId    *string `json:"-"`
	MeetupLocationId *int    `json:"-"`
}

type BulletinFeedPage struct {
	Posts      []*BulletinPost `json:"posts"`
	NextCursor *string         `json:"next_cursor"`
}

// BulletinPostRequest creates a post. Lat/Lng are the device's position; the
// server snaps them before anything is stored.
type BulletinPostRequest struct {
	Category         string  `json:"category"`
	Title            string  `json:"title"`
	Body             string  `json:"body"`
	PriceSfluv       *string `json:"price_sfluv"`
	AcceptsSfluv     bool    `json:"accepts_sfluv"`
	AcceptsCash      bool    `json:"accepts_cash"`
	Lat              float64 `json:"lat"`
	Lng              float64 `json:"lng"`
	MeetupLocationId *int    `json:"meetup_location_id"`
	LinkedEventId    *string `json:"linked_event_id"`
}

// BulletinPostPatch edits a post. Every field is optional. Status may only move
// between active, sold and claimed; Renew pushes the expiry out again.
type BulletinPostPatch struct {
	Title            *string `json:"title"`
	Body             *string `json:"body"`
	PriceSfluv       *string `json:"price_sfluv"`
	AcceptsSfluv     *bool   `json:"accepts_sfluv"`
	AcceptsCash      *bool   `json:"accepts_cash"`
	MeetupLocationId *int    `json:"meetup_location_id"`
	ClearMeetup      bool    `json:"clear_meetup"`
	Status           *string `json:"status"`
	Renew            bool    `json:"renew"`
}

// BulletinPostRow is the stored form of a post, as the db layer returns it.
type BulletinPostRow struct {
	Id               int64
	AuthorId         string
	Category         string
	Title            string
	Body             string
	PriceSfluv       *string
	AcceptsSfluv     bool
	AcceptsCash      bool
	ApproxLat        float64
	ApproxLng        float64
	AreaLabel        string
	MeetupLocationId *int
	LinkedEventId    *string
	Status           string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	DistanceMiles    *float64
}

type BulletinFeedRequest struct {
	ViewerId    string
	Lat         float64
	Lng         float64
	RadiusMiles float64
	Category    string
	// Cursor is the (distance, id) of the last post on the previous page.
	AfterDistance *float64
	AfterId       *int64
	Limit         int
}

type BulletinFeedbackRequest struct {
	ToUserId string `json:"to_user_id"`
	Positive bool   `json:"positive"`
}

// Messaging.

const (
	MessageKindText    = "text"
	MessageKindPayment = "payment"
	MessageKindSystem  = "system"
)

type ConversationPostChip struct {
	Id           int64   `json:"id"`
	Title        string  `json:"title"`
	Category     string  `json:"category"`
	PriceSfluv   *string `json:"price_sfluv"`
	AcceptsSfluv bool    `json:"accepts_sfluv"`
	AcceptsCash  bool    `json:"accepts_cash"`
	Status       string  `json:"status"`
	AuthorId     string  `json:"author_id"`
	PhotoId      *int64  `json:"photo_id"`
}

type Message struct {
	Id              int64     `json:"id"`
	ConversationId  int64     `json:"conversation_id"`
	SenderId        string    `json:"sender_id"`
	IsMine          bool      `json:"is_mine"`
	Kind            string    `json:"kind"`
	Body            string    `json:"body"`
	TxHash          *string   `json:"tx_hash,omitempty"`
	AmountSfluvBase *string   `json:"amount_sfluv_base,omitempty"`
	PaymentVerified bool      `json:"payment_verified"`
	CreatedAt       time.Time `json:"created_at"`
}

type Conversation struct {
	Id            int64                 `json:"id"`
	OtherUser     PublicUser            `json:"other_user"`
	Post          *ConversationPostChip `json:"post"`
	LastMessage   *Message              `json:"last_message"`
	UnreadCount   int                   `json:"unread_count"`
	LastMessageAt time.Time             `json:"last_message_at"`
	// The other person's primary wallet, so "Send SFLUV" can prefill the
	// existing send flow. Only returned on the single-conversation endpoint,
	// and empty when the thread's post is cash-only.
	PayAddress string `json:"pay_address,omitempty"`
	// OtherUserInactive means the other person no longer has access to
	// messages — they were taken out of the chat beta. The conversation, its
	// members and its history are all untouched; what changes is that the
	// client greys them out and stops offering to send, and that they receive
	// no pushes from it. Putting them back in the beta reverses all of that on
	// its own, because none of it removed anything.
	OtherUserInactive bool `json:"other_user_inactive,omitempty"`
	BlockedByMe       bool `json:"blocked_by_me"`
}

type ConversationRow struct {
	Id            int64
	UserLow       string
	UserHigh      string
	PostId        *int64
	LastMessageAt time.Time
	LastReadId    int64
}

type NewConversationRequest struct {
	UserId string `json:"user_id"`
	PostId *int64 `json:"post_id"`
}

type NewMessageRequest struct {
	Body string `json:"body"`
}

type PaymentMessageRequest struct {
	TxHash string `json:"tx_hash"`
}

type MessagesPage struct {
	Messages []*Message `json:"messages"`
	HasMore  bool       `json:"has_more"`
}

type MessagingSummary struct {
	UnreadConversations int `json:"unread_conversations"`
}

// Search.

type UserSearchMerchant struct {
	LocationId  int    `json:"location_id"`
	Name        string `json:"name"`
	OwnerUserId string `json:"owner_user_id"`
}

type UserSearchResult struct {
	Users     []PublicUser         `json:"users"`
	Merchants []UserSearchMerchant `json:"merchants"`
}

// Safety.

var ContentReportReasons = map[string]bool{
	"prohibited_item": true,
	"scam":            true,
	"harassment":      true,
	"spam":            true,
	"unsafe":          true,
	"other":           true,
}

type ContentReportRequest struct {
	TargetType string `json:"target_type"`
	TargetId   string `json:"target_id"`
	Reason     string `json:"reason"`
	Details    string `json:"details"`
}

type BlockRequest struct {
	UserId string `json:"user_id"`
}

// AdminBulletinReport groups every open report on one target, so the admin sees
// "3 reports" once rather than three rows.
type AdminBulletinReport struct {
	TargetType   string               `json:"target_type"`
	TargetId     string               `json:"target_id"`
	ReportCount  int                  `json:"report_count"`
	FirstAt      time.Time            `json:"first_at"`
	Reports      []AdminReportEntry   `json:"reports"`
	ReportedUser *AdminReportedUser   `json:"reported_user"`
	Post         *BulletinPost        `json:"post,omitempty"`
	Message      *Message             `json:"message,omitempty"`
	PriorActions []AdminModerationRow `json:"prior_actions"`
}

type AdminReportEntry struct {
	Id         int64     `json:"id"`
	ReporterId string    `json:"reporter_id"`
	Reporter   string    `json:"reporter"`
	Reason     string    `json:"reason"`
	Details    string    `json:"details"`
	CreatedAt  time.Time `json:"created_at"`
}

type AdminReportedUser struct {
	UserId         string     `json:"user_id"`
	PublicName     string     `json:"public_name"`
	ContactEmail   string     `json:"contact_email"`
	PostCount      int        `json:"post_count"`
	ReportCount    int        `json:"report_count"`
	SuspendedUntil *time.Time `json:"suspended_until"`
}

type AdminModerationRow struct {
	Action    string    `json:"action"`
	Note      string    `json:"note"`
	AdminId   string    `json:"admin_id"`
	CreatedAt time.Time `json:"created_at"`
}

// AdminModerationRequest resolves every open report on one target.
// Action is one of dismiss, remove_post, remove_message, warn, suspend, unsuspend.
type AdminModerationRequest struct {
	TargetType   string `json:"target_type"`
	TargetId     string `json:"target_id"`
	Action       string `json:"action"`
	Note         string `json:"note"`
	SuspendDays  int    `json:"suspend_days"`
	TargetUserId string `json:"target_user_id"`
}
