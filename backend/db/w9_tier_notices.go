package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The warning tiers a person's annual total can reach.
//
// The first three are courtesies: the money is still paid, and the point is
// that nobody's first notice that a tax form exists should be their reward
// going missing. Only the last two withhold anything.
const (
	W9TierNotice   = "notice_400"  // a polite heads-up; still paid
	W9TierWarning  = "warning_500" // firmer; still paid
	W9TierEscrowed = "escrow_600"  // the crossing payment is held
	W9TierBlocked  = "blocked"     // everything after is refused
)

// W9TierSeverity orders the tiers so a later one can supersede an earlier.
// Someone who jumps from nothing straight past the limit should see the serious
// warning, not the friendly one they technically also crossed.
func W9TierSeverity(tier string) int {
	switch tier {
	case W9TierNotice:
		return 1
	case W9TierWarning:
		return 2
	case W9TierEscrowed:
		return 3
	case W9TierBlocked:
		return 4
	default:
		return 0
	}
}

// W9TierIsBlocking reports whether reaching this tier means money stopped.
func W9TierIsBlocking(tier string) bool {
	return tier == W9TierEscrowed || tier == W9TierBlocked
}

// W9TierNotice records a tier being reached, and whether the person has seen it.
type W9TierNoticeRow struct {
	Tier           string     `json:"tier"`
	NotifiedAt     time.Time  `json:"notified_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
}

// RecordW9TierReached notes that a person has reached a tier, returning true the
// first time.
//
// The first-time answer is what gates the push: crossing a line is news, and
// being told about it twice is noise.
func (a *AppDB) RecordW9TierReached(ctx context.Context, userID string, taxYear int, tier string) (bool, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(tier) == "" {
		return false, nil
	}

	tag, err := a.db.Exec(ctx, `
		INSERT INTO w9_tier_notices (user_id, tax_year, tier)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, tax_year, tier) DO NOTHING;
	`, userID, taxYear, tier)
	if err != nil {
		return false, fmt.Errorf("error recording w9 tier %s for %s: %w", tier, userID, err)
	}

	fresh := tag.RowsAffected() > 0
	if fresh {
		if err := a.clearW9NotificationReads(ctx, userID, taxYear); err != nil {
			return fresh, err
		}
	}
	return fresh, nil
}

// clearW9NotificationReads un-dismisses this year's W-9 bell entry.
//
// There is deliberately one W-9 entry per year rather than one per tier —
// stacking "you are approaching a form" under "your money is being held" is
// noise, and the second makes the first untrue. But one entry means one key,
// and dismissing it wrote a mark against that key that outlived the tier it
// was about: somebody who dismissed the 400 notice then crossed 500 got the
// modal with no bell entry behind it, so closing it left nothing to go back to.
//
// Reaching a new tier is a new thing to say, even under the same key, so the
// mark is cleared and the entry returns. Both types are cleared because which
// one is showing depends on whether money is held, and a person who has just
// been refused should not be looking at a dismissal they made before that.
func (a *AppDB) clearW9NotificationReads(ctx context.Context, userID string, taxYear int) error {
	// Keys built here rather than concatenated in SQL. Written as
	// `'w9_required:' || $2::text` the cast makes Postgres infer $2 as text,
	// and pgx then refuses to encode an int for it: "unable to encode 2026 into
	// text format for text (OID 25)". That error is returned all the way up and
	// then dropped by callers that treat a notification failure as not worth
	// failing a payout over — correctly, but it meant the bell entry stayed
	// dismissed through every tier that followed and nothing anywhere said why.
	if _, err := a.db.Exec(ctx, `
		DELETE FROM improver_notification_reads
		WHERE user_id = $1 AND notification_key = ANY($2);
	`, userID, []string{
		fmt.Sprintf("w9_required:%d", taxYear),
		fmt.Sprintf("w9_escrow_held:%d", taxYear),
	}); err != nil {
		return fmt.Errorf("error clearing w9 notification reads for %s: %w", userID, err)
	}
	return nil
}

// GetOutstandingW9Tier returns the most serious tier this person has reached
// that they have not yet acknowledged, or an empty row when there is nothing to
// show them.
//
// Only one modal is ever shown at a time. Someone who crosses two lines in a
// single payout should see the more serious one; queueing both would mean
// dismissing a friendly notice to reveal a warning that contradicts it.
func (a *AppDB) GetOutstandingW9Tier(ctx context.Context, userID string, taxYear int) (*W9TierNoticeRow, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, nil
	}

	var row W9TierNoticeRow
	err := a.db.QueryRow(ctx, `
		SELECT tier, notified_at, acknowledged_at
		FROM w9_tier_notices
		WHERE user_id = $1 AND tax_year = $2
		-- Returns the highest tier regardless of acknowledgement, and reports
		-- acknowledged_at alongside it so the caller can decide what that means.
		--
		-- Filtering acknowledged rows out here was the bug: the status response
		-- then carried no tier at all, so every client path that wants to re-open
		-- the modal -- the blocked re-arm, and tapping the W-9 notification -- had
		-- nothing to act on, and "Not now" became permanent. Whether an
		-- acknowledgement settles a tier is a policy question, and policy belongs
		-- with the caller that knows why it is asking.
		ORDER BY CASE tier
			WHEN 'blocked' THEN 4
			WHEN 'escrow_600' THEN 3
			WHEN 'warning_500' THEN 2
			WHEN 'notice_400' THEN 1
			ELSE 0
		END DESC
		LIMIT 1;
	`, userID, taxYear).Scan(&row.Tier, &row.NotifiedAt, &row.AcknowledgedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error loading the outstanding w9 tier for %s: %w", userID, err)
	}
	return &row, nil
}

// AcknowledgeW9Tier records that the person dismissed a tier's modal.
func (a *AppDB) AcknowledgeW9Tier(ctx context.Context, userID string, taxYear int, tier string) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE w9_tier_notices SET acknowledged_at = NOW()
		WHERE user_id = $1 AND tax_year = $2 AND tier = $3 AND acknowledged_at IS NULL;
	`, userID, taxYear, tier); err != nil {
		return fmt.Errorf("error acknowledging w9 tier %s for %s: %w", tier, userID, err)
	}
	return nil
}

// RearmW9BlockedTier makes the blocked modal show again.
//
// The first three tiers are dismissed once and stay dismissed — they are
// warnings, and nagging someone who has been told is just noise. Blocking is
// different: at that point the modal is the only thing standing between the
// person and being paid, so putting it away must not make it go away. It is
// re-armed whenever a payout is actually refused, so the explanation arrives
// with the failure rather than long after it.
func (a *AppDB) RearmW9BlockedTier(ctx context.Context, userID string, taxYear int) error {
	if _, err := a.db.Exec(ctx, `
		INSERT INTO w9_tier_notices (user_id, tax_year, tier)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, tax_year, tier)
		DO UPDATE SET acknowledged_at = NULL, notified_at = NOW();
	`, userID, taxYear, W9TierBlocked); err != nil {
		return fmt.Errorf("error re-arming the blocked w9 notice for %s: %w", userID, err)
	}
	// A refusal is news every time, so the bell entry comes back with the modal
	// rather than staying dismissed from some earlier round.
	return a.clearW9NotificationReads(ctx, userID, taxYear)
}

// ClearW9TierNotices removes every tier notice for a person and year.
//
// Called when a filing clears. The warnings existed to get the form filed; once
// it is in, leaving them behind would mean a modal telling somebody to do
// something they have already done.
func (a *AppDB) ClearW9TierNotices(ctx context.Context, userID string, taxYear int) error {
	if _, err := a.db.Exec(ctx, `
		DELETE FROM w9_tier_notices WHERE user_id = $1 AND tax_year = $2;
	`, userID, taxYear); err != nil {
		return fmt.Errorf("error clearing w9 tier notices for %s: %w", userID, err)
	}
	return nil
}
