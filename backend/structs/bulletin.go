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
	// Set only in search results: this person is in the searcher's own
	// contacts, and PublicName above is the name the searcher saved rather than
	// the one the account chose. Lets the client mark the row so a saved contact
	// is distinguishable from a stranger with a similar name.
	SavedContact bool `json:"saved_contact,omitempty"`
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
	// A message whose content is its attachments. Body may still be set — a
	// caption — but the card is the point.
	MessageKindAttachment = "attachment"
)

// Conversation kinds.
const (
	ConversationKindDirect = "direct"
	ConversationKindGroup  = "group"
)

// Membership roles in a group. A direct thread has neither: both people are
// plain members and there is nothing to administer.
const (
	ConversationRoleOwner  = "owner"
	ConversationRoleAdmin  = "admin"
	ConversationRoleMember = "member"
)

// Attachment kinds. The first three carry content; the rest point at something
// that lives elsewhere and is resolved when the message is read.
const (
	AttachmentKindImage       = "image"
	AttachmentKindFile        = "file"
	AttachmentKindLink        = "link"
	AttachmentKindTransaction = "transaction"
	AttachmentKindMerchant    = "merchant"
	AttachmentKindEvent       = "event"
	AttachmentKindSupport     = "support"
)

// IsAttachmentKind reports whether a client sent a kind this build stores.
func IsAttachmentKind(kind string) bool {
	switch kind {
	case AttachmentKindImage, AttachmentKindFile, AttachmentKindLink,
		AttachmentKindTransaction, AttachmentKindMerchant, AttachmentKindEvent,
		AttachmentKindSupport:
		return true
	}
	return false
}

// MessageAttachment is one card under a message.
//
// Every kind shares this one shape rather than being a union, because the client
// renders them in one list and a discriminated union in JSON costs more at the
// call site than the empty fields do. Which fields are meaningful follows from
// Kind, and the server never sets a field a kind does not use.
type MessageAttachment struct {
	Id       int64  `json:"id"`
	Kind     string `json:"kind"`
	Position int    `json:"position"`

	// image, file.
	ContentType string `json:"content_type,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	SizeBytes   *int   `json:"size_bytes,omitempty"`
	// Set for images, so the thread reserves the right space before the bytes
	// arrive instead of reflowing under the reader.
	ImageWidth  *int `json:"image_width,omitempty"`
	ImageHeight *int `json:"image_height,omitempty"`

	// link. Frozen at send time; see the migration's note on why.
	LinkURL         string `json:"link_url,omitempty"`
	LinkTitle       string `json:"link_title,omitempty"`
	LinkDescription string `json:"link_description,omitempty"`
	LinkImageURL    string `json:"link_image_url,omitempty"`
	LinkSiteName    string `json:"link_site_name,omitempty"`

	// transaction.
	TxHash string `json:"tx_hash,omitempty"`
	// Resolved from the indexer when the message is read. Nil when the transfer
	// is not indexed yet or has gone.
	TxAmountSfluvBase *string `json:"tx_amount_sfluv_base,omitempty"`
	TxFrom            string  `json:"tx_from,omitempty"`
	TxTo              string  `json:"tx_to,omitempty"`
	TxAt              *int64  `json:"tx_at,omitempty"`

	// merchant.
	LocationId   *int     `json:"location_id,omitempty"`
	LocationName string   `json:"location_name,omitempty"`
	LocationAddr string   `json:"location_address,omitempty"`
	LocationLat  *float64 `json:"location_lat,omitempty"`
	LocationLng  *float64 `json:"location_lng,omitempty"`

	// event.
	EventId    string `json:"event_id,omitempty"`
	EventTitle string `json:"event_title,omitempty"`
	EventStart *int64 `json:"event_start_at,omitempty"`
	EventEnd   *int64 `json:"event_end_at,omitempty"`

	// support.
	SupportRequestId *int64 `json:"support_request_id,omitempty"`
	SupportReference string `json:"support_reference,omitempty"`
	SupportSubject   string `json:"support_subject,omitempty"`
	SupportCategory  string `json:"support_category,omitempty"`

	// Set when the thing a reference points at is gone, so the client renders a
	// dead card instead of an empty one.
	Unavailable bool `json:"unavailable,omitempty"`
}

// MessageReaction is one emoji on one message, with who used it.
//
// Aggregated server-side rather than sent as rows: the client shows "😂 3" and
// whether you are one of the three, and doing the grouping here means a message
// with forty reactions is four lines of JSON rather than forty.
type MessageReaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	// Whether the reader is one of them, which is what makes the chip a toggle.
	Mine bool `json:"mine"`
	// Public names of up to a handful of reactors, for the long-press list.
	Names []string `json:"names,omitempty"`
}

// MessageQuote is the slice of a replied-to message shown above a reply.
//
// Deliberately not the whole Message: a reply only needs enough to recognise
// what it answers, and nesting full messages makes a thread of replies grow
// quadratically. Nil when the quoted message has since been deleted, which is
// why the reply's own reply_to_id survives the delete.
type MessageQuote struct {
	Id         int64  `json:"id"`
	SenderId   string `json:"sender_id"`
	SenderName string `json:"sender_name,omitempty"`
	Kind       string `json:"kind"`
	// Truncated to a line's worth.
	Body string `json:"body"`
	// What the quoted message's first attachment was, so a reply to a photo
	// shows "Photo" rather than an empty line.
	AttachmentKind string `json:"attachment_kind,omitempty"`
}

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
	Id             int64  `json:"id"`
	ConversationId int64  `json:"conversation_id"`
	SenderId       string `json:"sender_id"`
	IsMine         bool   `json:"is_mine"`
	// The sender's public name. Only populated in group threads, where a bubble
	// has to say who said it; in a direct thread the header already does.
	SenderName      string    `json:"sender_name,omitempty"`
	Kind            string    `json:"kind"`
	Body            string    `json:"body"`
	TxHash          *string   `json:"tx_hash,omitempty"`
	AmountSfluvBase *string   `json:"amount_sfluv_base,omitempty"`
	PaymentVerified bool      `json:"payment_verified"`
	CreatedAt       time.Time `json:"created_at"`

	ReplyToId   *int64              `json:"reply_to_id,omitempty"`
	ReplyTo     *MessageQuote       `json:"reply_to,omitempty"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`
	Reactions   []MessageReaction   `json:"reactions,omitempty"`
}

// ConversationMember is one person in a group, as the other members see them.
type ConversationMember struct {
	User PublicUser `json:"user"`
	Role string     `json:"role"`
	// They left of their own accord or were removed. Kept in the list only
	// where their messages are still in the thread, so a bubble always has a
	// name against it.
	Left bool `json:"left,omitempty"`
	// No longer holds the chat beta, so they cannot read this. Same meaning as
	// OtherUserInactive on a direct thread.
	Inactive bool `json:"inactive,omitempty"`
}

type Conversation struct {
	Id   int64  `json:"id"`
	Kind string `json:"kind"`

	// ── Direct threads ──────────────────────────────────────────────────────
	//
	// OtherUser is the zero value in a group. It is not a pointer because every
	// client in the field reads other_user.public_name unconditionally, and a
	// null there would crash them; a group sends an empty one and sets Title
	// instead. The next mobile release reads Kind first, at which point this
	// could become a pointer — it is not worth a forced update today.
	OtherUser PublicUser `json:"other_user"`

	// ── Groups ──────────────────────────────────────────────────────────────
	Title string `json:"title,omitempty"`
	// The group picture, as an attachment id to fetch. Nil means initials.
	IconAttachmentId *int64 `json:"icon_attachment_id,omitempty"`
	OwnerId          string `json:"owner_id,omitempty"`
	// The reader's own role, so the client knows which controls to offer
	// without working it out from OwnerId and the member list.
	MyRole string `json:"my_role,omitempty"`
	// Only on the single-conversation endpoint: the inbox does not need forty
	// member records per row.
	Members []ConversationMember `json:"members,omitempty"`
	// Always set, including in the inbox, because the row shows "4 people".
	MemberCount int `json:"member_count,omitempty"`

	Post        *ConversationPostChip `json:"post"`
	LastMessage *Message              `json:"last_message"`
	// Set only when the newest thing in the thread is a reaction rather than a
	// message, so the inbox's one preview line describes the actual latest
	// activity. Nil otherwise.
	LatestReaction *ConversationReaction `json:"latest_reaction,omitempty"`
	UnreadCount    int                   `json:"unread_count"`
	LastMessageAt  time.Time             `json:"last_message_at"`
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

// ConversationRow is the stored thread, as the db layer returns it.
//
// UserLow and UserHigh stay plain strings and are empty in a group. The column
// is nullable now, so the db layer coalesces — a pointer here would make
// otherMember() and every payment path nil-check for a case they have already
// excluded by checking Kind.
// ConversationReaction is the newest reaction in a thread, when it is more
// recent than the newest message.
//
// Surfaced so the inbox can say "Sanchez reacted 😂 to 'see you there'" the way
// iMessage does. It is NOT a message and is deliberately not faked into one:
// the thread itself renders reactions attached to their bubble, and inventing a
// message row for one would put it in the transcript as a line somebody said.
type ConversationReaction struct {
	Emoji string `json:"emoji"`
	// Who reacted. Empty for the reader's own, which the client renders as "You".
	UserName string `json:"user_name,omitempty"`
	IsMine   bool   `json:"is_mine"`
	// A short slice of what was reacted to, so the row says which message.
	MessageExcerpt string    `json:"message_excerpt,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type ConversationRow struct {
	Id               int64
	Kind             string
	Title            string
	IconAttachmentId *int64
	OwnerId          string
	UserLow          string
	UserHigh         string
	PostId           *int64
	LastMessageAt    time.Time
	LastReadId       int64
	// The role of the member this row was loaded for.
	MyRole string
}

// IsGroup is the one test every caller should use, rather than looking at
// whether UserLow happens to be empty.
func (c *ConversationRow) IsGroup() bool {
	return c.Kind == ConversationKindGroup
}

// NewGroupRequest creates a group chat.
type NewGroupRequest struct {
	Title string `json:"title"`
	// The other members. The caller is added as owner and does not need to be
	// listed; listing them anyway is accepted and ignored.
	UserIds []string `json:"user_ids"`
}

// GroupPatch changes a group's name or picture.
type GroupPatch struct {
	Title *string `json:"title"`
	// Clears the picture. Setting one is a separate multipart upload.
	ClearIcon bool `json:"clear_icon"`
}

// GroupMemberRequest adds, removes or re-roles one member.
type GroupMemberRequest struct {
	UserId string `json:"user_id"`
	// One of the ConversationRole constants. Only meaningful on a role change.
	Role string `json:"role"`
}

// ReactionRequest toggles one emoji on one message.
type ReactionRequest struct {
	Emoji string `json:"emoji"`
}

// AttachmentReferenceRequest attaches something that already exists rather than
// uploading bytes: a transaction, a merchant, an event, a support ticket.
//
// Separate from the multipart upload route because it is a JSON body with no
// file in it, and folding the two together would mean a handler that parses
// either depending on Content-Type.
type AttachmentReferenceRequest struct {
	Kind string `json:"kind"`
	// A caption, sent as the message body.
	Body string `json:"body"`
	// Set the one that matches Kind.
	TxHash           string `json:"tx_hash,omitempty"`
	LocationId       *int   `json:"location_id,omitempty"`
	EventId          string `json:"event_id,omitempty"`
	SupportRequestId *int64 `json:"support_request_id,omitempty"`
	LinkURL          string `json:"link_url,omitempty"`
	ReplyToId        *int64 `json:"reply_to_id,omitempty"`
}

// InChatPaymentRequest is the Apple-Cash-style send: an amount typed in the
// composer rather than a trip through the wallet's send screen.
//
// The client still signs and broadcasts the transfer itself — this is not a
// custodial send — so what arrives here is the hash of something that has
// already happened, exactly as AttachConversationPayment receives it. The
// difference is only in where the amount was typed, which is why they share the
// same verification path.
type InChatPaymentRequest struct {
	TxHash string `json:"tx_hash"`
	// What the sender meant to send, so the bubble can show an amount before
	// the indexer confirms one. Replaced by the indexed amount once known.
	AmountSfluvBase string `json:"amount_sfluv_base,omitempty"`
	Note            string `json:"note,omitempty"`
}

type NewConversationRequest struct {
	UserId string `json:"user_id"`
	PostId *int64 `json:"post_id"`
}

type NewMessageRequest struct {
	Body string `json:"body"`
	// The message this one answers. Ignored when it names a message in another
	// thread or one that has been deleted — see the resolution in InsertMessage.
	ReplyToId *int64 `json:"reply_to_id,omitempty"`
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

// TypingEvent is the body of a realtime typing event.
type TypingEvent struct {
	ConversationId int64  `json:"conversation_id"`
	UserId         string `json:"user_id"`
	UserName       string `json:"user_name"`
	Typing         bool   `json:"typing"`
}

// RealtimeNotification is a notification delivered over the socket instead of
// as a push, for a client that is already open.
//
// The fields mirror what a push carries — title, body, and the data bag the
// client already routes on — so the in-app banner and the system banner are
// built from the same thing and a tap lands in the same place.
type RealtimeNotification struct {
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
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
