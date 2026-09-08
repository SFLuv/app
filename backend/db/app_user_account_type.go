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
	// close the door permanently: withdrawing them reopens it.
	PendingLocations int `json:"pending_locations"`
	// RejectedLocations counts applications that came back refused. They block
	// too — a personal account should not be holding a record of having traded
	// as a business — and, like pending ones, can be withdrawn.
	RejectedLocations int `json:"rejected_locations"`
	// CanRevert is the single answer the screen acts on.
	CanRevert bool `json:"can_revert"`
}

// GetMerchantRevertEligibility counts what stands between this account and a
// personal one.
//
// Any live listing counts, in any of the three states. An approved one is on
// the map taking money; a pending one is in the review queue; and a rejected
// one is still a record of this account having traded as a business, which a
// personal account has no place holding.
//
// None of the three is a dead end: an application that has not been approved
// can be withdrawn, which retires the row and clears the way. That is what
// makes counting rejected safe rather than a trap.
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
			), 0),
			COALESCE((
				SELECT COUNT(*) FROM locations l
				WHERE l.owner_id = u.id AND l.active = TRUE AND l.approval = FALSE
			), 0)
		FROM users u
		WHERE u.id = $1
		AND u.active = TRUE;
	`, userId).Scan(
		&result.AccountType,
		&result.ApprovedLocations,
		&result.PendingLocations,
		&result.RejectedLocations,
	)
	if err != nil {
		return nil, err
	}

	result.CanRevert = result.AccountType == structs.AccountTypeMerchant &&
		result.ApprovedLocations == 0 &&
		result.PendingLocations == 0 &&
		result.RejectedLocations == 0

	return result, nil
}

// Where an account-type change came from. Recorded on the event so a row that
// looks wrong can be traced to whoever decided it.
const (
	AccountTypeSourceSignup = "signup"
	AccountTypeSourceSelf   = "self"
	AccountTypeSourceAdmin  = "admin"
)

// recordAccountTypeChange writes the history a tax export will need, and moves
// users.merchant_since with it.
//
// Money arriving before somebody was a merchant is not sales. Incoming
// transfers are read off the chain, which records an address and an amount and
// nothing about what the account was at the time, so without a date to split on
// a report cannot tell a year of personal receipts from a year of takings.
//
// merchant_since is the start of the CURRENT stint and is cleared on a revert;
// the events table keeps every change, which is what a report covering a period
// containing a flip has to read. Both are written here so they cannot disagree.
//
// A no-op change writes nothing: re-selecting the type somebody already has is
// not an event, and logging it would put false boundaries in the history.
func recordAccountTypeChange(
	ctx context.Context,
	tx pgx.Tx,
	userId string,
	previous string,
	next string,
	source string,
) error {
	if previous == next {
		return nil
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_account_type_events (user_id, previous_account_type, account_type, source)
		VALUES ($1, $2, $3, $4);
	`, userId, previous, next, source); err != nil {
		return fmt.Errorf("error recording account type change for %s: %w", userId, err)
	}

	if next == structs.AccountTypeMerchant {
		// COALESCE, so a merchant who never left keeps the date they started.
		// Only a revert clears it, and the clause below is what does that.
		// The flag clears whether or not the date moves: this conversion was
		// observed, so from here on the row is a fact rather than the backfill's
		// upper-bound guess. COALESCE on the date itself, so a merchant who
		// never left keeps the day they started.
		if _, err := tx.Exec(ctx, `
			UPDATE users
			SET merchant_since = COALESCE(merchant_since, NOW()),
				merchant_since_inferred = FALSE
			WHERE id = $1;
		`, userId); err != nil {
			return fmt.Errorf("error stamping merchant_since for %s: %w", userId, err)
		}
		return nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users SET merchant_since = NULL, merchant_since_inferred = FALSE WHERE id = $1;
	`, userId); err != nil {
		return fmt.Errorf("error clearing merchant_since for %s: %w", userId, err)
	}
	return nil
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

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error opening transaction to set account type: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
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

	if err := recordAccountTypeChange(
		ctx, tx, result.UserId, result.PreviousAccountType, result.AccountType, AccountTypeSourceSelf,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("error committing account type change: %w", err)
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
