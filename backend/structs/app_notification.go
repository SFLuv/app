package structs

// Notification types. The feed is deliberately open-ended: new kinds can be
// added without a client release as long as clients render an unknown type
// using title/body and ignore the typed payload.
const (
	NotificationTypeWorkflowPayoutPending = "workflow_payout_pending"

	// Tax types. Unlike the workflow types these are not role-scoped: anybody
	// who earns past the reporting threshold owes a form, whether they are an
	// improver, a volunteer, or neither.
	NotificationTypeW9Required   = "w9_required"
	NotificationTypeW9EscrowHeld = "w9_escrow_held"
)

// ImproverNotification is one entry in the improver notification feed.
//
// Notifications are derived from live state rather than stored, so "resolved"
// is not a flag anybody has to maintain: an item disappears from the feed when
// the underlying condition clears (for a pending payout, when it settles).
// Only Seen is persisted, which is what drives the unread bubble.
type ImproverNotification struct {
	// Key is stable for the underlying condition so that marking it seen
	// survives the feed being recomputed on every request.
	Key       string `json:"key"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
	Seen      bool   `json:"seen"`
	SeenAt    *int64 `json:"seen_at,omitempty"`

	// Context for deep-linking. Empty for notification types that do not
	// relate to a specific workflow.
	WorkflowId    string `json:"workflow_id,omitempty"`
	WorkflowTitle string `json:"workflow_title,omitempty"`
	StepId        string `json:"step_id,omitempty"`
	StepTitle     string `json:"step_title,omitempty"`
	IsManager     bool   `json:"is_manager,omitempty"`
	AmountSfluv   uint64 `json:"amount_sfluv,omitempty"`
	PayoutError   string `json:"payout_error,omitempty"`

	// Tax context. TaxYear identifies which year's filing is outstanding;
	// EscrowedCount and OldestEscrowedAt let a client show how much is waiting
	// and how long it has been. Older clients ignore these and fall back to
	// title and body, which is why the feed was built this way.
	TaxYear          int    `json:"tax_year,omitempty"`
	EscrowedCount    int    `json:"escrowed_count,omitempty"`
	OldestEscrowedAt int64  `json:"oldest_escrowed_at,omitempty"`
	BackPaySfluv     string `json:"back_pay_sfluv,omitempty"`
}

// ImproverNotificationFeed is the payload behind the notification bell.
// UnseenCount drives the bubble number, and HasUnseen drives the dot on the
// improver tab icon — both are sent explicitly so clients never have to derive
// them from a possibly-paginated list.
type ImproverNotificationFeed struct {
	Notifications []ImproverNotification `json:"notifications"`
	UnseenCount   int                    `json:"unseen_count"`
	HasUnseen     bool                   `json:"has_unseen"`
	Total         int                    `json:"total"`
}

// ImproverNotificationSeenRequest marks specific notifications seen, or all of
// them when Keys is empty.
type ImproverNotificationSeenRequest struct {
	Keys []string `json:"keys"`
	All  bool     `json:"all"`
}
