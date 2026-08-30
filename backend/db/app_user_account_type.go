package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// ErrMerchantAccountIsPermanent says an account cannot be turned back into a
// personal one. Becoming a merchant is a one-way door once a listing exists,
// because a location that is on the map, or queued to be, belongs to a merchant
// account by definition.
var ErrMerchantAccountIsPermanent = errors.New("this merchant account still has locations and cannot be turned back into a personal account")

// MerchantRevertEligibility is what the settings screen needs in order to
// decide whether to offer the way back, and what to say when it does not.
type MerchantRevertEligibility struct {
	AccountType string `json:"account_type"`
	// ApprovedLocations counts live listings. One is enough to close the door
	// for good — the shop is on the map and taking money.
	ApprovedLocations int `json:"approved_locations"`
	// PendingLocations counts applications still awaiting review. These do not
	// close the door permanently: cancelling them reopens it.
	PendingLocations int `json:"pending_locations"`
	// CanRevert is the single answer the screen acts on.
	CanRevert bool `json:"can_revert"`
}

// GetMerchantRevertEligibility counts what stands between this account and a
// personal one.
//
// Rejected listings are deliberately not counted. A merchant whose only
// application came back rejected has nothing on the map and nothing in the
// queue, and holding them in a merchant account over a listing that was refused
// would be punishing them for having asked.
func (a *AppDB) GetMerchantRevertEligibility(ctx context.Context, userId string) (*MerchantRevertEligibility, error) {
	result := &MerchantRevertEligibility{}

	err := a.db.QueryRow(ctx, `
		SELECT
			u.account_type,
			COALESCE((
				SELECT COUNT(*) FROM locations l
				WHERE l.owner_id = u.id AND l.active = TRUE AND l.approval = TRUE
			), 0),
			COALESCE((
				SELECT COUNT(*) FROM locations l
				WHERE l.owner_id = u.id AND l.active = TRUE AND l.approval IS NULL
			), 0)
		FROM users u
		WHERE u.id = $1
		AND u.active = TRUE;
	`, userId).Scan(&result.AccountType, &result.ApprovedLocations, &result.PendingLocations)
	if err != nil {
		return nil, err
	}

	result.CanRevert = result.AccountType == structs.AccountTypeMerchant &&
		result.ApprovedLocations == 0 &&
		result.PendingLocations == 0

	return result, nil
}

// SetOwnAccountType is the self-serve half of SetUserAccountType: the merchant
// choice made from the settings screen rather than repaired by an admin.
//
// The eligibility check and the write are one statement on purpose. Counting
// first and updating after leaves a window in which an admin approves the
// merchant's last pending listing between the two, and the account is turned
// personal underneath a shop that is now live on the map.
//
// Switching to merchant is never refused. It is the direction that costs
// nothing to undo while no listing exists, and the direction the settings screen
// has already warned about.
func (a *AppDB) SetOwnAccountType(ctx context.Context, userId string, accountType string) (*structs.AdminUserAccountTypeResponse, error) {
	if !structs.IsValidAccountType(accountType) {
		return nil, fmt.Errorf("invalid account type: %q", accountType)
	}

	row := a.db.QueryRow(ctx, `
		UPDATE
			users AS u
		SET
			account_type = $2,
			-- The person has now answered the question themselves, whichever way
			-- they answered it, so the web app stops offering them the choice.
			account_type_selected_at = COALESCE(u.account_type_selected_at, NOW()),
			web_merchant_prompt_seen_at = COALESCE(u.web_merchant_prompt_seen_at, NOW())
		FROM
			users AS prev
		WHERE
			u.id = prev.id
		AND
			u.id = $1
		AND
			u.active = TRUE
		AND (
			$2 <> $3
			OR NOT EXISTS (
				SELECT 1 FROM locations l
				WHERE l.owner_id = u.id
				AND l.active = TRUE
				AND (l.approval = TRUE OR l.approval IS NULL)
			)
		)
		RETURNING
			u.id,
			prev.account_type,
			u.account_type,
			u.merchant_onboarding_completed_at;
	`, userId, accountType, structs.AccountTypeRegular)

	result := &structs.AdminUserAccountTypeResponse{}
	if err := row.Scan(
		&result.UserId,
		&result.PreviousAccountType,
		&result.AccountType,
		&result.MerchantOnboardingCompletedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No row matched. Either the account is gone, or — far more likely —
			// the locations guard in the WHERE clause refused the revert. The
			// caller distinguishes the two by re-reading eligibility.
			return nil, ErrMerchantAccountIsPermanent
		}
		return nil, err
	}

	return result, nil
}

// MarkWebMerchantPromptSeen records that the web app has offered this account
// the merchant option, so it never offers it again.
//
// COALESCE rather than a plain assignment: the stamp answers "have they been
// asked", and moving it forward on every sign-in would lose the date they first
// were. Idempotent, because the client stamps it as it renders the prompt and
// may well render it twice before a reload.
func (a *AppDB) MarkWebMerchantPromptSeen(ctx context.Context, userId string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE users
		SET web_merchant_prompt_seen_at = COALESCE(web_merchant_prompt_seen_at, NOW())
		WHERE id = $1
		AND active = TRUE;
	`, userId)
	if err != nil {
		return fmt.Errorf("error marking the web merchant prompt seen for %s: %w", userId, err)
	}
	return nil
}
