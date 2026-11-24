package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// NotificationService posts internal organization alerts and delivers them.
//
// It replaces a Telegram bot that did the same job against a group chat. What
// carried over is the part that was never about Telegram: an alert names a
// condition, posting one is idempotent in its key, and clearing the condition
// retracts it. What went away is a third-party dependency, an outbound webhook,
// a 64-byte limit on what a button could say, a 48-hour window after which
// messages could no longer be deleted, and an allowlist of chat ids to keep
// strangers from pressing our buttons. In-app, the person pressing the button
// is already authenticated and already known to be a recipient.
type NotificationService struct {
	db     *db.AppDB
	logger *logger.LogCloser

	// push sends to one user's devices. Injected rather than called directly
	// so the alert layer does not depend on the Expo plumbing, and so a test
	// can watch what would have been sent.
	push func(ctx context.Context, userId string, title string, body string, data map[string]string)

	// chat posts one line into a group chat as the support bot, for teams that
	// asked for their notifications in a chat. Injected for the same reason as
	// push: the chat plumbing lives on AppService.
	chat func(ctx context.Context, conversationId int64, line string) error

	// announce sends a new notification over the socket to recipients with
	// the app open; announceResolved tells recipients one was handled. Both
	// live on AppService, with the socket.
	announce         func(userIds []string, title, body string, data map[string]string)
	announceResolved func(ctx context.Context, id int64, key string, userIds []string)

	// houseMu guards the cached house organization id. The lookup is a query
	// per alert otherwise, and alerts are posted from a once-a-minute sweep.
	houseMu sync.Mutex
	houseId int64
}

// NewNotificationService builds the alert layer.
func NewNotificationService(appDb *db.AppDB, log *logger.LogCloser) *NotificationService {
	return &NotificationService{db: appDb, logger: log}
}

// SetAnnouncer wires the socket: new notifications to open apps, and handled
// ones cleared from every app and phone they reached.
func (n *NotificationService) SetAnnouncer(
	announce func(userIds []string, title, body string, data map[string]string),
	announceResolved func(ctx context.Context, id int64, key string, userIds []string),
) {
	if n == nil {
		return
	}
	n.announce = announce
	n.announceResolved = announceResolved
}

// SetPusher wires device delivery. Set after construction because the pusher
// lives on AppService, which needs this service to exist first.
func (n *NotificationService) SetPusher(
	push func(ctx context.Context, userId string, title string, body string, data map[string]string),
) {
	if n == nil {
		return
	}
	n.push = push
}

// SetChatPoster wires group-chat delivery.
func (n *NotificationService) SetChatPoster(chat func(ctx context.Context, conversationId int64, line string) error) {
	if n == nil {
		return
	}
	n.chat = chat
}

func (n *NotificationService) logf(format string, args ...any) {
	if n == nil || n.logger == nil {
		return
	}
	n.logger.Logf(format, args...)
}

// houseOrganization resolves and caches the organization alerts are addressed
// to.
//
// A missing house organization is not an error worth shouting about on every
// sweep — it is what a fresh database looks like — so it is reported once per
// process and then stays quiet.
func (n *NotificationService) houseOrganization(ctx context.Context) (int64, error) {
	n.houseMu.Lock()
	defer n.houseMu.Unlock()
	if n.houseId != 0 {
		return n.houseId, nil
	}
	id, err := n.db.HouseOrganizationId(ctx)
	if err != nil {
		return 0, err
	}
	n.houseId = id
	return id, nil
}

// Alert is one notification for the hub.
type Alert struct {
	// Topic is what it is about, and decides which teams hear about it. One
	// of the topics in structs.NotificationTopicCatalog.
	Topic string
	// Key is stable for the condition. Two posts with the same key while the
	// first is live are the same alert, however far apart.
	Key string
	// Kind selects the action buttons.
	Kind string
	// Title is the notification's headline, on its detail page and in email.
	Title string
	// Subtitle is who or what it is about in a few words: the requester's
	// email, a name, an event. The app's list shows the topic and this line
	// only; everything else waits for a tap. Empty uses Title.
	Subtitle string
	Body     string
	// Fields are label/value detail rows.
	Fields []structs.NotificationField
	// Payload is what an action needs to do its work.
	Payload map[string]string
	// Urgent sorts it to the top and is for things that are actively broken
	// rather than things that need a decision.
	Urgent bool
	// Informational is for a notification that asks for nothing: a signup, a
	// signature. It is delivered like any other and goes straight to history,
	// so it never sits in anybody's active list.
	Informational bool
	// ReplyTo is where a reply to the team's email should go: the person who
	// wrote in, so answering from an inbox reaches them.
	ReplyTo string
	// ChatLine replaces the line a team chat is sent, for a caller that knows
	// better than the generic one what is safe to show to everybody in a chat.
	ChatLine string
	// OnlyTeamId sends it to one team, through its own rules, instead of to
	// every team the topic is routed to. Team test sends use it.
	OnlyTeamId int64
}

// Post puts a notification in the hub and delivers it to the teams its topic
// is routed to.
//
// Safe to call on a timer with the same alert forever. The first call creates,
// delivers and pushes; later calls refresh the text in place and only deliver
// to people who did not have it yet (somebody added to a team since), so a
// condition that has held for a day has buzzed each phone once. Email and chat
// go out on the first call only.
func (n *NotificationService) Post(ctx context.Context, alert Alert) error {
	if n == nil {
		return nil
	}
	topic, known := structs.NotificationTopic(alert.Topic)
	if !known {
		return fmt.Errorf("unknown notification topic %q", alert.Topic)
	}
	if strings.TrimSpace(alert.Key) == "" {
		return fmt.Errorf("an alert key is required")
	}

	orgId, err := n.houseOrganization(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNoHouseOrganization) {
			// Nothing to address it to. Said once, at the point where somebody
			// setting the system up would be looking for it.
			n.logf("notifications: dropping alert %s — there is no house organization yet", alert.Key)
			return nil
		}
		return err
	}

	id, created, err := n.db.PostInternalAlert(ctx, db.InternalAlert{
		OrganizationId: orgId,
		Set:            topic.Set,
		Topic:          topic.Id,
		AlertKey:       alert.Key,
		Kind:           alert.Kind,
		Title:          alert.Title,
		Subtitle:       subtitleOf(alert),
		Body:           alert.Body,
		Fields:         alert.Fields,
		Payload:        alert.Payload,
		Urgent:         alert.Urgent,
	})
	if err != nil {
		return err
	}

	var plan *db.NotificationDeliveryPlan
	if alert.OnlyTeamId != 0 {
		plan, err = n.db.NotificationTeamPlan(ctx, orgId, alert.OnlyTeamId, topic.Id)
	} else {
		plan, err = n.db.NotificationDeliveryPlan(ctx, orgId, topic.Id)
	}
	if err != nil {
		// Stored, so it is in the hub and in the admin panel's list; nobody
		// was told. Recorded as such so the list says so.
		if created {
			_ = n.db.SetInternalAlertDelivery(ctx, id, structs.NotificationDelivery{
				ReachedNobody: true, Failures: []string{"routing: " + err.Error()},
			})
		}
		return err
	}

	recipients, err := n.db.DeliverInternalAlertTo(ctx, id, plan.Members)
	if err != nil {
		return err
	}

	pushed := 0
	data := map[string]string{
		"type":             "internal_" + alert.Kind,
		"notification_set": topic.Set,
		"topic":            topic.Id,
		"alert_key":        alert.Key,
		"internal_id":      strconv.FormatInt(id, 10),
	}
	pushTitle, pushBody := hubPushText(topic, alert)
	// Everybody it just reached who has the app open sees it now, as an in-app
	// banner, and their list refreshes; the push below still goes.
	if n.announce != nil && len(recipients) > 0 {
		n.announce(recipients, pushTitle, pushBody, data)
	}
	if n.push != nil && len(recipients) > 0 {
		for _, userId := range recipients {
			if !plan.Push[userId] {
				continue
			}
			n.push(ctx, userId, pushTitle, pushBody, data)
			pushed++
		}
	}

	if !created {
		return nil
	}

	delivery := structs.NotificationDelivery{
		Teams:      plan.Teams,
		CatchAll:   plan.CatchAll,
		Recipients: len(recipients),
		Pushed:     pushed,
	}
	if delivery.Teams == nil {
		delivery.Teams = []structs.NotificationDeliveryTeam{}
	}
	if plan.Empty() {
		// The failure this whole hub exists to prevent, so it is loud: kept,
		// listed in the admin panel as reaching nobody, and logged.
		delivery.ReachedNobody = true
		n.logf("notifications: %s (%s) reached NOBODY — no team takes %q and there is no catch-all", alert.Key, topic.Label, topic.Id)
	}

	if len(plan.Emails) > 0 || len(plan.Chats) > 0 {
		// Email and chat leave the request's path: the person who wrote in is
		// not kept waiting on Mailgun, and the record is written once they are
		// done.
		go n.deliverOutside(context.WithoutCancel(ctx), id, topic, alert, plan, delivery)
	} else if err := n.db.SetInternalAlertDelivery(ctx, id, delivery); err != nil {
		n.logf("notifications: %s", err)
	}

	if alert.Informational {
		if err := n.db.ResolveInternalAlertInformational(ctx, id); err != nil {
			n.logf("notifications: could not file %s to history: %s", alert.Key, err)
		}
	}

	return nil
}

// deliverOutside sends a new notification to its teams' addresses and chats,
// then records where it went.
func (n *NotificationService) deliverOutside(
	ctx context.Context, id int64, topic structs.NotificationTopicDefinition, alert Alert,
	plan *db.NotificationDeliveryPlan, delivery structs.NotificationDelivery,
) {
	defer func() {
		if rec := recover(); rec != nil {
			n.logf("notifications: recovered while delivering %s: %v", alert.Key, rec)
		}
	}()

	if len(plan.Emails) > 0 {
		sender := utils.NewEmailSender()
		if sender == nil {
			delivery.Failures = append(delivery.Failures, "email: no mailer is configured")
		} else {
			html := hubEmailHTML(topic, alert, plan.Teams)
			for _, address := range plan.Emails {
				if err := sender.SendEmailWithReplyTo(address, "", hubEmailSubject(topic, alert), html,
					utils.NotificationFromEmail(), "SFLuv notifications", alert.ReplyTo); err != nil {
					delivery.Failures = append(delivery.Failures, "email to "+address+": "+err.Error())
					continue
				}
				delivery.Emailed = append(delivery.Emailed, address)
			}
		}
	}

	if len(plan.Chats) > 0 {
		line := hubChatMessage(topic, alert)
		for _, chat := range plan.Chats {
			if n.chat == nil {
				delivery.Failures = append(delivery.Failures, "chat: chat delivery is not wired")
				break
			}
			if err := n.chat(ctx, chat, line); err != nil {
				delivery.Failures = append(delivery.Failures, fmt.Sprintf("chat %d: %s", chat, err))
				continue
			}
			delivery.Chats = append(delivery.Chats, chat)
		}
	}

	if delivery.Recipients == 0 && len(delivery.Emailed) == 0 && len(delivery.Chats) == 0 {
		delivery.ReachedNobody = true
		n.logf("notifications: %s (%s) reached NOBODY — every address and chat failed: %s",
			alert.Key, topic.Label, strings.Join(delivery.Failures, "; "))
	} else if len(delivery.Failures) > 0 {
		n.logf("notifications: %s partly delivered: %s", alert.Key, strings.Join(delivery.Failures, "; "))
	}
	if err := n.db.SetInternalAlertDelivery(ctx, id, delivery); err != nil {
		n.logf("notifications: %s", err)
	}
}

// pushPreviewRunes is how much of the message a push carries after the
// subtitle: the gist, not the whole thing, which opens with a tap.
const pushPreviewRunes = 160

// subtitleOf is the line under the topic in the app's list.
func subtitleOf(alert Alert) string {
	if subtitle := oneLineText(alert.Subtitle); subtitle != "" {
		return subtitle
	}
	return oneLineText(alert.Title)
}

// hubPushText is what a phone shows: the topic as the title, then who or what
// it is about, the headline when it says more than that, and the start of the
// message, each on its own line.
func hubPushText(topic structs.NotificationTopicDefinition, alert Alert) (string, string) {
	lines := []string{}
	subtitle := subtitleOf(alert)
	if subtitle != "" {
		lines = append(lines, subtitle)
	}
	if title := oneLineText(alert.Title); title != "" && title != subtitle {
		lines = append(lines, title)
	}
	if body := oneLineText(alert.Body); body != "" {
		lines = append(lines, truncateRunes(body, pushPreviewRunes))
	}
	return topic.Label, strings.Join(lines, "\n")
}

// hubEmailSubject leads with the topic, so an inbox sorts by it, then who or
// what it is about.
func hubEmailSubject(topic structs.NotificationTopicDefinition, alert Alert) string {
	if subtitle := subtitleOf(alert); subtitle != "" {
		return topic.Label + ": " + subtitle
	}
	return topic.Label
}

// hubEmailHTML is a notification as an email: the headline, the details as a
// table, then the message itself with its line breaks kept.
func hubEmailHTML(topic structs.NotificationTopicDefinition, alert Alert, teams []structs.NotificationDeliveryTeam) string {
	var content strings.Builder
	if subtitle := subtitleOf(alert); subtitle != "" && subtitle != oneLineText(alert.Title) {
		content.WriteString(`<p style="margin:0 0 16px; font-size:15px; line-height:1.5; color:#374151;">`)
		content.WriteString(utils.EscapeEmailHTML(subtitle))
		content.WriteString(`</p>`)
	}
	if len(alert.Fields) > 0 {
		content.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:collapse; margin:0 0 20px;">`)
		for _, field := range alert.Fields {
			if strings.TrimSpace(field.Value) == "" {
				continue
			}
			content.WriteString(fmt.Sprintf(`<tr>
    <td style="padding:10px 12px 10px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:150px; vertical-align:top;">%s</td>
    <td style="padding:10px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; white-space:pre-wrap; word-break:break-word;">%s</td>
  </tr>`, utils.EscapeEmailHTML(field.Label), utils.EscapeEmailHTML(field.Value)))
		}
		content.WriteString(`</table>`)
	}
	if body := strings.TrimSpace(alert.Body); body != "" {
		content.WriteString(`<p style="margin:0 0 8px; font-size:12px; font-weight:600; letter-spacing:0.04em; text-transform:uppercase; color:#6b7280;">Message</p>`)
		content.WriteString(`<div style="margin:0 0 20px; padding:16px; border-radius:12px; background:#f9fafb; border:1px solid #e5e7eb; font-size:14px; line-height:1.6; color:#111827; white-space:pre-wrap; word-break:break-word;">`)
		content.WriteString(utils.EscapeEmailHTML(body))
		content.WriteString(`</div>`)
	}
	if alert.ReplyTo != "" {
		content.WriteString(`<p style="margin:0; font-size:13px; color:#374151;">Reply to this email to answer them.</p>`)
	}

	names := make([]string, 0, len(teams))
	for _, team := range teams {
		names = append(names, team.Name)
	}
	footer := "SFLuv notification hub"
	if len(names) > 0 {
		footer = "Sent to " + strings.Join(names, ", ") + " · change where this goes in the Notification Hub"
	}
	return utils.BuildStyledEmailWithFooter(alert.Title, topic.Label, content.String(), footer)
}

// hubChatMessage is what a team chat gets, one thing per line: the topic, then
// either the caller's own chat text (for what is safe to show everybody in a
// chat) or the headline, each detail, and the start of the message.
func hubChatMessage(topic structs.NotificationTopicDefinition, alert Alert) string {
	var out strings.Builder
	out.WriteString(topic.Label)
	if line := strings.TrimSpace(alert.ChatLine); line != "" {
		out.WriteString("\n")
		out.WriteString(line)
		return out.String()
	}
	if title := oneLineText(alert.Title); title != "" {
		out.WriteString("\n")
		out.WriteString(title)
	}
	for _, field := range alert.Fields {
		if value := oneLineText(field.Value); value != "" {
			out.WriteString("\n")
			out.WriteString(field.Label + ": " + value)
		}
	}
	if body := strings.TrimSpace(alert.Body); body != "" {
		out.WriteString("\n\n")
		out.WriteString(truncateRunes(body, supportChatPreviewRunes))
	}
	return out.String()
}

// Resolve retracts an alert by key, for a condition that cleared on its own.
func (n *NotificationService) Resolve(ctx context.Context, key string, outcome string) error {
	if n == nil {
		return nil
	}
	orgId, err := n.houseOrganization(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNoHouseOrganization) {
			return nil
		}
		return err
	}
	id, err := n.db.LiveInternalAlertId(ctx, orgId, key)
	if err != nil || id == 0 {
		return err
	}
	resolved, err := n.db.ResolveInternalAlert(ctx, orgId, key, structs.ResolutionCleared, "", outcome)
	if err != nil {
		return err
	}
	if resolved {
		n.resolved(ctx, id, key)
	}
	return nil
}

// resolved tells everybody a notification went to that it is done.
func (n *NotificationService) resolved(ctx context.Context, id int64, key string) {
	if n.announceResolved == nil {
		return
	}
	recipients, err := n.db.InternalAlertRecipients(ctx, id)
	if err != nil {
		n.logf("notifications: could not load who %s went to: %s", key, err)
		return
	}
	n.announceResolved(ctx, id, key, recipients)
}

// LiveKeys lists the unresolved alert keys under a prefix, for a sweep that
// reconciles its full set of conditions each run.
func (n *NotificationService) LiveKeys(ctx context.Context, prefix string) ([]string, error) {
	if n == nil {
		return nil, nil
	}
	orgId, err := n.houseOrganization(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNoHouseOrganization) {
			return nil, nil
		}
		return nil, err
	}
	return n.db.LiveInternalAlertKeys(ctx, orgId, prefix)
}

// Live loads one live alert, for a caller that needs to know what it already
// said — the uptime poller rewrites its own alert as an outage goes on.
func (n *NotificationService) Live(ctx context.Context, key string) (*db.InternalAlertRow, error) {
	if n == nil {
		return nil, nil
	}
	orgId, err := n.houseOrganization(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNoHouseOrganization) {
			return nil, nil
		}
		return nil, err
	}
	return n.db.LiveInternalAlert(ctx, orgId, key)
}

// StartHousekeeping prunes the ledger on a slow timer.
//
// Leader-only, like the other jobs: the work is idempotent, so a second runner
// would be harmless rather than wrong, but there is no reason to pay for it
// twice.
func (n *NotificationService) StartHousekeeping(ctx context.Context) {
	if n == nil {
		return
	}
	go func() {
		// Once on boot, then daily. The first run is what clears a backlog
		// after a deploy that was down for a while.
		timer := time.NewTimer(5 * time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			if removed, err := n.db.PruneNotificationHistory(ctx); err != nil {
				n.logf("notifications: housekeeping failed: %s", err)
			} else if removed > 0 {
				n.logf("notifications: pruned %d expired notification rows", removed)
			}
			timer.Reset(24 * time.Hour)
		}
	}()
}

// LogConfig says, at boot, which topics reach nobody.
//
// The failure this is here to catch is a silent one: notifications being
// posted correctly and reaching nobody, which looks exactly like a system with
// nothing wrong with it. Each one is also marked in the admin panel as it
// happens; this is the place it is visible without going looking.
func (n *NotificationService) LogConfig(ctx context.Context, log *logger.LogCloser) {
	if n == nil || log == nil {
		return
	}
	orgId, err := n.houseOrganization(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNoHouseOrganization) {
			log.Logf("notifications: no house organization yet — notifications will be dropped until one exists")
			return
		}
		log.Logf("notifications: could not resolve the house organization: %s", err)
		return
	}

	routes, err := n.db.GetNotificationRoutes(ctx, orgId)
	if err != nil {
		log.Logf("notifications: could not read routes: %s", err)
		return
	}
	if len(routes[structs.NotificationTopicUnrouted]) > 0 {
		log.Logf("notifications: a catch-all team receives every topic without a team of its own")
		return
	}
	unrouted := []string{}
	for _, topic := range structs.NotificationTopicCatalog() {
		if len(routes[topic.Id]) == 0 {
			unrouted = append(unrouted, topic.Id)
		}
	}
	if len(unrouted) == 0 {
		log.Logf("notifications: every topic has a team")
		return
	}
	log.Logf("notifications: NOBODY receives %d topic(s), and there is no catch-all — they will be kept in the hub and seen by no one: %s",
		len(unrouted), strings.Join(unrouted, ", "))
}
