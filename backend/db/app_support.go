package db

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// referenceAlphabet omits the characters people mis-read or mis-type when
// reading a reference off a screen and saying it down a phone: O/0, I/1, S/5.
const referenceAlphabet = "ABCDEFGHJKLMNPQRTUVWXYZ2346789"

// NewSupportReference generates a quotable reference, e.g. SUP-7F3KQ2.
func NewSupportReference() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("SUP-")
	for _, b := range buf {
		sb.WriteByte(referenceAlphabet[int(b)%len(referenceAlphabet)])
	}
	return sb.String(), nil
}

// CreateSupportRequest stores a submission, generating its reference.
//
// A reference collision is astronomically unlikely but not impossible, and
// losing somebody's message to one would be indefensible, so a clash is simply
// retried.
// source and userId say where the request came from. userId is empty for the
// website form, which has no signed-in caller; it is set for one submitted from
// inside the app, and is what lets the alert offer to open a chat rather than
// only an email.
func (a *AppDB) CreateSupportRequest(ctx context.Context, sub structs.SupportRequestSubmission, source string, userId string) (*structs.SupportRequest, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = structs.SupportSourceWebsite
	}
	var owner any
	if trimmed := strings.TrimSpace(userId); trimmed != "" {
		owner = trimmed
	}

	for attempt := 0; attempt < 5; attempt++ {
		reference, err := NewSupportReference()
		if err != nil {
			return nil, fmt.Errorf("generating a support reference: %w", err)
		}

		out := &structs.SupportRequest{}
		err = a.db.QueryRow(ctx, `
			INSERT INTO support_requests (reference, email, name, subject, category, message, source, status, user_id)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $8, $7, $9)
			RETURNING id, reference, email, name, subject, COALESCE(category, ''), message, source, status, created_at, user_id;
		`, reference,
			strings.TrimSpace(sub.Email),
			strings.TrimSpace(sub.Name),
			strings.TrimSpace(sub.Subject),
			strings.TrimSpace(sub.SubjectSlug),
			strings.TrimSpace(sub.Message),
			structs.SupportRequestNew,
			source,
			owner,
		).Scan(&out.Id, &out.Reference, &out.Email, &out.Name, &out.Subject,
			&out.Category, &out.Message, &out.Source, &out.Status, &out.CreatedAt, &out.UserId)
		if err == nil {
			return out, nil
		}
		if isUniqueViolation(err) {
			continue
		}
		return nil, err
	}
	return nil, errors.New("could not allocate a unique support reference")
}

// ListSupportRequests returns requests newest first, optionally only archived
// or only open ones.
func (a *AppDB) ListSupportRequests(ctx context.Context, status string) ([]*structs.SupportRequest, error) {
	query := `
		SELECT id, reference, email, name, subject, COALESCE(category, ''), message, source, status,
			created_at, archived_at, COALESCE(archived_by, ''), user_id
		FROM support_requests
	`
	args := []any{}
	if status != "" {
		query += " WHERE status = $1"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC;"

	rows, err := a.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*structs.SupportRequest{}
	for rows.Next() {
		r := &structs.SupportRequest{}
		if err := rows.Scan(&r.Id, &r.Reference, &r.Email, &r.Name, &r.Subject,
			&r.Category, &r.Message, &r.Source, &r.Status, &r.CreatedAt, &r.ArchivedAt, &r.ArchivedBy,
			&r.UserId); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetSupportRequest reads one by id.
func (a *AppDB) GetSupportRequest(ctx context.Context, id int64) (*structs.SupportRequest, error) {
	r := &structs.SupportRequest{}
	err := a.db.QueryRow(ctx, `
		SELECT id, reference, email, name, subject, COALESCE(category, ''), message, source, status,
			created_at, archived_at, COALESCE(archived_by, ''), user_id
		FROM support_requests
		WHERE id = $1;
	`, id).Scan(&r.Id, &r.Reference, &r.Email, &r.Name, &r.Subject,
		&r.Category, &r.Message, &r.Source, &r.Status, &r.CreatedAt, &r.ArchivedAt, &r.ArchivedBy,
		&r.UserId)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

// SetSupportRequestArchived archives or reopens a request.
//
// Returns the updated row, or nil when there was nothing to update — which is
// how an already-archived request is told apart from a missing one by the
// caller that has to answer a button press.
func (a *AppDB) SetSupportRequestArchived(ctx context.Context, id int64, archived bool, by string) (*structs.SupportRequest, error) {
	status := structs.SupportRequestNew
	if archived {
		status = structs.SupportRequestArchived
	}

	r := &structs.SupportRequest{}
	err := a.db.QueryRow(ctx, `
		UPDATE support_requests
		SET status = $2,
			archived_at = CASE WHEN $3 THEN NOW() ELSE NULL END,
			archived_by = CASE WHEN $3 THEN $4 ELSE NULL END
		WHERE id = $1 AND status <> $2
		RETURNING id, reference, email, name, subject, message, source, status,
			created_at, archived_at, COALESCE(archived_by, '');
	`, id, status, archived, by).Scan(&r.Id, &r.Reference, &r.Email, &r.Name, &r.Subject,
		&r.Message, &r.Source, &r.Status, &r.CreatedAt, &r.ArchivedAt, &r.ArchivedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}
