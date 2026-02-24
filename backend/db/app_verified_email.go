package db

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func normalizeVerifiedEmailInput(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("email is required")
	}

	parsed, err := mail.ParseAddress(raw)
	if err != nil || strings.TrimSpace(parsed.Address) == "" {
		return "", "", fmt.Errorf("invalid email format")
	}

	email := strings.TrimSpace(parsed.Address)
	normalized := strings.ToLower(email)
	return email, normalized, nil
}

func userVerifiedEmailStatus(verifiedAt *time.Time, expiresAt *time.Time) structs.UserVerifiedEmailStatus {
	if verifiedAt != nil {
		return structs.UserVerifiedEmailStatusVerified
	}
	if expiresAt != nil && time.Now().UTC().After(expiresAt.UTC()) {
		return structs.UserVerifiedEmailStatusExpired
	}
	return structs.UserVerifiedEmailStatusPending
}

func buildUserVerifiedEmail(
	id string,
	userId string,
	email string,
	verifiedAt *time.Time,
	verificationSentAt *time.Time,
	verificationTokenExpiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) *structs.UserVerifiedEmail {
	return &structs.UserVerifiedEmail{
		Id:                         id,
		UserId:                     userId,
		Email:                      email,
		Status:                     userVerifiedEmailStatus(verifiedAt, verificationTokenExpiresAt),
		VerifiedAt:                 verifiedAt,
		VerificationSentAt:         verificationSentAt,
		VerificationTokenExpiresAt: verificationTokenExpiresAt,
		CreatedAt:                  createdAt,
		UpdatedAt:                  updatedAt,
	}
}

func scanUserVerifiedEmailRow(row interface {
	Scan(...any) error
}) (*structs.UserVerifiedEmail, error) {
	var id string
	var userId string
	var email string
	var verifiedAt *time.Time
	var verificationSentAt *time.Time
	var verificationTokenExpiresAt *time.Time
	var createdAt time.Time
	var updatedAt time.Time

	if err := row.Scan(
		&id,
		&userId,
		&email,
		&verifiedAt,
		&verificationSentAt,
		&verificationTokenExpiresAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}

	return buildUserVerifiedEmail(id, userId, email, verifiedAt, verificationSentAt, verificationTokenExpiresAt, createdAt, updatedAt), nil
}

func (a *AppDB) GetUserVerifiedEmails(ctx context.Context, userId string) ([]*structs.UserVerifiedEmail, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			user_id,
			email,
			verified_at,
			verification_sent_at,
			verification_token_expires_at,
			created_at,
			updated_at
		FROM
			user_verified_emails
		WHERE
			user_id = $1
		ORDER BY
			CASE WHEN verified_at IS NULL THEN 1 ELSE 0 END,
			created_at ASC;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error querying user verified emails: %s", err)
	}
	defer rows.Close()

	results := []*structs.UserVerifiedEmail{}
	for rows.Next() {
		record, err := scanUserVerifiedEmailRow(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning user verified email row: %s", err)
		}
		results = append(results, record)
	}
	return results, nil
}

func (a *AppDB) CreateOrRefreshUserEmailVerification(ctx context.Context, userId string, email string) (*structs.UserVerifiedEmail, string, error) {
	email, normalized, err := normalizeVerifiedEmailInput(email)
	if err != nil {
		return nil, "", err
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)

	var existingId string
	var existingVerifiedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT
			id,
			verified_at
		FROM
			user_verified_emails
		WHERE
			user_id = $1
		AND
			email_normalized = $2
		FOR UPDATE;
	`, userId, normalized).Scan(&existingId, &existingVerifiedAt)
	if err != nil && err != pgx.ErrNoRows {
		return nil, "", fmt.Errorf("error checking existing user verified email: %s", err)
	}

	if existingVerifiedAt != nil {
		return nil, "", fmt.Errorf("email already verified")
	}

	token := uuid.NewString()
	expiresAt := time.Now().UTC().Add(30 * time.Minute)
	emailId := existingId
	if err == pgx.ErrNoRows {
		emailId = uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO user_verified_emails
				(
					id,
					user_id,
					email,
					email_normalized,
					verification_token,
					verification_sent_at,
					verification_token_expires_at
				)
			VALUES
				($1, $2, $3, $4, $5, NOW(), $6);
		`, emailId, userId, email, normalized, token, expiresAt)
		if err != nil {
			return nil, "", fmt.Errorf("error creating user verified email: %s", err)
		}
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE
				user_verified_emails
			SET
				email = $2,
				email_normalized = $3,
				verification_token = $4,
				verification_sent_at = NOW(),
				verification_token_expires_at = $5,
				updated_at = NOW()
			WHERE
				id = $1;
		`, emailId, email, normalized, token, expiresAt)
		if err != nil {
			return nil, "", fmt.Errorf("error refreshing user verified email token: %s", err)
		}
	}

	record, err := scanUserVerifiedEmailRow(tx.QueryRow(ctx, `
		SELECT
			id,
			user_id,
			email,
			verified_at,
			verification_sent_at,
			verification_token_expires_at,
			created_at,
			updated_at
		FROM
			user_verified_emails
		WHERE
			id = $1;
	`, emailId))
	if err != nil {
		return nil, "", fmt.Errorf("error loading user verified email after create/refresh: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}
	return record, token, nil
}

func (a *AppDB) ResendUserEmailVerification(ctx context.Context, userId string, emailId string) (*structs.UserVerifiedEmail, string, error) {
	emailId = strings.TrimSpace(emailId)
	if emailId == "" {
		return nil, "", fmt.Errorf("email_id is required")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)

	var verifiedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT
			verified_at
		FROM
			user_verified_emails
		WHERE
			id = $1
		AND
			user_id = $2
		FOR UPDATE;
	`, emailId, userId).Scan(&verifiedAt)
	if err == pgx.ErrNoRows {
		return nil, "", fmt.Errorf("verified email record not found")
	}
	if err != nil {
		return nil, "", fmt.Errorf("error loading verified email record: %s", err)
	}
	if verifiedAt != nil {
		return nil, "", fmt.Errorf("email is already verified")
	}

	token := uuid.NewString()
	expiresAt := time.Now().UTC().Add(30 * time.Minute)
	_, err = tx.Exec(ctx, `
		UPDATE
			user_verified_emails
		SET
			verification_token = $2,
			verification_sent_at = NOW(),
			verification_token_expires_at = $3,
			updated_at = NOW()
		WHERE
			id = $1;
	`, emailId, token, expiresAt)
	if err != nil {
		return nil, "", fmt.Errorf("error resending user email verification: %s", err)
	}

	record, err := scanUserVerifiedEmailRow(tx.QueryRow(ctx, `
		SELECT
			id,
			user_id,
			email,
			verified_at,
			verification_sent_at,
			verification_token_expires_at,
			created_at,
			updated_at
		FROM
			user_verified_emails
		WHERE
			id = $1;
	`, emailId))
	if err != nil {
		return nil, "", fmt.Errorf("error loading verified email after resend: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}
	return record, token, nil
}

func (a *AppDB) VerifyUserEmailToken(ctx context.Context, token string) (*structs.UserVerifiedEmail, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("verification token is required")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id string
	var userId string
	var email string
	var verifiedAt *time.Time
	var verificationTokenExpiresAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT
			id,
			user_id,
			email,
			verified_at,
			verification_token_expires_at
		FROM
			user_verified_emails
		WHERE
			verification_token = $1
		FOR UPDATE;
	`, token).Scan(&id, &userId, &email, &verifiedAt, &verificationTokenExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("invalid verification token")
	}
	if err != nil {
		return nil, fmt.Errorf("error loading verification token: %s", err)
	}

	if verifiedAt != nil {
		return nil, fmt.Errorf("email already verified")
	}
	if verificationTokenExpiresAt == nil || time.Now().UTC().After(verificationTokenExpiresAt.UTC()) {
		return nil, fmt.Errorf("verification token has expired")
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			user_verified_emails
		SET
			verified_at = NOW(),
			verification_token = NULL,
			verification_sent_at = NULL,
			verification_token_expires_at = NULL,
			updated_at = NOW()
		WHERE
			id = $1;
	`, id)
	if err != nil {
		return nil, fmt.Errorf("error marking user email verified: %s", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			users
		SET
			contact_email = CASE
				WHEN COALESCE(NULLIF(TRIM(contact_email), ''), '') = '' THEN $2
				ELSE contact_email
			END
		WHERE
			id = $1;
	`, userId, email)
	if err != nil {
		return nil, fmt.Errorf("error updating user contact email after verification: %s", err)
	}

	record, err := scanUserVerifiedEmailRow(tx.QueryRow(ctx, `
		SELECT
			id,
			user_id,
			email,
			verified_at,
			verification_sent_at,
			verification_token_expires_at,
			created_at,
			updated_at
		FROM
			user_verified_emails
		WHERE
			id = $1;
	`, id))
	if err != nil {
		return nil, fmt.Errorf("error loading user verified email after verification: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return record, nil
}

func (a *AppDB) IsVerifiedEmailForUser(ctx context.Context, userId string, email string) (bool, error) {
	_, normalized, err := normalizeVerifiedEmailInput(email)
	if err != nil {
		return false, err
	}

	row := a.db.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT
					1
				FROM
					user_verified_emails
				WHERE
					user_id = $1
				AND
					email_normalized = $2
				AND
					verified_at IS NOT NULL
			);
	`, userId, normalized)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, fmt.Errorf("error checking verified email status: %s", err)
	}
	return exists, nil
}
