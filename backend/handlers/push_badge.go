package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

// ── The number on the app's icon ────────────────────────────────────────────
//
// iOS draws no count on an app's icon unless something sets one, and nothing
// did: every push went out without a badge, and the app never set one either.
// So the icon stayed bare however much was waiting.
//
// The number is what is waiting: every unread message, plus everything the
// bell counts. Each push carries it as it stands once that push's own message
// or notification has been stored, so the icon is right the moment the push
// lands. The app sets it again whenever it is open and the counts move.

// pushBadge is the icon's number for userId, or nil when either half cannot be
// read. Nil leaves the icon as it is: an old number is better than a wrong one.
func (a *AppService) pushBadge(ctx context.Context, userId string) *int {
	messages := 0
	// Somebody outside the chat beta cannot open their conversations, so
	// counting them would put up a number nothing can clear.
	if a.BetaFeatureAllowed(ctx, userId, structs.BetaFeatureChat) {
		n, err := a.db.CountUnreadMessages(ctx, userId)
		if err != nil {
			a.logger.Logf("error counting unread messages for %s's badge: %s", userId, err)
			return nil
		}
		messages = n
	}
	notifications, err := a.db.CountOutstandingNotifications(ctx, userId)
	if err != nil {
		a.logger.Logf("error counting notifications for %s's badge: %s", userId, err)
		return nil
	}
	total := messages + notifications
	return &total
}

// pushTokensFor is each device userId can be pushed to, once per token.
func (a *AppService) pushTokensFor(ctx context.Context, userId string) []string {
	subscriptions, err := a.db.GetMobilePushSubscriptionsByUser(ctx, userId)
	if err != nil {
		a.logger.Logf("error loading push subscriptions for %s: %s", userId, err)
		return nil
	}
	tokens := []string{}
	seen := map[string]struct{}{}
	for _, s := range subscriptions {
		if s == nil || !s.Active || !s.DeviceRegistered {
			continue
		}
		token := strings.TrimSpace(s.Token)
		if token == "" {
			continue
		}
		if _, dup := seen[token]; dup {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	return tokens
}

// syncBadge sends userId's devices their icon number and nothing else — no
// banner, no sound — for a change the phone did not see happen. A reply sent
// from a notification is the case: it reads the thread, and the app may never
// have run to notice.
//
// Queued behind that person's message pushes, so it cannot overtake one and
// leave the icon on a number from before it.
func (a *AppService) syncBadge(userId string) {
	a.pushes.enqueue(userId, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		badge := a.pushBadge(ctx, userId)
		if badge == nil {
			return
		}
		for _, token := range a.pushTokensFor(ctx, userId) {
			if _, err := sendExpoPushNotificationWith(ctx, token, "", "", map[string]string{"type": "badge"}, expoPushOptions{Badge: badge}); err != nil {
				a.logger.Logf("error syncing %s's badge: %s", userId, err)
			}
		}
	})
}
