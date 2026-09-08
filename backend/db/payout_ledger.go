package db

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// TaxSchemaDDL is the baseline definition of the tax and payout tables.
//
// Shared by CreateTables and by migration 1.41 so a fresh database and an
// upgraded one cannot end up with different schemas — a class of bug that only
// ever shows up in production, months later, as a missing column.
const TaxSchemaDDL = `
		-- The vendor holds the tax identity. We hold a pointer to it, and
		-- nothing else: no name, no address, and above all no TIN. Keeping the
		-- payee separate from the yearly filing is what lets a 1099 be filed
		-- later against the same vendor record without asking anyone to refile.
		CREATE TABLE IF NOT EXISTS tax_payees (
			user_id           TEXT PRIMARY KEY,
			provider          TEXT NOT NULL DEFAULT '',
			provider_payee_id TEXT NOT NULL DEFAULT '',
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE UNIQUE INDEX IF NOT EXISTS tax_payees_provider_payee_idx
			ON tax_payees (provider, provider_payee_id)
			WHERE provider_payee_id <> '';

		-- One row per person per tax year. This is the answer to "may we pay
		-- this person yet".
		--
		-- legacy_approved carries the approvals from the old system: those
		-- people stay unblocked and are never asked to refile, but there is no
		-- TIN behind the record, so it cannot support a 1099 without one.
		CREATE TABLE IF NOT EXISTS w9_filings (
			id                     BIGSERIAL PRIMARY KEY,
			user_id                TEXT NOT NULL,
			tax_year               INTEGER NOT NULL,
			status                 TEXT NOT NULL DEFAULT 'not_started',
			provider               TEXT NOT NULL DEFAULT '',
			provider_request_id    TEXT NOT NULL DEFAULT '',
			form_url               TEXT NOT NULL DEFAULT '',
			form_url_expires_at    TIMESTAMPTZ,
			threshold_crossed_at   TIMESTAMPTZ,
			requested_at           TIMESTAMPTZ,
			completed_at           TIMESTAMPTZ,
			tin_type               TEXT NOT NULL DEFAULT '',
			-- The TIN match is asynchronous and independent of signing, so it is
			-- recorded separately. Release happens on the signature.
			tin_match              TEXT NOT NULL DEFAULT '',
			tin_match_at           TIMESTAMPTZ,
			cleared_by_user_id     TEXT NOT NULL DEFAULT '',
			clear_reason           TEXT NOT NULL DEFAULT '',
			last_provider_status   TEXT NOT NULL DEFAULT '',
			last_provider_event_at TIMESTAMPTZ,
			-- When we last ASKED, as opposed to when the answer last changed.
			-- last_provider_event_at only moves on completion, so it cannot
			-- pace the sweep: a filing that never changes looks equally stale
			-- forever and gets re-read on every pass.
			last_polled_at         TIMESTAMPTZ,
			created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT w9_filings_status_check CHECK (status IN (
				'not_started','requested','in_progress','completed',
				'legacy_approved','manually_cleared','invalid'
			))
		);

		CREATE UNIQUE INDEX IF NOT EXISTS w9_filings_user_year_idx
			ON w9_filings (user_id, tax_year);
		CREATE INDEX IF NOT EXISTS w9_filings_status_idx ON w9_filings (status);
		CREATE INDEX IF NOT EXISTS w9_filings_year_status_idx ON w9_filings (tax_year, status);
		CREATE INDEX IF NOT EXISTS w9_filings_request_idx
			ON w9_filings (provider_request_id) WHERE provider_request_id <> '';

		-- Every platform-originated payout, whether it went out or is being
		-- held. Escrow is a state here rather than a second table: with two
		-- tables "what we paid" and "what we owe" can disagree, and releasing
		-- becomes a cross-table move that has to stay in step with an on-chain
		-- send. Here releasing is an UPDATE.
		--
		-- tax_year is the year earned and drives the threshold. paid_tax_year
		-- is the year the money actually moved, which is what a 1099 reports.
		-- They differ whenever a payout is held across new year.
		CREATE TABLE IF NOT EXISTS payout_ledger (
			id                      BIGSERIAL PRIMARY KEY,
			idempotency_key         TEXT NOT NULL,
			user_id                 TEXT,
			recipient_address       TEXT NOT NULL,
			chain_id                BIGINT NOT NULL DEFAULT 0,
			tax_year                INTEGER NOT NULL,
			paid_tax_year           INTEGER,
			source                  TEXT NOT NULL,
			source_ref              TEXT NOT NULL DEFAULT '',
			amount_base             NUMERIC(78,0) NOT NULL,
			state                   TEXT NOT NULL,
			escrowed_at             TIMESTAMPTZ,
			expired_at              TIMESTAMPTZ,
			back_pay_requested_at   TIMESTAMPTZ,
			released_at             TIMESTAMPTZ,
			paid_at                 TIMESTAMPTZ,
			tx_hash                 TEXT NOT NULL DEFAULT '',
			tx_chain_id             BIGINT,
			attempt_started_at      TIMESTAMPTZ,
			attempts                INTEGER NOT NULL DEFAULT 0,
			last_error              TEXT NOT NULL DEFAULT '',
			counts_toward_threshold BOOLEAN NOT NULL DEFAULT TRUE,
			shadow_decision         TEXT NOT NULL DEFAULT '',
			created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			-- No 'expired' and no 'back_pay_requested'. Escrow holds exactly one
			-- payment — the next is refused, not held — so nothing accumulates
			-- and nothing has to lapse into an owed-money queue.
			CONSTRAINT payout_ledger_state_check CHECK (state IN (
				'pending','escrowed','releasing','paid','failed','cancelled'
			))
		);

		CREATE UNIQUE INDEX IF NOT EXISTS payout_ledger_idempotency_idx
			ON payout_ledger (idempotency_key);
		-- One payout per source record — but only among rows that still stand.
		-- A failed attempt must not hold the slot forever: the faucet being
		-- misconfigured for one call would otherwise make that redemption code
		-- permanently unredeemable, with no way back short of hand-editing the
		-- ledger.
		CREATE UNIQUE INDEX IF NOT EXISTS payout_ledger_source_ref_idx
			ON payout_ledger (source, source_ref)
			WHERE source_ref <> '' AND state NOT IN ('failed', 'cancelled');
		CREATE INDEX IF NOT EXISTS payout_ledger_user_year_idx
			ON payout_ledger (user_id, tax_year) WHERE counts_toward_threshold;
		CREATE INDEX IF NOT EXISTS payout_ledger_open_state_idx
			ON payout_ledger (state)
			WHERE state IN ('escrowed','releasing','pending');
		CREATE INDEX IF NOT EXISTS payout_ledger_escrowed_at_idx
			ON payout_ledger (state, escrowed_at);
		CREATE INDEX IF NOT EXISTS payout_ledger_recipient_year_idx
			ON payout_ledger (recipient_address, tax_year);
		CREATE INDEX IF NOT EXISTS payout_ledger_tx_hash_idx
			ON payout_ledger (tx_hash) WHERE tx_hash <> '';

		-- Escrow that is still reserved, and subtracted from the faucet's
		-- spendable balance. It is money already owed to somebody.
		CREATE OR REPLACE VIEW escrowed_payouts AS
			SELECT * FROM payout_ledger WHERE state = 'escrowed';

		-- The webhook inbox. The unique constraint is the idempotency boundary:
		-- a vendor redelivering an event cannot release the same escrow twice.
		CREATE TABLE IF NOT EXISTS w9_provider_events (
			id                  BIGSERIAL PRIMARY KEY,
			provider            TEXT NOT NULL,
			event_id            TEXT NOT NULL,
			event_type          TEXT NOT NULL DEFAULT '',
			provider_request_id TEXT NOT NULL DEFAULT '',
			payload             JSONB NOT NULL DEFAULT '{}'::jsonb,
			received_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			processed_at        TIMESTAMPTZ,
			process_error       TEXT NOT NULL DEFAULT ''
		);

		CREATE UNIQUE INDEX IF NOT EXISTS w9_provider_events_unique_idx
			ON w9_provider_events (provider, event_id);
		CREATE INDEX IF NOT EXISTS w9_provider_events_unprocessed_idx
			ON w9_provider_events (received_at) WHERE processed_at IS NULL;

		-- Which warning tiers a person has reached, and whether they have seen
		-- each one.
		--
		-- The escalation is the point: a polite notice at the first line, a
		-- firmer one at the second, and only then does money stop. Nobody's
		-- first indication that a tax form exists should be their reward going
		-- missing.
		CREATE TABLE IF NOT EXISTS w9_tier_notices (
			user_id         TEXT NOT NULL,
			tax_year        INTEGER NOT NULL,
			tier            TEXT NOT NULL,
			notified_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			acknowledged_at TIMESTAMPTZ,
			PRIMARY KEY (user_id, tax_year, tier)
		);

		CREATE INDEX IF NOT EXISTS w9_tier_notices_outstanding_idx
			ON w9_tier_notices (user_id, tax_year) WHERE acknowledged_at IS NULL;

		-- Reminder dedup, same shape as volunteer_reminder_sends.
		CREATE TABLE IF NOT EXISTS w9_escrow_reminders (
			user_id      TEXT NOT NULL,
			tax_year     INTEGER NOT NULL,
			reminder_seq INTEGER NOT NULL,
			sent_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, tax_year, reminder_seq)
		);

		-- Tax records outlive the account they belong to. Apple's account
		-- deletion rule exempts data a developer is legally required to keep,
		-- and the IRS expects a payer to be able to reconstruct 1099 data for
		-- three years, or four where backup withholding applied.
		--
		-- So on deletion the ledger is pseudonymised rather than dropped: the
		-- money trail survives under an opaque key, the person does not. This
		-- table is what remembers when that trail may finally go, and which
		-- vendor payee has to be deleted at the same time — the vendor is the
		-- only place a real TIN ever lived.
		CREATE TABLE IF NOT EXISTS tax_retention_records (
			retention_key     TEXT PRIMARY KEY,
			tax_years         INTEGER[] NOT NULL DEFAULT '{}',
			provider          TEXT NOT NULL DEFAULT '',
			provider_payee_id TEXT NOT NULL DEFAULT '',
			purge_after       DATE NOT NULL,
			provider_deleted_at TIMESTAMPTZ,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS tax_retention_records_purge_idx
			ON tax_retention_records (purge_after);
	`

// Ledger states. Escrow is a state here rather than a separate table so that
// "what we owe" and "what we paid" cannot drift apart.
const (
	PayoutStatePending          = "pending"
	PayoutStateEscrowed         = "escrowed"
	PayoutStateExpired          = "expired"
	PayoutStateBackPayRequested = "back_pay_requested"
	PayoutStateReleasing        = "releasing"
	PayoutStatePaid             = "paid"
	PayoutStateFailed           = "failed"
	PayoutStateCancelled        = "cancelled"
)

// Where a payout came from. Used to settle the originating record once the
// money actually moves.
const (
	PayoutSourceRedemptionCode  = "redemption_code"
	PayoutSourceWorkflowStep    = "workflow_step"
	PayoutSourceWorkflowManager = "workflow_manager"
	PayoutSourceAdminManual     = "admin_manual"
	PayoutSourceHistorical      = "historical_ponder"
)

// Filing states.
const (
	W9StatusNotStarted      = "not_started"
	W9StatusRequested       = "requested"
	W9StatusInProgress      = "in_progress"
	W9StatusCompleted       = "completed"
	W9StatusLegacyApproved  = "legacy_approved"
	W9StatusManuallyCleared = "manually_cleared"
	W9StatusInvalid         = "invalid"
)

// W9StatusClears reports whether a filing state means we may pay this person.
//
// legacy_approved is included: those people completed what the old system asked
// of them, and re-blocking them because we changed our storage would be
// punishing them for our refactor.
func W9StatusClears(status string) bool {
	switch status {
	case W9StatusCompleted, W9StatusLegacyApproved, W9StatusManuallyCleared:
		return true
	default:
		return false
	}
}

// InsertPayoutIntent records a payout before any decision is made about it.
//
// The idempotency key is the whole point: a caller that retries — a redemption
// replayed, a workflow sweeper running twice — gets the original row back
// rather than a second obligation. Returns created=false when the row already
// existed.
func (a *AppDB) InsertPayoutIntent(ctx context.Context, tx pgx.Tx, row *structs.PayoutLedgerRow) (created bool, err error) {
	query := `
		INSERT INTO payout_ledger (
			idempotency_key, user_id, recipient_address, chain_id, tax_year,
			source, source_ref, amount_base, state, counts_toward_threshold
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id;
	`
	var userID any
	if strings.TrimSpace(row.UserID) != "" {
		userID = row.UserID
	}

	var id int64
	scanErr := tx.QueryRow(ctx, query,
		row.IdempotencyKey, userID, row.RecipientAddress, row.ChainID, row.TaxYear,
		row.Source, row.SourceRef, row.AmountBase, PayoutStatePending, row.CountsTowardThreshold,
	).Scan(&id)

	if errors.Is(scanErr, pgx.ErrNoRows) {
		existing, err := a.getPayoutByIdempotencyKeyTx(ctx, tx, row.IdempotencyKey)
		if err != nil {
			return false, err
		}
		*row = *existing
		return false, nil
	}
	if scanErr != nil {
		return false, fmt.Errorf("error recording payout intent: %w", scanErr)
	}

	row.ID = id
	row.State = PayoutStatePending
	return true, nil
}

const payoutLedgerColumns = `
	id, idempotency_key, COALESCE(user_id, ''), recipient_address, chain_id,
	tax_year, COALESCE(paid_tax_year, 0), source, source_ref, amount_base::text,
	state, escrowed_at, expired_at, released_at, paid_at, tx_hash,
	attempts, last_error, counts_toward_threshold, created_at
`

func scanPayoutRow(row interface{ Scan(...any) error }) (*structs.PayoutLedgerRow, error) {
	var out structs.PayoutLedgerRow
	var escrowedAt, expiredAt, releasedAt, paidAt *time.Time
	if err := row.Scan(
		&out.ID, &out.IdempotencyKey, &out.UserID, &out.RecipientAddress, &out.ChainID,
		&out.TaxYear, &out.PaidTaxYear, &out.Source, &out.SourceRef, &out.AmountBase,
		&out.State, &escrowedAt, &expiredAt, &releasedAt, &paidAt, &out.TxHash,
		&out.Attempts, &out.LastError, &out.CountsTowardThreshold, &out.CreatedAt,
	); err != nil {
		return nil, err
	}
	out.EscrowedAt, out.ExpiredAt, out.ReleasedAt, out.PaidAt = escrowedAt, expiredAt, releasedAt, paidAt
	return &out, nil
}

func (a *AppDB) getPayoutByIdempotencyKeyTx(ctx context.Context, tx pgx.Tx, key string) (*structs.PayoutLedgerRow, error) {
	row, err := scanPayoutRow(tx.QueryRow(ctx,
		`SELECT `+payoutLedgerColumns+` FROM payout_ledger WHERE idempotency_key = $1;`, key))
	if err != nil {
		return nil, fmt.Errorf("error loading payout %q: %w", key, err)
	}
	return row, nil
}

// LockUserTaxYear serialises threshold decisions for one person and year.
//
// Without it two payouts landing together can both read a below-threshold total
// and both decide to pay, letting someone through the limit. An advisory lock
// is used rather than SELECT FOR UPDATE because on the first payout there is no
// filing row to lock. It is released when the transaction ends.
func LockUserTaxYear(ctx context.Context, tx pgx.Tx, userID string, taxYear int) error {
	if strings.TrimSpace(userID) == "" {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1)::bigint);`,
		fmt.Sprintf("w9:%s:%d", userID, taxYear),
	); err != nil {
		return fmt.Errorf("error locking tax year for %s: %w", userID, err)
	}
	return nil
}

// SumCountedPayoutsForUserYear totals what this person has been credited with
// this year — paid, held, expired or awaiting back pay.
//
// Expired and back-pay rows are included deliberately: the money was earned,
// and letting an escrow window lapse must not quietly reset someone's progress
// toward the threshold.
func (a *AppDB) SumCountedPayoutsForUserYear(ctx context.Context, tx pgx.Tx, userID string, taxYear int) (*big.Int, error) {
	if strings.TrimSpace(userID) == "" {
		return big.NewInt(0), nil
	}

	var total string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_base), 0)::text
		FROM payout_ledger
		WHERE user_id = $1
		AND tax_year = $2
		AND counts_toward_threshold = TRUE
		-- 'pending' counts. A pending row is a committed intent to pay that has
		-- not reached the chain yet; leaving it out lets a second payout land in
		-- that window, read a stale total, and decide it is also below the line.
		-- Rows that go on to fail are marked 'failed' and stop counting.
		AND state IN ('pending','escrowed','releasing','paid');
	`, userID, taxYear).Scan(&total); err != nil {
		return nil, fmt.Errorf("error summing payouts for %s: %w", userID, err)
	}

	parsed, ok := new(big.Int).SetString(total, 10)
	if !ok {
		return nil, fmt.Errorf("unreadable payout total %q for %s", total, userID)
	}
	return parsed, nil
}

// HasOpenEscrow reports whether this person already has money held or owed for
// the year. Once they do, everything after is held too — a payment that slips
// through between two held ones would be incoherent.
func (a *AppDB) HasOpenEscrow(ctx context.Context, tx pgx.Tx, userID string, taxYear int) (bool, error) {
	if strings.TrimSpace(userID) == "" {
		return false, nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM payout_ledger
			WHERE user_id = $1 AND tax_year = $2
			AND state IN ('escrowed','releasing')
		);
	`, userID, taxYear).Scan(&exists); err != nil {
		return false, fmt.Errorf("error checking open escrow for %s: %w", userID, err)
	}
	return exists, nil
}

func (a *AppDB) MarkPayoutEscrowed(ctx context.Context, tx pgx.Tx, id int64) error {
	if _, err := tx.Exec(ctx, `
		UPDATE payout_ledger
		SET state = 'escrowed', escrowed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND state = 'pending';
	`, id); err != nil {
		return fmt.Errorf("error escrowing payout %d: %w", id, err)
	}
	return nil
}

// MarkPayoutShadowDecision records what would have happened without changing
// what does. Shadow mode is how a new gate is proven against real traffic
// before it is allowed to hold anyone's money.
func (a *AppDB) MarkPayoutShadowDecision(ctx context.Context, tx pgx.Tx, id int64, decision string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE payout_ledger SET shadow_decision = $2, updated_at = NOW() WHERE id = $1;
	`, id, decision); err != nil {
		return fmt.Errorf("error recording shadow decision for payout %d: %w", id, err)
	}
	return nil
}

func (a *AppDB) RecordPayoutTxHash(ctx context.Context, id int64, txHash string, chainID int64) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE payout_ledger SET tx_hash = $2, tx_chain_id = $3, updated_at = NOW() WHERE id = $1;
	`, id, txHash, chainID); err != nil {
		return fmt.Errorf("error recording tx hash for payout %d: %w", id, err)
	}
	return nil
}

func (a *AppDB) MarkPayoutPaid(ctx context.Context, id int64, txHash string, paidTaxYear int) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE payout_ledger
		SET state = 'paid', paid_at = NOW(), released_at = COALESCE(released_at, NOW()),
		    tx_hash = COALESCE(NULLIF($2, ''), tx_hash), paid_tax_year = $3,
		    last_error = '', updated_at = NOW()
		WHERE id = $1;
	`, id, txHash, paidTaxYear); err != nil {
		return fmt.Errorf("error marking payout %d paid: %w", id, err)
	}
	return nil
}

// ReturnPayoutToEscrow puts a failed release back where it came from. A failure
// must never lose the row: the money is still owed.
func (a *AppDB) ReturnPayoutToEscrow(ctx context.Context, id int64, previousState string, reason string) error {
	if previousState != PayoutStateEscrowed && previousState != PayoutStateBackPayRequested {
		previousState = PayoutStateEscrowed
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE payout_ledger
		SET state = $2, last_error = $3, attempt_started_at = NULL, updated_at = NOW()
		WHERE id = $1 AND state = 'releasing';
	`, id, previousState, reason); err != nil {
		return fmt.Errorf("error returning payout %d to %s (%s): %w", id, previousState, reason, err)
	}
	return nil
}

// ClaimEscrowedPayoutsForRelease moves this person's held rows to 'releasing'
// and hands them back. The state change is the lock — two sweepers cannot claim
// the same row, which is the same trick the workflow payout code already uses.
func (a *AppDB) ClaimEscrowedPayoutsForRelease(ctx context.Context, userID string, taxYear int, states []string) ([]*structs.PayoutLedgerRow, error) {
	rows, err := a.db.Query(ctx, `
		UPDATE payout_ledger
		SET state = 'releasing', attempt_started_at = NOW(), attempts = attempts + 1,
		    released_at = NOW(), updated_at = NOW()
		WHERE id IN (
			SELECT id FROM payout_ledger
			WHERE user_id = $1 AND tax_year = $2 AND state = ANY($3)
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+payoutLedgerColumns+`;
	`, userID, taxYear, states)
	if err != nil {
		return nil, fmt.Errorf("error claiming payouts for release: %w", err)
	}
	defer rows.Close()

	claimed := []*structs.PayoutLedgerRow{}
	for rows.Next() {
		row, err := scanPayoutRow(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning claimed payout: %w", err)
		}
		claimed = append(claimed, row)
	}
	return claimed, rows.Err()
}

// ExpireEscrowedPayouts ends the automatic window.
//
// Money stops being reserved against the faucet and the obligation becomes a
// back-pay claim instead. It is not forgiven — the row survives, still counts
// toward the threshold, and is still owed. It just stops being something the
// system will pay on its own.
func (a *AppDB) ExpireEscrowedPayouts(ctx context.Context, olderThan time.Time) ([]*structs.PayoutLedgerRow, error) {
	rows, err := a.db.Query(ctx, `
		UPDATE payout_ledger
		SET state = 'expired', expired_at = NOW(), updated_at = NOW()
		WHERE state = 'escrowed' AND escrowed_at IS NOT NULL AND escrowed_at < $1
		RETURNING `+payoutLedgerColumns+`;
	`, olderThan)
	if err != nil {
		return nil, fmt.Errorf("error expiring escrowed payouts: %w", err)
	}
	defer rows.Close()

	expired := []*structs.PayoutLedgerRow{}
	for rows.Next() {
		row, err := scanPayoutRow(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning expired payout: %w", err)
		}
		expired = append(expired, row)
	}
	return expired, rows.Err()
}

// RequestBackPayForUserYear turns expired rows into an admin queue item. Called
// when someone finally files: they get their still-held money automatically,
// and everything that lapsed becomes a claim for an admin to approve.
func (a *AppDB) RequestBackPayForUserYear(ctx context.Context, userID string, taxYear int) (int64, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE payout_ledger
		SET state = 'back_pay_requested', back_pay_requested_at = NOW(), updated_at = NOW()
		WHERE user_id = $1 AND tax_year = $2 AND state = 'expired';
	`, userID, taxYear)
	if err != nil {
		return 0, fmt.Errorf("error requesting back pay for %s: %w", userID, err)
	}
	return tag.RowsAffected(), nil
}

// EscrowedTotalBase is the money we are holding and have not yet paid.
//
// Only 'escrowed' counts. Expired and back-pay rows are obligations but are no
// longer reserved — that is the whole point of the expiry window, and the
// reason the faucet can be spent down and topped back up before a back pay.
func (a *AppDB) EscrowedTotalBase(ctx context.Context) (*big.Int, error) {
	var total string
	if err := a.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_base), 0)::text
		FROM payout_ledger WHERE state IN ('escrowed','releasing');
	`).Scan(&total); err != nil {
		return nil, fmt.Errorf("error totalling escrowed payouts: %w", err)
	}
	parsed, ok := new(big.Int).SetString(total, 10)
	if !ok {
		return nil, fmt.Errorf("unreadable escrow total %q", total)
	}
	return parsed, nil
}

// OutstandingBackPayTotalBase is owed but unreserved. Shown next to the escrow
// figure so an admin can top the faucet up before approving a claim.
func (a *AppDB) OutstandingBackPayTotalBase(ctx context.Context) (*big.Int, error) {
	var total string
	if err := a.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_base), 0)::text
		FROM payout_ledger WHERE state IN ('expired','back_pay_requested');
	`).Scan(&total); err != nil {
		return nil, fmt.Errorf("error totalling outstanding back pay: %w", err)
	}
	parsed, ok := new(big.Int).SetString(total, 10)
	if !ok {
		return nil, fmt.Errorf("unreadable back pay total %q", total)
	}
	return parsed, nil
}

// AttributeUnlinkedPayouts assigns past payouts to a user once one of their
// wallets is linked.
//
// Redemption is unauthenticated, so a QR can be scanned to an address belonging
// to no account, and those payouts go through ungated because there is nobody
// to ask for a form. This closes that gap the moment identity appears: without
// it, redeeming to an unlinked address and linking it afterwards would be a
// permanent way around the threshold.
func (a *AppDB) AttributeUnlinkedPayouts(ctx context.Context, userID string, addresses []string) (int64, error) {
	if strings.TrimSpace(userID) == "" || len(addresses) == 0 {
		return 0, nil
	}
	lowered := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if trimmed := strings.ToLower(strings.TrimSpace(address)); trimmed != "" {
			lowered = append(lowered, trimmed)
		}
	}
	if len(lowered) == 0 {
		return 0, nil
	}

	tag, err := a.db.Exec(ctx, `
		UPDATE payout_ledger
		SET user_id = $1, updated_at = NOW()
		WHERE user_id IS NULL AND LOWER(TRIM(recipient_address)) = ANY($2);
	`, userID, lowered)
	if err != nil {
		return 0, fmt.Errorf("error attributing payouts to %s: %w", userID, err)
	}
	return tag.RowsAffected(), nil
}

// ListPayoutsForUserYear backs the escrow panel a person sees.
func (a *AppDB) ListPayoutsForUserYear(ctx context.Context, userID string, taxYear int, states []string) ([]*structs.PayoutLedgerRow, error) {
	rows, err := a.db.Query(ctx,
		`SELECT `+payoutLedgerColumns+`
		 FROM payout_ledger
		 WHERE user_id = $1 AND tax_year = $2 AND state = ANY($3)
		 ORDER BY created_at ASC;`, userID, taxYear, states)
	if err != nil {
		return nil, fmt.Errorf("error listing payouts for %s: %w", userID, err)
	}
	defer rows.Close()

	out := []*structs.PayoutLedgerRow{}
	for rows.Next() {
		row, err := scanPayoutRow(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning payout row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// BeginTx opens a transaction for callers outside this package.
//
// The payout gate needs the ledger write, the per-person lock, the threshold
// read and the escrow decision to be one atomic step; without that, two
// payouts landing together can both decide they are below the line.
func (a *AppDB) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return a.db.Begin(ctx)
}

// EscrowHolder is one person with money held or owed.
type EscrowHolder struct {
	UserID           string
	TaxYear          int
	OldestEscrowedAt *time.Time
	EscrowedBase     string
	EscrowedCount    int
}

// ListUsersWithOpenEscrow backs the reminder sweep.
func (a *AppDB) ListUsersWithOpenEscrow(ctx context.Context) ([]EscrowHolder, error) {
	rows, err := a.db.Query(ctx, `
		SELECT user_id, tax_year, MIN(escrowed_at), COALESCE(SUM(amount_base),0)::text, COUNT(*)
		FROM payout_ledger
		WHERE user_id IS NOT NULL AND state IN ('escrowed','releasing')
		GROUP BY user_id, tax_year;
	`)
	if err != nil {
		return nil, fmt.Errorf("error listing people with held money: %w", err)
	}
	defer rows.Close()

	out := []EscrowHolder{}
	for rows.Next() {
		var holder EscrowHolder
		if err := rows.Scan(&holder.UserID, &holder.TaxYear, &holder.OldestEscrowedAt,
			&holder.EscrowedBase, &holder.EscrowedCount); err != nil {
			return nil, fmt.Errorf("error scanning escrow holder: %w", err)
		}
		out = append(out, holder)
	}
	return out, rows.Err()
}

// ListUsersWithHeldMoneyAndClearedFilings finds escrow that should already have
// gone out — a release interrupted by a crash, or one whose transfer was
// failing when the filing completed. Without this pass that money waits for a
// human to notice.
func (a *AppDB) ListUsersWithHeldMoneyAndClearedFilings(ctx context.Context) ([]EscrowHolder, error) {
	rows, err := a.db.Query(ctx, `
		SELECT p.user_id, p.tax_year, MIN(p.escrowed_at), COALESCE(SUM(p.amount_base),0)::text, COUNT(*)
		FROM payout_ledger p
		JOIN w9_filings f ON f.user_id = p.user_id AND f.tax_year = p.tax_year
		WHERE p.state = 'escrowed'
		AND f.status IN ('completed','legacy_approved','manually_cleared')
		GROUP BY p.user_id, p.tax_year;
	`)
	if err != nil {
		return nil, fmt.Errorf("error looking for unreleased escrow: %w", err)
	}
	defer rows.Close()

	out := []EscrowHolder{}
	for rows.Next() {
		var holder EscrowHolder
		if err := rows.Scan(&holder.UserID, &holder.TaxYear, &holder.OldestEscrowedAt,
			&holder.EscrowedBase, &holder.EscrowedCount); err != nil {
			return nil, fmt.Errorf("error scanning stuck escrow: %w", err)
		}
		out = append(out, holder)
	}
	return out, rows.Err()
}

// SumUserEscrowAndBackPay returns what is held and what is owed for one person.
func (a *AppDB) SumUserEscrowAndBackPay(ctx context.Context, userID string, taxYear int) (escrowed *big.Int, escrowedCount int, backPay *big.Int, backPayCount int, err error) {
	var escrowedStr, backPayStr string
	if err := a.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount_base) FILTER (WHERE state IN ('escrowed','releasing')), 0)::text,
			COUNT(*) FILTER (WHERE state IN ('escrowed','releasing')),
			'0'::text,
			0
		FROM payout_ledger WHERE user_id = $1 AND tax_year = $2;
	`, userID, taxYear).Scan(&escrowedStr, &escrowedCount, &backPayStr, &backPayCount); err != nil {
		return nil, 0, nil, 0, fmt.Errorf("error summing holds for %s: %w", userID, err)
	}
	escrowed, _ = new(big.Int).SetString(escrowedStr, 10)
	backPay, _ = new(big.Int).SetString(backPayStr, 10)
	if escrowed == nil {
		escrowed = big.NewInt(0)
	}
	if backPay == nil {
		backPay = big.NewInt(0)
	}
	return escrowed, escrowedCount, backPay, backPayCount, nil
}

// SumUserEarnedForYear is the annual total the threshold is measured against —
// across every wallet the person holds and every chain, which is precisely what
// the per-wallet, per-chain design that preceded this could not do.
//
// Counts 'pending', matching sumPayoutsForUserYear. A tier is recorded before
// the transfer, deliberately, so the modal is waiting by the time somebody
// looks at the reward that triggered it — which meant this sum was read during
// the window where the row exists but has not reached the chain. The modal
// opened reading "0 of 600", then corrected itself a second later once the row
// turned 'paid'. A pending row is a committed intent to pay; rows that go on to
// fail become 'failed' and stop counting here, so nothing is over-reported for
// longer than the send takes.
func (a *AppDB) SumUserEarnedForYear(ctx context.Context, userID string, taxYear int) (*big.Int, error) {
	var total string
	if err := a.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_base), 0)::text
		FROM payout_ledger
		WHERE user_id = $1 AND tax_year = $2 AND counts_toward_threshold = TRUE
		AND state IN ('pending','escrowed','releasing','paid');
	`, userID, taxYear).Scan(&total); err != nil {
		return nil, fmt.Errorf("error summing annual earnings for %s: %w", userID, err)
	}
	parsed, ok := new(big.Int).SetString(total, 10)
	if !ok {
		return nil, fmt.Errorf("unreadable annual total %q", total)
	}
	return parsed, nil
}

// ListW9AdminRows is the per-person table in the Tax & Escrow panel: everyone
// with money held or owed, plus their filing state.
func (a *AppDB) ListW9AdminRows(ctx context.Context, taxYear int) ([]structs.W9AdminRow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			p.user_id,
			COALESCE(NULLIF(TRIM(u.contact_name), ''), ''),
			COALESCE(NULLIF(TRIM(u.contact_email), ''), ''),
			p.tax_year,
			COALESCE(f.status, 'not_started'),
			COALESCE(SUM(p.amount_base) FILTER (
				WHERE p.counts_toward_threshold
				AND p.state IN ('escrowed','expired','back_pay_requested','releasing','paid')), 0)::text,
			COALESCE(SUM(p.amount_base) FILTER (WHERE p.state IN ('escrowed','releasing')), 0)::text,
			COALESCE(SUM(p.amount_base) FILTER (WHERE p.state IN ('expired','back_pay_requested')), 0)::text,
			COUNT(*) FILTER (WHERE p.state IN ('expired','back_pay_requested')),
			MIN(p.escrowed_at) FILTER (WHERE p.state IN ('escrowed','releasing')),
			f.completed_at,
			COUNT(*) FILTER (WHERE p.state = 'back_pay_requested') > 0
		FROM payout_ledger p
		LEFT JOIN users u ON u.id = p.user_id
		LEFT JOIN w9_filings f ON f.user_id = p.user_id AND f.tax_year = p.tax_year
		WHERE p.user_id IS NOT NULL
		AND p.tax_year = $1
		GROUP BY p.user_id, u.contact_name, u.contact_email, p.tax_year, f.status, f.completed_at
		HAVING COUNT(*) FILTER (
			WHERE p.state IN ('escrowed','releasing','expired','back_pay_requested')) > 0
		ORDER BY MIN(p.escrowed_at) ASC NULLS LAST;
	`, taxYear)
	if err != nil {
		return nil, fmt.Errorf("error listing w9 admin rows: %w", err)
	}
	defer rows.Close()

	out := []structs.W9AdminRow{}
	for rows.Next() {
		var row structs.W9AdminRow
		if err := rows.Scan(
			&row.UserID, &row.ContactName, &row.ContactEmail, &row.TaxYear, &row.FilingStatus,
			&row.EarnedSfluv, &row.EscrowedSfluv, &row.BackPaySfluv, &row.BackPayCount,
			&row.OldestEscrowAt, &row.CompletedAt, &row.NeedsBackPayNow,
		); err != nil {
			return nil, fmt.Errorf("error scanning w9 admin row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// MarkPayoutFailed takes a payout out of the running total.
//
// A pending row counts toward the threshold so that a concurrent payout cannot
// slip past it. That is only safe if a payout which never reaches the chain
// stops counting — otherwise one failed send would inflate somebody's annual
// total forever and hold their money for the rest of the year.
func (a *AppDB) MarkPayoutFailed(ctx context.Context, id int64, reason string) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE payout_ledger
		SET state = 'failed', last_error = $2, updated_at = NOW()
		WHERE id = $1 AND state = 'pending';
	`, id, reason); err != nil {
		return fmt.Errorf("error marking payout %d failed: %w", id, err)
	}
	return nil
}

// Exec runs a statement against the app database. Present for tests that need
// to age rows or set up state that no production path would create.
func (a *AppDB) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := a.db.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// List1099Candidates is the year-end answer to "who do we owe a 1099-NEC".
//
// Summed on paid_tax_year, not tax_year. A 1099-NEC is cash-basis: it reports
// what was actually paid during the year, so a reward escrowed in December and
// released in February belongs to the later year. Gating uses the earned year;
// reporting uses this one. Conflating them is how a first filing season goes
// wrong.
//
// Only 'paid' rows count. Escrowed, expired and awaiting-back-pay money has not
// reached anybody, so it is not yet reportable income — it will be, in whatever
// year it lands.
//
// Everyone at or over the threshold is returned, including those we cannot file
// for. A payee who is over the line with no TIN on file is the single most
// important row in this report: it is the one that needs a human before January.
func (a *AppDB) List1099Candidates(ctx context.Context, taxYear int, thresholdBase *big.Int) ([]structs.Form1099Row, error) {
	if thresholdBase == nil || thresholdBase.Sign() <= 0 {
		thresholdBase = big.NewInt(600_000_000)
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			p.user_id,
			COALESCE(NULLIF(TRIM(u.contact_name), ''), ''),
			COALESCE(NULLIF(TRIM(u.contact_email), ''), ''),
			COALESCE(SUM(p.amount_base), 0)::text,
			COUNT(*),
			COALESCE(f.status, 'not_started'),
			COALESCE(f.tin_type, '')
		FROM payout_ledger p
		LEFT JOIN users u ON u.id = p.user_id
		LEFT JOIN w9_filings f ON f.user_id = p.user_id AND f.tax_year = $1
		WHERE p.user_id IS NOT NULL
		AND p.state = 'paid'
		AND p.paid_tax_year = $1
		AND p.counts_toward_threshold = TRUE
		GROUP BY p.user_id, u.contact_name, u.contact_email, f.status, f.tin_type
		ORDER BY SUM(p.amount_base) DESC;
	`, taxYear)
	if err != nil {
		return nil, fmt.Errorf("error listing 1099 candidates for %d: %w", taxYear, err)
	}
	defer rows.Close()

	out := []structs.Form1099Row{}
	for rows.Next() {
		var row structs.Form1099Row
		var paidBase string
		if err := rows.Scan(&row.UserID, &row.ContactName, &row.ContactEmail,
			&paidBase, &row.PayoutCount, &row.FilingStatus, &row.TINType); err != nil {
			return nil, fmt.Errorf("error scanning 1099 candidate: %w", err)
		}
		row.TaxYear = taxYear
		row.PaidSfluv = paidBase

		paid, ok := new(big.Int).SetString(paidBase, 10)
		if !ok {
			paid = big.NewInt(0)
		}
		row.Reportable = paid.Cmp(thresholdBase) >= 0

		// A form needs a tax identification number behind it. A completed filing
		// has one at the vendor; a legacy approval does not — those people were
		// cleared to be paid by the old system, which never collected a TIN.
		switch {
		case !row.Reportable:
			row.Fileable = false
		case row.FilingStatus == W9StatusCompleted:
			row.Fileable = true
		case row.FilingStatus == W9StatusLegacyApproved:
			row.BlockedReason = "cleared under the old system, which never collected a tax ID — needs a W-9 before filing"
		case row.FilingStatus == W9StatusManuallyCleared:
			row.BlockedReason = "cleared manually by an admin, so no tax ID is on file"
		default:
			row.BlockedReason = "no completed W-9 on file"
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// MarkPayoutCancelled records a payout that was refused before any money moved.
//
// Cancelled rows do not count toward the annual total, and that matters: a
// refused payment is not income, and leaving it counted would push the person
// further past a line they were already blocked at — so filing their W-9 would
// release less than they had actually earned.
func (a *AppDB) MarkPayoutCancelled(ctx context.Context, id int64, reason string) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE payout_ledger
		SET state = 'cancelled', last_error = $2, updated_at = NOW()
		WHERE id = $1 AND state = 'pending';
	`, id, reason); err != nil {
		return fmt.Errorf("error cancelling payout %d: %w", id, err)
	}
	return nil
}
