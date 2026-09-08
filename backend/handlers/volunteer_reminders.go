package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// volunteerReminderLookahead bounds the window the sweep scans. It must exceed
// the largest configurable lead time (168h) so a user asking for a week's
// notice is still found.
const volunteerReminderLookahead = 8 * 24 * time.Hour

// SendDueVolunteerReminders pushes reminders for events a user has signed up
// for, at their configured lead time.
//
// Matching runs on the signup's user_id or a VERIFIED email, never an account's
// unverified contact email: matching unverified addresses would let anyone type
// a stranger's email into a public form and make that stranger's phone buzz
// with a genuine-looking SFLuv push. The cost is that unverified users get no
// reminder, which is fixable in-app and a far better failure mode.
//
// Deduplication is a primary key on (user_id, event_id), claimed BEFORE the push
// is sent, so a crash mid-sweep or two overlapping sweeps cannot double-notify.
func (a *AppService) SendDueVolunteerReminders(ctx context.Context) {
	if a.bot == nil || a.bot.db == nil {
		return
	}

	now := time.Now()
	signups, err := a.bot.db.GetUpcomingVolunteerSignups(ctx, now.Unix(), now.Add(volunteerReminderLookahead).Unix())
	if err != nil {
		a.logger.Logf("error loading upcoming volunteer signups for reminders: %s", err)
		return
	}
	if len(signups) == 0 {
		return
	}

	// Resolve every distinct signup email to the accounts that have verified it.
	emails := make([]string, 0, len(signups))
	seen := map[string]struct{}{}
	for _, signup := range signups {
		email := strings.ToLower(strings.TrimSpace(signup.Email))
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}

	verifiedOwners, err := a.db.GetUsersMatchingSignupEmails(ctx, emails, nil)
	if err != nil {
		a.logger.Logf("error matching signup emails for reminders: %s", err)
		return
	}

	// Collapse to one candidate per (user, event): several of a user's emails
	// may match the same event, and the same user may hold both an in-app and a
	// web signup for it.
	type candidate struct {
		userId  string
		eventId string
		title   string
		startAt int64
	}
	candidates := map[string]candidate{}

	for _, signup := range signups {
		owners := []string{}
		if signup.UserId != nil && strings.TrimSpace(*signup.UserId) != "" {
			owners = append(owners, *signup.UserId)
		}
		owners = append(owners, verifiedOwners[strings.ToLower(strings.TrimSpace(signup.Email))]...)

		for _, owner := range owners {
			key := owner + "\x00" + signup.EventId
			if _, exists := candidates[key]; exists {
				continue
			}
			candidates[key] = candidate{
				userId: owner, eventId: signup.EventId, title: signup.EventTitle, startAt: signup.StartAt,
			}
		}
	}

	sent := 0
	for _, item := range candidates {
		if ctx.Err() != nil {
			return
		}

		enabled, hoursBefore, err := a.db.GetVolunteerReminderPreference(ctx, item.userId)
		if err != nil || !enabled {
			continue
		}

		// Fire only once the event is inside the user's chosen lead time.
		dueAt := item.startAt - int64(hoursBefore)*3600
		if now.Unix() < dueAt {
			continue
		}

		// Claim before sending: a claim that is rolled back by a crash is a
		// missed reminder, whereas sending before claiming risks a duplicate.
		claimed, err := a.db.ClaimVolunteerReminderSend(ctx, item.userId, item.eventId)
		if err != nil {
			a.logger.Logf("error claiming volunteer reminder for %s/%s: %s", item.userId, item.eventId, err)
			continue
		}
		if !claimed {
			continue
		}

		if a.sendVolunteerReminderPush(ctx, item.userId, item.eventId, item.title, item.startAt) {
			sent++
		}
	}

	if sent > 0 {
		a.logger.Logf("sent %d volunteer event reminder(s)", sent)
	}
}

// sendVolunteerReminderPush delivers to every active registered device for the
// user. It rides the tokens the app already registers — a reminder for a user
// with no registered device is correctly a no-op rather than an error.
func (a *AppService) sendVolunteerReminderPush(ctx context.Context, userId string, eventId string, title string, startAt int64) bool {
	subscriptions, err := a.db.GetMobilePushSubscriptionsByUser(ctx, userId)
	if err != nil {
		a.logger.Logf("error loading push subscriptions for %s: %s", userId, err)
		return false
	}

	body := fmt.Sprintf("%s starts %s.", title, humanizeLeadTime(time.Until(time.Unix(startAt, 0))))

	delivered := false
	tokensSeen := map[string]struct{}{}
	for _, subscription := range subscriptions {
		if subscription == nil || !subscription.Active || !subscription.DeviceRegistered {
			continue
		}
		token := strings.TrimSpace(subscription.Token)
		if token == "" {
			continue
		}
		// One push per device, not per subscription row.
		if _, ok := tokensSeen[token]; ok {
			continue
		}
		tokensSeen[token] = struct{}{}

		// event_id lets the app deep-link straight to the event detail screen.
		if _, err := sendExpoPushNotification(ctx, token, "Volunteer event reminder", body, map[string]string{
			"type":     "volunteer_event_reminder",
			"event_id": eventId,
		}); err != nil {
			a.logger.Logf("error sending volunteer reminder push to %s: %s", userId, err)
			continue
		}
		delivered = true
	}

	return delivered
}

// humanizeLeadTime renders the remaining time the way a notification should
// read, rather than as a raw duration.
func humanizeLeadTime(remaining time.Duration) string {
	switch {
	case remaining <= 0:
		return "now"
	case remaining < time.Hour:
		return fmt.Sprintf("in %d minutes", int(remaining.Minutes()))
	case remaining < 2*time.Hour:
		return "in about an hour"
	case remaining < 24*time.Hour:
		return fmt.Sprintf("in %d hours", int(remaining.Hours()))
	case remaining < 48*time.Hour:
		return "tomorrow"
	default:
		return fmt.Sprintf("in %d days", int(remaining.Hours()/24))
	}
}
