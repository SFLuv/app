package structs

import "strings"

import "time"

// SupportRequest is one "get in touch" submission from the website.
type SupportRequest struct {
	Id int64 `json:"id"`
	// Reference is what the sender is given to quote back, and what appears in
	// the Telegram alert. Short and unambiguous on purpose: someone reads it
	// off a phone screen and types it into a reply.
	Reference string `json:"reference"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	// Category is the stable routing slug behind Subject's display label, and
	// is what narrows which recipients the alert reaches. Empty when the
	// sender's form did not send one, or sent one we do not recognise.
	Category string `json:"category,omitempty"`
	Message  string `json:"message"`
	// Source distinguishes where the request was written.
	Source string `json:"source"`
	// UserId is the account that submitted it, when it came from a signed-in
	// caller. Nil for the website form, which has no session.
	//
	// What it is for: an alert about a request from an account we can reach in
	// the app can offer to open a chat with them, which is faster for both ends
	// than an email thread. Never shown to the sender, and never used to
	// authenticate anything — a support request's authority is the email
	// address it has always been.
	UserId *string `json:"user_id,omitempty"`
	// Status is "new" or "archived". Archiving is the only state change: a
	// request is never deleted, so an archived one can still be read.
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	ArchivedBy string     `json:"archived_by,omitempty"`
}

// SupportRequestStatus values.
const (
	SupportRequestNew      = "new"
	SupportRequestArchived = "archived"
)

// Where a support request was written.
const (
	SupportSourceWebsite = "website"
	SupportSourceApp     = "app"
)

// SupportRequestSubmission is the public form's body.
// Support request categories.
//
// These slugs are the webpage's ?subject= values, and they are the routing key:
// a recipient can narrow which categories reach them. The webpage owns the
// labels a sender reads; this list owns which values route, which is why the
// slug is sent alongside the label rather than the label being parsed.
//
// Adding one is a change in both repositories. An unknown slug is treated as
// no category rather than refused — see SupportCategoryOrEmpty.
const (
	SupportCategoryGeneral          = "general"
	SupportCategoryBecomeAnImprover = "become-an-improver"
	SupportCategoryMerchant         = "merchant-onboarding"
	SupportCategoryEvents           = "events"
	SupportCategoryDonations        = "donations"
	SupportCategoryPress            = "press"
	SupportCategoryTechnical        = "technical-support"
	SupportCategoryOther            = "other"
)

// SupportCategoryDefinition describes one category for the settings screen.
type SupportCategoryDefinition struct {
	Id    string `json:"id"`
	Label string `json:"label"`
}

// SupportCategoryCatalog is every category a recipient can subscribe to.
func SupportCategoryCatalog() []SupportCategoryDefinition {
	return []SupportCategoryDefinition{
		{Id: SupportCategoryGeneral, Label: "General questions"},
		{Id: SupportCategoryBecomeAnImprover, Label: "Becoming an improver"},
		{Id: SupportCategoryMerchant, Label: "Merchant onboarding"},
		{Id: SupportCategoryEvents, Label: "Events and volunteering"},
		{Id: SupportCategoryDonations, Label: "Donations"},
		{Id: SupportCategoryPress, Label: "Press or media"},
		{Id: SupportCategoryTechnical, Label: "Technical support"},
		{Id: SupportCategoryOther, Label: "Something else"},
	}
}

// IsSupportCategory reports whether a slug is one we route on.
func IsSupportCategory(id string) bool {
	for _, category := range SupportCategoryCatalog() {
		if category.Id == id {
			return true
		}
	}
	return false
}

// SupportCategoryOrEmpty normalizes what a client sent.
//
// An unknown slug becomes empty rather than an error, and empty means the
// request is not categorised — which the delivery rule sends to everybody on
// the support set. A newer webpage introducing a category this backend has not
// heard of must not drop the message on the floor; it reaches everyone, which
// is exactly what happened before categories existed.
func SupportCategoryOrEmpty(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if IsSupportCategory(id) {
		return id
	}
	return ""
}

type SupportRequestSubmission struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	// SubjectSlug is the stable routing key behind Subject's display label.
	SubjectSlug string `json:"subject_slug"`
	Message     string `json:"message"`
	// Company is a honeypot: a real person never sees the field, so anything
	// in it means a bot filled the form in.
	Company string `json:"company"`
}
