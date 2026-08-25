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

const w9FilingColumns = `
	id, user_id, tax_year, status, provider, provider_request_id, form_url,
	form_url_expires_at, threshold_crossed_at, requested_at, completed_at,
	tin_type, cleared_by_user_id, clear_reason, last_provider_status,
	last_provider_event_at, COALESCE(tin_match, '')
`

func scanW9Filing(row interface{ Scan(...any) error }) (*structs.W9Filing, error) {
	var out structs.W9Filing
	var formExpires, crossed, requested, completed, lastEvent *time.Time
	if err := row.Scan(
		&out.ID, &out.UserID, &out.TaxYear, &out.Status, &out.Provider,
		&out.ProviderRequestID, &out.FormURL, &formExpires, &crossed, &requested,
		&completed, &out.TINType, &out.ClearedByUserID, &out.ClearReason,
		&out.LastProviderStatus, &lastEvent, &out.TINMatch,
	); err != nil {
		return nil, err
	}
	out.FormURLExpiresAt, out.ThresholdCrossedAt = formExpires, crossed
	out.RequestedAt, out.CompletedAt, out.LastProviderEventAt = requested, completed, lastEvent
	return &out, nil
}

// GetW9Filing returns a person's filing for a year, or nil when they have none.
// No row is not an error: most people never earn enough to need one.
func (a *AppDB) GetW9Filing(ctx context.Context, userID string, taxYear int) (*structs.W9Filing, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, nil
	}
	filing, err := scanW9Filing(a.db.QueryRow(ctx,
		`SELECT `+w9FilingColumns+` FROM w9_filings WHERE user_id = $1 AND tax_year = $2;`,
		userID, taxYear))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error loading w9 filing for %s: %w", userID, err)
	}
	return filing, nil
}

// GetW9FilingByProviderRequestID finds a filing by the vendor's own handle.
//
// The webhook identifies its subject by SubmissionId and nothing else — it does
// not know our user or tax year — so this is the only way in from a callback.
// Absent is not an error: a delivery for a submission we have no record of is
// something to acknowledge and drop, not to fail on.
func (a *AppDB) GetW9FilingByProviderRequestID(ctx context.Context, providerRequestID string) (*structs.W9Filing, error) {
	if strings.TrimSpace(providerRequestID) == "" {
		return nil, nil
	}
	filing, err := scanW9Filing(a.db.QueryRow(ctx,
		`SELECT `+w9FilingColumns+` FROM w9_filings WHERE provider_request_id = $1 LIMIT 1;`,
		providerRequestID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error loading w9 filing for submission %s: %w", providerRequestID, err)
	}
	return filing, nil
}

// GetW9FilingStatusTx reads just the status inside a transaction, for the
// payout gate. Absent means not_started.
func (a *AppDB) GetW9FilingStatusTx(ctx context.Context, tx pgx.Tx, userID string, taxYear int) (string, error) {
	if strings.TrimSpace(userID) == "" {
		return W9StatusNotStarted, nil
	}
	var status string
	err := tx.QueryRow(ctx,
		`SELECT status FROM w9_filings WHERE user_id = $1 AND tax_year = $2;`,
		userID, taxYear).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return W9StatusNotStarted, nil
	}
	if err != nil {
		return "", fmt.Errorf("error reading w9 filing status for %s: %w", userID, err)
	}
	return status, nil
}

// EnsureW9FilingRequestedTx records that this person now owes a form.
//
// It never downgrades a filing: someone already cleared stays cleared, and
// threshold_crossed_at keeps its first value so the record shows when the
// obligation actually began rather than when it was last touched.
func (a *AppDB) EnsureW9FilingRequestedTx(ctx context.Context, tx pgx.Tx, userID string, taxYear int) error {
	if strings.TrimSpace(userID) == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO w9_filings (user_id, tax_year, status, threshold_crossed_at, requested_at)
		VALUES ($1, $2, 'requested', NOW(), NOW())
		ON CONFLICT (user_id, tax_year) DO UPDATE
		SET status = CASE
				WHEN w9_filings.status IN ('completed','legacy_approved','manually_cleared')
					THEN w9_filings.status
				ELSE 'requested'
			END,
			threshold_crossed_at = COALESCE(w9_filings.threshold_crossed_at, NOW()),
			requested_at = COALESCE(w9_filings.requested_at, NOW()),
			updated_at = NOW();
	`, userID, taxYear); err != nil {
		return fmt.Errorf("error recording w9 requirement for %s: %w", userID, err)
	}
	return nil
}

// SaveW9ProviderRequest stores the vendor handles for a filing.
func (a *AppDB) SaveW9ProviderRequest(ctx context.Context, userID string, taxYear int, provider string, requestID string, formURL string, expires *time.Time) error {
	if _, err := a.db.Exec(ctx, `
		INSERT INTO w9_filings (user_id, tax_year, status, provider, provider_request_id, form_url, form_url_expires_at, requested_at)
		VALUES ($1, $2, 'requested', $3, $4, $5, $6, NOW())
		ON CONFLICT (user_id, tax_year) DO UPDATE
		SET provider = EXCLUDED.provider,
			provider_request_id = EXCLUDED.provider_request_id,
			form_url = EXCLUDED.form_url,
			form_url_expires_at = EXCLUDED.form_url_expires_at,
			status = CASE
				WHEN w9_filings.status IN ('completed','legacy_approved','manually_cleared')
					THEN w9_filings.status
				ELSE 'requested'
			END,
			requested_at = COALESCE(w9_filings.requested_at, NOW()),
			updated_at = NOW();
	`, userID, taxYear, provider, requestID, formURL, expires); err != nil {
		return fmt.Errorf("error saving w9 provider request for %s: %w", userID, err)
	}
	return nil
}

// MarkW9FilingCompleted records that the vendor has the form.
//
// Returns changed=false when the filing was already cleared, which is what
// makes a redelivered webhook harmless: the second delivery finds nothing to do
// and no escrow is released twice.
func (a *AppDB) MarkW9FilingCompleted(ctx context.Context, userID string, taxYear int, tinType string, providerStatus string) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE w9_filings
		SET status = 'completed', completed_at = COALESCE(completed_at, NOW()),
			tin_type = COALESCE(NULLIF($3, ''), tin_type),
			last_provider_status = $4, last_provider_event_at = NOW(), updated_at = NOW()
		WHERE user_id = $1 AND tax_year = $2
		AND status NOT IN ('completed','legacy_approved','manually_cleared');
	`, userID, taxYear, tinType, providerStatus)
	if err != nil {
		return false, fmt.Errorf("error completing w9 filing for %s: %w", userID, err)
	}
	return tag.RowsAffected() > 0, nil
}

// ManuallyClearW9Filing is the admin override, for the cases a vendor cannot
// resolve. A reason is required because this bypasses the entire control.
func (a *AppDB) ManuallyClearW9Filing(ctx context.Context, userID string, taxYear int, adminUserID string, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("a reason is required to clear a w9 filing manually")
	}
	if _, err := a.db.Exec(ctx, `
		INSERT INTO w9_filings (user_id, tax_year, status, completed_at, cleared_by_user_id, clear_reason)
		VALUES ($1, $2, 'manually_cleared', NOW(), $3, $4)
		ON CONFLICT (user_id, tax_year) DO UPDATE
		SET status = 'manually_cleared', completed_at = COALESCE(w9_filings.completed_at, NOW()),
			cleared_by_user_id = EXCLUDED.cleared_by_user_id,
			clear_reason = EXCLUDED.clear_reason, updated_at = NOW();
	`, userID, taxYear, adminUserID, reason); err != nil {
		return fmt.Errorf("error clearing w9 filing for %s: %w", userID, err)
	}
	return nil
}

// GetUserIDByProviderRequest resolves a webhook back to a person.
func (a *AppDB) GetUserIDByProviderRequest(ctx context.Context, providerRequestID string) (string, int, error) {
	var userID string
	var taxYear int
	err := a.db.QueryRow(ctx, `
		SELECT user_id, tax_year FROM w9_filings WHERE provider_request_id = $1 LIMIT 1;
	`, providerRequestID).Scan(&userID, &taxYear)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("error resolving w9 request %q: %w", providerRequestID, err)
	}
	return userID, taxYear, nil
}

// ListW9FilingsAwaitingProvider backs the poller. Polling is the guarantee that
// a dropped webhook cannot hold someone's money indefinitely.
func (a *AppDB) ListW9FilingsAwaitingProvider(ctx context.Context, limit int) ([]*structs.W9Filing, error) {
	if limit <= 0 {
		limit = 100
	}
	// Completed filings stay in the poll set until their TIN match resolves.
	// Release happens on the signature, but a rejected match still has to be
	// recorded — and it only ever arrives on a later poll, so stopping at
	// 'completed' would mean never hearing about it.
	rows, err := a.db.Query(ctx,
		`SELECT `+w9FilingColumns+`
		 FROM w9_filings
		 WHERE provider_request_id <> ''
		 AND (
		   status IN ('requested','in_progress')
		   OR (status = 'completed' AND COALESCE(tin_match,'') IN ('', 'pending'))
		 )
		 ORDER BY COALESCE(last_provider_event_at, requested_at) ASC NULLS FIRST
		 LIMIT $1;`, limit)
	if err != nil {
		return nil, fmt.Errorf("error listing w9 filings awaiting the provider: %w", err)
	}
	defer rows.Close()

	out := []*structs.W9Filing{}
	for rows.Next() {
		filing, err := scanW9Filing(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning w9 filing: %w", err)
		}
		out = append(out, filing)
	}
	return out, rows.Err()
}

// UpsertTaxPayee stores the vendor's payee handle for a person.
func (a *AppDB) UpsertTaxPayee(ctx context.Context, userID string, provider string, payeeID string) error {
	if _, err := a.db.Exec(ctx, `
		INSERT INTO tax_payees (user_id, provider, provider_payee_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET provider = EXCLUDED.provider, provider_payee_id = EXCLUDED.provider_payee_id, updated_at = NOW();
	`, userID, provider, payeeID); err != nil {
		return fmt.Errorf("error saving tax payee for %s: %w", userID, err)
	}
	return nil
}

func (a *AppDB) GetTaxPayeeID(ctx context.Context, userID string) (string, error) {
	var payeeID string
	err := a.db.QueryRow(ctx,
		`SELECT provider_payee_id FROM tax_payees WHERE user_id = $1;`, userID).Scan(&payeeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("error loading tax payee for %s: %w", userID, err)
	}
	return payeeID, nil
}

// RecordProviderEvent stores a webhook and reports whether it is new.
//
// The unique index on (provider, event_id) is the idempotency boundary: a
// vendor redelivering the same event gets fresh=false and nothing happens
// twice.
func (a *AppDB) RecordProviderEvent(ctx context.Context, provider string, eventID string, eventType string, requestID string, payload []byte) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		INSERT INTO w9_provider_events (provider, event_id, event_type, provider_request_id, payload)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (provider, event_id) DO NOTHING;
	`, provider, eventID, eventType, requestID, string(payload))
	if err != nil {
		return false, fmt.Errorf("error recording provider event %q: %w", eventID, err)
	}
	return tag.RowsAffected() > 0, nil
}

func (a *AppDB) MarkProviderEventProcessed(ctx context.Context, provider string, eventID string, processErr string) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE w9_provider_events SET processed_at = NOW(), process_error = $3
		WHERE provider = $1 AND event_id = $2;
	`, provider, eventID, processErr); err != nil {
		return fmt.Errorf("error marking provider event %q processed: %w", eventID, err)
	}
	return nil
}

// ShouldSendEscrowReminder claims a reminder slot, so a restarted sweeper does
// not push the same nudge twice.
func (a *AppDB) ShouldSendEscrowReminder(ctx context.Context, userID string, taxYear int, seq int) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		INSERT INTO w9_escrow_reminders (user_id, tax_year, reminder_seq)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;
	`, userID, taxYear, seq)
	if err != nil {
		return false, fmt.Errorf("error claiming escrow reminder for %s: %w", userID, err)
	}
	return tag.RowsAffected() > 0, nil
}

// RecordTINMatch stores the outcome of the vendor's asynchronous check.
//
// A rejected match does not undo a released payout — that money is already with
// the person, and taking it back because a background check disagreed a day
// later would be indefensible. Instead the filing is marked invalid, which stops
// it clearing the NEXT payout, and the person is asked for a corrected W-9.
func (a *AppDB) RecordTINMatch(ctx context.Context, userID string, taxYear int, match string) (bool, error) {
	if strings.TrimSpace(match) == "" {
		return false, nil
	}

	tag, err := a.db.Exec(ctx, `
		UPDATE w9_filings
		SET tin_match = $3,
			tin_match_at = NOW(),
			-- Only a rejection changes the filing's standing, and only forward:
			-- it stops clearing future payouts. Nothing already paid is touched.
			status = CASE WHEN $3 = 'rejected' THEN 'invalid' ELSE status END,
			updated_at = NOW()
		WHERE user_id = $1 AND tax_year = $2
		AND COALESCE(tin_match, '') IS DISTINCT FROM $3;
	`, userID, taxYear, match)
	if err != nil {
		return false, fmt.Errorf("error recording the tin match for %s: %w", userID, err)
	}
	return tag.RowsAffected() > 0, nil
}
