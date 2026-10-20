package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
)

// What an alert's buttons do.
//
// Buttons are a property of the alert's kind rather than rows stored beside it.
// A press names the notification and the action id; the server loads the alert,
// looks the action up against the alert's own kind, and ignores everything else
// the client sent. So there is nothing to forge: an action id that does not
// belong to that kind is not found, and the payload the action works from is
// the one the server wrote when it raised the alert.
//
// That is a real simplification over the Telegram version this replaces, which
// had to mint a server-side token per button and store it in its own table —
// not for authorization's sake but because callback_data was capped at 64
// bytes and arrived from the open internet. Neither is true of an authenticated
// request from our own app.
//
// Two separate questions are asked before anything runs, and conflating them
// was the thing most worth avoiding here:
//
//   - May this person SEE the alert? Yes if they are designated for its set.
//   - May this person ACT on it? For anything that changes the state of the
//     platform, only if they already hold the role that endpoint requires.
//
// Being added to "admin alerts" is a decision about what somebody wants to
// read. It is not a grant of the power to approve merchants, and the buttons
// for that are simply absent for a recipient who is not an admin.

// notificationPress is one button press, after the alert has been loaded.
type notificationPress struct {
	Alert *db.InternalAlertRow
	// UserId is the presser.
	UserId string
	// Actor is how they are recorded in the outcome line.
	Actor string
}

func (p notificationPress) payload(key string) string {
	if p.Alert == nil {
		return ""
	}
	return p.Alert.Payload[key]
}

// notificationActionSpec is one button.
type notificationActionSpec struct {
	Id    string
	Label string
	Style string
	// Confirm, when set, is the question to ask before running.
	Confirm string
	// RequiresAdmin gates the ones that change the platform. See the note
	// above: subscription decides delivery, this decides authority.
	RequiresAdmin bool
	// Run performs the action and returns the sentence the history row keeps.
	Run func(ctx context.Context, a *AppService, press notificationPress) (outcome string, err error)
}

// notificationActions maps an alert kind to its buttons, in the order they are
// shown.
//
// A kind with no entry gets no buttons, which is the right rendering for an
// alert that retires itself: there is nothing a person can do about a low gas
// balance from a phone except put gas in the account, and a button that only
// hid the alert would make an unfixed problem look handled.
var notificationActions = map[string][]notificationActionSpec{
	structs.InternalKindRoleRequest: {
		{
			Id:            "approve",
			Label:         "Approve",
			Style:         structs.NotificationActionStylePrimary,
			RequiresAdmin: true,
			Run:           runRoleDecisionAction(true),
		},
		{
			Id:            "reject",
			Label:         "Reject",
			Style:         structs.NotificationActionStyleDanger,
			Confirm:       "Reject this request?",
			RequiresAdmin: true,
			Run:           runRoleDecisionAction(false),
		},
	},
	structs.InternalKindUptime: {
		{
			Id:    "ack",
			Label: "Acknowledge",
			Style: structs.NotificationActionStylePrimary,
			Run:   runUptimeAckAction,
		},
	},
	structs.InternalKindSupportRequest: {
		{
			Id:    "archive",
			Label: "Archive",
			Style: structs.NotificationActionStyleDefault,
			Run:   runSupportArchiveAction,
		},
	},
}

// actionsForAlert is the button list a particular person should see on a
// particular alert.
//
// Resolved alerts carry no buttons: they are history, and the thing they were
// asking about has been settled. A reply-by-email link is added for support
// requests, which is a link the device opens rather than a call back to us —
// the one case the Telegram version needed a server-side redirect for, because
// a chat client would not accept a mailto in a button.
func actionsForAlert(alert *db.InternalAlertRow, resolved bool, isAdmin bool) []structs.NotificationActionButton {
	if alert == nil || resolved {
		return nil
	}

	out := []structs.NotificationActionButton{}

	if alert.Kind == structs.InternalKindSupportRequest {
		if email := strings.TrimSpace(alert.Payload["email"]); email != "" {
			out = append(out, structs.NotificationActionButton{
				Id:    "respond",
				Label: "Reply by email",
				Style: structs.NotificationActionStylePrimary,
				URL:   supportReplyMailto(alert),
			})
		}
	}

	for _, spec := range notificationActions[alert.Kind] {
		if spec.RequiresAdmin && !isAdmin {
			continue
		}
		out = append(out, structs.NotificationActionButton{
			Id:      spec.Id,
			Label:   spec.Label,
			Style:   spec.Style,
			Confirm: spec.Confirm,
		})
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// findNotificationAction resolves an action id against an alert's kind.
func findNotificationAction(kind string, id string) (notificationActionSpec, bool) {
	for _, spec := range notificationActions[kind] {
		if spec.Id == id {
			return spec, true
		}
	}
	return notificationActionSpec{}, false
}

// ── Role requests ────────────────────────────────────────────────────────────

// runRoleDecisionAction approves or rejects a role request.
//
// It calls the same code the admin panel calls, which for a merchant means
// deriving wallets and granting redeemer rights. A second implementation of
// that behind a notification button was never acceptable, which is why
// applyLocationApproval was extracted in the first place.
func runRoleDecisionAction(approve bool) func(context.Context, *AppService, notificationPress) (string, error) {
	return func(ctx context.Context, a *AppService, press notificationPress) (string, error) {
		role := press.payload("role")
		subject := press.payload("subject")
		label := press.payload("label")
		if label == "" {
			label = subject
		}

		status := "rejected"
		if approve {
			status = "approved"
		}

		switch role {
		case "affiliate":
			if _, err := a.db.UpdateAffiliate(ctx, &structs.AffiliateUpdateRequest{
				UserId: subject,
				Status: &status,
			}); err != nil {
				return "", fmt.Errorf("updating affiliate %s: %w", subject, err)
			}

		case "improver":
			if _, err := a.db.UpdateImprover(ctx, &structs.ImproverUpdateRequest{
				UserId: subject,
				Status: &status,
			}); err != nil {
				return "", fmt.Errorf("updating improver %s: %w", subject, err)
			}

		case "merchant":
			id, err := strconv.ParseUint(subject, 10, 64)
			if err != nil {
				return "", fmt.Errorf("merchant location id %q: %w", subject, err)
			}
			// Retryable outcomes are reported as refusals rather than
			// failures, because pressing again is the right move: the alert
			// stays live and the button stays usable.
			switch a.applyLocationApproval(ctx, structs.UpdateLocationApprovalRequest{Id: uint(id), Approval: &approve}) {
			case locationApprovalChainUnavailable:
				return "", errRetryable("Could not reach the chain to create this location's wallets. Try again in a moment.")
			case locationApprovalWalletMoved:
				return "", errRetryable("Another of this merchant's locations claimed that wallet. Try again.")
			case locationApprovalFailed:
				return "", errors.New("location approval failed")
			}

		default:
			return "", fmt.Errorf("unknown role %q", role)
		}

		verb := "Approved"
		if !approve {
			verb = "Rejected"
		}
		return fmt.Sprintf("%s by %s", verb, press.Actor), nil
	}
}

// ── Uptime ───────────────────────────────────────────────────────────────────

// runUptimeAckAction acknowledges an outage, and refuses while it is ongoing.
//
// That refusal is the point of the button. Acknowledging a live problem would
// clear the one indicator that it is happening, so the check is against the
// poller's current reading rather than against anybody's belief that it is
// fixed. A new outage after an acknowledged one raises a fresh alert, because
// the acknowledged one is resolved and the uniqueness rule only covers live
// rows.
func runUptimeAckAction(ctx context.Context, a *AppService, press notificationPress) (string, error) {
	if a.uptime == nil {
		return "", errors.New("uptime monitoring is not running")
	}
	state := a.uptime.State()
	if !state.Healthy {
		return "", errRetryable("Still down — " + state.Detail + ". The alert stays until it recovers.")
	}
	return fmt.Sprintf("Down for %s. Acknowledged by %s.", state.LastOutageFor(), press.Actor), nil
}

// ── Support requests ─────────────────────────────────────────────────────────

// runSupportArchiveAction archives a website support request.
//
// Not admin-gated: archiving is inbox housekeeping rather than a change to the
// platform, and whoever was designated to receive support requests is the
// person meant to be doing it. Being already archived is a success — somebody
// got there first, in the admin panel or from their own phone, and the alert
// should go either way.
func runSupportArchiveAction(ctx context.Context, a *AppService, press notificationPress) (string, error) {
	id, err := strconv.ParseInt(press.payload("id"), 10, 64)
	if err != nil {
		return "", fmt.Errorf("support request id %q: %w", press.payload("id"), err)
	}

	request, err := a.db.SetSupportRequestArchived(ctx, id, true, press.Actor)
	if err != nil {
		return "", fmt.Errorf("archiving support request %d: %w", id, err)
	}
	if request == nil {
		return fmt.Sprintf("Already archived when %s got here", press.Actor), nil
	}
	return fmt.Sprintf("Archived by %s", press.Actor), nil
}

// supportReplyMailto builds the reply draft the device opens.
//
// The reference goes in the subject so a reply threads against the request it
// answers, and the original message is quoted so whoever is replying is not
// writing blind from a notification.
func supportReplyMailto(alert *db.InternalAlertRow) string {
	email := strings.TrimSpace(alert.Payload["email"])
	reference := strings.TrimSpace(alert.Payload["reference"])
	subject := strings.TrimSpace(alert.Payload["subject"])
	if subject == "" {
		subject = "your message to SFLuv"
	}

	replySubject := fmt.Sprintf("Re: %s", subject)
	if reference != "" {
		replySubject = fmt.Sprintf("Re: %s [%s]", subject, reference)
	}

	body := strings.Builder{}
	body.WriteString("\n\n---\n")
	if reference != "" {
		body.WriteString("Your request: " + reference + "\n")
	}
	if original := strings.TrimSpace(alert.Payload["message"]); original != "" {
		for line := range strings.SplitSeq(original, "\n") {
			body.WriteString("> " + line + "\n")
		}
	}

	return fmt.Sprintf("mailto:%s?subject=%s&body=%s",
		mailtoEscape(email), mailtoEscape(replySubject), mailtoEscape(body.String()))
}

// mailtoEscape percent-encodes a mailto component.
//
// url.QueryEscape is the right encoder except for one detail: it writes a space
// as "+", which is correct for a form body and wrong in a mailto, where mail
// clients show the plus literally. Every reply draft would open with
// "Re:+your+message".
func mailtoEscape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// ── Refusals ─────────────────────────────────────────────────────────────────

// retryableError is an outcome that is expected and not a bug: the chain was
// unreachable, the outage is still happening, somebody else got there first.
// The alert stays live and the presser is told why in a sentence.
type retryableError struct{ message string }

func (e retryableError) Error() string { return e.message }

func errRetryable(message string) error { return retryableError{message: message} }

func isRetryable(err error) bool {
	var retryable retryableError
	return errors.As(err, &retryable)
}
