package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

func normalizeAdminMCPEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (a *AppDB) GetAdminMCPAllowedEmails(ctx context.Context) ([]*structs.AdminMCPAllowedEmail, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			email,
			COALESCE(created_by_user_id, ''),
			created_at,
			COALESCE(revoked_by_user_id, ''),
			revoked_at,
			revoked_at IS NULL AS active
		FROM admin_mcp_allowed_emails
		ORDER BY revoked_at IS NOT NULL ASC, email ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying admin mcp allowed emails: %w", err)
	}
	defer rows.Close()

	emails := []*structs.AdminMCPAllowedEmail{}
	for rows.Next() {
		var row structs.AdminMCPAllowedEmail
		if err := rows.Scan(&row.Email, &row.CreatedByUserID, &row.CreatedAt, &row.RevokedByUserID, &row.RevokedAt, &row.Active); err != nil {
			return nil, fmt.Errorf("error scanning admin mcp allowed email: %w", err)
		}
		emails = append(emails, &row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating admin mcp allowed emails: %w", err)
	}
	return emails, nil
}

func (a *AppDB) UpsertAdminMCPAllowedEmail(ctx context.Context, email string, createdByUserID string) (*structs.AdminMCPAllowedEmail, error) {
	email = normalizeAdminMCPEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return nil, fmt.Errorf("valid email is required")
	}

	row := a.db.QueryRow(ctx, `
		INSERT INTO admin_mcp_allowed_emails(email, created_by_user_id, created_at)
		VALUES($1, $2, NOW())
		ON CONFLICT (email)
		DO UPDATE SET
			created_by_user_id = EXCLUDED.created_by_user_id,
			created_at = NOW(),
			revoked_at = NULL,
			revoked_by_user_id = NULL
		RETURNING
			email,
			COALESCE(created_by_user_id, ''),
			created_at,
			COALESCE(revoked_by_user_id, ''),
			revoked_at,
			revoked_at IS NULL AS active;
	`, email, createdByUserID)

	var out structs.AdminMCPAllowedEmail
	if err := row.Scan(&out.Email, &out.CreatedByUserID, &out.CreatedAt, &out.RevokedByUserID, &out.RevokedAt, &out.Active); err != nil {
		return nil, fmt.Errorf("error upserting admin mcp allowed email: %w", err)
	}
	return &out, nil
}

func (a *AppDB) RevokeAdminMCPAllowedEmail(ctx context.Context, email string, revokedByUserID string) error {
	email = normalizeAdminMCPEmail(email)
	if email == "" {
		return fmt.Errorf("email is required")
	}

	tag, err := a.db.Exec(ctx, `
		UPDATE admin_mcp_allowed_emails
		SET
			revoked_at = $3,
			revoked_by_user_id = $2
		WHERE email = $1
		AND revoked_at IS NULL;
	`, email, revokedByUserID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("error revoking admin mcp allowed email: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("allowed email not found")
	}
	return nil
}
