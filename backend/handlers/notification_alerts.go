package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
)

// Role requests, posted to the admin set.
//
// These sit alongside the emails that already go out rather than replacing
// them. The emails are what reach somebody at a desk; the alert is what can be
// decided from a phone, and what stops asking the moment one person decides it.
//
// The alert is a condition — "this request is waiting on a decision" — so the
// key is the subject rather than the moment, and a duplicate request cannot
// produce a second alert for the same pending thing.

// roleRequestAlert is one request waiting on a decision.
type roleRequestAlert struct {
	// Role is "affiliate", "improver" or "merchant", and selects which code
	// the approve button runs.
	Role string
	// Subject is the id the decision is applied to: a user id for affiliate
	// and improver, a location id for a merchant.
	Subject string
	// Label is who this is, in words — the name shown on the alert and in the
	// history line after a decision.
	Label string
	// Fields are the details somebody needs to decide without opening the
	// admin panel.
	Fields []structs.NotificationField
}

// roleRequestTitles are the human names for each role.
var roleRequestTitles = map[string]string{
	"affiliate": "Affiliate request",
	"improver":  "Improver request",
	"merchant":  "Merchant approval",
}

// postRoleRequestAlert puts a pending request in front of the admin set.
//
// Failures are logged and swallowed by every caller: a request that was
// recorded but not announced is recoverable from the admin panel, and failing
// the submission because an alert could not be posted would turn our problem
// into the applicant's.
func (a *AppService) postRoleRequestAlert(ctx context.Context, alert roleRequestAlert) {
	if a.notifications == nil {
		return
	}

	title := roleRequestTitles[alert.Role]
	if title == "" {
		title = strings.ToUpper(alert.Role[:1]) + alert.Role[1:] + " request"
	}

	label := strings.TrimSpace(alert.Label)
	if label == "" {
		label = alert.Subject
	}

	fields := append([]structs.NotificationField{{Label: "Who", Value: label}}, alert.Fields...)

	if err := a.notifications.Post(ctx, Alert{
		Set:    structs.NotificationSetAdmin,
		Key:    fmt.Sprintf("role:%s:%s", alert.Role, alert.Subject),
		Kind:   structs.InternalKindRoleRequest,
		Title:  title,
		Body:   fmt.Sprintf("%s is waiting on a decision.", label),
		Fields: fields,
		Payload: map[string]string{
			"role":    alert.Role,
			"subject": alert.Subject,
			"label":   label,
		},
		// Not urgent. Somebody waiting on a decision is not the same as
		// something being broken, and sorting these above an outage would make
		// the top of the list meaningless.
		Urgent: false,
	}); err != nil {
		a.logger.Logf("error posting the %s request alert for %s: %s", alert.Role, alert.Subject, err)
	}
}

// resolveRoleRequestAlert retracts a pending-request alert when the decision
// was made somewhere else.
//
// Called from the admin panel's own endpoints, so approving in the browser
// clears the alert from everybody's phone. Without this the two surfaces
// disagree: the request is decided and the alert still asks about it.
func (a *AppService) resolveRoleRequestAlert(ctx context.Context, role string, subject string, outcome string) {
	if a.notifications == nil {
		return
	}
	key := fmt.Sprintf("role:%s:%s", role, subject)
	if err := a.notifications.Resolve(ctx, key, outcome); err != nil {
		a.logger.Logf("error retracting the %s request alert for %s: %s", role, subject, err)
	}
}
