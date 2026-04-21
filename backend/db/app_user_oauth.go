package db

import (
	"context"
	"fmt"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

func scanUserOAuthCredential(row interface {
	Scan(...any) error
}) (*structs.UserOAuthCredential, error) {
	credential := &structs.UserOAuthCredential{}
	if err := row.Scan(
		&credential.UserID,
		&credential.Provider,
		&credential.ProviderSubject,
		&credential.ProviderEmail,
		&credential.IsPrivateRelay,
		&credential.AccessTokenEncrypted,
		&credential.RefreshTokenEncrypted,
		&credential.AccessTokenExpiresAt,
		&credential.RefreshTokenExpiresAt,
		&credential.Scopes,
		&credential.CreatedAt,
		&credential.UpdatedAt,
		&credential.RevokedAt,
	); err != nil {
		return nil, err
	}
	return credential, nil
}

func (a *AppDB) UpsertUserOAuthCredential(ctx context.Context, credential *structs.UserOAuthCredential) error {
	if credential == nil {
		return fmt.Errorf("oauth credential is required")
	}
	if credential.UserID == "" {
		return fmt.Errorf("oauth credential user id is required")
	}
	if credential.Provider == "" {
		return fmt.Errorf("oauth credential provider is required")
	}

	_, err := a.db.Exec(ctx, `
		INSERT INTO user_oauth_credentials
			(
				user_id,
				provider,
				provider_subject,
				provider_email,
				is_private_relay,
				access_token_encrypted,
				refresh_token_encrypted,
				access_token_expires_at,
				refresh_token_expires_at,
				scopes,
				updated_at,
				revoked_at
			)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NULL)
		ON CONFLICT (user_id, provider)
		DO UPDATE SET
			provider_subject = EXCLUDED.provider_subject,
			provider_email = EXCLUDED.provider_email,
			is_private_relay = EXCLUDED.is_private_relay,
			access_token_encrypted = EXCLUDED.access_token_encrypted,
			refresh_token_encrypted = EXCLUDED.refresh_token_encrypted,
			access_token_expires_at = EXCLUDED.access_token_expires_at,
			refresh_token_expires_at = EXCLUDED.refresh_token_expires_at,
			scopes = EXCLUDED.scopes,
			updated_at = NOW(),
			revoked_at = NULL;
	`,
		credential.UserID,
		credential.Provider,
		credential.ProviderSubject,
		credential.ProviderEmail,
		credential.IsPrivateRelay,
		credential.AccessTokenEncrypted,
		credential.RefreshTokenEncrypted,
		credential.AccessTokenExpiresAt,
		credential.RefreshTokenExpiresAt,
		credential.Scopes,
	)
	return err
}

func (a *AppDB) GetUserOAuthCredential(ctx context.Context, userID string, provider string) (*structs.UserOAuthCredential, error) {
	return scanUserOAuthCredential(a.db.QueryRow(ctx, `
		SELECT
			user_id,
			provider,
			provider_subject,
			provider_email,
			is_private_relay,
			access_token_encrypted,
			refresh_token_encrypted,
			access_token_expires_at,
			refresh_token_expires_at,
			scopes,
			created_at,
			updated_at,
			revoked_at
		FROM
			user_oauth_credentials
		WHERE
			user_id = $1
		AND
			provider = $2;
	`, userID, provider))
}

func (a *AppDB) MarkUserOAuthCredentialRevoked(ctx context.Context, userID string, provider string, revokedAt time.Time) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			user_oauth_credentials
		SET
			revoked_at = $3,
			updated_at = NOW()
		WHERE
			user_id = $1
		AND
			provider = $2;
	`, userID, provider, revokedAt.UTC())
	return err
}

func (a *AppDB) DeleteUserOAuthCredentials(ctx context.Context, userID string) error {
	_, err := a.db.Exec(ctx, `
		DELETE FROM user_oauth_credentials
		WHERE user_id = $1;
	`, userID)
	return err
}

func (a *AppDB) DeleteUserOAuthCredential(ctx context.Context, userID string, provider string) error {
	_, err := a.db.Exec(ctx, `
		DELETE FROM user_oauth_credentials
		WHERE user_id = $1
		AND provider = $2;
	`, userID, provider)
	return err
}

func (a *AppDB) FindActiveUsersByVerifiedEmail(ctx context.Context, email string) ([]*structs.User, error) {
	_, normalized, err := normalizeVerifiedEmailInput(email)
	if err != nil {
		return nil, err
	}

	rows, err := a.db.Query(ctx, `
		SELECT DISTINCT ON (u.id)
			u.id,
			u.is_admin,
			u.is_merchant,
			u.is_organizer,
			u.is_improver,
			u.is_proposer,
			u.is_voter,
			u.is_issuer,
			u.is_supervisor,
			u.is_affiliate,
			u.contact_email,
			u.contact_phone,
			u.contact_name,
			u.primary_wallet_address,
			u.paypal_eth,
			u.last_redemption
		FROM
			users u
		JOIN
			user_verified_emails uve
		ON
			uve.user_id = u.id
		WHERE
			u.active = TRUE
		AND
			uve.active = TRUE
		AND
			uve.verified_at IS NOT NULL
		AND
			uve.email_normalized = $1
		ORDER BY
			u.id ASC;
	`, normalized)
	if err != nil {
		return nil, fmt.Errorf("error looking up active users by verified email: %s", err)
	}
	defer rows.Close()

	results := []*structs.User{}
	for rows.Next() {
		user := &structs.User{}
		if err := rows.Scan(
			&user.Id,
			&user.IsAdmin,
			&user.IsMerchant,
			&user.IsOrganizer,
			&user.IsImprover,
			&user.IsProposer,
			&user.IsVoter,
			&user.IsIssuer,
			&user.IsSupervisor,
			&user.IsAffiliate,
			&user.Email,
			&user.Phone,
			&user.Name,
			&user.PrimaryWalletAddress,
			&user.PayPalEth,
			&user.LastRedemption,
		); err != nil {
			return nil, fmt.Errorf("error scanning active user by verified email: %s", err)
		}
		user.Exists = true
		results = append(results, user)
	}

	return results, rows.Err()
}

func (a *AppDB) UserExistsAndActive(ctx context.Context, userID string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM users
			WHERE id = $1
			AND active = TRUE
		);
	`, userID)

	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (a *AppDB) HasAnyUserRecord(ctx context.Context, userID string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM users
			WHERE id = $1
		);
	`, userID)

	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
