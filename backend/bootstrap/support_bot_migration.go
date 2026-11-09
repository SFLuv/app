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

// migrateSupportBotProfile is the 1.80 migration body: a name, so support
// notifications read as ordinary messages from SFLuv Support. Not discoverable,
// so nobody can find the account to start a conversation with it.
func migrateSupportBotProfile(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		UPDATE users SET contact_name = $2 WHERE id = $1;
	`, structs.SupportBotUserId, structs.SupportBotName); err != nil {
		return fmt.Errorf("error naming the support bot user: %w", err)
	}
	if _, err := pools.App.Exec(ctx, `
		INSERT INTO user_profiles(user_id, public_name, discoverable)
		VALUES ($1, $2, FALSE)
		ON CONFLICT (user_id) DO UPDATE SET public_name = EXCLUDED.public_name;
	`, structs.SupportBotUserId, structs.SupportBotName); err != nil {
		return fmt.Errorf("error adding the support bot profile: %w", err)
	}
	return nil
}
