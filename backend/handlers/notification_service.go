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

// Alert is one condition to put on the board.
type Alert struct {
	// Set is which group of people this is for.
	Set string
	// Key is stable for the condition. Two posts with the same key are the
	// same alert, however far apart.
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
	// SupportCategory narrows who receives this, and is only meaningful for
	// support requests. Empty means uncategorised, which reaches everybody on
	// the set — the behaviour before categories existed.
	SupportCategory string
}

// Post puts an alert on the board and delivers it to the set's subscribers.
//
// Safe to call on a timer with the same alert forever. The first call creates
// and pushes; later calls refresh the text in place and push nothing, so a
// condition that has held for a day has buzzed one phone once.
func (n *NotificationService) Post(ctx context.Context, alert Alert) error {
	if n == nil {
		return nil
	}
	if !structs.IsNotificationSet(alert.Set) {
		return fmt.Errorf("unknown notification set %q", alert.Set)
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
		OrganizationId:  orgId,
		Set:             alert.Set,
		AlertKey:        alert.Key,
		Kind:            alert.Kind,
		Title:           alert.Title,
		Body:            alert.Body,
		Fields:          alert.Fields,
		Payload:         alert.Payload,
		Urgent:          alert.Urgent,
		SupportCategory: alert.SupportCategory,
	})
	if err != nil {
		return err
	}

	recipients, err := n.db.DeliverInternalAlert(ctx, id)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		if created {
			// Worth a line: an alert with no recipients is indistinguishable
			// from a working system from the outside, and this is the only
			// place that difference is visible.
			n.logf("notifications: %s posted with no recipients — nobody receives the %q set", alert.Key, alert.Set)
		}
		return nil
	}

	// Only a new alert buzzes. A refreshed one has already been delivered to
	// everybody who was subscribed when it was raised, and DeliverInternalAlert
	// returns only people who had not received it, so a member added to a set
	// mid-condition still gets their one push.
	if n.push != nil {
		data := map[string]string{
			"type":             "internal_" + alert.Kind,
			"notification_set": alert.Set,
			"alert_key":        alert.Key,
		}
		for _, userId := range recipients {
			n.push(ctx, userId, alert.Title, alert.Body, data)
		}
	}

	return nil
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

// LogConfig says who receives what at boot.
//
// The failure this is here to catch is a silent one: alerts being posted
// correctly and reaching nobody, which looks exactly like a system with
// nothing wrong with it. Naming the empty sets at startup is the only place
// that is visible without going looking.
func (n *NotificationService) LogConfig(ctx context.Context, log *logger.LogCloser) {
	if n == nil || log == nil {
		return
	}
	orgId, err := n.houseOrganization(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNoHouseOrganization) {
			log.Logf("notifications: no house organization yet — internal alerts will be dropped until one exists")
			return
		}
		log.Logf("notifications: could not resolve the house organization: %s", err)
		return
	}

	subscriptions, err := n.db.GetOrganizationNotificationSubscriptions(ctx, orgId)
	if err != nil {
		log.Logf("notifications: could not read subscriptions: %s", err)
		return
	}

	counts := map[string]int{}
	for _, sets := range subscriptions {
		for _, set := range sets {
			counts[set]++
		}
	}
	for _, set := range structs.NotificationSetCatalog() {
		if counts[set.Id] == 0 {
			log.Logf("notifications: NOBODY receives %q (%s) — these alerts will be posted and seen by no one", set.Id, set.Label)
			continue
		}
		log.Logf("notifications: %q goes to %d member(s)", set.Id, counts[set.Id])
	}
}
