package structs

import "time"

// Internal organization notifications: the operational alerts that used to go
// to a staff email thread, delivered to the people who signed up for them.
//
// The model is "condition, not event", and everything else here follows from
// it. An alert describes something that is true right now and stops existing
// when it stops being true, so an inbox with nothing in it means nothing needs
// attention. That is only worth anything if alerts retire themselves reliably,
// which is why each one declares how it ends rather than all of them sharing a
// dismiss button.

// Notification sets. A member is designated to receive one or more of these,
// and a set is the unit of subscription — not individual alert kinds, because
// the useful grouping is by who cares rather than by what broke.
//
// Only the house organization has sets today. Partner organizations will want
// their own eventually, which is why subscriptions are scoped to an
// organization rather than being a global flag on a user.
const (
	// NotificationSetDev is infrastructure: balances running out, the backend
	// not answering. Nobody outside the people who can fix those wants them.
	NotificationSetDev = "dev"
	// NotificationSetAdmin is work waiting on a human decision — somebody
	// asking to become an affiliate, a merchant, an improver.
	NotificationSetAdmin = "admin"
	// NotificationSetSupport is the public "get in touch" queue.
	NotificationSetSupport = "support"
)

// NotificationSetDefinition describes one set for the settings screen.
type NotificationSetDefinition struct {
	Id    string `json:"id"`
	Label string `json:"label"`
	// Description is shown under the label. It says what arrives and roughly
	// how often, because "do you want admin alerts" is unanswerable without
	// knowing whether that means twice a day or forty times an hour.
	Description string `json:"description"`
}

// NotificationSetCatalog is every set a member can be designated for.
func NotificationSetCatalog() []NotificationSetDefinition {
	return []NotificationSetDefinition{
		{
			Id:          NotificationSetDev,
			Label:       "Dev alerts",
			Description: "Protocol accounts running low on gas or rewards, and the backend failing its health check. Quiet unless something is wrong.",
		},
		{
			Id:          NotificationSetAdmin,
			Label:       "Admin alerts",
			Description: "Affiliate, merchant and improver requests waiting on a decision. Approve or reject them from the notification itself.",
		},
		{
			Id:          NotificationSetSupport,
			Label:       "Support requests",
			Description: "Messages sent through the website's Get in touch form. Carries the sender's email and their message.",
		},
	}
}

// IsNotificationSet reports whether a set id is one we know.
func IsNotificationSet(id string) bool {
	for _, set := range NotificationSetCatalog() {
		if set.Id == id {
			return true
		}
	}
	return false
}

// Internal alert kinds. The kind selects the action buttons, so adding a kind
// without adding it to the handler's registry gives an alert with no buttons
// rather than a broken one.
const (
	// InternalKindFunding is a protocol account below its floor. No buttons:
	// the only fix is sending money, and the alert retires itself when the
	// next sweep sees the balance back above the floor.
	InternalKindFunding = "funding"
	// InternalKindUptime is the backend failing its health check. Has an
	// acknowledge button, which is refused while it is still failing.
	InternalKindUptime = "uptime"
	// InternalKindRoleRequest is somebody asking for a role. Approve and
	// reject run the same code the admin panel runs.
	InternalKindRoleRequest = "role_request"
	// InternalKindSupportRequest is a website support message. Archiving it
	// retires the alert and marks the request archived in the admin panel.
	InternalKindSupportRequest = "support_request"
)

// Resolutions: how an alert ended, kept on the history row.
const (
	// ResolutionCleared means the condition stopped being true on its own.
	ResolutionCleared = "cleared"
	// ResolutionActed means somebody pressed a button that did something.
	ResolutionActed = "acted"
	// ResolutionAcked means somebody said they had seen it, and nothing else.
	ResolutionAcked = "acked"
)

// NotificationField is one label/value detail row under an alert's body.
type NotificationField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// NotificationActionStyle hints at how prominent a button should be. Clients
// that do not recognise a style render the button plainly rather than dropping
// it, so an action is never invisible because of its decoration.
const (
	NotificationActionStyleDefault = "default"
	NotificationActionStylePrimary = "primary"
	NotificationActionStyleDanger  = "danger"
)

// NotificationActionButton is one button on an alert.
//
// Id is what the client posts back. It is meaningful only together with the
// notification's id — the server looks the action up against the alert's kind
// rather than trusting anything about it — so a client cannot invent an action
// or run one belonging to a different alert.
type NotificationActionButton struct {
	Id    string `json:"id"`
	Label string `json:"label"`
	Style string `json:"style,omitempty"`
	// URL makes this a link rather than a call: the client opens it instead of
	// posting back. Used for "reply by email", which is a mailto the device
	// handles and the server has no part in.
	URL string `json:"url,omitempty"`
	// Confirm asks before running, for the ones that are hard to walk back.
	Confirm string `json:"confirm,omitempty"`
}

// UserNotification is one row of the notification screen, from either source.
//
// One shape covers both an internal alert and an ordinary push because the
// screen shows them in one list. The fields an ordinary push does not have are
// omitted, which is also what an older client sees and ignores.
type UserNotification struct {
	Id        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	// Data is the push payload, so a history row can still route a tap.
	Data map[string]string `json:"data,omitempty"`
	Seen bool              `json:"seen"`

	// Internal is set for an organization alert and carries everything that
	// makes one different from a push: who it is for, what it is about, what
	// can be done about it, and how it ended if it has.
	Internal *InternalNotificationDetail `json:"internal,omitempty"`
}

// InternalNotificationDetail is the alert half of a notification row.
type InternalNotificationDetail struct {
	Id        int64               `json:"id"`
	Set       string              `json:"set"`
	SetLabel  string              `json:"set_label"`
	AlertKey  string              `json:"alert_key"`
	Fields    []NotificationField `json:"fields,omitempty"`
	Urgent    bool                `json:"urgent"`
	UpdatedAt time.Time           `json:"updated_at"`
	// Actions is empty for an alert that retires itself, which is the honest
	// representation: there is nothing a person can do about a low balance
	// from a phone except put money in the account.
	Actions []NotificationActionButton `json:"actions,omitempty"`

	Resolved   bool       `json:"resolved"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	// Outcome is the sentence the history row shows: "Approved by Dana", "Back
	// up after 4m". Empty for one that simply stopped being true.
	Outcome string `json:"outcome,omitempty"`
}

// NotificationFeed is the notification screen's payload.
//
// Active and History are separate lists rather than one list with a flag,
// because they are different things and are paginated differently. Active is
// everything still asking for something and is always sent whole — it is
// bounded by how much can actually be wrong at once. History is a page of what
// is already dealt with, walked with a cursor.
type NotificationFeed struct {
	Active  []UserNotification `json:"active"`
	History []UserNotification `json:"history"`
	// NextCursor continues the history. Empty means the end of it.
	NextCursor string `json:"next_cursor,omitempty"`
	// UnseenCount drives the badge. It counts unseen ordinary notifications
	// plus live internal alerts, which is "things outstanding" rather than
	// "things unread": an alert somebody has glanced at is still unhandled.
	UnseenCount int `json:"unseen_count"`
	// Sets names which sets this user receives, so the screen can say why an
	// alert reached them and, when they receive none, say that instead of
	// showing an empty list with no explanation.
	Sets []string `json:"sets"`
}

// NotificationSeenRequest marks ordinary notifications seen. Ids are the
// user_notifications rows the client has on screen.
type NotificationSeenRequest struct {
	Ids []int64 `json:"ids"`
	// All marks every ordinary notification seen, which is what opening the
	// screen does.
	All bool `json:"all"`
}

// NotificationActionRequest runs one button.
type NotificationActionRequest struct {
	Action string `json:"action"`
}

// NotificationActionResponse reports what running a button did.
//
// Refused is its own outcome rather than an error status, because the refusals
// here are ordinary and expected — acknowledging an outage that is still
// happening, approving a request somebody else just approved — and the client
// should show the sentence rather than "something went wrong".
type NotificationActionResponse struct {
	Ok      bool   `json:"ok"`
	Refused bool   `json:"refused,omitempty"`
	Message string `json:"message"`
	// Feed is the caller's refreshed feed, so a press needs no follow-up read.
	Feed *NotificationFeed `json:"feed,omitempty"`
}

// OrganizationNotificationRecipient is one member's subscriptions, for the
// organization settings screen.
type OrganizationNotificationRecipient struct {
	UserId string   `json:"user_id"`
	Name   string   `json:"name,omitempty"`
	Email  string   `json:"email,omitempty"`
	Role   string   `json:"role"`
	Sets   []string `json:"sets"`
}

// OrganizationNotificationSettings is the settings payload.
type OrganizationNotificationSettings struct {
	Catalog    []NotificationSetDefinition         `json:"catalog"`
	Recipients []OrganizationNotificationRecipient `json:"recipients"`
	CanEdit    bool                                `json:"can_edit"`
	// LiveCounts is how many unresolved alerts each set currently holds, shown
	// beside the set so somebody deciding whether to subscribe can see what
	// they would be signing up for.
	LiveCounts map[string]int `json:"live_counts"`
}

// SetOrganizationNotificationSetsRequest replaces one member's subscriptions.
type SetOrganizationNotificationSetsRequest struct {
	UserId string   `json:"user_id"`
	Sets   []string `json:"sets"`
}
