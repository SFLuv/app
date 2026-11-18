package handlers

import (
	"context"
	"errors"
	"fmt"
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

	// houseMu guards the cached house organization id. The lookup is a query
	// per alert otherwise, and alerts are posted from a once-a-minute sweep.
	houseMu sync.Mutex
	houseId int64
}

// NewNotificationService builds the alert layer.
func NewNotificationService(appDb *db.AppDB, log *logger.LogCloser) *NotificationService {
	return &NotificationService{db: appDb, logger: log}
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
	Kind  string
	Title string
	Body  string
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
		Body:           alert.Body,
		Fields:         alert.Fields,
		Payload:        alert.Payload,
		Urgent:         alert.Urgent,
	})
	if err != nil {
		return err
	}

	plan, err := n.db.NotificationDeliveryPlan(ctx, orgId, topic.Id)
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
	if n.push != nil && len(recipients) > 0 {
		data := map[string]string{
			"type":             "internal_" + alert.Kind,
			"notification_set": topic.Set,
			"topic":            topic.Id,
			"alert_key":        alert.Key,
		}
		for _, userId := range recipients {
			if !plan.Push[userId] {
				continue
			}
			n.push(ctx, userId, alert.Title, alert.Body, data)
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
				if err := sender.SendEmailWithReplyTo(address, "", alert.Title, html,
					utils.NotificationFromEmail(), "SFLuv notifications", alert.ReplyTo); err != nil {
					delivery.Failures = append(delivery.Failures, "email to "+address+": "+err.Error())
					continue
				}
				delivery.Emailed = append(delivery.Emailed, address)
			}
		}
	}

	if len(plan.Chats) > 0 {
		line := alert.ChatLine
		if strings.TrimSpace(line) == "" {
			line = hubChatLine(alert)
		}
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

// hubEmailHTML is the email a team address or member receives.
func hubEmailHTML(topic structs.NotificationTopicDefinition, alert Alert, teams []structs.NotificationDeliveryTeam) string {
	var content strings.Builder
	if body := strings.TrimSpace(alert.Body); body != "" {
		content.WriteString(`<p style="margin:0 0 18px; font-size:14px; line-height:1.55; color:#111827; white-space:pre-wrap;">`)
		content.WriteString(utils.EscapeEmailHTML(body))
		content.WriteString(`</p>`)
	}
	if len(alert.Fields) > 0 {
		content.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">`)
		for _, field := range alert.Fields {
			if strings.TrimSpace(field.Value) == "" {
				continue
			}
			content.WriteString(fmt.Sprintf(`<tr>
    <td style="padding:10px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:150px; vertical-align:top;">%s</td>
    <td style="padding:10px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; white-space:pre-wrap; word-break:break-word;">%s</td>
  </tr>`, utils.EscapeEmailHTML(field.Label), utils.EscapeEmailHTML(field.Value)))
		}
		content.WriteString(`</table>`)
	}
	if alert.ReplyTo != "" {
		content.WriteString(`<p style="margin:18px 0 0; font-size:13px; color:#374151;">Reply to this email to answer them.</p>`)
	}

	names := make([]string, 0, len(teams))
	for _, team := range teams {
		names = append(names, team.Name)
	}
	footer := "SFLuv notification hub"
	if len(names) > 0 {
		footer = "Sent to " + strings.Join(names, ", ") + " · change who gets this on the Organization page"
	}
	return utils.BuildStyledEmailWithFooter(alert.Title, topic.Label, content.String(), footer)
}

// hubChatLine is the generic line a team chat gets: the title, the details on
// one line, and the start of the body.
func hubChatLine(alert Alert) string {
	parts := []string{strings.TrimSpace(alert.Title)}
	for _, field := range alert.Fields {
		if value := oneLineText(field.Value); value != "" {
			parts = append(parts, field.Label+": "+value)
		}
	}
	line := strings.Join(parts, " · ")
	if body := oneLineText(alert.Body); body != "" {
		line += "\n" + truncateRunes(body, supportChatPreviewRunes)
	}
	return line
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
	_, err = n.db.ResolveInternalAlert(ctx, orgId, key, structs.ResolutionCleared, "", outcome)
	return err
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
