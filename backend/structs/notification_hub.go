package structs

import (
	"strings"
	"time"
)

// The notification hub.
//
// Every way somebody reaches us, and everything the platform needs a person
// for, posts one notification here under a topic. Nothing else decides who
// hears about it. A topic is routed to one or more teams, a team is a set of
// house-organization members, and each team says how it wants to be told: in
// the app, by push, by email to its members, to a shared inbox, in a group
// chat. Before this, each touchpoint picked its own channel (an environment
// variable holding an address, a chat id, a set of subscribers), and the ones
// that picked nothing told nobody.
//
// Two rules follow. A topic nobody has routed still lands in the hub, where the
// admin panel lists it as reaching nobody, and it is handed to the catch-all
// route if one is set. And teams are bounded by the house organization: a
// member who leaves it stops receiving, whatever team rows still name them.

// Notification groups. These are what the old notification sets were, kept as
// the grouping a topic is shown under and as the set name the apps already
// display beside an alert.
const (
	NotificationSetDev      = "dev"
	NotificationSetAdmin    = "admin"
	NotificationSetSupport  = "support"
	NotificationSetActivity = "activity"
)

// NotificationSetDefinition names one group for the screens.
type NotificationSetDefinition struct {
	Id          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// NotificationSetCatalog is every group, in the order screens show them.
func NotificationSetCatalog() []NotificationSetDefinition {
	return []NotificationSetDefinition{
		{Id: NotificationSetSupport, Label: "Support and email",
			Description: "People writing to us: the Get in touch form, the app's Contact SFLuv, merchant PIN help, and email to our addresses."},
		{Id: NotificationSetAdmin, Label: "Requests and decisions",
			Description: "Things waiting on a person: role and merchant requests, workflow proposals, events, credentials, W-9 problems, content reports."},
		{Id: NotificationSetDev, Label: "Infrastructure",
			Description: "Accounts running low, the backend failing its health check, and payouts that failed or never left."},
		{Id: NotificationSetActivity, Label: "Activity",
			Description: "Things worth knowing that need nothing doing: event signups and form signatures. They go straight to history."},
	}
}

// NotificationSetLabel is a group's display name, or the id if unknown.
func NotificationSetLabel(id string) string {
	for _, set := range NotificationSetCatalog() {
		if set.Id == id {
			return set.Label
		}
	}
	return id
}

// NotificationTopicDefinition is one thing the hub can be told.
type NotificationTopicDefinition struct {
	Id          string `json:"id"`
	Set         string `json:"set"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// The catch-all route. Not a topic anything posts under: a team routed here
// receives every notification whose own topic has no team.
const NotificationTopicUnrouted = "unrouted"

// Topics. Role and support topics are built from their role or category by the
// helpers below; the rest are constants.
const (
	NotificationTopicFunding       = "dev.funding"
	NotificationTopicUptime        = "dev.uptime"
	NotificationTopicPayoutFailure = "dev.payout_failure"
	NotificationTopicPayoutFunding = "dev.workflow_funding"

	NotificationTopicOrganizationRole = "role.organization"
	NotificationTopicWorkflowProposal = "workflow.proposal"
	NotificationTopicEventRequest     = "event.request"
	NotificationTopicEventFunding     = "event.funding"
	NotificationTopicCredentialReq    = "credential.request"
	NotificationTopicW9               = "w9.support_needed"
	NotificationTopicContentReport    = "safety.report"
	NotificationTopicFormWithdrawal   = "site_form.withdrawal"

	NotificationTopicPinHelp = "support.pin_help"

	NotificationTopicEmailSupport  = "email.support"
	NotificationTopicEmailAdmin    = "email.admin"
	NotificationTopicEmailReply    = "email.reply"
	NotificationTopicEmailOther    = "email.other"
	NotificationTopicEmailFiltered = "email.filtered"

	NotificationTopicEventSignup   = "event.signup"
	NotificationTopicFormSignature = "site_form.signature"

	supportTopicPrefix                    = "support."
	NotificationTopicSupportUncategorized = "support.uncategorized"
	roleTopicPrefix                       = "role."
)

// Role request topics. One per role, so merchant approvals and improver
// requests can go to different people.
var roleTopicLabels = []struct{ Role, Label, Description string }{
	{"merchant", "Merchant approvals", "A new merchant location waiting for review. Approve or reject from the notification."},
	{"affiliate", "Affiliate requests", "Somebody asking to become an affiliate. Approve or reject from the notification."},
	{"improver", "Improver requests", "Somebody asking to become an improver. Approve or reject from the notification."},
	{"proposer", "Proposer requests", "Somebody asking to become a proposer. Approve or reject from the notification."},
	{"supervisor", "Supervisor requests", "Somebody asking to become a supervisor. Approve or reject from the notification."},
	{"issuer", "Issuer requests", "Somebody asking to become a credential issuer. Approve or reject from the notification."},
}

// NotificationTopicForRole is the topic a role request posts under.
func NotificationTopicForRole(role string) string {
	return roleTopicPrefix + strings.ToLower(strings.TrimSpace(role))
}

// NotificationTopicForSupportCategory is the topic a support request posts
// under. An empty or unknown category has its own topic rather than borrowing
// one, so it can be routed like any other and cannot vanish.
func NotificationTopicForSupportCategory(category string) string {
	if category = SupportCategoryOrEmpty(category); category != "" {
		return supportTopicPrefix + category
	}
	return NotificationTopicSupportUncategorized
}

// NotificationTopicCatalog is every topic, grouped and in display order.
func NotificationTopicCatalog() []NotificationTopicDefinition {
	out := []NotificationTopicDefinition{}

	for _, category := range SupportCategoryCatalog() {
		out = append(out, NotificationTopicDefinition{
			Id: supportTopicPrefix + category.Id, Set: NotificationSetSupport,
			Label:       "Support: " + category.Label,
			Description: "A Get in touch or Contact SFLuv message on this topic.",
		})
	}
	out = append(out,
		NotificationTopicDefinition{Id: NotificationTopicSupportUncategorized, Set: NotificationSetSupport,
			Label:       "Support: no topic",
			Description: "A support message whose form sent no topic, or one this backend does not know."},
		NotificationTopicDefinition{Id: NotificationTopicPinHelp, Set: NotificationSetSupport,
			Label:       "Merchant PIN help",
			Description: "A merchant locked out of merchant mode asking for their PIN to be reset."},
		NotificationTopicDefinition{Id: NotificationTopicEmailSupport, Set: NotificationSetSupport,
			Label:       "Email to support@",
			Description: "Mail that arrived at the support address."},
		NotificationTopicDefinition{Id: NotificationTopicEmailAdmin, Set: NotificationSetSupport,
			Label:       "Email to admin@",
			Description: "Mail that arrived at the admin address."},
		NotificationTopicDefinition{Id: NotificationTopicEmailReply, Set: NotificationSetSupport,
			Label:       "Replies to our emails",
			Description: "Somebody answering an email we sent from no_reply@."},
		NotificationTopicDefinition{Id: NotificationTopicEmailOther, Set: NotificationSetSupport,
			Label:       "Email to other addresses",
			Description: "Mail to any other address on our receiving domain."},
		NotificationTopicDefinition{Id: NotificationTopicEmailFiltered, Set: NotificationSetActivity,
			Label:       "Filtered email",
			Description: "Mail flagged as spam or sent automatically (out-of-office, bounces). Kept, archived, and announced here only."},
	)

	for _, role := range roleTopicLabels {
		out = append(out, NotificationTopicDefinition{
			Id: NotificationTopicForRole(role.Role), Set: NotificationSetAdmin,
			Label: role.Label, Description: role.Description,
		})
	}
	out = append(out,
		NotificationTopicDefinition{Id: NotificationTopicOrganizationRole, Set: NotificationSetAdmin,
			Label:       "Organization role requests",
			Description: "An organization asking for a role (affiliate, proposer, supervisor, issuer). Decided in the admin panel."},
		NotificationTopicDefinition{Id: NotificationTopicWorkflowProposal, Set: NotificationSetAdmin,
			Label:       "Workflow proposals",
			Description: "A new workflow, an edit or a deletion waiting on votes."},
		NotificationTopicDefinition{Id: NotificationTopicEventRequest, Set: NotificationSetAdmin,
			Label:       "Event requests",
			Description: "An affiliate submitting or changing a volunteer event."},
		NotificationTopicDefinition{Id: NotificationTopicEventFunding, Set: NotificationSetAdmin,
			Label:       "Events needing funding",
			Description: "A volunteer event that cannot pay out until it is funded."},
		NotificationTopicDefinition{Id: NotificationTopicCredentialReq, Set: NotificationSetAdmin,
			Label:       "Credential requests",
			Description: "An improver asking for a credential. Issuers holding it are emailed as well."},
		NotificationTopicDefinition{Id: NotificationTopicW9, Set: NotificationSetAdmin,
			Label:       "W-9 problems",
			Description: "Somebody whose W-9 failed the IRS check twice and needs a person."},
		NotificationTopicDefinition{Id: NotificationTopicContentReport, Set: NotificationSetAdmin,
			Label:       "Content reports",
			Description: "A post, a person or a message reported from the app."},
		NotificationTopicDefinition{Id: NotificationTopicFormWithdrawal, Set: NotificationSetAdmin,
			Label:       "Consent withdrawals",
			Description: "Somebody asking to withdraw a waiver or consent they signed on sfluv.org."},

		NotificationTopicDefinition{Id: NotificationTopicFunding, Set: NotificationSetDev,
			Label:       "Accounts running low",
			Description: "A protocol account below its gas or rewards floor. Clears itself when topped up."},
		NotificationTopicDefinition{Id: NotificationTopicUptime, Set: NotificationSetDev,
			Label:       "Backend down",
			Description: "The backend failing its health check."},
		NotificationTopicDefinition{Id: NotificationTopicPayoutFailure, Set: NotificationSetDev,
			Label:       "Payout failures",
			Description: "A workflow payout that failed, or one marked paid that never left the faucet."},
		NotificationTopicDefinition{Id: NotificationTopicPayoutFunding, Set: NotificationSetDev,
			Label:       "Workflow funding shortfalls",
			Description: "A recurring workflow reaching its start with too little in the faucet."},

		NotificationTopicDefinition{Id: NotificationTopicEventSignup, Set: NotificationSetActivity,
			Label:       "Event signups",
			Description: "Somebody signing up for a volunteer event."},
		NotificationTopicDefinition{Id: NotificationTopicFormSignature, Set: NotificationSetActivity,
			Label:       "Form signatures",
			Description: "Somebody signing a waiver or form on sfluv.org."},
	)
	return out
}

// NotificationTopic looks a topic up. The catch-all is not a topic.
func NotificationTopic(id string) (NotificationTopicDefinition, bool) {
	for _, topic := range NotificationTopicCatalog() {
		if topic.Id == id {
			return topic, true
		}
	}
	return NotificationTopicDefinition{}, false
}

// IsRoutableTopic is whether a route may name this id: any topic, or the
// catch-all.
func IsRoutableTopic(id string) bool {
	if id == NotificationTopicUnrouted {
		return true
	}
	_, ok := NotificationTopic(id)
	return ok
}

// ── Touchpoints ──────────────────────────────────────────────────────────────

// NotificationTouchpointDefinition is one way somebody reaches us, or one thing
// the platform raises on its own, and the topics it posts under. The settings
// screen's visualizer draws from this, so it is the one place the map from a
// form or a sweep to its topics is written down.
type NotificationTouchpointDefinition struct {
	Id          string   `json:"id"`
	Group       string   `json:"group"`
	Label       string   `json:"label"`
	Topics      []string `json:"topics"`
	Description string   `json:"description"`
	// Outside is a touchpoint that is not in our systems at all, so it cannot
	// post to the hub. Listed so the map does not pretend it is covered.
	Outside bool `json:"outside,omitempty"`
}

func supportTopics(categories ...string) []string {
	out := make([]string, 0, len(categories))
	for _, category := range categories {
		out = append(out, NotificationTopicForSupportCategory(category))
	}
	return out
}

// NotificationTouchpointCatalog is every touchpoint, grouped and in display
// order. Every topic in the topic catalog is reached by at least one.
func NotificationTouchpointCatalog() []NotificationTouchpointDefinition {
	website := []string{
		SupportCategoryGeneral, SupportCategoryBecomeAnImprover, SupportCategoryMerchant, SupportCategoryEvents,
		SupportCategoryDonations, SupportCategoryPress, SupportCategoryTechnical, SupportCategoryW9, SupportCategoryOther,
	}
	app := []string{
		SupportCategoryTechnical, SupportCategoryGeneral, SupportCategoryMerchant, SupportCategoryBecomeAnImprover,
		SupportCategoryEvents, SupportCategoryW9, SupportCategoryOther,
	}
	return []NotificationTouchpointDefinition{
		{Id: "web.get_in_touch", Group: "Website", Label: "Get in touch form",
			Topics:      append(supportTopics(website...), NotificationTopicSupportUncategorized),
			Description: "sfluv.org/get-in-touch. Each topic the sender picks is its own topic here; an older form that sends none lands under Support: no topic."},
		{Id: "web.delete_account", Group: "Website", Label: "Account deletion help",
			Topics:      supportTopics(SupportCategoryAccountDeletion),
			Description: "sfluv.org/delete-account, for someone who cannot delete their account from the app."},
		{Id: "web.w9_page", Group: "Website", Label: "W-9 page",
			Topics:      supportTopics(SupportCategoryW9),
			Description: "sfluv.org/submit-w9, still linked from old emails. Sends people to the app's W-9 prompt, or to Get in touch with W-9 picked."},
		{Id: "web.become_merchant", Group: "Website", Label: "Become a Merchant",
			Topics:      []string{NotificationTopicForRole("merchant")},
			Description: "Links to merchant onboarding in the web app; the application posts a merchant approval."},
		{Id: "web.form_signature", Group: "Website", Label: "Forms and waivers",
			Topics:      []string{NotificationTopicFormSignature},
			Description: "A signature on any site form. Goes straight to history."},
		{Id: "web.form_withdrawal", Group: "Website", Label: "Consent withdrawal",
			Topics:      []string{NotificationTopicFormWithdrawal},
			Description: "The withdrawal form. The first 20 an hour are announced one by one, the rest on one standing alert."},
		{Id: "web.event_signup", Group: "Website", Label: "Event signup",
			Topics:      []string{NotificationTopicEventSignup},
			Description: "A volunteer signing up on the website or in the app. Goes straight to history."},
		{Id: "web.tree_steward", Group: "Website", Label: "Tree Steward signup", Outside: true, Topics: []string{},
			Description: "A Google Form outside our systems. It would need to move into the site's own forms to reach the hub."},
		{Id: "web.social", Group: "Website", Label: "Social media DMs", Outside: true, Topics: []string{},
			Description: "Instagram, X, Facebook and LinkedIn, read by whoever runs the accounts."},

		{Id: "app.role_requests", Group: "Web and mobile app", Label: "Role and merchant requests",
			Topics: []string{NotificationTopicForRole("merchant"), NotificationTopicForRole("affiliate"),
				NotificationTopicForRole("improver"), NotificationTopicForRole("proposer")},
			Description: "Merchant locations and affiliate, improver and proposer requests. Approve or reject from the notification."},
		{Id: "app.supervisor_issuer", Group: "Web and mobile app", Label: "Supervisor and issuer requests",
			Topics:      []string{NotificationTopicForRole("supervisor"), NotificationTopicForRole("issuer")},
			Description: "Approve or reject from the notification."},
		{Id: "app.organization_roles", Group: "Web and mobile app", Label: "Organization role requests",
			Topics:      []string{NotificationTopicOrganizationRole},
			Description: "An organization asking for a role from its Organization page. Cleared when an admin sets the status."},
		{Id: "app.workflow_proposals", Group: "Web and mobile app", Label: "Workflow proposals",
			Topics:      []string{NotificationTopicWorkflowProposal},
			Description: "New workflows, edits and deletions waiting on votes. Cleared when the vote decides."},
		{Id: "app.event_requests", Group: "Web and mobile app", Label: "Affiliate event requests",
			Topics:      []string{NotificationTopicEventRequest},
			Description: "An affiliate submitting or changing a volunteer event. Cleared when approved or rejected."},
		{Id: "app.pin_help", Group: "Web and mobile app", Label: "Merchant PIN help",
			Topics:      []string{NotificationTopicPinHelp},
			Description: "From settings on the web, and Forgot your PIN? on the till. One live alert per merchant."},
		{Id: "app.credential_requests", Group: "Web and mobile app", Label: "Credential requests",
			Topics:      []string{NotificationTopicCredentialReq},
			Description: "Issuers who can grant it are emailed directly as well; urgent here when none can."},
		{Id: "app.contact", Group: "Web and mobile app", Label: "Contact SFLuv (mobile)",
			Topics:      supportTopics(app...),
			Description: "Settings → Account → Contact SFLuv, and the till setup screen's support link. Routed by topic like the website."},
		{Id: "app.till_setup", Group: "Web and mobile app", Label: "Till setup screen (mobile)",
			Topics:      supportTopics(SupportCategoryTechnical),
			Description: "A merchant whose location has no payment wallet yet. Opens Contact SFLuv, which starts on Something isn't working."},
		{Id: "app.payout_retry", Group: "Web and mobile app", Label: "Payout retry (mobile)",
			Topics:      []string{NotificationTopicPayoutFailure},
			Description: "An improver retrying a failed payout. If it fails again, it posts under Payout failures."},
		{Id: "app.content_reports", Group: "Web and mobile app", Label: "Report content",
			Topics:      []string{NotificationTopicContentReport},
			Description: "A post, a person or a message reported from the app. One alert per reported thing, cleared when resolved."},

		{Id: "email.reply", Group: "Email", Label: "Replies to our emails",
			Topics:      []string{NotificationTopicEmailReply},
			Description: "Answers to mail we send from no_reply@. A reply is marked with the request it answers."},
		{Id: "email.support", Group: "Email", Label: "Mail to support@",
			Topics:      []string{NotificationTopicEmailSupport},
			Description: "Forwarded from the Google inbox to the receiving domain."},
		{Id: "email.admin", Group: "Email", Label: "Mail to admin@",
			Topics:      []string{NotificationTopicEmailAdmin},
			Description: "Forwarded from the Google inbox to the receiving domain."},
		{Id: "email.other", Group: "Email", Label: "Mail to other addresses",
			Topics:      []string{NotificationTopicEmailOther},
			Description: "Anything else at the receiving domain."},
		{Id: "email.filtered", Group: "Email", Label: "Spam and auto-replies",
			Topics:      []string{NotificationTopicEmailFiltered},
			Description: "Kept and archived, and filed to history so nobody is woken for it."},

		{Id: "platform.funding", Group: "The platform itself", Label: "Accounts running low",
			Topics:      []string{NotificationTopicFunding},
			Description: "The funding sweep, every few minutes. Clears itself when the account is topped up."},
		{Id: "platform.uptime", Group: "The platform itself", Label: "Backend down",
			Topics:      []string{NotificationTopicUptime},
			Description: "The health poller. Acknowledging is refused while it is still down."},
		{Id: "platform.payouts", Group: "The platform itself", Label: "Payouts failed or never sent",
			Topics:      []string{NotificationTopicPayoutFailure},
			Description: "A workflow payout that failed, including after an improver's retry, or one marked paid that never left."},
		{Id: "platform.series_funding", Group: "The platform itself", Label: "Workflow funding shortfall",
			Topics:      []string{NotificationTopicPayoutFunding},
			Description: "A recurring workflow reaching its start with too little unallocated in the faucet."},
		{Id: "platform.event_funding", Group: "The platform itself", Label: "Event needs funding",
			Topics:      []string{NotificationTopicEventFunding},
			Description: "A recurring event that could not mint its codes. Cleared once the faucet is topped up."},
		{Id: "platform.w9", Group: "The platform itself", Label: "W-9 failed twice",
			Topics:      []string{NotificationTopicW9},
			Description: "Somebody whose W-9 failed the IRS check twice. The chat line leaves out their tax details."},
	}
}

// NotificationTopicActivity is how busy one topic has been.
type NotificationTopicActivity struct {
	LastAt *time.Time `json:"last_at,omitempty"`
	// Last24h counts notifications raised in the last day.
	Last24h int `json:"last_24h"`
	// ReachedNobody30d counts the last 30 days' that reached nobody.
	ReachedNobody30d int `json:"reached_nobody_30d"`
}

// ── Teams ────────────────────────────────────────────────────────────────────

// NotificationTeam is a group of house-organization members, and how they want
// to be told.
type NotificationTeam struct {
	Id          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Members get every notification routed to the team in the app's
	// notification screen. That part is not optional: it is what the hub is.
	// The rest are extra ways to be told.
	PushMembers  bool   `json:"push_members"`
	EmailMembers bool   `json:"email_members"`
	EmailAddress string `json:"email_address"`
	// ChatConversationId is a group chat the team's notifications are posted
	// into by the support bot. Nil for none.
	ChatConversationId *int64 `json:"chat_conversation_id,omitempty"`
	// ChatName is the chat's title, filled in for the settings screen.
	ChatName  string    `json:"chat_name,omitempty"`
	MemberIds []string  `json:"member_ids"`
	Topics    []string  `json:"topics"`
	CreatedAt time.Time `json:"created_at"`
}

// NotificationTeamInput creates or changes a team's details.
type NotificationTeamInput struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	PushMembers        bool   `json:"push_members"`
	EmailMembers       bool   `json:"email_members"`
	EmailAddress       string `json:"email_address"`
	ChatConversationId *int64 `json:"chat_conversation_id"`
}

// NotificationTeamMembersRequest replaces a team's members.
type NotificationTeamMembersRequest struct {
	UserIds []string `json:"user_ids"`
}

// NotificationRouteRequest replaces which teams receive one topic.
type NotificationRouteRequest struct {
	Topic   string  `json:"topic"`
	TeamIds []int64 `json:"team_ids"`
}

// NotificationHubMember is one house-organization member, as the team editor
// lists them.
type NotificationHubMember struct {
	UserId string `json:"user_id"`
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	Role   string `json:"role"`
	// Devices is how many phones a push can reach, so the editor can say that
	// somebody on a push team will not actually be buzzed.
	Devices int `json:"devices"`
}

// NotificationDeliveryTeam names a team a notification went to.
type NotificationDeliveryTeam struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

// NotificationDelivery is where one notification went, kept on it so the hub
// can show what reached nobody.
type NotificationDelivery struct {
	Teams []NotificationDeliveryTeam `json:"teams"`
	// CatchAll is set when the topic had no team and the catch-all route
	// carried it instead.
	CatchAll   bool     `json:"catch_all,omitempty"`
	Recipients int      `json:"recipients"`
	Pushed     int      `json:"pushed"`
	Emailed    []string `json:"emailed,omitempty"`
	Chats      []int64  `json:"chats,omitempty"`
	Failures   []string `json:"failures,omitempty"`
	// ReachedNobody is the one to look for: no member, no address, no chat.
	ReachedNobody bool `json:"reached_nobody"`
}

// NotificationHubEntry is one notification in the hub's recent list.
type NotificationHubEntry struct {
	Id         int64                 `json:"id"`
	Topic      string                `json:"topic"`
	TopicLabel string                `json:"topic_label"`
	Set        string                `json:"set"`
	Kind       string                `json:"kind"`
	Title      string                `json:"title"`
	Body       string                `json:"body"`
	CreatedAt  time.Time             `json:"created_at"`
	ResolvedAt *time.Time            `json:"resolved_at,omitempty"`
	Resolution string                `json:"resolution,omitempty"`
	Outcome    string                `json:"outcome,omitempty"`
	Delivery   *NotificationDelivery `json:"delivery,omitempty"`
}

// NotificationHubView is the admin panel's notification settings.
type NotificationHubView struct {
	Sets    []NotificationSetDefinition   `json:"sets"`
	Topics  []NotificationTopicDefinition `json:"topics"`
	Teams   []NotificationTeam            `json:"teams"`
	Members []NotificationHubMember       `json:"members"`
	// Routes is topic → team ids, including the catch-all under "unrouted".
	Routes map[string][]int64 `json:"routes"`
	// LiveCounts is unresolved notifications per topic.
	LiveCounts map[string]int         `json:"live_counts"`
	Recent     []NotificationHubEntry `json:"recent"`
	// ReachedNobody counts the last 30 days' notifications that reached no
	// person, address or chat.
	ReachedNobody int  `json:"reached_nobody"`
	CanEdit       bool `json:"can_edit"`
	// InboundEmail says whether the backend can receive mail at all, so the
	// screen can say the email topics are dead until it is set up.
	InboundEmail bool `json:"inbound_email"`
	// Touchpoints is every way in and the topics each posts under, for the
	// visualizer.
	Touchpoints []NotificationTouchpointDefinition `json:"touchpoints"`
	// TopicActivity is per topic: when it last fired, how often in the last
	// day, and how often it reached nobody in the last 30.
	TopicActivity map[string]NotificationTopicActivity `json:"topic_activity"`
	// GeneratedAt is when this view was read, so a live screen can say how
	// fresh it is.
	GeneratedAt time.Time `json:"generated_at"`
}
