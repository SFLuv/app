package structs

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
	Message   string `json:"message"`
	// Source distinguishes the website form from anything added later.
	Source string `json:"source"`
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

// SupportRequestSubmission is the public form's body.
type SupportRequestSubmission struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Message string `json:"message"`
	// Company is a honeypot: a real person never sees the field, so anything
	// in it means a bot filled the form in.
	Company string `json:"company"`
}
