package handlers

import (
	"context"
	"strings"
)

// Device delivery, and the history row that outlives it.
//
// Before this, a notification existed in exactly two places: the OS tray, and
// a fifty-item list in AsyncStorage on whichever phone received it. Reinstall
// the app and the history was gone; sign in on a second device and it had
// never existed. So every notification now also gets a row in the user's
// ledger, which is what the notification screen reads and paginates.
//
// The two are deliberately independent. A push that fails to send still leaves
// the history row, because the notification is still true and the user should
// find it when they next open the screen. A history row that fails to write
// still sends the push, because telling somebody now matters more than being
// able to show it to them later.

// recordNotification writes one history row, logging rather than returning a
// failure.
//
// Every caller is on the notification-sending path, where there is nothing
// useful to do about a failed insert and refusing to send would be worse than
// losing the row.
func (a *AppService) recordNotification(
	ctx context.Context, userId string, kind string, title string, body string, data map[string]string,
) {
	if strings.TrimSpace(userId) == "" {
		return
	}
	if _, err := a.db.RecordUserNotification(ctx, userId, kind, title, body, data); err != nil {
		a.logger.Logf("error recording a %s notification for %s: %s", kind, userId, err)
	}
}

// PushToUser sends to every registered device a user has, and is the fan-out
// the alert layer is given.
//
// Deduplicated by token because a user can have the same token under more than
// one subscription row — one per watched address — and the point of a push is
// that the phone buzzes once.
//
// It does not write a history row. Internal alerts get theirs when the alert is
// delivered to its recipients, which is a different and earlier moment, and
// writing one here as well would show every alert twice.
func (a *AppService) PushToUser(
	ctx context.Context, userId string, title string, body string, data map[string]string,
) {
	subscriptions, err := a.db.GetMobilePushSubscriptionsByUser(ctx, userId)
	if err != nil {
		a.logger.Logf("error loading push subscriptions for %s: %s", userId, err)
		return
	}

	seen := map[string]struct{}{}
	for _, subscription := range subscriptions {
		if subscription == nil || !subscription.Active || !subscription.DeviceRegistered {
			continue
		}
		token := strings.TrimSpace(subscription.Token)
		if token == "" {
			continue
		}
		if _, already := seen[token]; already {
			continue
		}
		seen[token] = struct{}{}

		if _, err := sendExpoPushNotification(ctx, token, title, body, data); err != nil {
			a.logger.Logf("error pushing to %s: %s", userId, err)
		}
	}
}
