package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func (a *AppDB) GetUserIdByWalletAddress(ctx context.Context, address string) (*string, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			owner
		FROM
			wallets
		WHERE
			LOWER(eoa_address) = LOWER($1)
		OR
			LOWER(smart_address) = LOWER($1)
		LIMIT 1;
	`, address)

	var owner string
	err := row.Scan(&owner)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &owner, nil
}

func (a *AppDB) GetUserContactEmail(ctx context.Context, userId string) (*string, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			contact_email
		FROM
			users
		WHERE
			id = $1;
	`, userId)

	var email sql.NullString
	err := row.Scan(&email)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if email.Valid {
		return &email.String, nil
	}
	return nil, nil
}

func (a *AppDB) GetW9WalletEarning(ctx context.Context, wallet string, year int) (*structs.W9WalletEarning, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			wallet_address,
			year,
			amount_received::text,
			user_id,
			w9_required,
			w9_required_at,
			last_tx_hash,
			last_tx_timestamp
		FROM
			w9_wallet_earnings
		WHERE
			wallet_address = LOWER($1)
		AND
			year = $2;
	`, wallet, year)

	var earning structs.W9WalletEarning
	var userId sql.NullString
	var requiredAt sql.NullTime
	var lastTxHash sql.NullString
	var lastTxTimestamp sql.NullInt64

	err := row.Scan(
		&earning.WalletAddress,
		&earning.Year,
		&earning.AmountReceived,
		&userId,
		&earning.W9Required,
		&requiredAt,
		&lastTxHash,
		&lastTxTimestamp,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if userId.Valid {
		earning.UserId = &userId.String
	}
	if requiredAt.Valid {
		t := requiredAt.Time
		earning.W9RequiredAt = &t
	}
	if lastTxHash.Valid {
		earning.LastTxHash = &lastTxHash.String
	}
	if lastTxTimestamp.Valid {
		ts := int(lastTxTimestamp.Int64)
		earning.LastTxTimestamp = &ts
	}

	return &earning, nil
}

func (a *AppDB) UpsertW9WalletEarning(ctx context.Context, earning *structs.W9WalletEarning) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO w9_wallet_earnings (
			wallet_address,
			year,
			amount_received,
			user_id,
			w9_required,
			w9_required_at,
			last_tx_hash,
			last_tx_timestamp,
			updated_at
		) VALUES (
			LOWER($1),
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			NOW()
		)
		ON CONFLICT (wallet_address, year)
		DO UPDATE SET
			amount_received = EXCLUDED.amount_received,
			user_id = COALESCE(EXCLUDED.user_id, w9_wallet_earnings.user_id),
			w9_required = w9_wallet_earnings.w9_required OR EXCLUDED.w9_required,
			w9_required_at = COALESCE(w9_wallet_earnings.w9_required_at, EXCLUDED.w9_required_at),
			last_tx_hash = COALESCE(EXCLUDED.last_tx_hash, w9_wallet_earnings.last_tx_hash),
			last_tx_timestamp = COALESCE(EXCLUDED.last_tx_timestamp, w9_wallet_earnings.last_tx_timestamp),
			updated_at = NOW();
	`, earning.WalletAddress, earning.Year, earning.AmountReceived, earning.UserId, earning.W9Required, earning.W9RequiredAt, earning.LastTxHash, earning.LastTxTimestamp)
	if err != nil {
		return fmt.Errorf("error upserting w9 wallet earnings: %s", err)
	}
	return nil
}

func (a *AppDB) UpsertW9Submission(ctx context.Context, submission *structs.W9Submission) (*structs.W9Submission, error) {
	row := a.db.QueryRow(ctx, `
		INSERT INTO w9_submissions (
			wallet_address,
			year,
			email,
			pending_approval,
			submitted_at,
			w9_url
		) VALUES (
			LOWER($1),
			$2,
			$3,
			TRUE,
			NOW(),
			$4
		)
		ON CONFLICT (wallet_address, year)
		DO UPDATE SET
			email = EXCLUDED.email,
			pending_approval = TRUE,
			submitted_at = NOW(),
			w9_url = EXCLUDED.w9_url
		RETURNING
			id,
			wallet_address,
			year,
			email,
			submitted_at,
			pending_approval,
			approved_at,
			approved_by_user_id,
			w9_url;
	`, submission.WalletAddress, submission.Year, submission.Email, submission.W9URL)

	var stored structs.W9Submission
	var approvedAt sql.NullTime
	var approvedBy sql.NullString
	var w9Url sql.NullString
	err := row.Scan(
		&stored.Id,
		&stored.WalletAddress,
		&stored.Year,
		&stored.Email,
		&stored.SubmittedAt,
		&stored.PendingApproval,
		&approvedAt,
		&approvedBy,
		&w9Url,
	)
	if err != nil {
		return nil, err
	}

	if approvedAt.Valid {
		t := approvedAt.Time
		stored.ApprovedAt = &t
	}
	if approvedBy.Valid {
		stored.ApprovedByUserId = &approvedBy.String
	}
	if w9Url.Valid {
		stored.W9URL = &w9Url.String
	}

	return &stored, nil
}

func (a *AppDB) GetW9SubmissionByWalletYear(ctx context.Context, wallet string, year int) (*structs.W9Submission, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id,
			wallet_address,
			year,
			email,
			submitted_at,
			pending_approval,
			approved_at,
			approved_by_user_id,
			w9_url
		FROM
			w9_submissions
		WHERE
			wallet_address = LOWER($1)
		AND
			year = $2;
	`, wallet, year)

	var submission structs.W9Submission
	var approvedAt sql.NullTime
	var approvedBy sql.NullString
	var w9Url sql.NullString
	err := row.Scan(
		&submission.Id,
		&submission.WalletAddress,
		&submission.Year,
		&submission.Email,
		&submission.SubmittedAt,
		&submission.PendingApproval,
		&approvedAt,
		&approvedBy,
		&w9Url,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if approvedAt.Valid {
		t := approvedAt.Time
		submission.ApprovedAt = &t
	}
	if approvedBy.Valid {
		submission.ApprovedByUserId = &approvedBy.String
	}
	if w9Url.Valid {
		submission.W9URL = &w9Url.String
	}

	return &submission, nil
}

func (a *AppDB) GetPendingW9Submissions(ctx context.Context) ([]*structs.W9Submission, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			wallet_address,
			year,
			email,
			submitted_at,
			pending_approval,
			approved_at,
			approved_by_user_id,
			w9_url
		FROM
			w9_submissions
		WHERE
			pending_approval = TRUE
		ORDER BY
			submitted_at ASC;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	submissions := []*structs.W9Submission{}
	for rows.Next() {
		var submission structs.W9Submission
		var approvedAt sql.NullTime
		var approvedBy sql.NullString
		var w9Url sql.NullString
		err = rows.Scan(
			&submission.Id,
			&submission.WalletAddress,
			&submission.Year,
			&submission.Email,
			&submission.SubmittedAt,
			&submission.PendingApproval,
			&approvedAt,
			&approvedBy,
			&w9Url,
		)
		if err != nil {
			continue
		}

		if approvedAt.Valid {
			t := approvedAt.Time
			submission.ApprovedAt = &t
		}
		if approvedBy.Valid {
			submission.ApprovedByUserId = &approvedBy.String
		}
		if w9Url.Valid {
			submission.W9URL = &w9Url.String
		}

		submissions = append(submissions, &submission)
	}

	return submissions, nil
}

func (a *AppDB) ApproveW9Submission(ctx context.Context, id int, approvedBy string) (*structs.W9Submission, error) {
	row := a.db.QueryRow(ctx, `
		UPDATE
			w9_submissions
		SET
			pending_approval = FALSE,
			approved_at = NOW(),
			approved_by_user_id = $2
		WHERE
			id = $1
		RETURNING
			id,
			wallet_address,
			year,
			email,
			submitted_at,
			pending_approval,
			approved_at,
			approved_by_user_id,
			w9_url;
	`, id, approvedBy)

	var submission structs.W9Submission
	var approvedAt sql.NullTime
	var approvedByUser sql.NullString
	var w9Url sql.NullString
	err := row.Scan(
		&submission.Id,
		&submission.WalletAddress,
		&submission.Year,
		&submission.Email,
		&submission.SubmittedAt,
		&submission.PendingApproval,
		&approvedAt,
		&approvedByUser,
		&w9Url,
	)
	if err != nil {
		return nil, err
	}

	if approvedAt.Valid {
		t := approvedAt.Time
		submission.ApprovedAt = &t
	}
	if approvedByUser.Valid {
		submission.ApprovedByUserId = &approvedByUser.String
	}
	if w9Url.Valid {
		submission.W9URL = &w9Url.String
	}

	return &submission, nil
}
