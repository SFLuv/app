package handlers

import (
	"context"
	"strconv"

	"github.com/SFLuv/app/backend/realtime"
)

// AnnounceNotification sends a new notification down the socket to whoever of
// userIds has the app open, beside its push. The app draws it as an in-app
// banner (unless it is already showing where the notification points) and
// refreshes its list, rather than waiting for the next fetch.
//
// The push still goes. While the socket is live the app keeps the phone's own
// banner quiet, so the person sees one banner, and the push stays in the
// notification centre for later.
func (a *AppService) AnnounceNotification(userIds []string, title, body string, data map[string]string) {
	a.PublishNotification(userIds, title, body, data)
}

// AnnounceNotificationResolved tells everybody a team notification went to that
// it has been handled, so it disappears for all of them at once: from the
// list and any banner of an app that is open, over the socket, and from every
// phone's notification centre, by a silent push that also corrects the icon's
// count.
func (a *AppService) AnnounceNotificationResolved(ctx context.Context, internalId int64, alertKey string, userIds []string) {
	if len(userIds) == 0 {
		return
	}
	if a.realtime != nil {
		a.realtime.Publish(userIds, realtime.Event{
			Kind: realtime.EventNotificationResolved,
			Data: map[string]any{"internal_id": internalId, "alert_key": alertKey},
		})
	}
	dismiss := map[string]string{
		"dismiss_alert_key":   alertKey,
		"dismiss_internal_id": strconv.FormatInt(internalId, 10),
	}
	for _, userId := range userIds {
		a.syncBadgeDismissing(userId, dismiss)
	}
}

// announceResolvedById looks up who a resolved notification went to and tells
// them. For the paths that resolve by id.
func (a *AppService) announceResolvedById(ctx context.Context, internalId int64, alertKey string) {
	recipients, err := a.db.InternalAlertRecipients(ctx, internalId)
	if err != nil {
		a.logger.Logf("notifications: could not load who %s went to: %s", alertKey, err)
		return
	}
	a.AnnounceNotificationResolved(ctx, internalId, alertKey, recipients)
}
