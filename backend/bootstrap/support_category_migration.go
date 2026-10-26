package bootstrap

import (
	"context"
	"fmt"
)

// migrateSupportCategories is the 1.70 migration body: routing a support
// request by what it is about, not only to everyone who takes support
// requests.
//
// Two changes, and the second one's default is the whole design.
//
// internal_notifications gains support_category, so the delivery query can
// filter on it. It is nullable and only ever set for support requests: every
// other alert kind leaves it null and is unaffected.
//
// support_category_subscriptions says who wants which categories. The rule the
// delivery query implements is that **no rows means every category** — so the
// people already receiving support requests keep receiving all of them, and
// nothing changes for anybody until they choose to narrow it. The alternative,
// treating an empty set as "nothing", would silently stop delivering support
// requests to every existing recipient on the deploy that ran this, which is
// the one outcome a routing change must not have.
//
// Narrowing is per-person rather than per-organization because that is what
// the ask is: the merchant onboarding lead wants merchant onboarding, and the
// person on technical support does not want press enquiries. It is scoped to
// the organization as well as the user, matching organization_notification_
// subscriptions, because a subscription is already a fact about a membership
// rather than about a person.
func migrateSupportCategories(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		-- Null for every alert that is not a support request, which is all of
		-- them except the one kind.
		ALTER TABLE internal_notifications
			ADD COLUMN IF NOT EXISTS support_category TEXT;

		-- Kept on the request as well as on the alert, so the admin panel can
		-- show what a request was about after its alert has been resolved.
		ALTER TABLE support_requests
			ADD COLUMN IF NOT EXISTS category TEXT;

		CREATE TABLE IF NOT EXISTS support_category_subscriptions(
			organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			category TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (organization_id, user_id, category)
		);

		-- The delivery query asks "does this person want this category", for
		-- the handful of people subscribed to the support set.
		CREATE INDEX IF NOT EXISTS support_category_subscriptions_user_idx
			ON support_category_subscriptions(organization_id, user_id);
	`); err != nil {
		return fmt.Errorf("error creating support category routing: %w", err)
	}

	return nil
}
