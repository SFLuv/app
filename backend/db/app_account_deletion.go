package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

const (
	accountDeletionGracePeriod  = 30 * 24 * time.Hour
	deleteReasonAccountDeletion = "account_deletion"
	deleteReasonContactDelete   = "contact_delete"
	deleteReasonPonderDelete    = "ponder_delete"
	deleteReasonWalletSettings  = "wallet_settings_update"
)

var (
	ErrUserPendingDeletion       = errors.New("user account is scheduled for deletion")
	ErrUserDeletionNotScheduled  = errors.New("user account is not scheduled for deletion")
	ErrUserDeletionWindowExpired = errors.New("account deletion can no longer be canceled")
)

type userDeletionState struct {
	UserID               string
	PrimaryWalletAddress string
	Active               bool
	DeleteDate           *time.Time
	DeleteReason         *string
	DeletionRequestedAt  *time.Time
	DeletionCanceledAt   *time.Time
	DeletionCompletedAt  *time.Time
}

func normalizeDeleteReason(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func buildAccountDeletionStatus(state *userDeletionState, now time.Time) *structs.AccountDeletionStatusResponse {
	status := structs.AccountDeletionStatusActive
	canCancel := false

	if state != nil && !state.Active {
		status = structs.AccountDeletionStatusScheduled
		if state.DeleteDate != nil && !now.UTC().Before(state.DeleteDate.UTC()) {
			status = structs.AccountDeletionStatusReadyForManualPurge
		}
		canCancel = state.DeleteDate != nil && now.UTC().Before(state.DeleteDate.UTC()) && normalizeDeleteReason(state.DeleteReason) == deleteReasonAccountDeletion
	}

	return &structs.AccountDeletionStatusResponse{
		UserId:      state.UserID,
		Status:      status,
		DeleteDate:  state.DeleteDate,
		RequestedAt: state.DeletionRequestedAt,
		CanceledAt:  state.DeletionCanceledAt,
		CompletedAt: state.DeletionCompletedAt,
		CanCancel:   canCancel,
	}
}

func scanUserDeletionState(row interface {
	Scan(...any) error
}) (*userDeletionState, error) {
	var state userDeletionState
	if err := row.Scan(
		&state.UserID,
		&state.PrimaryWalletAddress,
		&state.Active,
		&state.DeleteDate,
		&state.DeleteReason,
		&state.DeletionRequestedAt,
		&state.DeletionCanceledAt,
		&state.DeletionCompletedAt,
	); err != nil {
		return nil, err
	}
	return &state, nil
}

func loadUserDeletionState(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID string) (*userDeletionState, error) {
	return scanUserDeletionState(q.QueryRow(ctx, `
		SELECT
			id,
			primary_wallet_address,
			active,
			delete_date,
			delete_reason,
			deletion_requested_at,
			deletion_canceled_at,
			deletion_completed_at
		FROM
			users
		WHERE
			id = $1;
	`, userID))
}

func (a *AppDB) UserIsActive(ctx context.Context, userID string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			active
		FROM
			users
		WHERE
			id = $1;
	`, userID)

	var active bool
	err := row.Scan(&active)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return active, nil
}

func (a *AppDB) GetAccountDeletionStatus(ctx context.Context, userID string, now time.Time) (*structs.AccountDeletionStatusResponse, error) {
	state, err := loadUserDeletionState(ctx, a.db, userID)
	if err != nil {
		return nil, err
	}
	return buildAccountDeletionStatus(state, now), nil
}

func (a *AppDB) GetAccountDeletionPreview(ctx context.Context, userID string, now time.Time) (*structs.AccountDeletionPreview, error) {
	state, err := loadUserDeletionState(ctx, a.db, userID)
	if err != nil {
		return nil, err
	}
	if !state.Active {
		return nil, ErrUserPendingDeletion
	}

	counts := structs.AccountDeletionCounts{}
	if err := a.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM wallets WHERE owner = $1 AND active = TRUE),
			(SELECT COUNT(*) FROM contacts WHERE owner = $1 AND active = TRUE),
			(SELECT COUNT(*) FROM locations WHERE owner_id = $1 AND active = TRUE),
			(
				SELECT COUNT(*)
				FROM location_hours lh
				JOIN locations l ON l.id = lh.location_id
				WHERE l.owner_id = $1
				AND l.active = TRUE
				AND lh.active = TRUE
			),
			(
				SELECT COUNT(*)
				FROM location_payment_wallets lpw
				JOIN locations l ON l.id = lpw.location_id
				WHERE l.owner_id = $1
				AND l.active = TRUE
				AND lpw.active = TRUE
			),
			(SELECT COUNT(*) FROM ponder_subscriptions WHERE owner = $1 AND active = TRUE),
			(SELECT COUNT(*) FROM user_verified_emails WHERE user_id = $1 AND active = TRUE),
			(SELECT COUNT(*) FROM memos WHERE owner = $1 AND active = TRUE);
	`, userID).Scan(
		&counts.Wallets,
		&counts.Contacts,
		&counts.Locations,
		&counts.LocationHours,
		&counts.LocationWallets,
		&counts.PonderSubscriptions,
		&counts.VerifiedEmails,
		&counts.Memos,
	); err != nil {
		return nil, fmt.Errorf("error loading account deletion preview counts: %w", err)
	}

	wallets, err := a.GetWalletsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	walletAddresses := make([]string, 0, len(wallets))
	for _, wallet := range wallets {
		if wallet == nil {
			continue
		}
		if wallet.SmartAddress != nil && strings.TrimSpace(*wallet.SmartAddress) != "" {
			walletAddresses = append(walletAddresses, strings.TrimSpace(*wallet.SmartAddress))
			continue
		}
		if strings.TrimSpace(wallet.EoaAddress) != "" {
			walletAddresses = append(walletAddresses, strings.TrimSpace(wallet.EoaAddress))
		}
	}

	status := buildAccountDeletionStatus(state, now)
	return &structs.AccountDeletionPreview{
		UserId:               userID,
		Status:               status.Status,
		DeleteDate:           status.DeleteDate,
		RequestedAt:          status.RequestedAt,
		CanCancel:            status.CanCancel,
		PrimaryWalletAddress: state.PrimaryWalletAddress,
		WalletAddresses:      walletAddresses,
		Counts:               counts,
	}, nil
}

func markUserOwnedRowsInactive(ctx context.Context, tx pgx.Tx, userID string, deleteAt time.Time, reason string) error {
	statements := []string{
		`UPDATE user_verified_emails SET active = FALSE, delete_date = $2, delete_reason = $3, updated_at = NOW() WHERE user_id = $1 AND active = TRUE;`,
		`UPDATE wallets SET active = FALSE, delete_date = $2, delete_reason = $3 WHERE owner = $1 AND active = TRUE;`,
		`UPDATE contacts SET active = FALSE, delete_date = $2, delete_reason = $3 WHERE owner = $1 AND active = TRUE;`,
		`UPDATE ponder_subscriptions SET active = FALSE, delete_date = $2, delete_reason = $3 WHERE owner = $1 AND active = TRUE;`,
		`UPDATE memos SET active = FALSE, delete_date = $2, delete_reason = $3 WHERE owner = $1 AND active = TRUE;`,
		`UPDATE affiliates SET active = FALSE, delete_date = $2, delete_reason = $3 WHERE user_id = $1 AND active = TRUE;`,
		`UPDATE proposers SET active = FALSE, delete_date = $2, delete_reason = $3 WHERE user_id = $1 AND active = TRUE;`,
		`UPDATE improvers SET active = FALSE, delete_date = $2, delete_reason = $3 WHERE user_id = $1 AND active = TRUE;`,
		`UPDATE supervisors SET active = FALSE, delete_date = $2, delete_reason = $3 WHERE user_id = $1 AND active = TRUE;`,
		`UPDATE issuers SET active = FALSE, delete_date = $2, delete_reason = $3 WHERE user_id = $1 AND active = TRUE;`,
		`UPDATE locations SET active = FALSE, delete_date = $2, delete_reason = $3 WHERE owner_id = $1 AND active = TRUE;`,
		`UPDATE location_hours lh SET active = FALSE, delete_date = $2, delete_reason = $3 FROM locations l WHERE lh.location_id = l.id AND l.owner_id = $1 AND lh.active = TRUE;`,
		`UPDATE location_payment_wallets lpw SET active = FALSE, delete_date = $2, delete_reason = $3 FROM locations l WHERE lpw.location_id = l.id AND l.owner_id = $1 AND lpw.active = TRUE;`,
	}

	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement, userID, deleteAt.UTC(), reason); err != nil {
			return err
		}
	}
	return nil
}

func restoreUserOwnedRowsForAccountDeletionCancel(ctx context.Context, tx pgx.Tx, userID string) error {
	statements := []string{
		`UPDATE user_verified_emails SET active = TRUE, delete_date = NULL, delete_reason = NULL, updated_at = NOW() WHERE user_id = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE wallets SET active = TRUE, delete_date = NULL, delete_reason = NULL WHERE owner = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE contacts SET active = TRUE, delete_date = NULL, delete_reason = NULL WHERE owner = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE ponder_subscriptions SET active = TRUE, delete_date = NULL, delete_reason = NULL WHERE owner = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE memos SET active = TRUE, delete_date = NULL, delete_reason = NULL WHERE owner = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE affiliates SET active = TRUE, delete_date = NULL, delete_reason = NULL WHERE user_id = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE proposers SET active = TRUE, delete_date = NULL, delete_reason = NULL WHERE user_id = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE improvers SET active = TRUE, delete_date = NULL, delete_reason = NULL WHERE user_id = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE supervisors SET active = TRUE, delete_date = NULL, delete_reason = NULL WHERE user_id = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE issuers SET active = TRUE, delete_date = NULL, delete_reason = NULL WHERE user_id = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE locations SET active = TRUE, delete_date = NULL, delete_reason = NULL WHERE owner_id = $1 AND delete_reason = $2 AND active = FALSE;`,
		`UPDATE location_hours lh SET active = TRUE, delete_date = NULL, delete_reason = NULL FROM locations l WHERE lh.location_id = l.id AND l.owner_id = $1 AND lh.delete_reason = $2 AND lh.active = FALSE;`,
		`UPDATE location_payment_wallets lpw SET active = TRUE, delete_date = NULL, delete_reason = NULL FROM locations l WHERE lpw.location_id = l.id AND l.owner_id = $1 AND lpw.delete_reason = $2 AND lpw.active = FALSE;`,
	}

	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement, userID, deleteReasonAccountDeletion); err != nil {
			return err
		}
	}
	return nil
}

func (a *AppDB) ScheduleAccountDeletion(ctx context.Context, userID string, now time.Time) (*structs.AccountDeletionStatusResponse, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	state, err := loadUserDeletionState(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if !state.Active {
		return nil, ErrUserPendingDeletion
	}

	deleteAt := now.UTC().Add(accountDeletionGracePeriod)
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET
			active = FALSE,
			delete_date = $2,
			delete_reason = $3,
			deletion_requested_at = $4,
			deletion_canceled_at = NULL,
			deletion_completed_at = NULL
		WHERE
			id = $1;
	`, userID, deleteAt, deleteReasonAccountDeletion, now.UTC()); err != nil {
		return nil, err
	}

	if err := markUserOwnedRowsInactive(ctx, tx, userID, deleteAt, deleteReasonAccountDeletion); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetAccountDeletionStatus(ctx, userID, now)
}

func (a *AppDB) CancelAccountDeletion(ctx context.Context, userID string, now time.Time) (*structs.AccountDeletionStatusResponse, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	state, err := loadUserDeletionState(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if state.Active || state.DeleteDate == nil || normalizeDeleteReason(state.DeleteReason) != deleteReasonAccountDeletion {
		return nil, ErrUserDeletionNotScheduled
	}
	if !now.UTC().Before(state.DeleteDate.UTC()) {
		return nil, ErrUserDeletionWindowExpired
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET
			active = TRUE,
			delete_date = NULL,
			delete_reason = NULL,
			deletion_canceled_at = $2
		WHERE
			id = $1;
	`, userID, now.UTC()); err != nil {
		return nil, err
	}

	if err := restoreUserOwnedRowsForAccountDeletionCancel(ctx, tx, userID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetAccountDeletionStatus(ctx, userID, now)
}

func (a *AppDB) ListUsersReadyForPurge(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			users
		WHERE
			active = FALSE
		AND
			delete_reason = $1
		AND
			delete_date IS NOT NULL
		AND
			delete_date <= $2
		ORDER BY
			delete_date ASC,
			id ASC;
	`, deleteReasonAccountDeletion, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	userIDs := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, rows.Err()
}

func (a *AppDB) PurgeDeletedUser(ctx context.Context, userID string, now time.Time) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	state, err := loadUserDeletionState(ctx, tx, userID)
	if err != nil {
		return err
	}
	if state.Active || state.DeleteDate == nil || normalizeDeleteReason(state.DeleteReason) != deleteReasonAccountDeletion {
		return ErrUserDeletionNotScheduled
	}
	if now.UTC().Before(state.DeleteDate.UTC()) {
		return ErrUserDeletionWindowExpired
	}

	statements := []string{
		`DELETE FROM location_payment_wallets lpw USING locations l WHERE lpw.location_id = l.id AND l.owner_id = $1;`,
		`DELETE FROM location_hours lh USING locations l WHERE lh.location_id = l.id AND l.owner_id = $1;`,
		`DELETE FROM locations WHERE owner_id = $1;`,
		`DELETE FROM contacts WHERE owner = $1;`,
		`DELETE FROM ponder_subscriptions WHERE owner = $1;`,
		`DELETE FROM memos WHERE owner = $1;`,
		`DELETE FROM affiliates WHERE user_id = $1;`,
		`DELETE FROM proposers WHERE user_id = $1;`,
		`DELETE FROM improvers WHERE user_id = $1;`,
		`DELETE FROM supervisors WHERE user_id = $1;`,
		`DELETE FROM issuers WHERE user_id = $1;`,
		`DELETE FROM user_verified_emails WHERE user_id = $1;`,
		`DELETE FROM user_oauth_credentials WHERE user_id = $1;`,
		`DELETE FROM wallets WHERE owner = $1;`,
		`DELETE FROM users WHERE id = $1;`,
	}

	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement, userID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
