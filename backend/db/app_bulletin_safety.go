package db

import (
	"context"
	"errors"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

// ErrAlreadyReported means this reporter already reported this target.
var ErrAlreadyReported = errors.New("already reported")

// BulletinAutoHideReports is how many distinct people must report a post before
// it is hidden from everyone pending review. One report alone does nothing
// automatic, so a single person cannot take down posts they dislike.
const BulletinAutoHideReports = 3

// --- Blocks ---------------------------------------------------------------------

func (a *AppDB) BlockUser(ctx context.Context, blocker, blocked string) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO user_blocks(blocker_id, blocked_id) VALUES ($1, $2)
		ON CONFLICT (blocker_id, blocked_id) DO NOTHING;
	`, blocker, blocked)
	return err
}

func (a *AppDB) UnblockUser(ctx context.Context, blocker, blocked string) error {
	_, err := a.db.Exec(ctx, `DELETE FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2;`, blocker, blocked)
	return err
}

func (a *AppDB) GetBlockedUserIds(ctx context.Context, blocker string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT blocked_id FROM user_blocks WHERE blocker_id = $1 ORDER BY created_at DESC;
	`, blocker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (a *AppDB) HasBlocked(ctx context.Context, blocker, blocked string) (bool, error) {
	var ok bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2);
	`, blocker, blocked).Scan(&ok)
	return ok, err
}

// --- Reports --------------------------------------------------------------------

// HasReported reports whether this person already reported this target, so a
// repeat report (even of something since hidden) gets the same calm answer.
func (a *AppDB) HasReported(ctx context.Context, reporter, targetType, targetId string) (bool, error) {
	var ok bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM content_reports
			WHERE reporter_id = $1 AND target_type = $2 AND target_id = $3
		);
	`, reporter, targetType, targetId).Scan(&ok)
	return ok, err
}

// CreateContentReport records a report and returns how many distinct open
// reports the target now has, so the caller can apply the auto-hide threshold.
func (a *AppDB) CreateContentReport(ctx context.Context, reporter string, req *structs.ContentReportRequest, reportedUser string) (int, error) {
	var reportedUserArg any
	if reportedUser != "" {
		reportedUserArg = reportedUser
	}
	_, err := a.db.Exec(ctx, `
		INSERT INTO content_reports(reporter_id, target_type, target_id, reported_user_id, reason, details)
		VALUES ($1, $2, $3, $4, $5, $6);
	`, reporter, req.TargetType, req.TargetId, reportedUserArg, req.Reason, req.Details)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrAlreadyReported
		}
		return 0, err
	}

	var open int
	err = a.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT reporter_id) FROM content_reports
		WHERE target_type = $1 AND target_id = $2 AND status = 'open';
	`, req.TargetType, req.TargetId).Scan(&open)
	return open, err
}

type OpenReportTarget struct {
	TargetType     string
	TargetId       string
	ReportedUserId *string
	Count          int
	FirstAt        time.Time
}

// ListOpenReportTargets groups open reports by what they are about, oldest
// first, so the queue is worked in the order people raised things.
func (a *AppDB) ListOpenReportTargets(ctx context.Context) ([]OpenReportTarget, error) {
	rows, err := a.db.Query(ctx, `
		SELECT target_type, target_id, MAX(reported_user_id), COUNT(DISTINCT reporter_id), MIN(created_at)
		FROM content_reports
		WHERE status = 'open'
		GROUP BY target_type, target_id
		ORDER BY MIN(created_at)
		LIMIT 200;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OpenReportTarget{}
	for rows.Next() {
		var t OpenReportTarget
		if err := rows.Scan(&t.TargetType, &t.TargetId, &t.ReportedUserId, &t.Count, &t.FirstAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (a *AppDB) GetOpenReportsForTarget(ctx context.Context, targetType, targetId string) ([]structs.AdminReportEntry, error) {
	rows, err := a.db.Query(ctx, `
		SELECT r.id, r.reporter_id, `+displayUserReporter+`, r.reason, r.details, r.created_at
		FROM content_reports r
		LEFT JOIN users u ON u.id = r.reporter_id
		WHERE r.target_type = $1 AND r.target_id = $2 AND r.status = 'open'
		ORDER BY r.created_at;
	`, targetType, targetId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []structs.AdminReportEntry{}
	for rows.Next() {
		var e structs.AdminReportEntry
		if err := rows.Scan(&e.Id, &e.ReporterId, &e.Reporter, &e.Reason, &e.Details, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// displayUserReporter names a reporter for admins: their contact name or email.
const displayUserReporter = `COALESCE(NULLIF(u.contact_name, ''), NULLIF(u.contact_email, ''), r.reporter_id)`

// ResolveReportsForTarget closes every open report on a target with the given
// status and records who reviewed them.
func (a *AppDB) ResolveReportsForTarget(ctx context.Context, targetType, targetId, status, adminId string) ([]int64, error) {
	rows, err := a.db.Query(ctx, `
		UPDATE content_reports
		SET status = $3, reviewed_by = $4, reviewed_at = NOW()
		WHERE target_type = $1 AND target_id = $2 AND status = 'open'
		RETURNING id;
	`, targetType, targetId, status, adminId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (a *AppDB) RecordModerationAction(ctx context.Context, adminId, targetUserId string, reportId *int64, action, note string) error {
	var target any
	if targetUserId != "" {
		target = targetUserId
	}
	_, err := a.db.Exec(ctx, `
		INSERT INTO bulletin_moderation_actions(admin_id, target_user_id, report_id, action, note)
		VALUES ($1, $2, $3, $4, $5);
	`, adminId, target, reportId, action, note)
	return err
}

func (a *AppDB) GetModerationHistory(ctx context.Context, userId string) ([]structs.AdminModerationRow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT action, note, admin_id, created_at
		FROM bulletin_moderation_actions
		WHERE target_user_id = $1
		ORDER BY created_at DESC
		LIMIT 20;
	`, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []structs.AdminModerationRow{}
	for rows.Next() {
		var r structs.AdminModerationRow
		if err := rows.Scan(&r.Action, &r.Note, &r.AdminId, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetReportedUserSummary is the context an admin sees about the person a report
// is against.
func (a *AppDB) GetReportedUserSummary(ctx context.Context, userId string) (*structs.AdminReportedUser, error) {
	var s structs.AdminReportedUser
	err := a.db.QueryRow(ctx, `
		SELECT
			u.id,
			COALESCE(up.public_name, ''),
			COALESCE(u.contact_email, ''),
			(SELECT COUNT(*) FROM bulletin_posts p WHERE p.author_id = u.id),
			(SELECT COUNT(*) FROM content_reports r WHERE r.reported_user_id = u.id),
			up.bulletin_suspended_until
		FROM users u
		LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE u.id = $1;
	`, userId).Scan(&s.UserId, &s.PublicName, &s.ContactEmail, &s.PostCount, &s.ReportCount, &s.SuspendedUntil)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// SetBulletinSuspension suspends (until > now) or lifts (until nil) a user's
// Bulletin access. A user who never set up a profile gets one with a neutral
// name so the suspension has somewhere to live.
func (a *AppDB) SetBulletinSuspension(ctx context.Context, userId string, until *time.Time, reason string) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO user_profiles(user_id, public_name, bulletin_suspended_until, bulletin_suspended_reason)
		VALUES ($1, 'SFLuv neighbor', $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			bulletin_suspended_until = EXCLUDED.bulletin_suspended_until,
			bulletin_suspended_reason = EXCLUDED.bulletin_suspended_reason,
			updated_at = NOW();
	`, userId, until, reason)
	return err
}
