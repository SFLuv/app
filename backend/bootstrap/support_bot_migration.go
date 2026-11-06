package bootstrap

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

// migrateSupportBotUser is the 1.79 migration body: the sender of the lines
// that say a support request came in.
//
// A row because messages.sender_id is a required reference to users. Using a
// staff member as the sender would not do: the sender of a message is the one
// person its push skips, so whoever it was would never hear about anything.
//
// Inactive, so it is not offered anywhere a person would be. It has no way to
// sign in, and posting does not go through anything that checks.
func migrateSupportBotUser(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		INSERT INTO users(id, contact_name, active)
		VALUES ($1, 'SFLuv', false)
		ON CONFLICT (id) DO NOTHING;
	`, structs.SupportBotUserId); err != nil {
		return fmt.Errorf("error adding the support bot user: %w", err)
	}
	return nil
}
