package db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (a *AppDB) IsProposer(ctx context.Context, id string) (bool, error) {
	return a.getBoolUserRole(ctx, id, "is_proposer")
}

func (a *AppDB) IsImprover(ctx context.Context, id string) (bool, error) {
	return a.getBoolUserRole(ctx, id, "is_improver")
}

func (a *AppDB) IsVoter(ctx context.Context, id string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			(is_voter OR is_admin)
		FROM
			users
		WHERE
			id = $1;
	`, id)
	var value bool
	err := row.Scan(&value)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return value, nil
}

func (a *AppDB) IsIssuer(ctx context.Context, id string) (bool, error) {
	return a.getBoolUserRole(ctx, id, "is_issuer")
}

func (a *AppDB) IsSupervisor(ctx context.Context, id string) (bool, error) {
	return a.getBoolUserRole(ctx, id, "is_supervisor")
}

func (a *AppDB) getBoolUserRole(ctx context.Context, id string, column string) (bool, error) {
	query := fmt.Sprintf(`
		SELECT
			%s
		FROM
			users
		WHERE
			id = $1;
	`, column)

	row := a.db.QueryRow(ctx, query, id)
	var value bool
	err := row.Scan(&value)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return value, nil
}

func normalizeEthereumAddressForField(value string, fieldName string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", fmt.Errorf("%s is required", fieldName)
	}
	if !common.IsHexAddress(normalized) {
		return "", fmt.Errorf("%s must be a valid ethereum address", fieldName)
	}
	return common.HexToAddress(normalized).Hex(), nil
}

func normalizeEthereumAddress(value string) (string, error) {
	return normalizeEthereumAddressForField(value, "primary rewards account")
}

func getDefaultPrimaryRewardsAccountForUser(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userId string) (string, error) {
	var primaryWalletAddress string
	err := querier.QueryRow(ctx, `
		SELECT
			COALESCE(NULLIF(TRIM(primary_wallet_address), ''), '')
		FROM
			users
		WHERE
			id = $1;
	`, userId).Scan(&primaryWalletAddress)
	if err == nil {
		primaryWalletAddress = strings.TrimSpace(primaryWalletAddress)
		if primaryWalletAddress != "" && common.IsHexAddress(primaryWalletAddress) {
			return common.HexToAddress(primaryWalletAddress).Hex(), nil
		}
	} else if err != pgx.ErrNoRows {
		return "", err
	}

	var account string
	err = querier.QueryRow(ctx, `
		SELECT
			NULLIF(TRIM(w.smart_address), '')
		FROM
			wallets w
		WHERE
			w.owner = $1
		AND
			w.is_eoa = false
		AND
			w.smart_index = 0
		ORDER BY
			CASE
				WHEN LOWER(TRIM(COALESCE(w.eoa_address, ''))) = COALESCE((
					SELECT
						LOWER(TRIM(e.eoa_address))
					FROM
						wallets e
					WHERE
						e.owner = $1
					AND
						e.is_eoa = true
					ORDER BY
						e.id ASC
					LIMIT 1
				), '') THEN 0
				ELSE 1
			END,
			w.id ASC
		LIMIT 1;
	`, userId).Scan(&account)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	account = strings.TrimSpace(account)
	if account == "" {
		return "", nil
	}
	if !common.IsHexAddress(account) {
		return "", nil
	}
	return common.HexToAddress(account).Hex(), nil
}

func applyDefaultPrimaryRewardsAccountForUser(ctx context.Context, querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, userId string, defaultRewardsAccount string) error {
	defaultRewardsAccount = strings.TrimSpace(defaultRewardsAccount)
	if defaultRewardsAccount == "" {
		return nil
	}

	_, err := querier.Exec(ctx, `
		UPDATE
			improvers i
		SET
			primary_rewards_account = $2,
			updated_at = NOW()
		WHERE
			i.user_id = $1
		AND
			(
				TRIM(COALESCE(i.primary_rewards_account, '')) = ''
				OR (
					i.created_at = i.updated_at
					AND
						EXISTS (
							SELECT 1
							FROM wallets w
							WHERE
								w.owner = i.user_id
							AND
								w.is_eoa = false
							AND
								w.smart_index = 0
							AND
								LOWER(TRIM(COALESCE(w.smart_address, ''))) = LOWER($2)
							AND
								LOWER(TRIM(COALESCE(w.eoa_address, ''))) = LOWER(TRIM(COALESCE(i.primary_rewards_account, '')))
						)
				)
			);
	`, userId, defaultRewardsAccount)
	if err != nil {
		return fmt.Errorf("error defaulting improver rewards account: %s", err)
	}

	_, err = querier.Exec(ctx, `
		UPDATE
			supervisors s
		SET
			primary_rewards_account = $2,
			updated_at = NOW()
		WHERE
			s.user_id = $1
		AND
			(
				TRIM(COALESCE(s.primary_rewards_account, '')) = ''
				OR (
					s.created_at = s.updated_at
					AND
						EXISTS (
							SELECT 1
							FROM wallets w
							WHERE
								w.owner = s.user_id
							AND
								w.is_eoa = false
							AND
								w.smart_index = 0
							AND
								LOWER(TRIM(COALESCE(w.smart_address, ''))) = LOWER($2)
							AND
								LOWER(TRIM(COALESCE(w.eoa_address, ''))) = LOWER(TRIM(COALESCE(s.primary_rewards_account, '')))
						)
				)
			);
	`, userId, defaultRewardsAccount)
	if err != nil {
		return fmt.Errorf("error defaulting supervisor rewards account: %s", err)
	}

	return nil
}

func syncDefaultPrimaryRewardsAccountsForUser(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, userId string) (string, error) {
	defaultRewardsAccount, err := getDefaultPrimaryRewardsAccountForUser(ctx, querier, userId)
	if err != nil {
		return "", err
	}
	if err := applyDefaultPrimaryRewardsAccountForUser(ctx, querier, userId, defaultRewardsAccount); err != nil {
		return "", err
	}
	return defaultRewardsAccount, nil
}

func (a *AppDB) UpsertProposerRequest(ctx context.Context, userId string, organization string, email string) (*structs.Proposer, error) {
	organization = strings.TrimSpace(organization)
	email = strings.ToLower(strings.TrimSpace(email))
	if organization == "" {
		return nil, fmt.Errorf("organization is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	isVerified, err := a.IsVerifiedEmailForUser(ctx, userId, email)
	if err != nil {
		return nil, err
	}
	if !isVerified {
		return nil, fmt.Errorf("email must be verified before requesting proposer status")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			proposers
		WHERE
			user_id = $1;
	`, userId).Scan(&status)
	if err == pgx.ErrNoRows {
		_, err = tx.Exec(ctx, `
			INSERT INTO proposers
				(user_id, organization, email, status)
			VALUES
				($1, $2, $3, 'pending');
		`, userId, organization, email)
		if err != nil {
			return nil, fmt.Errorf("error inserting proposer request: %s", err)
		}
	} else if err != nil {
		return nil, err
	} else {
		if status == "approved" {
			return nil, fmt.Errorf("proposer already approved")
		}

		_, err = tx.Exec(ctx, `
			UPDATE
				proposers
			SET
				organization = $2,
				email = $3,
				status = 'pending',
				updated_at = NOW()
			WHERE
				user_id = $1;
		`, userId, organization, email)
		if err != nil {
			return nil, fmt.Errorf("error updating proposer request: %s", err)
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			users
		SET
			is_proposer = false,
			contact_email = COALESCE(NULLIF($2, ''), contact_email)
		WHERE
			id = $1;
	`, userId, email)
	if err != nil {
		return nil, fmt.Errorf("error resetting proposer status: %s", err)
	}

	proposer, err := getProposerByUser(ctx, tx, userId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return proposer, nil
}

func (a *AppDB) GetProposerByUser(ctx context.Context, userId string) (*structs.Proposer, error) {
	return getProposerByUser(ctx, a.db, userId)
}

func (a *AppDB) GetProposers(ctx context.Context, search string, page, count int) ([]*structs.Proposer, error) {
	if count <= 0 {
		count = 20
	}
	offset := page * count
	likeSearch := "%" + search + "%"
	rows, err := a.db.Query(ctx, `
		SELECT
			user_id,
			organization,
			email,
			nickname,
			status,
			created_at,
			updated_at
		FROM
			proposers
		WHERE
			(organization ILIKE $1 OR email ILIKE $1 OR COALESCE(nickname, '') ILIKE $1)
		ORDER BY
			created_at DESC
		LIMIT $2
		OFFSET $3;
	`, likeSearch, count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying proposers: %s", err)
	}
	defer rows.Close()

	results := []*structs.Proposer{}
	for rows.Next() {
		proposer := structs.Proposer{}
		err = rows.Scan(
			&proposer.UserId,
			&proposer.Organization,
			&proposer.Email,
			&proposer.Nickname,
			&proposer.Status,
			&proposer.CreatedAt,
			&proposer.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning proposer: %s", err)
		}
		results = append(results, &proposer)
	}

	return results, nil
}

func (a *AppDB) UpdateProposer(ctx context.Context, req *structs.ProposerUpdateRequest) (*structs.Proposer, error) {
	if req.UserId == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	if req.Status != nil {
		switch *req.Status {
		case "pending", "approved", "rejected":
		default:
			return nil, fmt.Errorf("invalid proposer status")
		}
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	cmd, err := tx.Exec(ctx, `
		UPDATE
			proposers
		SET
			nickname = COALESCE($2, nickname),
			status = COALESCE($3, status),
			updated_at = NOW()
		WHERE
			user_id = $1;
	`, req.UserId, req.Nickname, req.Status)
	if err != nil {
		return nil, fmt.Errorf("error updating proposer: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, fmt.Errorf("proposer not found")
	}

	if req.Status != nil {
		isProposer := *req.Status == "approved"
		_, err = tx.Exec(ctx, `
			UPDATE
				users
			SET
				is_proposer = $1
			WHERE
				id = $2;
		`, isProposer, req.UserId)
		if err != nil {
			return nil, fmt.Errorf("error updating user proposer flag: %s", err)
		}
	}

	proposer, err := getProposerByUser(ctx, tx, req.UserId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return proposer, nil
}

func getProposerByUser(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userId string) (*structs.Proposer, error) {
	row := querier.QueryRow(ctx, `
		SELECT
			user_id,
			organization,
			email,
			nickname,
			status,
			created_at,
			updated_at
		FROM
			proposers
		WHERE
			user_id = $1;
	`, userId)

	proposer := structs.Proposer{}
	err := row.Scan(
		&proposer.UserId,
		&proposer.Organization,
		&proposer.Email,
		&proposer.Nickname,
		&proposer.Status,
		&proposer.CreatedAt,
		&proposer.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &proposer, nil
}

func (a *AppDB) UpsertImproverRequest(ctx context.Context, userId string, req *structs.ImproverRequest) (*structs.Improver, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	first := strings.TrimSpace(req.FirstName)
	last := strings.TrimSpace(req.LastName)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if first == "" || last == "" || email == "" {
		return nil, fmt.Errorf("first name, last name, and email are required")
	}
	isVerified, err := a.IsVerifiedEmailForUser(ctx, userId, email)
	if err != nil {
		return nil, err
	}
	if !isVerified {
		return nil, fmt.Errorf("email must be verified before requesting improver status")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	defaultRewardsAccount, err := getDefaultPrimaryRewardsAccountForUser(ctx, tx, userId)
	if err != nil {
		return nil, fmt.Errorf("error loading default improver rewards account: %s", err)
	}

	var existingStatus string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			improvers
		WHERE
			user_id = $1;
	`, userId).Scan(&existingStatus)
	if err == pgx.ErrNoRows {
		_, err = tx.Exec(ctx, `
			INSERT INTO improvers
				(user_id, first_name, last_name, email, primary_rewards_account, status)
			VALUES
				($1, $2, $3, $4, $5, 'approved');
		`, userId, first, last, email, defaultRewardsAccount)
		if err != nil {
			return nil, fmt.Errorf("error inserting improver request: %s", err)
		}
	} else if err != nil {
		return nil, err
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE
				improvers
			SET
				first_name = $2,
				last_name = $3,
				email = $4,
				primary_rewards_account = COALESCE(NULLIF(TRIM(primary_rewards_account), ''), $5, ''),
				status = 'approved',
				updated_at = NOW()
			WHERE
				user_id = $1;
		`, userId, first, last, email, defaultRewardsAccount)
		if err != nil {
			return nil, fmt.Errorf("error updating improver request: %s", err)
		}
	}

	fullName := strings.TrimSpace(first + " " + last)
	_, err = tx.Exec(ctx, `
		UPDATE
			users
		SET
			is_improver = true,
			contact_name = COALESCE(NULLIF($2, ''), contact_name),
			contact_email = COALESCE(NULLIF($3, ''), contact_email)
		WHERE
			id = $1;
	`, userId, fullName, email)
	if err != nil {
		return nil, fmt.Errorf("error updating user improver profile: %s", err)
	}

	improver, err := getImproverByUser(ctx, tx, userId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return improver, nil
}

func (a *AppDB) GetImproverByUser(ctx context.Context, userId string) (*structs.Improver, error) {
	return getImproverByUser(ctx, a.db, userId)
}

func (a *AppDB) GetImprovers(ctx context.Context, search string, page, count int) ([]*structs.Improver, error) {
	if count <= 0 {
		count = 20
	}
	offset := page * count
	likeSearch := "%" + search + "%"
	rows, err := a.db.Query(ctx, `
		SELECT
			user_id,
			first_name,
			last_name,
			email,
			primary_rewards_account,
			status,
			created_at,
			updated_at
		FROM
			improvers
		WHERE
			(first_name ILIKE $1 OR last_name ILIKE $1 OR email ILIKE $1)
		ORDER BY
			created_at DESC
		LIMIT $2
		OFFSET $3;
	`, likeSearch, count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying improvers: %s", err)
	}
	defer rows.Close()

	results := []*structs.Improver{}
	for rows.Next() {
		improver := structs.Improver{}
		err = rows.Scan(
			&improver.UserId,
			&improver.FirstName,
			&improver.LastName,
			&improver.Email,
			&improver.PrimaryRewardsAccount,
			&improver.Status,
			&improver.CreatedAt,
			&improver.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning improver: %s", err)
		}
		results = append(results, &improver)
	}

	return results, nil
}

func (a *AppDB) UpdateImprover(ctx context.Context, req *structs.ImproverUpdateRequest) (*structs.Improver, error) {
	if req.UserId == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	if req.Status != nil {
		switch *req.Status {
		case "pending", "approved", "rejected":
		default:
			return nil, fmt.Errorf("invalid improver status")
		}
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	cmd, err := tx.Exec(ctx, `
		UPDATE
			improvers
		SET
			status = COALESCE($2, status),
			updated_at = NOW()
		WHERE
			user_id = $1;
	`, req.UserId, req.Status)
	if err != nil {
		return nil, fmt.Errorf("error updating improver: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, fmt.Errorf("improver not found")
	}

	if req.Status != nil {
		isImprover := *req.Status == "approved"
		_, err = tx.Exec(ctx, `
			UPDATE
				users
			SET
				is_improver = $1
			WHERE
				id = $2;
		`, isImprover, req.UserId)
		if err != nil {
			return nil, fmt.Errorf("error updating user improver flag: %s", err)
		}

		if isImprover {
			defaultRewardsAccount, rewardsErr := getDefaultPrimaryRewardsAccountForUser(ctx, tx, req.UserId)
			if rewardsErr != nil {
				return nil, fmt.Errorf("error loading default improver rewards account: %s", rewardsErr)
			}
			if defaultRewardsAccount != "" {
				_, err = tx.Exec(ctx, `
					UPDATE
						improvers
					SET
						primary_rewards_account = COALESCE(NULLIF(TRIM(primary_rewards_account), ''), $2),
						updated_at = NOW()
					WHERE
						user_id = $1;
				`, req.UserId, defaultRewardsAccount)
				if err != nil {
					return nil, fmt.Errorf("error defaulting improver rewards account: %s", err)
				}
			}
		}
	}

	improver, err := getImproverByUser(ctx, tx, req.UserId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return improver, nil
}

func (a *AppDB) UpdateImproverPrimaryRewardsAccount(ctx context.Context, userId string, account string) (*structs.Improver, error) {
	userId = strings.TrimSpace(userId)
	if userId == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	normalizedAccount, err := normalizeEthereumAddress(account)
	if err != nil {
		return nil, err
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			improvers
		WHERE
			user_id = $1
		FOR UPDATE;
	`, userId).Scan(&status)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("improver not found")
	}
	if err != nil {
		return nil, fmt.Errorf("error loading improver: %s", err)
	}
	if status != "approved" {
		return nil, fmt.Errorf("improver must be approved")
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			improvers
		SET
			primary_rewards_account = $2,
			updated_at = NOW()
		WHERE
			user_id = $1;
	`, userId, normalizedAccount)
	if err != nil {
		return nil, fmt.Errorf("error updating improver rewards account: %s", err)
	}

	improver, err := getImproverByUser(ctx, tx, userId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return improver, nil
}

func getImproverByUser(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userId string) (*structs.Improver, error) {
	row := querier.QueryRow(ctx, `
		SELECT
			user_id,
			first_name,
			last_name,
			email,
			primary_rewards_account,
			status,
			created_at,
			updated_at
		FROM
			improvers
		WHERE
			user_id = $1;
	`, userId)

	improver := structs.Improver{}
	err := row.Scan(
		&improver.UserId,
		&improver.FirstName,
		&improver.LastName,
		&improver.Email,
		&improver.PrimaryRewardsAccount,
		&improver.Status,
		&improver.CreatedAt,
		&improver.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &improver, nil
}

func (a *AppDB) UpsertSupervisorRequest(ctx context.Context, userId string, organization string, email string) (*structs.Supervisor, error) {
	organization = strings.TrimSpace(organization)
	email = strings.ToLower(strings.TrimSpace(email))
	if organization == "" {
		return nil, fmt.Errorf("organization is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	isVerified, err := a.IsVerifiedEmailForUser(ctx, userId, email)
	if err != nil {
		return nil, err
	}
	if !isVerified {
		return nil, fmt.Errorf("email must be verified before requesting supervisor status")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	defaultRewardsAccount, err := getDefaultPrimaryRewardsAccountForUser(ctx, tx, userId)
	if err != nil {
		return nil, fmt.Errorf("error loading default supervisor rewards account: %s", err)
	}

	var status string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			supervisors
		WHERE
			user_id = $1;
	`, userId).Scan(&status)
	if err == pgx.ErrNoRows {
		_, err = tx.Exec(ctx, `
			INSERT INTO supervisors
				(user_id, organization, email, primary_rewards_account, status)
			VALUES
				($1, $2, $3, $4, 'pending');
		`, userId, organization, email, defaultRewardsAccount)
		if err != nil {
			return nil, fmt.Errorf("error inserting supervisor request: %s", err)
		}
	} else if err != nil {
		return nil, err
	} else {
		if status == "approved" {
			return nil, fmt.Errorf("supervisor already approved")
		}
		_, err = tx.Exec(ctx, `
			UPDATE
				supervisors
			SET
				organization = $2,
				email = $3,
				primary_rewards_account = COALESCE(NULLIF(TRIM(primary_rewards_account), ''), $4, ''),
				status = 'pending',
				updated_at = NOW()
			WHERE
				user_id = $1;
		`, userId, organization, email, defaultRewardsAccount)
		if err != nil {
			return nil, fmt.Errorf("error updating supervisor request: %s", err)
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			users
		SET
			is_supervisor = false,
			contact_email = COALESCE(NULLIF($2, ''), contact_email)
		WHERE
			id = $1;
	`, userId, email)
	if err != nil {
		return nil, fmt.Errorf("error resetting supervisor status: %s", err)
	}

	supervisor, err := getSupervisorByUser(ctx, tx, userId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return supervisor, nil
}

func (a *AppDB) GetSupervisorByUser(ctx context.Context, userId string) (*structs.Supervisor, error) {
	return getSupervisorByUser(ctx, a.db, userId)
}

func (a *AppDB) GetSupervisors(ctx context.Context, search string, page, count int) ([]*structs.Supervisor, error) {
	if count <= 0 {
		count = 20
	}
	offset := page * count
	likeSearch := "%" + search + "%"
	rows, err := a.db.Query(ctx, `
		SELECT
			user_id,
			organization,
			email,
			primary_rewards_account,
			nickname,
			status,
			created_at,
			updated_at
		FROM
			supervisors
		WHERE
			(organization ILIKE $1 OR email ILIKE $1 OR COALESCE(nickname, '') ILIKE $1)
		ORDER BY
			created_at DESC
		LIMIT $2
		OFFSET $3;
	`, likeSearch, count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying supervisors: %s", err)
	}
	defer rows.Close()

	results := []*structs.Supervisor{}
	for rows.Next() {
		supervisor := structs.Supervisor{}
		err = rows.Scan(
			&supervisor.UserId,
			&supervisor.Organization,
			&supervisor.Email,
			&supervisor.PrimaryRewardsAccount,
			&supervisor.Nickname,
			&supervisor.Status,
			&supervisor.CreatedAt,
			&supervisor.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning supervisor: %s", err)
		}
		results = append(results, &supervisor)
	}

	return results, nil
}

func (a *AppDB) GetApprovedSupervisors(ctx context.Context) ([]*structs.Supervisor, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			s.user_id,
			s.organization,
			s.email,
			s.primary_rewards_account,
			s.nickname,
			s.status,
			s.created_at,
			s.updated_at
		FROM
			supervisors s
		JOIN
			users u
		ON
			u.id = s.user_id
		WHERE
			s.status = 'approved'
		AND
			u.is_supervisor = true
		ORDER BY
			COALESCE(NULLIF(TRIM(s.nickname), ''), s.organization) ASC,
			s.created_at DESC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying approved supervisors: %s", err)
	}
	defer rows.Close()

	results := []*structs.Supervisor{}
	for rows.Next() {
		supervisor := structs.Supervisor{}
		if err := rows.Scan(
			&supervisor.UserId,
			&supervisor.Organization,
			&supervisor.Email,
			&supervisor.PrimaryRewardsAccount,
			&supervisor.Nickname,
			&supervisor.Status,
			&supervisor.CreatedAt,
			&supervisor.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning approved supervisor: %s", err)
		}
		results = append(results, &supervisor)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating approved supervisors: %s", err)
	}
	return results, nil
}

func (a *AppDB) UpdateSupervisor(ctx context.Context, req *structs.SupervisorUpdateRequest) (*structs.Supervisor, error) {
	if req.UserId == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	if req.Status != nil {
		switch *req.Status {
		case "pending", "approved", "rejected":
		default:
			return nil, fmt.Errorf("invalid supervisor status")
		}
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	cmd, err := tx.Exec(ctx, `
		UPDATE
			supervisors
		SET
			nickname = COALESCE($2, nickname),
			status = COALESCE($3, status),
			updated_at = NOW()
		WHERE
			user_id = $1;
	`, req.UserId, req.Nickname, req.Status)
	if err != nil {
		return nil, fmt.Errorf("error updating supervisor: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, fmt.Errorf("supervisor not found")
	}

	if req.Status != nil {
		isSupervisor := *req.Status == "approved"
		_, err = tx.Exec(ctx, `
			UPDATE
				users
			SET
				is_supervisor = $1
			WHERE
				id = $2;
		`, isSupervisor, req.UserId)
		if err != nil {
			return nil, fmt.Errorf("error updating user supervisor flag: %s", err)
		}

		if isSupervisor {
			defaultRewardsAccount, rewardsErr := getDefaultPrimaryRewardsAccountForUser(ctx, tx, req.UserId)
			if rewardsErr != nil {
				return nil, fmt.Errorf("error loading default supervisor rewards account: %s", rewardsErr)
			}
			if defaultRewardsAccount != "" {
				_, err = tx.Exec(ctx, `
					UPDATE
						supervisors
					SET
						primary_rewards_account = COALESCE(NULLIF(TRIM(primary_rewards_account), ''), $2),
						updated_at = NOW()
					WHERE
						user_id = $1;
				`, req.UserId, defaultRewardsAccount)
				if err != nil {
					return nil, fmt.Errorf("error defaulting supervisor rewards account: %s", err)
				}
			}
		}
	}

	supervisor, err := getSupervisorByUser(ctx, tx, req.UserId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return supervisor, nil
}

func (a *AppDB) UpdateSupervisorPrimaryRewardsAccount(ctx context.Context, userId string, account string) (*structs.Supervisor, error) {
	userId = strings.TrimSpace(userId)
	if userId == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	normalizedAccount, err := normalizeEthereumAddress(account)
	if err != nil {
		return nil, err
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			supervisors
		WHERE
			user_id = $1
		FOR UPDATE;
	`, userId).Scan(&status)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("supervisor not found")
	}
	if err != nil {
		return nil, fmt.Errorf("error loading supervisor: %s", err)
	}
	if status != "approved" {
		return nil, fmt.Errorf("supervisor must be approved")
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			supervisors
		SET
			primary_rewards_account = $2,
			updated_at = NOW()
		WHERE
			user_id = $1;
	`, userId, normalizedAccount)
	if err != nil {
		return nil, fmt.Errorf("error updating supervisor rewards account: %s", err)
	}

	supervisor, err := getSupervisorByUser(ctx, tx, userId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return supervisor, nil
}

func getSupervisorByUser(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userId string) (*structs.Supervisor, error) {
	row := querier.QueryRow(ctx, `
		SELECT
			user_id,
			organization,
			email,
			primary_rewards_account,
			nickname,
			status,
			created_at,
			updated_at
		FROM
			supervisors
		WHERE
			user_id = $1;
	`, userId)

	supervisor := structs.Supervisor{}
	err := row.Scan(
		&supervisor.UserId,
		&supervisor.Organization,
		&supervisor.Email,
		&supervisor.PrimaryRewardsAccount,
		&supervisor.Nickname,
		&supervisor.Status,
		&supervisor.CreatedAt,
		&supervisor.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &supervisor, nil
}

type normalizedWorkflowTemplateData struct {
	SeriesId             *string
	Recurrence           string
	StartAt              time.Time
	SupervisorUserId     *string
	SupervisorBounty     *uint64
	SupervisorDataFields []structs.WorkflowSupervisorDataField
	Roles                []structs.WorkflowRoleCreateInput
	Steps                []structs.WorkflowStepCreateInput
	TotalBounty          uint64
}

func normalizeWorkflowSupervisorDataFields(fields []structs.WorkflowSupervisorDataField) ([]structs.WorkflowSupervisorDataField, error) {
	if len(fields) == 0 {
		return []structs.WorkflowSupervisorDataField{}, nil
	}

	normalized := make([]structs.WorkflowSupervisorDataField, 0, len(fields))
	seenKeys := map[string]struct{}{}
	for _, field := range fields {
		key := strings.TrimSpace(field.Key)
		value := strings.TrimSpace(field.Value)
		if key == "" && value == "" {
			continue
		}
		if key == "" || value == "" {
			return nil, fmt.Errorf("supervisor data fields require both key and value")
		}
		keyLookup := strings.ToLower(key)
		if _, exists := seenKeys[keyLookup]; exists {
			return nil, fmt.Errorf("duplicate supervisor data field key: %s", key)
		}
		seenKeys[keyLookup] = struct{}{}
		normalized = append(normalized, structs.WorkflowSupervisorDataField{
			Key:   key,
			Value: value,
		})
	}

	return normalized, nil
}

func normalizeWorkflowTemplateData(req *structs.WorkflowTemplateCreateRequest, validCredentials map[string]struct{}) (*normalizedWorkflowTemplateData, error) {
	if req == nil {
		return nil, fmt.Errorf("template request is required")
	}

	recurrence := strings.TrimSpace(req.Recurrence)
	switch recurrence {
	case "one_time", "daily", "weekly", "monthly":
	default:
		return nil, fmt.Errorf("invalid recurrence")
	}

	if len(req.Roles) == 0 {
		return nil, fmt.Errorf("at least one workflow role is required")
	}
	if len(req.Steps) == 0 {
		return nil, fmt.Errorf("at least one workflow step is required")
	}

	roleIds := map[string]struct{}{}
	normalizedRoles := make([]structs.WorkflowRoleCreateInput, 0, len(req.Roles))
	for idx, roleInput := range req.Roles {
		roleTitle := strings.TrimSpace(roleInput.Title)
		if roleTitle == "" {
			return nil, fmt.Errorf("workflow role title is required")
		}

		roleClientId := strings.TrimSpace(roleInput.ClientId)
		if roleClientId == "" {
			roleClientId = fmt.Sprintf("role-%d", idx+1)
		}
		if _, exists := roleIds[roleClientId]; exists {
			return nil, fmt.Errorf("duplicate workflow role client_id: %s", roleClientId)
		}
		roleIds[roleClientId] = struct{}{}

		normalizedCredentials := make([]string, 0, len(roleInput.RequiredCredentials))
		seenCredentials := map[string]struct{}{}
		for _, credential := range roleInput.RequiredCredentials {
			credential = strings.TrimSpace(credential)
			if _, ok := validCredentials[credential]; !ok {
				return nil, fmt.Errorf("invalid workflow role credential: %s", credential)
			}
			if _, exists := seenCredentials[credential]; exists {
				continue
			}
			seenCredentials[credential] = struct{}{}
			normalizedCredentials = append(normalizedCredentials, credential)
		}
		if len(normalizedCredentials) == 0 {
			return nil, fmt.Errorf("workflow role requires at least one credential")
		}

		normalizedRoles = append(normalizedRoles, structs.WorkflowRoleCreateInput{
			ClientId:            roleClientId,
			Title:               roleTitle,
			RequiredCredentials: normalizedCredentials,
		})
	}

	totalBounty := uint64(0)
	var supervisorUserId *string
	if req.SupervisorUserId != nil {
		normalized := strings.TrimSpace(*req.SupervisorUserId)
		if normalized != "" {
			supervisorUserId = &normalized
		}
	}
	var supervisorBounty *uint64
	if req.SupervisorBounty != nil {
		value := *req.SupervisorBounty
		supervisorBounty = &value
		totalBounty += value
	} else if req.Manager != nil {
		value := req.Manager.Bounty
		supervisorBounty = &value
		totalBounty += value
	}

	normalizedSupervisorDataFields, err := normalizeWorkflowSupervisorDataFields(req.SupervisorDataFields)
	if err != nil {
		return nil, err
	}
	if len(normalizedSupervisorDataFields) > 0 && supervisorUserId == nil {
		return nil, fmt.Errorf("workflow supervisor user_id is required when supervisor data fields are provided")
	}

	normalizedSteps := make([]structs.WorkflowStepCreateInput, 0, len(req.Steps))
	for _, stepInput := range req.Steps {
		stepTitle := strings.TrimSpace(stepInput.Title)
		if stepTitle == "" {
			return nil, fmt.Errorf("workflow step title is required")
		}

		roleClientId := strings.TrimSpace(stepInput.RoleClientId)
		if roleClientId == "" {
			return nil, fmt.Errorf("workflow step requires a role assignment")
		}
		if _, exists := roleIds[roleClientId]; !exists {
			return nil, fmt.Errorf("workflow step references unknown role client_id: %s", roleClientId)
		}

		totalBounty += stepInput.Bounty
		normalizedItems := make([]structs.WorkflowWorkItemCreateInput, 0, len(stepInput.WorkItems))
		for _, itemInput := range stepInput.WorkItems {
			itemTitle := strings.TrimSpace(itemInput.Title)
			if itemTitle == "" {
				return nil, fmt.Errorf("workflow work item title is required")
			}
			if !itemInput.RequiresPhoto && !itemInput.RequiresWritten && !itemInput.RequiresDropdown {
				return nil, fmt.Errorf("workflow work item must require photo, written response, or dropdown")
			}

			photoRequiredCount := itemInput.PhotoRequiredCount
			if photoRequiredCount <= 0 {
				photoRequiredCount = 1
			}
			photoAllowAnyCount := itemInput.RequiresPhoto && itemInput.PhotoAllowAnyCount
			if !itemInput.RequiresPhoto {
				photoAllowAnyCount = false
			}

			photoAspectRatio, err := normalizeWorkflowPhotoAspectRatio(itemInput.PhotoAspectRatio)
			if err != nil {
				return nil, fmt.Errorf("workflow work item photo_aspect_ratio is invalid")
			}

			normalizedDropdownOptions := []structs.WorkflowDropdownOptionCreateInput{}
			if itemInput.RequiresDropdown {
				if len(itemInput.DropdownOptions) == 0 {
					return nil, fmt.Errorf("dropdown work item requires at least one option")
				}
				seenValues := map[string]struct{}{}
				for _, option := range itemInput.DropdownOptions {
					label := strings.TrimSpace(option.Label)
					if label == "" {
						return nil, fmt.Errorf("dropdown option label is required")
					}

					value := deriveDropdownValueFromLabel(label)
					if value == "" {
						return nil, fmt.Errorf("dropdown option label must include letters or numbers")
					}
					if _, exists := seenValues[value]; exists {
						return nil, fmt.Errorf("duplicate dropdown option label value: %s", value)
					}
					seenValues[value] = struct{}{}

					notifyEmails, notifyErr := normalizeValidatedWorkflowNotificationEmails(option.NotifyEmails)
					if notifyErr != nil {
						return nil, notifyErr
					}

					normalizedDropdownOptions = append(normalizedDropdownOptions, structs.WorkflowDropdownOptionCreateInput{
						Label:                   label,
						RequiresWrittenResponse: option.RequiresWrittenResponse,
						NotifyEmails:            notifyEmails,
						SendPicturesWithEmail:   option.SendPicturesWithEmail,
					})
				}
			}

			normalizedItems = append(normalizedItems, structs.WorkflowWorkItemCreateInput{
				Title:              itemTitle,
				Description:        strings.TrimSpace(itemInput.Description),
				Optional:           itemInput.Optional,
				RequiresPhoto:      itemInput.RequiresPhoto,
				CameraCaptureOnly:  itemInput.RequiresPhoto && itemInput.CameraCaptureOnly,
				PhotoRequiredCount: photoRequiredCount,
				PhotoAllowAnyCount: photoAllowAnyCount,
				PhotoAspectRatio:   photoAspectRatio,
				RequiresWritten:    itemInput.RequiresWritten,
				RequiresDropdown:   itemInput.RequiresDropdown,
				DropdownOptions:    normalizedDropdownOptions,
			})
		}

		normalizedSteps = append(normalizedSteps, structs.WorkflowStepCreateInput{
			Title:                stepTitle,
			Description:          strings.TrimSpace(stepInput.Description),
			Bounty:               stepInput.Bounty,
			RoleClientId:         roleClientId,
			AllowStepNotPossible: stepInput.AllowStepNotPossible,
			WorkItems:            normalizedItems,
		})
	}

	var seriesId *string
	if req.SeriesId != nil {
		trimmed := strings.TrimSpace(*req.SeriesId)
		if trimmed != "" {
			seriesId = &trimmed
		}
	}

	return &normalizedWorkflowTemplateData{
		SeriesId:             seriesId,
		Recurrence:           recurrence,
		StartAt:              time.Unix(0, 0).UTC(),
		SupervisorUserId:     supervisorUserId,
		SupervisorBounty:     supervisorBounty,
		SupervisorDataFields: normalizedSupervisorDataFields,
		Roles:                normalizedRoles,
		Steps:                normalizedSteps,
		TotalBounty:          totalBounty,
	}, nil
}

type normalizedWorkflowDefinitionData struct {
	Title                string
	Description          string
	Recurrence           string
	RecurrenceEndAt      *int64
	SupervisorRequired   bool
	SupervisorUserId     *string
	SupervisorBounty     uint64
	SupervisorDataFields []structs.WorkflowSupervisorDataField
	Roles                []structs.WorkflowRoleCreateInput
	Steps                []structs.WorkflowStepCreateInput
	TotalBounty          uint64
	WeeklyRequirement    uint64
}

func normalizeWorkflowDefinitionData(
	title string,
	description string,
	recurrence string,
	recurrenceEndAt *time.Time,
	supervisor *structs.WorkflowSupervisorCreateInput,
	supervisorDataFields []structs.WorkflowSupervisorDataField,
	roles []structs.WorkflowRoleCreateInput,
	steps []structs.WorkflowStepCreateInput,
	validCredentials map[string]struct{},
) (*normalizedWorkflowDefinitionData, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	description = strings.TrimSpace(description)
	recurrence = strings.TrimSpace(recurrence)

	templateReq := &structs.WorkflowTemplateCreateRequest{
		Recurrence:           recurrence,
		SupervisorDataFields: supervisorDataFields,
		Roles:                roles,
		Steps:                steps,
	}
	if supervisor != nil {
		supervisorId := strings.TrimSpace(supervisor.UserId)
		if supervisorId != "" {
			templateReq.SupervisorUserId = &supervisorId
		}
		supervisorBounty := supervisor.Bounty
		templateReq.SupervisorBounty = &supervisorBounty
	}

	normalizedTemplate, err := normalizeWorkflowTemplateData(templateReq, validCredentials)
	if err != nil {
		return nil, err
	}

	var normalizedEndAt *int64
	if recurrenceEndAt != nil {
		endAtUnix := recurrenceEndAt.UTC().Unix()
		normalizedEndAt = &endAtUnix
	}
	if normalizedTemplate.Recurrence == "one_time" {
		normalizedEndAt = nil
	}

	supervisorRequired := normalizedTemplate.SupervisorUserId != nil
	supervisorBounty := uint64(0)
	if normalizedTemplate.SupervisorBounty != nil {
		supervisorBounty = *normalizedTemplate.SupervisorBounty
	}

	return &normalizedWorkflowDefinitionData{
		Title:                title,
		Description:          description,
		Recurrence:           normalizedTemplate.Recurrence,
		RecurrenceEndAt:      normalizedEndAt,
		SupervisorRequired:   supervisorRequired,
		SupervisorUserId:     normalizedTemplate.SupervisorUserId,
		SupervisorBounty:     supervisorBounty,
		SupervisorDataFields: normalizedTemplate.SupervisorDataFields,
		Roles:                normalizedTemplate.Roles,
		Steps:                normalizedTemplate.Steps,
		TotalBounty:          normalizedTemplate.TotalBounty,
		WeeklyRequirement:    weeklyBountyRequirement(normalizedTemplate.TotalBounty, normalizedTemplate.Recurrence),
	}, nil
}

// upsertWorkflowStateVersionTx reuses an existing state/version id when the
// normalized definition payload is unchanged for a series.
func upsertWorkflowStateVersionTx(
	ctx context.Context,
	tx pgx.Tx,
	seriesId string,
	proposerId string,
	def *normalizedWorkflowDefinitionData,
	sourceWorkflowID *string,
	proposedByUserID *string,
) (string, error) {
	if def == nil {
		return "", fmt.Errorf("workflow definition is required")
	}

	rolesJSON, err := json.Marshal(def.Roles)
	if err != nil {
		return "", fmt.Errorf("error marshalling workflow state roles: %s", err)
	}
	stepsJSON, err := json.Marshal(def.Steps)
	if err != nil {
		return "", fmt.Errorf("error marshalling workflow state steps: %s", err)
	}
	supervisorDataJSON, err := json.Marshal(def.SupervisorDataFields)
	if err != nil {
		return "", fmt.Errorf("error marshalling workflow state supervisor data: %s", err)
	}

	var existingStateID string
	err = tx.QueryRow(ctx, `
		SELECT
			ws.id
		FROM
			workflow_states ws
		WHERE
			ws.series_id = $1
		AND
			ws.title = $2
		AND
			ws.description = $3
		AND
			ws.recurrence = $4
		AND
			ws.recurrence_end_at IS NOT DISTINCT FROM $5
		AND
			ws.supervisor_user_id IS NOT DISTINCT FROM $6
		AND
			ws.supervisor_bounty = $7
		AND
			ws.supervisor_data_json = $8::jsonb
		AND
			ws.roles_json = $9::jsonb
		AND
			ws.steps_json = $10::jsonb
		ORDER BY
			ws.created_at ASC,
			ws.id ASC
		LIMIT 1
		FOR UPDATE;
	`, seriesId, def.Title, def.Description, def.Recurrence, def.RecurrenceEndAt, def.SupervisorUserId, def.SupervisorBounty, string(supervisorDataJSON), string(rolesJSON), string(stepsJSON)).Scan(&existingStateID)
	if err == nil && strings.TrimSpace(existingStateID) != "" {
		return existingStateID, nil
	}
	if err != nil && err != pgx.ErrNoRows {
		return "", fmt.Errorf("error checking for existing workflow state version: %s", err)
	}

	stateID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO workflow_states(
			id,
			series_id,
			proposer_id,
			title,
			description,
			recurrence,
			recurrence_end_at,
			supervisor_user_id,
			supervisor_bounty,
			supervisor_data_json,
			roles_json,
			steps_json,
			source_workflow_id,
			proposed_by_user_id
		)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11::jsonb, $12::jsonb, $13, $14);
	`, stateID, seriesId, proposerId, def.Title, def.Description, def.Recurrence, def.RecurrenceEndAt, def.SupervisorUserId, def.SupervisorBounty, string(supervisorDataJSON), string(rolesJSON), string(stepsJSON), sourceWorkflowID, proposedByUserID)
	if err != nil {
		return "", fmt.Errorf("error inserting workflow state: %s", err)
	}

	return stateID, nil
}

func applyWorkflowStateVersionToSeriesTx(ctx context.Context, tx pgx.Tx, seriesId string, stateID string) error {
	cmd, err := tx.Exec(ctx, `
		UPDATE workflow_series s
		SET
			current_state_id = st.id,
			title = st.title,
			description = st.description,
			recurrence = st.recurrence,
			recurrence_end_at = st.recurrence_end_at,
			supervisor_data_json = st.supervisor_data_json,
			updated_at = unix_now()
		FROM
			workflow_states st
		WHERE
			s.id = $1
		AND
			st.id = $2
		AND
			st.series_id = s.id;
	`, seriesId, stateID)
	if err != nil {
		return fmt.Errorf("error applying workflow state to series: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("workflow series state update failed")
	}
	return nil
}

func syncWorkflowLinkedStatePresentationFieldsTx(ctx context.Context, tx pgx.Tx, seriesId string, stateID string) error {
	cmd, err := tx.Exec(ctx, `
		UPDATE workflow_states ws
		SET
			title = st.title,
			description = st.description
		FROM
			workflow_states st
		WHERE
			st.id = $2
		AND
			ws.id IN (
				SELECT DISTINCT
					w.workflow_state_id
				FROM
					workflows w
				WHERE
					w.series_id = $1
				AND
					w.workflow_state_id IS NOT NULL
			);
	`, seriesId, stateID)
	if err != nil {
		return fmt.Errorf("error syncing workflow state presentation fields: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil
	}
	return nil
}

func (a *AppDB) CreateWorkflowTemplate(
	ctx context.Context,
	creatorUserId string,
	req *structs.WorkflowTemplateCreateRequest,
	isDefault bool,
) (*structs.WorkflowTemplate, error) {
	validCredentials, err := a.getValidCredentialTypeSet(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading credential types: %s", err)
	}
	normalized, err := normalizeWorkflowTemplateData(req, validCredentials)
	if err != nil {
		return nil, err
	}

	templateTitle := strings.TrimSpace(req.TemplateTitle)
	templateDescription := strings.TrimSpace(req.TemplateDescription)
	if templateTitle == "" {
		return nil, fmt.Errorf("template_title is required")
	}

	var ownerUserId *string
	if !isDefault {
		ownerUserId = &creatorUserId
	}

	rolesJSON, err := json.Marshal(normalized.Roles)
	if err != nil {
		return nil, fmt.Errorf("error marshalling template roles: %s", err)
	}
	stepsJSON, err := json.Marshal(normalized.Steps)
	if err != nil {
		return nil, fmt.Errorf("error marshalling template steps: %s", err)
	}
	supervisorDataJSON, err := json.Marshal(normalized.SupervisorDataFields)
	if err != nil {
		return nil, fmt.Errorf("error marshalling template supervisor data fields: %s", err)
	}
	templateId := uuid.NewString()
	_, err = a.db.Exec(ctx, `
		INSERT INTO workflow_templates
			(
				id,
				template_title,
				template_description,
				owner_user_id,
				created_by_user_id,
					is_default,
					recurrence,
					start_at,
					series_id,
					supervisor_user_id,
					supervisor_bounty,
					supervisor_data_json,
					roles_json,
					steps_json
				)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13::jsonb, $14::jsonb);
		`, templateId, templateTitle, templateDescription, ownerUserId, creatorUserId, isDefault, normalized.Recurrence, normalized.StartAt.UTC().Unix(), normalized.SeriesId, normalized.SupervisorUserId, normalized.SupervisorBounty, string(supervisorDataJSON), string(rolesJSON), string(stepsJSON))
	if err != nil {
		return nil, fmt.Errorf("error creating workflow template: %s", err)
	}

	return a.GetWorkflowTemplateByID(ctx, templateId)
}

func (a *AppDB) GetWorkflowTemplateByID(ctx context.Context, templateId string) (*structs.WorkflowTemplate, error) {
	row := a.db.QueryRow(ctx, `
			SELECT
				id,
				template_title,
				template_description,
			owner_user_id,
			created_by_user_id,
			is_default,
				recurrence,
				start_at,
				series_id,
				supervisor_user_id,
				supervisor_bounty,
				COALESCE(supervisor_data_json, '[]'::jsonb),
				roles_json,
				steps_json,
				created_at,
				updated_at
		FROM
			workflow_templates
		WHERE
			id = $1;
	`, templateId)

	template := &structs.WorkflowTemplate{}
	var supervisorUserId *string
	var supervisorBounty *uint64
	var supervisorDataBytes []byte
	var rolesBytes []byte
	var stepsBytes []byte
	if err := row.Scan(
		&template.Id,
		&template.TemplateTitle,
		&template.TemplateDescription,
		&template.OwnerUserId,
		&template.CreatedByUserId,
		&template.IsDefault,
		&template.Recurrence,
		&template.StartAt,
		&template.SeriesId,
		&supervisorUserId,
		&supervisorBounty,
		&supervisorDataBytes,
		&rolesBytes,
		&stepsBytes,
		&template.CreatedAt,
		&template.UpdatedAt,
	); err != nil {
		return nil, err
	}

	template.SupervisorUserId = supervisorUserId
	template.SupervisorBounty = supervisorBounty
	template.SupervisorDataFields = []structs.WorkflowSupervisorDataField{}
	if len(supervisorDataBytes) > 0 {
		if err := json.Unmarshal(supervisorDataBytes, &template.SupervisorDataFields); err != nil {
			return nil, fmt.Errorf("error unmarshalling template supervisor data fields: %s", err)
		}
	}
	template.Manager = nil

	template.Roles = []structs.WorkflowRoleCreateInput{}
	if len(rolesBytes) > 0 {
		if err := json.Unmarshal(rolesBytes, &template.Roles); err != nil {
			return nil, fmt.Errorf("error unmarshalling template roles: %s", err)
		}
	}

	template.Steps = []structs.WorkflowStepCreateInput{}
	if len(stepsBytes) > 0 {
		if err := json.Unmarshal(stepsBytes, &template.Steps); err != nil {
			return nil, fmt.Errorf("error unmarshalling template steps: %s", err)
		}
	}

	return template, nil
}

func (a *AppDB) GetWorkflowTemplatesForProposer(ctx context.Context, proposerId string) ([]*structs.WorkflowTemplate, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			template_title,
			template_description,
			owner_user_id,
			created_by_user_id,
			is_default,
				recurrence,
				start_at,
				series_id,
				supervisor_user_id,
				supervisor_bounty,
				COALESCE(supervisor_data_json, '[]'::jsonb),
				roles_json,
				steps_json,
				created_at,
				updated_at
		FROM
			workflow_templates
		WHERE
			is_default = true
		OR
			owner_user_id = $1
		ORDER BY
			is_default DESC,
			created_at DESC;
	`, proposerId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow templates: %s", err)
	}
	defer rows.Close()

	templates := []*structs.WorkflowTemplate{}
	for rows.Next() {
		template := &structs.WorkflowTemplate{}
		var supervisorUserId *string
		var supervisorBounty *uint64
		var supervisorDataBytes []byte
		var rolesBytes []byte
		var stepsBytes []byte
		if err := rows.Scan(
			&template.Id,
			&template.TemplateTitle,
			&template.TemplateDescription,
			&template.OwnerUserId,
			&template.CreatedByUserId,
			&template.IsDefault,
			&template.Recurrence,
			&template.StartAt,
			&template.SeriesId,
			&supervisorUserId,
			&supervisorBounty,
			&supervisorDataBytes,
			&rolesBytes,
			&stepsBytes,
			&template.CreatedAt,
			&template.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow template: %s", err)
		}

		template.SupervisorUserId = supervisorUserId
		template.SupervisorBounty = supervisorBounty
		template.SupervisorDataFields = []structs.WorkflowSupervisorDataField{}
		if len(supervisorDataBytes) > 0 {
			if err := json.Unmarshal(supervisorDataBytes, &template.SupervisorDataFields); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow template supervisor data fields: %s", err)
			}
		}
		template.Manager = nil

		template.Roles = []structs.WorkflowRoleCreateInput{}
		if len(rolesBytes) > 0 {
			if err := json.Unmarshal(rolesBytes, &template.Roles); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow template roles: %s", err)
			}
		}

		template.Steps = []structs.WorkflowStepCreateInput{}
		if len(stepsBytes) > 0 {
			if err := json.Unmarshal(stepsBytes, &template.Steps); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow template steps: %s", err)
			}
		}

		templates = append(templates, template)
	}

	return templates, nil
}

func (a *AppDB) DeleteWorkflowTemplate(ctx context.Context, templateId, proposerId string, isAdmin bool) error {
	cmd, err := a.db.Exec(ctx, `
		DELETE FROM workflow_templates
		WHERE
			id = $1
		AND (
			(owner_user_id = $2 AND is_default = false)
			OR
			($3 = true AND is_default = true)
		);
	`, templateId, proposerId, isAdmin)
	if err != nil {
		return fmt.Errorf("error deleting workflow template: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("template not found or not deletable by user")
	}
	return nil
}

func (a *AppDB) CreateWorkflow(
	ctx context.Context,
	proposerId string,
	req *structs.WorkflowCreateRequest,
	startAt time.Time,
	recurrenceEndAt *time.Time,
) (*structs.Workflow, error) {
	if req == nil {
		return nil, fmt.Errorf("workflow request is required")
	}

	if req.SeriesId != nil && strings.TrimSpace(*req.SeriesId) != "" {
		return nil, fmt.Errorf("invalid series_id: manual series_id is not allowed")
	}

	validCredentialTypes, err := a.getValidCredentialTypeSet(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading credential types: %s", err)
	}

	definition, err := normalizeWorkflowDefinitionData(
		req.Title,
		req.Description,
		req.Recurrence,
		recurrenceEndAt,
		req.Supervisor,
		req.SupervisorDataFields,
		req.Roles,
		req.Steps,
		validCredentialTypes,
	)
	if err != nil {
		return nil, err
	}
	if definition.Recurrence != "one_time" && definition.RecurrenceEndAt != nil && *definition.RecurrenceEndAt < startAt.UTC().Unix() {
		return nil, fmt.Errorf("recurrence_end_at must be on or after start_at")
	}

	autoApproveWithoutVote := definition.TotalBounty == 0 && definition.SupervisorUserId != nil && *definition.SupervisorUserId == proposerId
	seriesId := uuid.NewString()

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var proposerStatus string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			proposers
		WHERE
			user_id = $1
		FOR UPDATE;
	`, proposerId).Scan(&proposerStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("proposer not found")
		}
		return nil, err
	}
	if proposerStatus != "approved" {
		return nil, fmt.Errorf("proposer is not approved")
	}

	if definition.SupervisorUserId != nil {
		var isSupervisor bool
		var supervisorStatus string
		err = tx.QueryRow(ctx, `
				SELECT
					u.is_supervisor,
					COALESCE(
						(
							SELECT
								s.status
							FROM
								supervisors s
							WHERE
								s.user_id = u.id
						),
						''
					)
				FROM
					users u
				WHERE
					u.id = $1
				FOR UPDATE;
			`, *definition.SupervisorUserId).Scan(&isSupervisor, &supervisorStatus)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, fmt.Errorf("workflow supervisor user not found")
			}
			return nil, fmt.Errorf("error validating workflow supervisor: %s", err)
		}
		if !isSupervisor || strings.TrimSpace(supervisorStatus) != "approved" {
			return nil, fmt.Errorf("workflow supervisor must be approved")
		}
	}

	isStartBlocked := false
	var blockedById *string

	workflowId := uuid.NewString()
	status := "pending"
	proposerActorID := proposerId

	supervisorDataJSON, err := json.Marshal(definition.SupervisorDataFields)
	if err != nil {
		return nil, fmt.Errorf("error marshalling workflow supervisor data fields: %s", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO workflow_series(
			id,
			proposer_id,
			title,
			description,
			recurrence,
			recurrence_end_at,
			supervisor_data_json
		)
		VALUES
			($1, $2, $3, $4, $5, $6, $7::jsonb);
	`, seriesId, proposerId, definition.Title, definition.Description, definition.Recurrence, definition.RecurrenceEndAt, string(supervisorDataJSON))
	if err != nil {
		return nil, fmt.Errorf("error inserting workflow series: %s", err)
	}

	stateID, err := upsertWorkflowStateVersionTx(ctx, tx, seriesId, proposerId, definition, nil, &proposerActorID)
	if err != nil {
		return nil, err
	}
	if err := applyWorkflowStateVersionToSeriesTx(ctx, tx, seriesId, stateID); err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO workflows
			(
				id,
				series_id,
				workflow_state_id,
				proposer_id,
				start_at,
				status,
				is_start_blocked,
				blocked_by_workflow_id,
				total_bounty,
				weekly_bounty_requirement,
					budget_weekly_deducted,
					budget_one_time_deducted,
					manager_required,
					manager_improver_id,
					manager_bounty
				)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15);
		`, workflowId, seriesId, stateID, proposerId, startAt.UTC().Unix(), status, isStartBlocked, blockedById, definition.TotalBounty, definition.WeeklyRequirement, 0, 0, definition.SupervisorRequired, definition.SupervisorUserId, definition.SupervisorBounty)
	if err != nil {
		return nil, fmt.Errorf("error inserting workflow: %s", err)
	}

	roleIds := map[string]string{}
	for _, roleInput := range definition.Roles {
		title := strings.TrimSpace(roleInput.Title)
		roleId := uuid.NewString()
		roleClientId := strings.TrimSpace(roleInput.ClientId)
		if _, exists := roleIds[roleClientId]; exists {
			return nil, fmt.Errorf("duplicate workflow role client_id: %s", roleClientId)
		}
		roleIds[roleClientId] = roleId

		_, err = tx.Exec(ctx, `
			INSERT INTO workflow_roles
				(id, workflow_id, title)
			VALUES
				($1, $2, $3);
		`, roleId, workflowId, title)
		if err != nil {
			return nil, fmt.Errorf("error inserting workflow role: %s", err)
		}

		for _, credential := range roleInput.RequiredCredentials {
			_, err = tx.Exec(ctx, `
				INSERT INTO workflow_role_credentials
					(role_id, credential_type)
				VALUES
					($1, $2);
			`, roleId, credential)
			if err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "workflow_role_credentials_credential_type_fk" {
					return nil, fmt.Errorf("invalid workflow role credential: %s", credential)
				}
				return nil, fmt.Errorf("error inserting workflow role credential: %s", err)
			}
		}
	}

	now := time.Now().UTC()
	for stepIndex, stepInput := range definition.Steps {
		stepTitle := strings.TrimSpace(stepInput.Title)

		stepId := uuid.NewString()
		stepStatus := "locked"
		if stepIndex == 0 && !startAt.After(now) {
			stepStatus = "available"
		}

		var roleId *string
		roleClientId := strings.TrimSpace(stepInput.RoleClientId)
		mappedRoleId, ok := roleIds[roleClientId]
		if !ok {
			return nil, fmt.Errorf("workflow step references unknown role client_id: %s", roleClientId)
		}
		roleId = &mappedRoleId

		_, err = tx.Exec(ctx, `
			INSERT INTO workflow_steps
				(id, series_id, workflow_id, step_order, title, description, bounty, allow_step_not_possible, role_id, status)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
		`, stepId, seriesId, workflowId, stepIndex+1, stepTitle, strings.TrimSpace(stepInput.Description), stepInput.Bounty, stepInput.AllowStepNotPossible, roleId, stepStatus)
		if err != nil {
			return nil, fmt.Errorf("error inserting workflow step: %s", err)
		}

		for itemIndex, itemInput := range stepInput.WorkItems {
			itemTitle := strings.TrimSpace(itemInput.Title)

			photoRequiredCount := itemInput.PhotoRequiredCount
			if photoRequiredCount <= 0 {
				photoRequiredCount = 1
			}
			photoAllowAnyCount := itemInput.RequiresPhoto && itemInput.PhotoAllowAnyCount
			if !itemInput.RequiresPhoto {
				photoAllowAnyCount = false
			}
			photoAspectRatio, err := normalizeWorkflowPhotoAspectRatio(itemInput.PhotoAspectRatio)
			if err != nil {
				return nil, fmt.Errorf("workflow work item photo_aspect_ratio is invalid")
			}

			dropdownOptions := []structs.WorkflowDropdownOption{}
			dropdownRequiresWritten := map[string]bool{}
			for _, option := range itemInput.DropdownOptions {
				label := strings.TrimSpace(option.Label)
				value := deriveDropdownValueFromLabel(label)
				if value == "" {
					continue
				}
				dropdownOptions = append(dropdownOptions, structs.WorkflowDropdownOption{
					Value:                   value,
					Label:                   label,
					RequiresWrittenResponse: option.RequiresWrittenResponse,
					NotifyEmails:            option.NotifyEmails,
					SendPicturesWithEmail:   option.SendPicturesWithEmail,
				})
				dropdownRequiresWritten[value] = option.RequiresWrittenResponse
			}

			dropdownOptionsJSON, err := json.Marshal(dropdownOptions)
			if err != nil {
				return nil, fmt.Errorf("error marshalling dropdown options: %s", err)
			}
			dropdownRequiresJSON, err := json.Marshal(dropdownRequiresWritten)
			if err != nil {
				return nil, fmt.Errorf("error marshalling dropdown requirement map: %s", err)
			}

			legacyNotifyEmailsJSON, err := json.Marshal([]string{})
			if err != nil {
				return nil, fmt.Errorf("error marshalling legacy notify emails: %s", err)
			}

			legacyNotifyValuesJSON, err := json.Marshal([]string{})
			if err != nil {
				return nil, fmt.Errorf("error marshalling legacy notify dropdown values: %s", err)
			}

			itemId := uuid.NewString()
			_, err = tx.Exec(ctx, `
				INSERT INTO workflow_step_items
					(
						id,
						step_id,
						item_order,
						title,
						description,
							is_optional,
							requires_photo,
							camera_capture_only,
							photo_required_count,
							photo_allow_any_count,
							photo_aspect_ratio,
							requires_written_response,
							requires_dropdown,
							dropdown_options,
							dropdown_requires_written_response,
							notify_emails,
						notify_on_dropdown_values
					)
						VALUES
							($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15::jsonb, $16::jsonb, $17::jsonb);
				`, itemId, stepId, itemIndex+1, itemTitle, strings.TrimSpace(itemInput.Description), itemInput.Optional, itemInput.RequiresPhoto, itemInput.RequiresPhoto && itemInput.CameraCaptureOnly, photoRequiredCount, photoAllowAnyCount, photoAspectRatio, itemInput.RequiresWritten, itemInput.RequiresDropdown, string(dropdownOptionsJSON), string(dropdownRequiresJSON), string(legacyNotifyEmailsJSON), string(legacyNotifyValuesJSON))
			if err != nil {
				return nil, fmt.Errorf("error inserting workflow work item: %s", err)
			}
		}
	}

	if autoApproveWithoutVote {
		if err := finalizeWorkflowApprovalTx(ctx, tx, workflowId, isStartBlocked, nil, "approve"); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetWorkflowByID(ctx, workflowId)
}

func (a *AppDB) GetWorkflowsByProposer(ctx context.Context, proposerId string) ([]*structs.Workflow, error) {
	totalVoters, err := a.CountEligibleVoters(ctx)
	if err != nil {
		return nil, fmt.Errorf("error counting eligible voters: %s", err)
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			w.id,
			w.series_id,
			w.workflow_state_id,
			w.proposer_id,
			COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')),
			COALESCE(st.description, s.description, ''),
			COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')),
			COALESCE(st.recurrence_end_at, s.recurrence_end_at),
			w.start_at,
			w.status,
			w.is_start_blocked,
			w.blocked_by_workflow_id,
			w.total_bounty,
			w.weekly_bounty_requirement,
			w.budget_weekly_deducted,
			w.budget_one_time_deducted,
			w.vote_quorum_reached_at,
			w.vote_finalize_at,
			w.vote_finalized_at,
			w.vote_finalized_by_user_id,
			w.vote_decision,
			w.manager_required,
			w.manager_improver_id,
			w.manager_bounty,
			w.manager_paid_out_at,
			w.manager_payout_error,
			w.manager_payout_last_try_at,
			w.manager_retry_requested_at,
			w.manager_retry_requested_by,
			w.created_at,
			w.updated_at,
			COALESCE(v.approve_count, 0) AS approve_count,
			COALESCE(v.deny_count, 0) AS deny_count
		FROM
			workflows w
		LEFT JOIN
			workflow_states st
		ON
			st.id = w.workflow_state_id
		LEFT JOIN
			workflow_series s
		ON
			s.id = w.series_id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE decision = 'approve') AS approve_count,
				COUNT(*) FILTER (WHERE decision = 'deny') AS deny_count
			FROM
				workflow_votes
			WHERE
				workflow_id = w.id
		) v
		ON
			true
		WHERE
			w.proposer_id = $1
		AND
			w.status <> 'deleted'
		ORDER BY
			w.created_at DESC;
	`, proposerId)
	if err != nil {
		return nil, fmt.Errorf("error querying proposer workflows: %s", err)
	}
	defer rows.Close()

	results := []*structs.Workflow{}
	for rows.Next() {
		workflow := &structs.Workflow{
			Roles:                []structs.WorkflowRole{},
			Steps:                []structs.WorkflowStep{},
			SupervisorDataFields: []structs.WorkflowSupervisorDataField{},
		}
		var approveCount int
		var denyCount int
		if err := rows.Scan(
			&workflow.Id,
			&workflow.SeriesId,
			&workflow.WorkflowStateId,
			&workflow.ProposerId,
			&workflow.Title,
			&workflow.Description,
			&workflow.Recurrence,
			&workflow.RecurrenceEndAt,
			&workflow.StartAt,
			&workflow.Status,
			&workflow.IsStartBlocked,
			&workflow.BlockedByWorkflowId,
			&workflow.TotalBounty,
			&workflow.WeeklyBountyRequirement,
			&workflow.BudgetWeeklyDeducted,
			&workflow.BudgetOneTimeDeducted,
			&workflow.VoteQuorumReachedAt,
			&workflow.VoteFinalizeAt,
			&workflow.VoteFinalizedAt,
			&workflow.VoteFinalizedByUserId,
			&workflow.VoteDecision,
			&workflow.ManagerRequired,
			&workflow.ManagerImproverId,
			&workflow.ManagerBounty,
			&workflow.ManagerPaidOutAt,
			&workflow.ManagerPayoutError,
			&workflow.ManagerPayoutLastTryAt,
			&workflow.SupervisorRetryRequestedAt,
			&workflow.SupervisorRetryRequestedBy,
			&workflow.CreatedAt,
			&workflow.UpdatedAt,
			&approveCount,
			&denyCount,
		); err != nil {
			return nil, fmt.Errorf("error scanning proposer workflow summary: %s", err)
		}

		workflow.SupervisorRequired = workflow.ManagerRequired
		workflow.SupervisorUserId = workflow.ManagerImproverId
		workflow.SupervisorBounty = workflow.ManagerBounty
		workflow.SupervisorPaidOutAt = workflow.ManagerPaidOutAt
		workflow.SupervisorPayoutError = workflow.ManagerPayoutError
		workflow.SupervisorPayoutLastTryAt = workflow.ManagerPayoutLastTryAt

		workflow.Votes = structs.WorkflowVotes{
			Approve:         approveCount,
			Deny:            denyCount,
			TotalVoters:     totalVoters,
			VotesCast:       approveCount + denyCount,
			QuorumThreshold: quorumVotesRequired(totalVoters),
			QuorumReachedAt: workflow.VoteQuorumReachedAt,
			FinalizeAt:      workflow.VoteFinalizeAt,
			FinalizedAt:     workflow.VoteFinalizedAt,
			Decision:        workflow.VoteDecision,
		}
		workflow.Votes.QuorumReached = workflow.Votes.VotesCast >= workflow.Votes.QuorumThreshold && totalVoters > 0

		results = append(results, workflow)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating proposer workflow summaries: %s", err)
	}

	return results, nil
}

func (a *AppDB) GetWorkflowByID(ctx context.Context, workflowId string) (*structs.Workflow, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			w.id,
			w.series_id,
			w.workflow_state_id,
			w.proposer_id,
			COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')),
			COALESCE(st.description, s.description, ''),
			COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')),
			COALESCE(st.recurrence_end_at, s.recurrence_end_at),
			COALESCE(st.supervisor_data_json, s.supervisor_data_json, '[]'::jsonb),
			w.start_at,
			w.status,
			w.is_start_blocked,
			w.blocked_by_workflow_id,
			w.total_bounty,
			w.weekly_bounty_requirement,
			w.budget_weekly_deducted,
			w.budget_one_time_deducted,
			w.vote_quorum_reached_at,
			w.vote_finalize_at,
			w.vote_finalized_at,
			w.vote_finalized_by_user_id,
			w.vote_decision,
			w.manager_required,
			w.manager_role_id,
			w.manager_improver_id,
			w.manager_bounty,
			w.manager_paid_out_at,
			w.manager_payout_error,
			w.manager_payout_last_try_at,
			w.manager_payout_in_progress,
			w.manager_retry_requested_at,
			w.manager_retry_requested_by,
			w.created_at,
			w.updated_at
		FROM
			workflows w
		LEFT JOIN
			workflow_states st
		ON
			st.id = w.workflow_state_id
		LEFT JOIN
			workflow_series s
		ON
			s.id = w.series_id
		WHERE
			w.id = $1;
	`, workflowId)

	workflow := &structs.Workflow{}
	var supervisorDataBytes []byte
	err := row.Scan(
		&workflow.Id,
		&workflow.SeriesId,
		&workflow.WorkflowStateId,
		&workflow.ProposerId,
		&workflow.Title,
		&workflow.Description,
		&workflow.Recurrence,
		&workflow.RecurrenceEndAt,
		&supervisorDataBytes,
		&workflow.StartAt,
		&workflow.Status,
		&workflow.IsStartBlocked,
		&workflow.BlockedByWorkflowId,
		&workflow.TotalBounty,
		&workflow.WeeklyBountyRequirement,
		&workflow.BudgetWeeklyDeducted,
		&workflow.BudgetOneTimeDeducted,
		&workflow.VoteQuorumReachedAt,
		&workflow.VoteFinalizeAt,
		&workflow.VoteFinalizedAt,
		&workflow.VoteFinalizedByUserId,
		&workflow.VoteDecision,
		&workflow.ManagerRequired,
		&workflow.ManagerRoleId,
		&workflow.ManagerImproverId,
		&workflow.ManagerBounty,
		&workflow.ManagerPaidOutAt,
		&workflow.ManagerPayoutError,
		&workflow.ManagerPayoutLastTryAt,
		&workflow.ManagerPayoutInProgress,
		&workflow.ManagerRetryRequestedAt,
		&workflow.ManagerRetryRequestedBy,
		&workflow.CreatedAt,
		&workflow.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	workflow.SupervisorDataFields = []structs.WorkflowSupervisorDataField{}
	if len(supervisorDataBytes) > 0 {
		if err := json.Unmarshal(supervisorDataBytes, &workflow.SupervisorDataFields); err != nil {
			return nil, fmt.Errorf("error unmarshalling workflow supervisor data fields: %s", err)
		}
	}

	workflow.SupervisorRequired = workflow.ManagerRequired
	workflow.SupervisorUserId = workflow.ManagerImproverId
	workflow.SupervisorBounty = workflow.ManagerBounty
	workflow.SupervisorPaidOutAt = workflow.ManagerPaidOutAt
	workflow.SupervisorPayoutError = workflow.ManagerPayoutError
	workflow.SupervisorPayoutLastTryAt = workflow.ManagerPayoutLastTryAt
	workflow.SupervisorRetryRequestedAt = workflow.ManagerRetryRequestedAt
	workflow.SupervisorRetryRequestedBy = workflow.ManagerRetryRequestedBy
	workflow.SupervisorTitle = nil
	workflow.SupervisorOrganization = nil
	if workflow.SupervisorUserId != nil && strings.TrimSpace(*workflow.SupervisorUserId) != "" {
		var supervisorTitle *string
		var supervisorOrganization *string
		if err := a.db.QueryRow(ctx, `
			SELECT
				NULLIF(TRIM(COALESCE(s.nickname, s.organization, u.contact_name, '')), ''),
				NULLIF(TRIM(COALESCE(s.organization, '')), '')
			FROM
				users u
			LEFT JOIN
				supervisors s
			ON
				s.user_id = u.id
			WHERE
				u.id = $1;
		`, *workflow.SupervisorUserId).Scan(&supervisorTitle, &supervisorOrganization); err == nil {
			workflow.SupervisorTitle = supervisorTitle
			workflow.SupervisorOrganization = supervisorOrganization
		}
	}

	roles, err := a.getWorkflowRoles(ctx, workflowId)
	if err != nil {
		return nil, err
	}
	workflow.Roles = roles

	steps, err := a.getWorkflowSteps(ctx, workflowId)
	if err != nil {
		return nil, err
	}
	workflow.Steps = steps

	votes, err := a.GetWorkflowVotes(ctx, workflowId)
	if err != nil {
		return nil, err
	}
	workflow.Votes = *votes

	return workflow, nil
}

func (a *AppDB) getWorkflowRoles(ctx context.Context, workflowId string) ([]structs.WorkflowRole, error) {
	rows, err := a.db.Query(ctx, `
			SELECT
				id,
				workflow_id,
				title,
				is_manager
		FROM
			workflow_roles
		WHERE
			workflow_id = $1
		AND
			is_manager = false;
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow roles: %s", err)
	}
	defer rows.Close()

	roles := []structs.WorkflowRole{}
	roleIndex := map[string]int{}
	for rows.Next() {
		role := structs.WorkflowRole{}
		if err := rows.Scan(&role.Id, &role.WorkflowId, &role.Title, &role.IsManager); err != nil {
			return nil, fmt.Errorf("error scanning workflow role: %s", err)
		}
		role.RequiredCredentials = []string{}
		roleIndex[role.Id] = len(roles)
		roles = append(roles, role)
	}

	credRows, err := a.db.Query(ctx, `
		SELECT
			role_id,
			credential_type
		FROM
			workflow_role_credentials
		WHERE
			role_id IN (
				SELECT id FROM workflow_roles WHERE workflow_id = $1
			);
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow role credentials: %s", err)
	}
	defer credRows.Close()

	for credRows.Next() {
		var roleId string
		var credential string
		if err := credRows.Scan(&roleId, &credential); err != nil {
			return nil, fmt.Errorf("error scanning workflow role credential: %s", err)
		}
		if idx, ok := roleIndex[roleId]; ok {
			roles[idx].RequiredCredentials = append(roles[idx].RequiredCredentials, credential)
		}
	}

	return roles, nil
}

func (a *AppDB) getWorkflowSteps(ctx context.Context, workflowId string) ([]structs.WorkflowStep, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			ws.id,
			ws.workflow_id,
			ws.step_order,
			ws.title,
			ws.description,
			ws.bounty,
			ws.allow_step_not_possible,
			ws.role_id,
			ws.assigned_improver_id,
			CASE
				WHEN COALESCE(NULLIF(BTRIM(i.first_name), ''), '') = '' THEN NULL
				WHEN COALESCE(NULLIF(BTRIM(i.last_name), ''), '') = '' THEN BTRIM(i.first_name)
				ELSE BTRIM(i.first_name) || ' ' || UPPER(LEFT(BTRIM(i.last_name), 1)) || '.'
			END AS assigned_improver_name,
			ws.status,
			ws.started_at,
			ws.completed_at,
			ws.payout_error,
			ws.payout_last_try_at,
			ws.payout_in_progress,
			ws.retry_requested_at,
			ws.retry_requested_by
		FROM
			workflow_steps ws
		LEFT JOIN
			improvers i
		ON
			i.user_id = ws.assigned_improver_id
		WHERE
			ws.workflow_id = $1
		ORDER BY
			ws.step_order ASC;
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow steps: %s", err)
	}
	defer rows.Close()

	steps := []structs.WorkflowStep{}
	stepIndex := map[string]int{}
	for rows.Next() {
		step := structs.WorkflowStep{}
		if err := rows.Scan(
			&step.Id,
			&step.WorkflowId,
			&step.StepOrder,
			&step.Title,
			&step.Description,
			&step.Bounty,
			&step.AllowStepNotPossible,
			&step.RoleId,
			&step.AssignedImproverId,
			&step.AssignedImproverName,
			&step.Status,
			&step.StartedAt,
			&step.CompletedAt,
			&step.PayoutError,
			&step.PayoutLastTryAt,
			&step.PayoutInProgress,
			&step.RetryRequestedAt,
			&step.RetryRequestedBy,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow step: %s", err)
		}
		step.WorkItems = []structs.WorkflowWorkItem{}
		step.Submission = nil
		stepIndex[step.Id] = len(steps)
		steps = append(steps, step)
	}

	itemRows, err := a.db.Query(ctx, `
			SELECT
				id,
				step_id,
				item_order,
				title,
				description,
				is_optional,
				requires_photo,
				camera_capture_only,
				photo_required_count,
				photo_allow_any_count,
				photo_aspect_ratio,
				requires_written_response,
				requires_dropdown,
				dropdown_options,
				dropdown_requires_written_response,
				notify_emails,
			notify_on_dropdown_values
		FROM
			workflow_step_items
		WHERE
			step_id IN (
				SELECT id FROM workflow_steps WHERE workflow_id = $1
			)
		ORDER BY
			item_order ASC;
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow work items: %s", err)
	}
	defer itemRows.Close()

	for itemRows.Next() {
		item := structs.WorkflowWorkItem{}
		var dropdownOptionsBytes []byte
		var dropdownRequiresBytes []byte
		var notifyEmailsBytes []byte
		var notifyValuesBytes []byte
		if err := itemRows.Scan(
			&item.Id,
			&item.StepId,
			&item.ItemOrder,
			&item.Title,
			&item.Description,
			&item.Optional,
			&item.RequiresPhoto,
			&item.CameraCaptureOnly,
			&item.PhotoRequiredCount,
			&item.PhotoAllowAnyCount,
			&item.PhotoAspectRatio,
			&item.RequiresWrittenResponse,
			&item.RequiresDropdown,
			&dropdownOptionsBytes,
			&dropdownRequiresBytes,
			&notifyEmailsBytes,
			&notifyValuesBytes,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow work item: %s", err)
		}

		item.DropdownOptions = []structs.WorkflowDropdownOption{}
		if item.PhotoRequiredCount <= 0 {
			item.PhotoRequiredCount = 1
		}
		if normalizedAspect, aspectErr := normalizeWorkflowPhotoAspectRatio(item.PhotoAspectRatio); aspectErr == nil {
			item.PhotoAspectRatio = normalizedAspect
		} else {
			item.PhotoAspectRatio = defaultWorkflowPhotoAspectRatio
		}
		if !item.RequiresPhoto {
			item.PhotoAllowAnyCount = false
		}
		if len(dropdownOptionsBytes) > 0 {
			if err := json.Unmarshal(dropdownOptionsBytes, &item.DropdownOptions); err != nil {
				return nil, fmt.Errorf("error unmarshalling dropdown options: %s", err)
			}
		}
		for idx := range item.DropdownOptions {
			item.DropdownOptions[idx].NotifyEmails = normalizeEmailList(item.DropdownOptions[idx].NotifyEmails)
		}

		item.DropdownRequiresWrittenMap = map[string]bool{}
		if len(dropdownRequiresBytes) > 0 {
			if err := json.Unmarshal(dropdownRequiresBytes, &item.DropdownRequiresWrittenMap); err != nil {
				return nil, fmt.Errorf("error unmarshalling dropdown requirement map: %s", err)
			}
		}

		legacyNotifyEmails := []string{}
		if len(notifyEmailsBytes) > 0 {
			if err := json.Unmarshal(notifyEmailsBytes, &legacyNotifyEmails); err != nil {
				return nil, fmt.Errorf("error unmarshalling notify emails: %s", err)
			}
		}

		legacyNotifyValues := []string{}
		if len(notifyValuesBytes) > 0 {
			if err := json.Unmarshal(notifyValuesBytes, &legacyNotifyValues); err != nil {
				return nil, fmt.Errorf("error unmarshalling notify dropdown values: %s", err)
			}
		}
		legacyNotifyEmails = normalizeEmailList(legacyNotifyEmails)
		if len(legacyNotifyEmails) > 0 && len(legacyNotifyValues) > 0 {
			legacyWatchValues := map[string]struct{}{}
			for _, value := range legacyNotifyValues {
				value = strings.TrimSpace(value)
				if value == "" {
					continue
				}
				legacyWatchValues[value] = struct{}{}
			}
			if len(legacyWatchValues) > 0 {
				for idx := range item.DropdownOptions {
					if len(item.DropdownOptions[idx].NotifyEmails) > 0 {
						continue
					}
					if _, ok := legacyWatchValues[item.DropdownOptions[idx].Value]; !ok {
						continue
					}
					item.DropdownOptions[idx].NotifyEmails = append([]string{}, legacyNotifyEmails...)
				}
			}
		}
		for idx := range item.DropdownOptions {
			item.DropdownOptions[idx].NotifyEmailCount = len(item.DropdownOptions[idx].NotifyEmails)
		}

		if idx, ok := stepIndex[item.StepId]; ok {
			steps[idx].WorkItems = append(steps[idx].WorkItems, item)
		}
	}

	submissionRows, err := a.db.Query(ctx, `
		SELECT
			id,
			workflow_id,
			step_id,
			improver_id,
			step_not_possible,
			step_not_possible_details,
			item_responses,
			submitted_at,
			updated_at
		FROM
			workflow_step_submissions
		WHERE
			workflow_id = $1;
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow step submissions: %s", err)
	}
	defer submissionRows.Close()

	for submissionRows.Next() {
		submission := structs.WorkflowStepSubmission{}
		var itemResponsesBytes []byte
		if err := submissionRows.Scan(
			&submission.Id,
			&submission.WorkflowId,
			&submission.StepId,
			&submission.ImproverId,
			&submission.StepNotPossible,
			&submission.StepNotPossibleDetails,
			&itemResponsesBytes,
			&submission.SubmittedAt,
			&submission.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow step submission: %s", err)
		}

		submission.ItemResponses = []structs.WorkflowStepItemResponse{}
		if len(itemResponsesBytes) > 0 {
			if err := json.Unmarshal(itemResponsesBytes, &submission.ItemResponses); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow step submission item responses: %s", err)
			}
		}

		if idx, ok := stepIndex[submission.StepId]; ok {
			steps[idx].Submission = &submission
		}
	}

	photoRows, err := a.db.Query(ctx, `
		SELECT
			id,
			workflow_id,
			step_id,
			item_id,
			submission_id,
			file_name,
			content_type,
			size_bytes,
			created_at
		FROM
			workflow_submission_photos
		WHERE
			workflow_id = $1
		ORDER BY
			created_at ASC;
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow submission photos: %s", err)
	}
	defer photoRows.Close()

	photosBySubmissionItem := map[string][]structs.WorkflowSubmissionPhoto{}
	for photoRows.Next() {
		photo := structs.WorkflowSubmissionPhoto{}
		if err := photoRows.Scan(
			&photo.Id,
			&photo.WorkflowId,
			&photo.StepId,
			&photo.ItemId,
			&photo.SubmissionId,
			&photo.FileName,
			&photo.ContentType,
			&photo.SizeBytes,
			&photo.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow submission photo: %s", err)
		}
		key := photo.SubmissionId + ":" + photo.ItemId
		photosBySubmissionItem[key] = append(photosBySubmissionItem[key], photo)
	}

	for stepIdx := range steps {
		if steps[stepIdx].Submission == nil {
			continue
		}
		submission := steps[stepIdx].Submission
		for responseIdx := range submission.ItemResponses {
			response := submission.ItemResponses[responseIdx]
			photoIDSet := map[string]struct{}{}
			photoIDs := make([]string, 0, len(response.PhotoIDs))
			for _, photoID := range response.PhotoIDs {
				photoID = strings.TrimSpace(photoID)
				if photoID == "" {
					continue
				}
				if _, exists := photoIDSet[photoID]; exists {
					continue
				}
				photoIDSet[photoID] = struct{}{}
				photoIDs = append(photoIDs, photoID)
			}

			photos := []structs.WorkflowSubmissionPhoto{}
			key := submission.Id + ":" + response.ItemId
			if mappedPhotos, ok := photosBySubmissionItem[key]; ok {
				for _, photo := range mappedPhotos {
					photos = append(photos, photo)
					if _, exists := photoIDSet[photo.Id]; exists {
						continue
					}
					photoIDSet[photo.Id] = struct{}{}
					photoIDs = append(photoIDs, photo.Id)
				}
			}

			response.PhotoIDs = photoIDs
			response.Photos = photos
			response.PhotoUploads = nil
			submission.ItemResponses[responseIdx] = response
		}
	}

	return steps, nil
}

func (a *AppDB) DeleteWorkflowByProposer(ctx context.Context, workflowId string, proposerId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			workflows
		WHERE
			id = $1
		AND
			proposer_id = $2
		FOR UPDATE;
	`, workflowId, proposerId).Scan(&status)
	if err != nil {
		return err
	}

	if status == "deleted" {
		return fmt.Errorf("workflow already archived")
	}

	if status != "pending" && status != "rejected" && status != "expired" && status != "failed" && status != "skipped" {
		return fmt.Errorf("workflow cannot be archived in current status")
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			workflows
		SET
			status = 'deleted',
			is_start_blocked = false,
			blocked_by_workflow_id = NULL,
			updated_at = unix_now()
		WHERE
			id = $1;
	`, workflowId)
	if err != nil {
		return fmt.Errorf("error archiving workflow: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func weeklyBountyRequirement(total uint64, recurrence string) uint64 {
	switch recurrence {
	case "daily":
		return total * 7
	case "weekly":
		return total
	case "monthly":
		return (total + 3) / 4
	default:
		return total
	}
}

func nextRecurringStartAt(startAt int64, recurrence string) (int64, error) {
	base := time.Unix(startAt, 0).UTC()
	switch recurrence {
	case "daily":
		return base.AddDate(0, 0, 1).Unix(), nil
	case "weekly":
		return base.AddDate(0, 0, 7).Unix(), nil
	case "monthly":
		return base.AddDate(0, 1, 0).Unix(), nil
	default:
		return 0, fmt.Errorf("workflow is not recurring")
	}
}

func ensureWorkflowSeriesClaimTx(
	ctx context.Context,
	tx pgx.Tx,
	seriesId string,
	stepOrder int,
	improverId string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO workflow_series_step_claims(
			series_id,
			step_order,
			improver_id,
			created_at,
			updated_at
		)
		VALUES
			($1, $2, $3, unix_now(), unix_now())
		ON CONFLICT (series_id, step_order)
		DO UPDATE SET
			improver_id = EXCLUDED.improver_id,
			updated_at = unix_now();
	`, seriesId, stepOrder, improverId)
	if err != nil {
		return fmt.Errorf("error persisting workflow series claim: %s", err)
	}
	return nil
}

func propagateWorkflowSeriesClaimTx(
	ctx context.Context,
	tx pgx.Tx,
	seriesId string,
	stepOrder int,
	improverId string,
	minStartAt int64,
) error {
	_, err := tx.Exec(ctx, `
		WITH candidates AS (
			SELECT
				ws.id
			FROM
				workflow_steps ws
			JOIN
				workflows w
			ON
				w.id = ws.workflow_id
			JOIN
				workflow_series sr
			ON
				sr.id = w.series_id
			WHERE
				w.series_id = $1
			AND
				COALESCE(NULLIF(TRIM(sr.recurrence), ''), 'one_time') <> 'one_time'
			AND
				w.start_at >= $4
			AND
				w.status IN ('approved', 'blocked', 'in_progress')
			AND
				ws.step_order = $2
			AND
				ws.assigned_improver_id IS NULL
			AND
				ws.status IN ('locked', 'available')
			AND
				(w.manager_improver_id IS NULL OR w.manager_improver_id <> $3)
			AND
				NOT EXISTS (
					SELECT
						1
					FROM
						workflow_steps existing
					WHERE
						existing.workflow_id = ws.workflow_id
					AND
						existing.assigned_improver_id = $3
				)
			AND
				NOT EXISTS (
					SELECT
						1
					FROM
						workflow_improver_absences abs
					WHERE
						abs.improver_id = $3
					AND
						abs.series_id = w.series_id
					AND
						abs.step_order = ws.step_order
					AND
						w.start_at >= abs.absent_from
					AND
						w.start_at < abs.absent_until
				)
			FOR UPDATE
		),
		assigned AS (
			UPDATE
				workflow_steps ws
			SET
				assigned_improver_id = $3,
				updated_at = unix_now()
			WHERE
				ws.id IN (SELECT id FROM candidates)
			RETURNING
				ws.id,
				ws.status
		)
		INSERT INTO workflow_step_notifications(step_id, user_id, notification_type)
		SELECT
			a.id,
			$3,
			'step_available'
		FROM
			assigned a
		WHERE
			a.status = 'available'
		ON CONFLICT DO NOTHING;
	`, seriesId, stepOrder, improverId, minStartAt)
	if err != nil {
		return fmt.Errorf("error propagating workflow series claim: %s", err)
	}
	return nil
}

func applyWorkflowSeriesClaimsToWorkflowTx(ctx context.Context, tx pgx.Tx, workflowId string) error {
	_, err := tx.Exec(ctx, `
		WITH workflow_meta AS (
			SELECT
				id,
				series_id,
				start_at,
				manager_improver_id
			FROM
				workflows
			WHERE
				id = $1
		),
		candidate_raw AS (
			SELECT
				ws.id AS step_id,
				ws.step_order,
				ws.status AS step_status,
				c.improver_id,
				ROW_NUMBER() OVER (
					PARTITION BY c.improver_id
					ORDER BY ws.step_order ASC
				) AS improver_rank
			FROM
				workflow_steps ws
			JOIN
				workflow_meta wm
			ON
				wm.id = ws.workflow_id
			JOIN
				workflow_series_step_claims c
			ON
				c.series_id = wm.series_id
			AND
				c.step_order = ws.step_order
			WHERE
				ws.workflow_id = $1
			AND
				ws.assigned_improver_id IS NULL
			AND
				ws.status IN ('locked', 'available')
			AND
				(wm.manager_improver_id IS NULL OR wm.manager_improver_id <> c.improver_id)
			AND
				NOT EXISTS (
					SELECT
						1
					FROM
						workflow_steps existing
					WHERE
						existing.workflow_id = ws.workflow_id
					AND
						existing.assigned_improver_id = c.improver_id
				)
			AND
				NOT EXISTS (
					SELECT
						1
					FROM
						workflow_improver_absences abs
					WHERE
						abs.improver_id = c.improver_id
					AND
						abs.series_id = wm.series_id
					AND
						abs.step_order = ws.step_order
					AND
						wm.start_at >= abs.absent_from
					AND
						wm.start_at < abs.absent_until
				)
		),
		candidates AS (
			SELECT
				cr.step_id,
				cr.improver_id,
				cr.step_status
			FROM
				candidate_raw cr
			WHERE
				cr.improver_rank = 1
		),
		assigned AS (
			UPDATE
				workflow_steps ws
			SET
				assigned_improver_id = c.improver_id,
				updated_at = unix_now()
			FROM
				candidates c
			WHERE
				ws.id = c.step_id
			RETURNING
				ws.id,
				ws.assigned_improver_id,
				ws.status
		)
		INSERT INTO workflow_step_notifications(step_id, user_id, notification_type)
		SELECT
			a.id,
			a.assigned_improver_id,
			'step_available'
		FROM
			assigned a
		WHERE
			a.status = 'available'
		ON CONFLICT DO NOTHING;
	`, workflowId)
	if err != nil {
		return fmt.Errorf("error applying workflow series claims to workflow: %s", err)
	}
	return nil
}

func ensureRecurringWorkflowSuccessorTx(
	ctx context.Context,
	tx pgx.Tx,
	workflowId string,
) (string, error) {
	type workflowSeed struct {
		Id               string
		SeriesId         string
		ProposerId       string
		StartAt          int64
		Status           string
		WorkflowStateID  *string
		Recurrence       string
		RecurrenceEndAt  *int64
		SupervisorUserID *string
		SupervisorBounty uint64
		RolesJSON        []byte
		StepsJSON        []byte
	}

	seed := workflowSeed{}
	err := tx.QueryRow(ctx, `
		SELECT
			w.id,
			w.series_id,
			s.proposer_id,
			w.start_at,
			w.status,
			COALESCE(st.id, w.workflow_state_id),
			COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')),
			COALESCE(st.recurrence_end_at, s.recurrence_end_at),
			st.supervisor_user_id,
			COALESCE(st.supervisor_bounty, 0),
			COALESCE(st.roles_json, '[]'::jsonb),
			COALESCE(st.steps_json, '[]'::jsonb)
		FROM
			workflows w
		JOIN
			workflow_series s
		ON
			s.id = w.series_id
		LEFT JOIN
			workflow_states st
		ON
			st.id = COALESCE(s.current_state_id, w.workflow_state_id)
		WHERE
			w.id = $1
		FOR UPDATE OF w, s;
	`, workflowId).Scan(
		&seed.Id,
		&seed.SeriesId,
		&seed.ProposerId,
		&seed.StartAt,
		&seed.Status,
		&seed.WorkflowStateID,
		&seed.Recurrence,
		&seed.RecurrenceEndAt,
		&seed.SupervisorUserID,
		&seed.SupervisorBounty,
		&seed.RolesJSON,
		&seed.StepsJSON,
	)
	if err != nil {
		return "", fmt.Errorf("error loading recurring workflow seed: %s", err)
	}
	if seed.WorkflowStateID == nil || strings.TrimSpace(*seed.WorkflowStateID) == "" {
		return "", fmt.Errorf("recurring workflow state is not configured")
	}
	if seed.Recurrence == "one_time" {
		return "", nil
	}
	switch seed.Status {
	case "completed", "paid_out", "failed", "skipped":
	default:
		return "", nil
	}

	_, err = tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtext($1), 0);
	`, seed.SeriesId)
	if err != nil {
		return "", fmt.Errorf("error locking workflow series for recurrence: %s", err)
	}

	nextStartAt, err := nextRecurringStartAt(seed.StartAt, seed.Recurrence)
	if err != nil {
		return "", err
	}
	if seed.RecurrenceEndAt != nil && nextStartAt > *seed.RecurrenceEndAt {
		return "", nil
	}

	existingWorkflowId := ""
	err = tx.QueryRow(ctx, `
		SELECT
			id
		FROM
			workflows
		WHERE
			series_id = $1
		AND
			start_at = $2
		AND
			status <> 'deleted'
		ORDER BY
			created_at DESC
		LIMIT 1;
	`, seed.SeriesId, nextStartAt).Scan(&existingWorkflowId)
	if err == nil {
		return existingWorkflowId, nil
	}
	if err != nil && err != pgx.ErrNoRows {
		return "", fmt.Errorf("error checking recurring workflow successor: %s", err)
	}

	nowUnix := time.Now().UTC().Unix()
	var futureWorkflowCount int
	err = tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflows
		WHERE
			series_id = $1
		AND
			status <> 'deleted'
		AND
			start_at > $2;
	`, seed.SeriesId, nowUnix).Scan(&futureWorkflowCount)
	if err != nil {
		return "", fmt.Errorf("error checking recurring future workflow window: %s", err)
	}
	if futureWorkflowCount > 0 {
		return "", nil
	}

	roles := []structs.WorkflowRoleCreateInput{}
	if len(seed.RolesJSON) > 0 {
		if err := json.Unmarshal(seed.RolesJSON, &roles); err != nil {
			return "", fmt.Errorf("error unmarshalling recurring workflow roles: %s", err)
		}
	}
	steps := []structs.WorkflowStepCreateInput{}
	if len(seed.StepsJSON) > 0 {
		if err := json.Unmarshal(seed.StepsJSON, &steps); err != nil {
			return "", fmt.Errorf("error unmarshalling recurring workflow steps: %s", err)
		}
	}
	if len(roles) == 0 || len(steps) == 0 {
		return "", fmt.Errorf("workflow state is missing roles or steps for recurrence")
	}

	totalBounty := seed.SupervisorBounty
	for _, step := range steps {
		totalBounty += step.Bounty
	}
	weeklyRequirement := weeklyBountyRequirement(totalBounty, seed.Recurrence)

	successorStatus := "approved"
	successorIsBlocked := false
	var blockedByWorkflowId *string

	successorId := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO workflows(
			id,
			series_id,
			workflow_state_id,
			proposer_id,
			start_at,
			status,
			is_start_blocked,
			blocked_by_workflow_id,
			total_bounty,
			weekly_bounty_requirement,
			budget_weekly_deducted,
			budget_one_time_deducted,
			manager_required,
			manager_improver_id,
			manager_bounty,
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at,
			vote_decision,
			approved_at,
			approved_by_user_id
		)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0, 0, $11, $12, $13, $14, $14, $14, 'approve', $14, NULL);
	`, successorId, seed.SeriesId, seed.WorkflowStateID, seed.ProposerId, nextStartAt, successorStatus, successorIsBlocked, blockedByWorkflowId, totalBounty, weeklyRequirement, seed.SupervisorUserID != nil, seed.SupervisorUserID, seed.SupervisorBounty, nowUnix)
	if err != nil {
		return "", fmt.Errorf("error inserting recurring workflow successor: %s", err)
	}

	roleIdMap := map[string]string{}
	for roleIndex, role := range roles {
		roleClientID := strings.TrimSpace(role.ClientId)
		if roleClientID == "" {
			roleClientID = fmt.Sprintf("role-%d", roleIndex+1)
		}
		if _, exists := roleIdMap[roleClientID]; exists {
			return "", fmt.Errorf("duplicate workflow role client_id in recurring state: %s", roleClientID)
		}

		newRoleID := uuid.NewString()
		roleIdMap[roleClientID] = newRoleID
		_, err = tx.Exec(ctx, `
			INSERT INTO workflow_roles(
				id,
				workflow_id,
				title,
				is_manager
			)
			VALUES
				($1, $2, $3, false);
		`, newRoleID, successorId, strings.TrimSpace(role.Title))
		if err != nil {
			return "", fmt.Errorf("error cloning recurring role: %s", err)
		}

		for _, credentialType := range role.RequiredCredentials {
			_, err = tx.Exec(ctx, `
				INSERT INTO workflow_role_credentials(
					role_id,
					credential_type
				)
				VALUES
					($1, $2);
			`, newRoleID, credentialType)
			if err != nil {
				return "", fmt.Errorf("error cloning recurring role credential: %s", err)
			}
		}
	}

	for stepIndex, step := range steps {
		stepTitle := strings.TrimSpace(step.Title)
		if stepTitle == "" {
			return "", fmt.Errorf("workflow state contains a step with empty title")
		}

		var roleID *string
		roleClientID := strings.TrimSpace(step.RoleClientId)
		if roleClientID != "" {
			mappedRoleID, ok := roleIdMap[roleClientID]
			if !ok {
				return "", fmt.Errorf("workflow step role cannot be mapped during recurrence generation")
			}
			roleID = &mappedRoleID
		}

		stepStatus := "locked"
		if stepIndex == 0 && !successorIsBlocked && nextStartAt <= nowUnix {
			stepStatus = "available"
		}

		newStepID := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO workflow_steps(
				id,
				series_id,
				workflow_id,
				step_order,
				title,
				description,
				bounty,
				allow_step_not_possible,
				role_id,
				status
			)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
		`, newStepID, seed.SeriesId, successorId, stepIndex+1, stepTitle, strings.TrimSpace(step.Description), step.Bounty, step.AllowStepNotPossible, roleID, stepStatus)
		if err != nil {
			return "", fmt.Errorf("error cloning recurring step: %s", err)
		}

		for itemIndex, item := range step.WorkItems {
			itemTitle := strings.TrimSpace(item.Title)
			if itemTitle == "" {
				return "", fmt.Errorf("workflow state contains a work item with empty title")
			}

			photoRequiredCount := item.PhotoRequiredCount
			if photoRequiredCount <= 0 {
				photoRequiredCount = 1
			}
			photoAllowAnyCount := item.RequiresPhoto && item.PhotoAllowAnyCount
			if !item.RequiresPhoto {
				photoAllowAnyCount = false
			}
			photoAspectRatio, aspectErr := normalizeWorkflowPhotoAspectRatio(item.PhotoAspectRatio)
			if aspectErr != nil {
				photoAspectRatio = defaultWorkflowPhotoAspectRatio
			}

			dropdownOptions := []structs.WorkflowDropdownOption{}
			dropdownRequiresWritten := map[string]bool{}
			for _, option := range item.DropdownOptions {
				label := strings.TrimSpace(option.Label)
				value := deriveDropdownValueFromLabel(label)
				if value == "" {
					continue
				}
				dropdownOptions = append(dropdownOptions, structs.WorkflowDropdownOption{
					Value:                   value,
					Label:                   label,
					RequiresWrittenResponse: option.RequiresWrittenResponse,
					NotifyEmails:            option.NotifyEmails,
				})
				dropdownRequiresWritten[value] = option.RequiresWrittenResponse
			}
			dropdownOptionsJSON, err := json.Marshal(dropdownOptions)
			if err != nil {
				return "", fmt.Errorf("error marshalling recurring dropdown options: %s", err)
			}
			dropdownRequiresWrittenJSON, err := json.Marshal(dropdownRequiresWritten)
			if err != nil {
				return "", fmt.Errorf("error marshalling recurring dropdown requirements: %s", err)
			}
			notifyEmailsJSON, err := json.Marshal([]string{})
			if err != nil {
				return "", fmt.Errorf("error marshalling recurring notify emails: %s", err)
			}
			notifyValuesJSON, err := json.Marshal([]string{})
			if err != nil {
				return "", fmt.Errorf("error marshalling recurring notify dropdown values: %s", err)
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO workflow_step_items(
					id,
					step_id,
					item_order,
					title,
					description,
					is_optional,
					requires_photo,
					camera_capture_only,
					photo_required_count,
					photo_allow_any_count,
					photo_aspect_ratio,
					requires_written_response,
					requires_dropdown,
					dropdown_options,
					dropdown_requires_written_response,
					notify_emails,
					notify_on_dropdown_values
				)
				VALUES
					($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15::jsonb, $16::jsonb, $17::jsonb);
			`, uuid.NewString(), newStepID, itemIndex+1, itemTitle, strings.TrimSpace(item.Description), item.Optional, item.RequiresPhoto, item.RequiresPhoto && item.CameraCaptureOnly, photoRequiredCount, photoAllowAnyCount, photoAspectRatio, item.RequiresWritten, item.RequiresDropdown, string(dropdownOptionsJSON), string(dropdownRequiresWrittenJSON), string(notifyEmailsJSON), string(notifyValuesJSON))
			if err != nil {
				return "", fmt.Errorf("error cloning recurring step item: %s", err)
			}
		}
	}

	if err := applyWorkflowSeriesClaimsToWorkflowTx(ctx, tx, successorId); err != nil {
		return "", err
	}

	return successorId, nil
}

func ensureRecurringWorkflowSeriesCatchUpTx(ctx context.Context, tx pgx.Tx, seriesId string, nowUnix int64) error {
	if strings.TrimSpace(seriesId) == "" {
		return nil
	}

	_, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtext($1), 0);
	`, seriesId)
	if err != nil {
		return fmt.Errorf("error locking workflow series for recurrence catch-up: %s", err)
	}

	const maxCatchUpIterations = 1024
	for iteration := 0; iteration < maxCatchUpIterations; iteration++ {
		var workflowId string
		var recurrence string
		var recurrenceEndAt *int64
		var startAt int64
		var status string

		err := tx.QueryRow(ctx, `
			SELECT
				w.id,
				COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')),
				COALESCE(st.recurrence_end_at, s.recurrence_end_at),
				w.start_at,
				w.status
			FROM
				workflows w
			JOIN
				workflow_series s
			ON
				s.id = w.series_id
			LEFT JOIN
				workflow_states st
			ON
				st.id = COALESCE(s.current_state_id, w.workflow_state_id)
			WHERE
				w.series_id = $1
			AND
				w.status <> 'deleted'
			ORDER BY
				w.start_at DESC,
				w.created_at DESC,
				w.id DESC
			LIMIT 1
			FOR UPDATE OF w, s;
		`, seriesId).Scan(&workflowId, &recurrence, &recurrenceEndAt, &startAt, &status)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error loading recurring workflow series latest state: %s", err)
		}

		if recurrence == "one_time" {
			return nil
		}

		nextStartAt, err := nextRecurringStartAt(startAt, recurrence)
		if err != nil {
			return err
		}
		if nextStartAt > nowUnix {
			return nil
		}

		if status == "approved" || status == "in_progress" || status == "blocked" {
			if _, err := tx.Exec(ctx, `
				UPDATE
					workflows
				SET
					status = 'skipped',
					is_start_blocked = false,
					blocked_by_workflow_id = NULL,
					updated_at = unix_now()
				WHERE
					id = $1
				AND
					status IN ('approved', 'in_progress', 'blocked');
			`, workflowId); err != nil {
				return fmt.Errorf("error skipping elapsed recurring workflow: %s", err)
			}
		}

		if recurrenceEndAt != nil && nextStartAt > *recurrenceEndAt {
			return nil
		}

		successorId, err := ensureRecurringWorkflowSuccessorTx(ctx, tx, workflowId)
		if err != nil {
			return err
		}
		if successorId == "" {
			return nil
		}
	}

	return fmt.Errorf("recurring workflow catch-up exceeded limit for series %s", seriesId)
}

func (a *AppDB) ensureRecurringWorkflowContinuity(ctx context.Context) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT DISTINCT
			s.id
		FROM
			workflow_series s
		LEFT JOIN
			workflow_states st
		ON
			st.id = s.current_state_id
		JOIN
			workflows w
		ON
			w.series_id = s.id
		WHERE
			COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')) <> 'one_time'
		AND
			w.status <> 'deleted'
		ORDER BY
			s.id ASC;
	`)
	if err != nil {
		return fmt.Errorf("error querying recurring workflow continuity series: %s", err)
	}
	seriesIDs := []string{}
	for rows.Next() {
		var seriesId string
		if err := rows.Scan(&seriesId); err != nil {
			rows.Close()
			return fmt.Errorf("error scanning recurring workflow continuity series: %s", err)
		}
		seriesIDs = append(seriesIDs, seriesId)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("error iterating recurring workflow continuity series: %s", err)
	}
	rows.Close()

	nowUnix := time.Now().UTC().Unix()
	for _, seriesId := range seriesIDs {
		if err := ensureRecurringWorkflowSeriesCatchUpTx(ctx, tx, seriesId, nowUnix); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (a *AppDB) AllocatedWorkflowBalance(ctx context.Context) (uint64, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COALESCE((
				SELECT
					SUM(ws.bounty)
				FROM
					workflow_steps ws
				JOIN
					workflows w
				ON
					w.id = ws.workflow_id
				WHERE
					w.status IN ('approved', 'blocked', 'in_progress', 'completed')
				AND
					ws.status != 'paid_out'
			), 0)
			+
			COALESCE((
				SELECT
					SUM(w.manager_bounty)
				FROM
					workflows w
				WHERE
					w.status IN ('approved', 'blocked', 'in_progress', 'completed')
				AND
					w.manager_required = true
				AND
					w.manager_bounty > 0
				AND
					w.manager_paid_out_at IS NULL
			), 0);
	`)
	var allocated uint64
	if err := row.Scan(&allocated); err != nil {
		return 0, err
	}
	return allocated, nil
}

func (a *AppDB) AllocatedWorkflowBalanceByProposer(ctx context.Context, proposerId string) (uint64, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COALESCE((
				SELECT
					SUM(ws.bounty)
				FROM
					workflow_steps ws
				JOIN
					workflows w
				ON
					w.id = ws.workflow_id
				WHERE
					w.proposer_id = $1
				AND
					w.status IN ('pending', 'approved', 'blocked', 'in_progress', 'completed')
				AND
					ws.status != 'paid_out'
			), 0)
			+
			COALESCE((
				SELECT
					SUM(w.manager_bounty)
				FROM
					workflows w
				WHERE
					w.proposer_id = $1
				AND
					w.status IN ('pending', 'approved', 'blocked', 'in_progress', 'completed')
				AND
					w.manager_required = true
				AND
					w.manager_bounty > 0
				AND
					w.manager_paid_out_at IS NULL
			), 0);
	`, proposerId)
	var allocated uint64
	if err := row.Scan(&allocated); err != nil {
		return 0, err
	}
	return allocated, nil
}

func (a *AppDB) GetActiveCredentialTypesForUser(ctx context.Context, userId string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			credential_type
		FROM
			user_credentials
		WHERE
			user_id = $1
		AND
			is_revoked = false
		ORDER BY
			credential_type ASC;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error querying active credentials: %s", err)
	}
	defer rows.Close()

	credentials := []string{}
	for rows.Next() {
		var credential string
		if err := rows.Scan(&credential); err != nil {
			return nil, fmt.Errorf("error scanning active credential: %s", err)
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func getActiveCredentialTypesTx(ctx context.Context, tx pgx.Tx, userId string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			credential_type
		FROM
			user_credentials
		WHERE
			user_id = $1
		AND
			is_revoked = false;
	`, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	credentials := []string{}
	for rows.Next() {
		var credential string
		if err := rows.Scan(&credential); err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func getCredentialTypeSetTx(ctx context.Context, tx pgx.Tx) (map[string]struct{}, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			value
		FROM
			credential_type_definitions;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying credential type definitions: %s", err)
	}
	defer rows.Close()

	set := map[string]struct{}{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("error scanning credential type definition: %s", err)
		}
		set[strings.TrimSpace(value)] = struct{}{}
	}
	return set, nil
}

func (a *AppDB) RefreshWorkflowStartAvailability(ctx context.Context) (*structs.WorkflowStartRefreshResult, error) {
	if err := a.ensureRecurringWorkflowContinuity(ctx); err != nil {
		return nil, fmt.Errorf("error ensuring recurring workflow continuity: %s", err)
	}

	if _, err := a.db.Exec(ctx, `
		UPDATE
			workflows
		SET
			is_start_blocked = false,
			blocked_by_workflow_id = NULL,
			status = 'approved',
			updated_at = unix_now()
		WHERE
			status = 'blocked'
		AND
			blocked_by_workflow_id IS NOT NULL
		AND
			blocked_by_workflow_id IN (
				SELECT id FROM workflows WHERE status IN ('completed', 'paid_out', 'skipped', 'failed', 'deleted')
			);
	`); err != nil {
		return nil, fmt.Errorf("error repairing blocked workflows with resolved predecessors: %s", err)
	}

	rows, err := a.db.Query(ctx, `
		WITH updated_steps AS (
			UPDATE workflow_steps ws
			SET
				status = 'available',
				updated_at = unix_now()
			FROM workflows w
			WHERE
				ws.workflow_id = w.id
			AND
				ws.step_order = 1
			AND
				ws.status = 'locked'
			AND
				w.status IN ('approved', 'in_progress')
			AND
					w.start_at <= unix_now()
				RETURNING
					ws.id AS step_id,
					ws.workflow_id,
					ws.title AS step_title,
					ws.assigned_improver_id
			),
				updated_workflows AS (
					SELECT
						u.step_id,
						u.workflow_id,
						u.step_title,
							u.assigned_improver_id,
							COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')) AS workflow_title,
							w.series_id,
							COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')) AS recurrence,
							w.start_at,
							w.total_bounty,
							w.weekly_bounty_requirement
					FROM
						updated_steps u
					JOIN
						workflows w
					ON
						w.id = u.workflow_id
					LEFT JOIN
						workflow_states st
					ON
						st.id = w.workflow_state_id
					LEFT JOIN
						workflow_series s
					ON
						s.id = w.series_id
				),
			inserted_notifications AS (
				INSERT INTO workflow_step_notifications(step_id, user_id, notification_type)
				SELECT
					step_id,
					assigned_improver_id,
					'step_available'
				FROM
					updated_workflows
				WHERE
					assigned_improver_id IS NOT NULL
				ON CONFLICT DO NOTHING
			RETURNING
				step_id,
				user_id
		)
			SELECT
				u.workflow_id,
				u.workflow_title,
				u.step_id,
				u.step_title,
				u.assigned_improver_id,
				COALESCE(NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''), COALESCE(us.contact_name, '')),
				COALESCE(i.email, us.contact_email, ''),
				(n.step_id IS NOT NULL),
				u.series_id,
				u.recurrence,
				u.start_at,
				u.total_bounty,
				u.weekly_bounty_requirement
			FROM
				updated_workflows u
			LEFT JOIN
				inserted_notifications n
			ON
				n.step_id = u.step_id
		AND
			n.user_id = u.assigned_improver_id
			LEFT JOIN
				users us
			ON
				us.id = u.assigned_improver_id
		LEFT JOIN
			improvers i
		ON
			i.user_id = u.assigned_improver_id;
	`)
	if err != nil {
		return nil, fmt.Errorf("error refreshing workflow step start availability: %s", err)
	}
	defer rows.Close()

	result := &structs.WorkflowStartRefreshResult{
		AvailabilityNotifications: []structs.WorkflowStepAvailabilityNotification{},
		SeriesFundingChecks:       []structs.WorkflowSeriesStartFundingCheck{},
	}
	seriesCheckSeen := map[string]struct{}{}
	for rows.Next() {
		var workflowId string
		var workflowTitle string
		var stepId string
		var stepTitle string
		var assignedImproverId *string
		var name string
		var email string
		var shouldNotify bool
		var seriesId string
		var recurrence string
		var startAt int64
		var totalBounty uint64
		var weeklyRequirement uint64
		if err := rows.Scan(
			&workflowId,
			&workflowTitle,
			&stepId,
			&stepTitle,
			&assignedImproverId,
			&name,
			&email,
			&shouldNotify,
			&seriesId,
			&recurrence,
			&startAt,
			&totalBounty,
			&weeklyRequirement,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow start availability update: %s", err)
		}

		if shouldNotify && assignedImproverId != nil {
			result.AvailabilityNotifications = append(result.AvailabilityNotifications, structs.WorkflowStepAvailabilityNotification{
				WorkflowId:    workflowId,
				WorkflowTitle: workflowTitle,
				StepId:        stepId,
				StepTitle:     stepTitle,
				UserId:        *assignedImproverId,
				Name:          name,
				Email:         email,
			})
		}

		if recurrence != "one_time" {
			if _, exists := seriesCheckSeen[workflowId]; !exists {
				seriesCheckSeen[workflowId] = struct{}{}
				result.SeriesFundingChecks = append(result.SeriesFundingChecks, structs.WorkflowSeriesStartFundingCheck{
					WorkflowId:              workflowId,
					WorkflowTitle:           workflowTitle,
					SeriesId:                seriesId,
					Recurrence:              recurrence,
					StartAt:                 startAt,
					TotalBounty:             totalBounty,
					WeeklyBountyRequirement: weeklyRequirement,
				})
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating workflow start availability updates: %s", err)
	}

	return result, nil
}

func (a *AppDB) GetImproverWorkflows(ctx context.Context, improverId string) ([]*structs.Workflow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			w.id
		FROM
			workflows w
		WHERE
			w.status IN ('approved', 'in_progress', 'completed', 'paid_out', 'blocked')
		ORDER BY
			CASE
				WHEN EXISTS (
					SELECT
						1
					FROM
						workflow_steps ws
					WHERE
						ws.workflow_id = w.id
						AND ws.assigned_improver_id = $1
						AND ws.status IN ('available', 'in_progress')
				) THEN 0
				ELSE 1
			END ASC,
			w.start_at DESC,
			w.created_at DESC
		LIMIT 500;
	`, improverId)
	if err != nil {
		return nil, fmt.Errorf("error querying improver workflows: %s", err)
	}
	defer rows.Close()

	workflowIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning improver workflow id: %s", err)
		}
		workflowIDs = append(workflowIDs, id)
	}

	workflows := make([]*structs.Workflow, 0, len(workflowIDs))
	for _, workflowId := range workflowIDs {
		workflow, err := a.GetWorkflowByID(ctx, workflowId)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, workflow)
	}
	return workflows, nil
}

func (a *AppDB) GetManagedWorkflowsByImprover(ctx context.Context, improverId string) ([]*structs.Workflow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			workflows
		WHERE
			manager_improver_id = $1
		ORDER BY
			start_at DESC,
			created_at DESC
		LIMIT 400;
	`, improverId)
	if err != nil {
		return nil, fmt.Errorf("error querying managed workflows: %s", err)
	}
	defer rows.Close()

	workflowIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning managed workflow id: %s", err)
		}
		workflowIDs = append(workflowIDs, id)
	}

	workflows := make([]*structs.Workflow, 0, len(workflowIDs))
	for _, workflowID := range workflowIDs {
		workflow, err := a.GetWorkflowByID(ctx, workflowID)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, workflow)
	}
	return workflows, nil
}

func (a *AppDB) GetUserEmailsByIDs(ctx context.Context, userIDs []string) (map[string]string, error) {
	normalizedIDs := make([]string, 0, len(userIDs))
	seen := map[string]struct{}{}
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		normalizedIDs = append(normalizedIDs, id)
	}
	if len(normalizedIDs) == 0 {
		return map[string]string{}, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			u.id,
			COALESCE(NULLIF(TRIM(i.email), ''), NULLIF(TRIM(u.contact_email), ''), '')
		FROM
			users u
		LEFT JOIN
			improvers i
		ON
			i.user_id = u.id
		WHERE
			u.id = ANY($1::text[]);
	`, normalizedIDs)
	if err != nil {
		return nil, fmt.Errorf("error querying user emails: %s", err)
	}
	defer rows.Close()

	emails := map[string]string{}
	for rows.Next() {
		var userID string
		var email string
		if err := rows.Scan(&userID, &email); err != nil {
			return nil, fmt.Errorf("error scanning user email: %s", err)
		}
		emails[userID] = strings.TrimSpace(email)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user emails: %s", err)
	}
	return emails, nil
}

func normalizeSupervisorSort(sortBy string) string {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "title":
		return "title"
	case "start_at":
		return "start_at"
	case "completed_at":
		return "completed_at"
	case "created_at":
		fallthrough
	default:
		return "created_at"
	}
}

func normalizeSupervisorSortDirection(sortDirection string) string {
	switch strings.ToLower(strings.TrimSpace(sortDirection)) {
	case "asc":
		return "ASC"
	default:
		return "DESC"
	}
}

func normalizeSupervisorDateField(dateField string) string {
	switch strings.ToLower(strings.TrimSpace(dateField)) {
	case "start_at":
		return "start_at"
	case "completed_at":
		return "completed_at"
	case "created_at":
		fallthrough
	default:
		return "created_at"
	}
}

func (a *AppDB) GetSupervisorWorkflows(
	ctx context.Context,
	supervisorID string,
	search string,
	statusFilter string,
	sortBy string,
	sortDirection string,
	dateField string,
	dateFrom *time.Time,
	dateTo *time.Time,
	page int,
	count int,
) (*structs.SupervisorWorkflowListResponse, error) {
	supervisorID = strings.TrimSpace(supervisorID)
	if supervisorID == "" {
		return nil, fmt.Errorf("supervisor_id is required")
	}
	if page < 0 {
		page = 0
	}
	if count <= 0 {
		count = 20
	}
	if count > 200 {
		count = 200
	}

	allowedStatus := map[string]struct{}{
		"pending":     {},
		"approved":    {},
		"rejected":    {},
		"in_progress": {},
		"completed":   {},
		"paid_out":    {},
		"blocked":     {},
		"expired":     {},
		"failed":      {},
		"skipped":     {},
		"deleted":     {},
	}
	normalizedStatus := strings.ToLower(strings.TrimSpace(statusFilter))
	if normalizedStatus != "" && normalizedStatus != "all" {
		if _, ok := allowedStatus[normalizedStatus]; !ok {
			return nil, fmt.Errorf("invalid status filter")
		}
	}

	orderBy := normalizeSupervisorSort(sortBy)
	orderDir := normalizeSupervisorSortDirection(sortDirection)
	normalizedDateField := normalizeSupervisorDateField(dateField)

	baseCTE := `
				WITH bw AS (
					SELECT
						w.id,
						w.series_id,
						COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')) AS title,
						w.status,
						COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')) AS recurrence,
						w.start_at,
						w.created_at,
						w.total_bounty,
					w.manager_bounty,
				(
					SELECT
						MAX(ws.completed_at)
					FROM
						workflow_steps ws
					WHERE
						ws.workflow_id = w.id
				) AS completed_at
					FROM
						workflows w
					LEFT JOIN
						workflow_states st
					ON
						st.id = w.workflow_state_id
					LEFT JOIN
						workflow_series s
				ON
					s.id = w.series_id
				WHERE
					w.manager_improver_id = $1
			)
	`

	conditions := []string{"1=1"}
	args := []any{supervisorID}
	argIndex := 2

	trimmedSearch := strings.TrimSpace(search)
	if trimmedSearch != "" {
		conditions = append(conditions, fmt.Sprintf("bw.title ILIKE $%d", argIndex))
		args = append(args, "%"+trimmedSearch+"%")
		argIndex++
	}

	if normalizedStatus != "" && normalizedStatus != "all" {
		conditions = append(conditions, fmt.Sprintf("bw.status = $%d", argIndex))
		args = append(args, normalizedStatus)
		argIndex++
	}

	dateColumn := "bw.created_at"
	if normalizedDateField == "start_at" {
		dateColumn = "bw.start_at"
	}
	if normalizedDateField == "completed_at" {
		dateColumn = "bw.completed_at"
	}
	if dateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("%s >= $%d", dateColumn, argIndex))
		args = append(args, dateFrom.UTC().Unix())
		argIndex++
	}
	if dateTo != nil {
		conditions = append(conditions, fmt.Sprintf("%s <= $%d", dateColumn, argIndex))
		args = append(args, dateTo.UTC().Unix())
		argIndex++
	}

	whereClause := strings.Join(conditions, " AND ")

	countQuery := baseCTE + fmt.Sprintf(`
		SELECT
			COUNT(*)
		FROM
			bw
		WHERE
			%s;
	`, whereClause)
	var total int
	if err := a.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("error counting supervisor workflows: %s", err)
	}

	orderColumn := "bw.created_at"
	switch orderBy {
	case "title":
		orderColumn = "bw.title"
	case "start_at":
		orderColumn = "bw.start_at"
	case "completed_at":
		orderColumn = "bw.completed_at"
	}

	offset := page * count
	listQuery := baseCTE + fmt.Sprintf(`
		SELECT
			bw.id,
			bw.series_id,
			bw.title,
			bw.status,
			bw.recurrence,
			bw.start_at,
			bw.created_at,
			bw.completed_at,
			bw.total_bounty,
			bw.manager_bounty
		FROM
			bw
		WHERE
			%s
		ORDER BY
			%s %s NULLS LAST,
			bw.created_at DESC
		LIMIT $%d
		OFFSET $%d;
	`, whereClause, orderColumn, orderDir, argIndex, argIndex+1)

	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, count, offset)
	rows, err := a.db.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, fmt.Errorf("error querying supervisor workflows: %s", err)
	}
	defer rows.Close()

	items := []*structs.SupervisorWorkflowListItem{}
	for rows.Next() {
		item := &structs.SupervisorWorkflowListItem{}
		if err := rows.Scan(
			&item.Id,
			&item.SeriesId,
			&item.Title,
			&item.Status,
			&item.Recurrence,
			&item.StartAt,
			&item.CreatedAt,
			&item.CompletedAt,
			&item.TotalBounty,
			&item.SupervisorBounty,
		); err != nil {
			return nil, fmt.Errorf("error scanning supervisor workflow list item: %s", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating supervisor workflows: %s", err)
	}

	return &structs.SupervisorWorkflowListResponse{
		Items: items,
		Total: total,
		Page:  page,
		Count: count,
	}, nil
}

func (a *AppDB) ClaimWorkflowManager(ctx context.Context, workflowId string, improverId string) (*structs.Workflow, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var workflowStatus string
	var managerRequired bool
	var managerRoleID *string
	var managerImproverID *string
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			manager_required,
			manager_role_id,
			manager_improver_id
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&workflowStatus, &managerRequired, &managerRoleID, &managerImproverID)
	if err != nil {
		return nil, err
	}
	if workflowStatus != "approved" && workflowStatus != "in_progress" && workflowStatus != "blocked" {
		return nil, fmt.Errorf("workflow is not available for manager claims")
	}
	if !managerRequired || managerRoleID == nil {
		return nil, fmt.Errorf("workflow manager role is not enabled")
	}
	if managerImproverID != nil {
		return nil, fmt.Errorf("workflow manager is already claimed")
	}

	var claimedAssignments int
	err = tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		AND
			assigned_improver_id = $2;
	`, workflowId, improverId).Scan(&claimedAssignments)
	if err != nil {
		return nil, fmt.Errorf("error checking existing workflow assignments: %s", err)
	}
	if claimedAssignments > 0 {
		return nil, fmt.Errorf("improver already assigned within this workflow")
	}

	requiredRows, err := tx.Query(ctx, `
		SELECT
			credential_type
		FROM
			workflow_role_credentials
		WHERE
			role_id = $1;
	`, *managerRoleID)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow manager credentials: %s", err)
	}
	defer requiredRows.Close()

	requiredCredentials := []string{}
	for requiredRows.Next() {
		var credential string
		if err := requiredRows.Scan(&credential); err != nil {
			return nil, fmt.Errorf("error scanning workflow manager credential: %s", err)
		}
		requiredCredentials = append(requiredCredentials, strings.TrimSpace(credential))
	}
	if len(requiredCredentials) == 0 {
		return nil, fmt.Errorf("workflow manager role has no credential requirements")
	}
	validCredentialTypes, err := getCredentialTypeSetTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, required := range requiredCredentials {
		if _, ok := validCredentialTypes[required]; !ok {
			return nil, fmt.Errorf("workflow manager role references unknown credential type: %s", required)
		}
	}

	activeCredentials, err := getActiveCredentialTypesTx(ctx, tx, improverId)
	if err != nil {
		return nil, fmt.Errorf("error querying improver credentials: %s", err)
	}
	activeSet := map[string]struct{}{}
	for _, credential := range activeCredentials {
		activeSet[credential] = struct{}{}
	}
	for _, required := range requiredCredentials {
		if _, ok := activeSet[required]; !ok {
			return nil, fmt.Errorf("missing required credentials for workflow manager")
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			workflows
		SET
			manager_improver_id = $2,
			updated_at = unix_now()
		WHERE
			id = $1;
	`, workflowId, improverId)
	if err != nil {
		return nil, fmt.Errorf("error assigning workflow manager: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a.GetWorkflowByID(ctx, workflowId)
}

func (a *AppDB) IsWorkflowManagedByImprover(ctx context.Context, workflowId string, improverId string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT
					1
				FROM
					workflows
				WHERE
					id = $1
				AND
					manager_improver_id = $2
			);
	`, workflowId, improverId)
	var managed bool
	if err := row.Scan(&managed); err != nil {
		return false, fmt.Errorf("error checking workflow manager assignment: %s", err)
	}
	return managed, nil
}

func (a *AppDB) IsImproverAssignedOrManagerForWorkflow(ctx context.Context, workflowId string, improverId string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT
					1
				FROM
					workflows w
				WHERE
					w.id = $1
				AND
					w.manager_improver_id = $2
			)
			OR
			EXISTS (
				SELECT
					1
				FROM
					workflow_steps ws
				WHERE
					ws.workflow_id = $1
				AND
					ws.assigned_improver_id = $2
			);
	`, workflowId, improverId)
	var allowed bool
	if err := row.Scan(&allowed); err != nil {
		return false, fmt.Errorf("error checking workflow improver assignment: %s", err)
	}
	return allowed, nil
}

func (a *AppDB) GetWorkflowSubmissionPhotoBlobByID(ctx context.Context, photoID string) (*structs.WorkflowSubmissionPhotoBlob, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id,
			workflow_id,
			step_id,
			item_id,
			submission_id,
			file_name,
			content_type,
			size_bytes,
			created_at,
			photo_data
		FROM
			workflow_submission_photos
		WHERE
			id = $1;
	`, photoID)

	photo := &structs.WorkflowSubmissionPhotoBlob{}
	if err := row.Scan(
		&photo.Id,
		&photo.WorkflowId,
		&photo.StepId,
		&photo.ItemId,
		&photo.SubmissionId,
		&photo.FileName,
		&photo.ContentType,
		&photo.SizeBytes,
		&photo.CreatedAt,
		&photo.PhotoData,
	); err != nil {
		return nil, err
	}
	return photo, nil
}

func (a *AppDB) GetWorkflowSubmissionPhotoExports(ctx context.Context, workflowID string) ([]*structs.WorkflowSubmissionPhotoExport, error) {
	rows, err := a.db.Query(ctx, `
			SELECT
				p.id,
				p.workflow_id,
			p.step_id,
			p.item_id,
			p.submission_id,
			p.file_name,
			p.content_type,
			p.size_bytes,
			p.created_at,
				p.photo_data,
				COALESCE(ws.step_order, 0),
				COALESCE(wsi.item_order, 0),
				COALESCE(wsi.title, ''),
				COALESCE(s.title, ''),
				w.start_at
			FROM
				workflow_submission_photos p
			LEFT JOIN
				workflows w
			ON
				w.id = p.workflow_id
			LEFT JOIN
				workflow_series s
			ON
				s.id = w.series_id
			LEFT JOIN
				workflow_steps ws
		ON
			ws.id = p.step_id
		LEFT JOIN
			workflow_step_items wsi
		ON
			wsi.id = p.item_id
		WHERE
			p.workflow_id = $1
		ORDER BY
			ws.step_order ASC,
			wsi.item_order ASC,
			p.created_at ASC;
	`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow submission photos export: %s", err)
	}
	defer rows.Close()

	results := []*structs.WorkflowSubmissionPhotoExport{}
	for rows.Next() {
		export := &structs.WorkflowSubmissionPhotoExport{}
		if err := rows.Scan(
			&export.Photo.Id,
			&export.Photo.WorkflowId,
			&export.Photo.StepId,
			&export.Photo.ItemId,
			&export.Photo.SubmissionId,
			&export.Photo.FileName,
			&export.Photo.ContentType,
			&export.Photo.SizeBytes,
			&export.Photo.CreatedAt,
			&export.Photo.PhotoData,
			&export.StepOrder,
			&export.ItemOrder,
			&export.ItemTitle,
			&export.WorkflowTitle,
			&export.WorkflowStartAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow submission photo export row: %s", err)
		}
		results = append(results, export)
	}

	return results, nil
}

func (a *AppDB) GetImproverAbsencePeriods(ctx context.Context, improverId string) ([]*structs.ImproverAbsencePeriod, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			improver_id,
			series_id,
			step_order,
			absent_from,
			absent_until,
			created_at,
			updated_at
		FROM
			workflow_improver_absences
		WHERE
			improver_id = $1
		ORDER BY
			absent_from DESC,
			created_at DESC
		LIMIT 200;
	`, improverId)
	if err != nil {
		return nil, fmt.Errorf("error querying improver absence periods: %s", err)
	}
	defer rows.Close()

	results := []*structs.ImproverAbsencePeriod{}
	for rows.Next() {
		absence := &structs.ImproverAbsencePeriod{}
		if err := rows.Scan(
			&absence.Id,
			&absence.ImproverId,
			&absence.SeriesId,
			&absence.StepOrder,
			&absence.AbsentFrom,
			&absence.AbsentUntil,
			&absence.CreatedAt,
			&absence.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning improver absence period: %s", err)
		}
		results = append(results, absence)
	}
	return results, nil
}

func (a *AppDB) CreateImproverAbsencePeriod(
	ctx context.Context,
	improverId string,
	seriesId string,
	stepOrder int,
	absentFrom time.Time,
	absentUntil time.Time,
) (*structs.ImproverAbsencePeriodCreateResult, error) {
	seriesId = strings.TrimSpace(seriesId)
	if seriesId == "" {
		return nil, fmt.Errorf("series_id is required")
	}
	if stepOrder <= 0 {
		return nil, fmt.Errorf("step_order must be greater than zero")
	}
	if !absentUntil.After(absentFrom) {
		return nil, fmt.Errorf("absent_until must be after absent_from")
	}

	absentFromUnix := absentFrom.UTC().Unix()
	absentUntilUnix := absentUntil.UTC().Unix()

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := ensureRecurringClaimExistsForAbsenceTx(ctx, tx, improverId, seriesId, stepOrder); err != nil {
		return nil, err
	}
	overlapCount, err := countImproverAbsenceOverlapTx(ctx, tx, improverId, seriesId, stepOrder, absentFromUnix, absentUntilUnix, "")
	if err != nil {
		return nil, err
	}
	if overlapCount > 0 {
		return nil, fmt.Errorf("overlapping absence period already exists")
	}

	absence, err := insertImproverAbsencePeriodTx(ctx, tx, improverId, seriesId, stepOrder, absentFromUnix, absentUntilUnix)
	if err != nil {
		return nil, err
	}
	targetedCount, releasedCount, err := releaseAssignmentsForImproverAbsenceTx(ctx, tx, improverId, seriesId, stepOrder, absentFromUnix, absentUntilUnix)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	result := &structs.ImproverAbsencePeriodCreateResult{
		Absence:       absence,
		ReleasedCount: releasedCount,
		SkippedCount:  targetedCount - releasedCount,
	}
	if result.SkippedCount < 0 {
		result.SkippedCount = 0
	}
	return result, nil
}

func (a *AppDB) UpdateImproverAbsencePeriod(
	ctx context.Context,
	improverId string,
	absenceId string,
	absentFrom time.Time,
	absentUntil time.Time,
) (*structs.ImproverAbsencePeriodCreateResult, error) {
	absenceId = strings.TrimSpace(absenceId)
	if absenceId == "" {
		return nil, fmt.Errorf("absence_id is required")
	}
	if !absentUntil.After(absentFrom) {
		return nil, fmt.Errorf("absent_until must be after absent_from")
	}

	absentFromUnix := absentFrom.UTC().Unix()
	absentUntilUnix := absentUntil.UTC().Unix()

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	existing := structs.ImproverAbsencePeriod{}
	err = tx.QueryRow(ctx, `
		SELECT
			id,
			improver_id,
			series_id,
			step_order,
			absent_from,
			absent_until,
			created_at,
			updated_at
		FROM
			workflow_improver_absences
		WHERE
			id = $1
		AND
			improver_id = $2
		FOR UPDATE;
	`, absenceId, improverId).Scan(
		&existing.Id,
		&existing.ImproverId,
		&existing.SeriesId,
		&existing.StepOrder,
		&existing.AbsentFrom,
		&existing.AbsentUntil,
		&existing.CreatedAt,
		&existing.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("error loading absence period for update: %s", err)
	}

	replacementClaimCount, err := countReplacementClaimsForImproverAbsenceTx(
		ctx,
		tx,
		improverId,
		existing.SeriesId,
		existing.StepOrder,
		existing.AbsentFrom,
		existing.AbsentUntil,
	)
	if err != nil {
		return nil, err
	}
	if replacementClaimCount > 0 {
		return nil, fmt.Errorf("another improver has already claimed work in this absence period")
	}

	overlapCount, err := countImproverAbsenceOverlapTx(
		ctx,
		tx,
		improverId,
		existing.SeriesId,
		existing.StepOrder,
		absentFromUnix,
		absentUntilUnix,
		existing.Id,
	)
	if err != nil {
		return nil, err
	}
	if overlapCount > 0 {
		return nil, fmt.Errorf("overlapping absence period already exists")
	}

	updated := structs.ImproverAbsencePeriod{}
	err = tx.QueryRow(ctx, `
		UPDATE
			workflow_improver_absences
		SET
			absent_from = $2,
			absent_until = $3,
			updated_at = unix_now()
		WHERE
			id = $1
		AND
			improver_id = $4
		RETURNING
			id,
			improver_id,
			series_id,
			step_order,
			absent_from,
			absent_until,
			created_at,
			updated_at;
	`, existing.Id, absentFromUnix, absentUntilUnix, improverId).Scan(
		&updated.Id,
		&updated.ImproverId,
		&updated.SeriesId,
		&updated.StepOrder,
		&updated.AbsentFrom,
		&updated.AbsentUntil,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("error updating improver absence period: %s", err)
	}

	targetedCount, releasedCount, err := releaseAssignmentsForImproverAbsenceTx(
		ctx,
		tx,
		improverId,
		updated.SeriesId,
		updated.StepOrder,
		updated.AbsentFrom,
		updated.AbsentUntil,
	)
	if err != nil {
		return nil, err
	}

	hasClaimMapping, err := hasWorkflowSeriesClaimMappingTx(ctx, tx, updated.SeriesId, updated.StepOrder, improverId)
	if err != nil {
		return nil, err
	}
	if hasClaimMapping {
		minStart := existing.AbsentFrom
		if updated.AbsentFrom < minStart {
			minStart = updated.AbsentFrom
		}
		if err := propagateWorkflowSeriesClaimTx(ctx, tx, updated.SeriesId, updated.StepOrder, improverId, minStart); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	result := &structs.ImproverAbsencePeriodCreateResult{
		Absence:       updated,
		ReleasedCount: releasedCount,
		SkippedCount:  targetedCount - releasedCount,
	}
	if result.SkippedCount < 0 {
		result.SkippedCount = 0
	}
	return result, nil
}

func (a *AppDB) DeleteImproverAbsencePeriod(
	ctx context.Context,
	improverId string,
	absenceId string,
) (*structs.ImproverAbsencePeriodDeleteResult, error) {
	absenceId = strings.TrimSpace(absenceId)
	if absenceId == "" {
		return nil, fmt.Errorf("absence_id is required")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	existing := structs.ImproverAbsencePeriod{}
	err = tx.QueryRow(ctx, `
		SELECT
			id,
			improver_id,
			series_id,
			step_order,
			absent_from,
			absent_until,
			created_at,
			updated_at
		FROM
			workflow_improver_absences
		WHERE
			id = $1
		AND
			improver_id = $2
		FOR UPDATE;
	`, absenceId, improverId).Scan(
		&existing.Id,
		&existing.ImproverId,
		&existing.SeriesId,
		&existing.StepOrder,
		&existing.AbsentFrom,
		&existing.AbsentUntil,
		&existing.CreatedAt,
		&existing.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("error loading absence period for deletion: %s", err)
	}

	replacementClaimCount, err := countReplacementClaimsForImproverAbsenceTx(
		ctx,
		tx,
		improverId,
		existing.SeriesId,
		existing.StepOrder,
		existing.AbsentFrom,
		existing.AbsentUntil,
	)
	if err != nil {
		return nil, err
	}
	if replacementClaimCount > 0 {
		return nil, fmt.Errorf("another improver has already claimed work in this absence period")
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM workflow_improver_absences
		WHERE
			id = $1
		AND
			improver_id = $2;
	`, existing.Id, improverId)
	if err != nil {
		return nil, fmt.Errorf("error deleting improver absence period: %s", err)
	}

	hasClaimMapping, err := hasWorkflowSeriesClaimMappingTx(ctx, tx, existing.SeriesId, existing.StepOrder, improverId)
	if err != nil {
		return nil, err
	}
	if hasClaimMapping {
		if err := propagateWorkflowSeriesClaimTx(ctx, tx, existing.SeriesId, existing.StepOrder, improverId, existing.AbsentFrom); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &structs.ImproverAbsencePeriodDeleteResult{Id: existing.Id}, nil
}

func ensureRecurringClaimExistsForAbsenceTx(
	ctx context.Context,
	tx pgx.Tx,
	improverId string,
	seriesId string,
	stepOrder int,
) error {
	var recurringClaims int
	err := tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_steps ws
		JOIN
			workflows w
		ON
			w.id = ws.workflow_id
		JOIN
			workflow_series sr
		ON
			sr.id = w.series_id
		WHERE
			ws.assigned_improver_id = $1
		AND
			w.series_id = $2
		AND
			COALESCE(NULLIF(TRIM(sr.recurrence), ''), 'one_time') <> 'one_time'
		AND
			ws.step_order = $3;
	`, improverId, seriesId, stepOrder).Scan(&recurringClaims)
	if err != nil {
		return fmt.Errorf("error validating recurring assignment for absence period: %s", err)
	}
	if recurringClaims == 0 {
		return fmt.Errorf("no claimed recurring workpiece found for this series and step")
	}
	return nil
}

func countImproverAbsenceOverlapTx(
	ctx context.Context,
	tx pgx.Tx,
	improverId string,
	seriesId string,
	stepOrder int,
	absentFromUnix int64,
	absentUntilUnix int64,
	excludeAbsenceId string,
) (int, error) {
	excludeAbsenceId = strings.TrimSpace(excludeAbsenceId)
	var overlapCount int
	var err error
	if excludeAbsenceId == "" {
		err = tx.QueryRow(ctx, `
			SELECT
				COUNT(*)
			FROM
				workflow_improver_absences
			WHERE
				improver_id = $1
			AND
				series_id = $2
			AND
				step_order = $3
			AND
				NOT ($5 <= absent_from OR $4 >= absent_until);
		`, improverId, seriesId, stepOrder, absentFromUnix, absentUntilUnix).Scan(&overlapCount)
	} else {
		err = tx.QueryRow(ctx, `
			SELECT
				COUNT(*)
			FROM
				workflow_improver_absences
			WHERE
				improver_id = $1
			AND
				series_id = $2
			AND
				step_order = $3
			AND
				id <> $6
			AND
				NOT ($5 <= absent_from OR $4 >= absent_until);
		`, improverId, seriesId, stepOrder, absentFromUnix, absentUntilUnix, excludeAbsenceId).Scan(&overlapCount)
	}
	if err != nil {
		return 0, fmt.Errorf("error checking overlapping absence period: %s", err)
	}
	return overlapCount, nil
}

func insertImproverAbsencePeriodTx(
	ctx context.Context,
	tx pgx.Tx,
	improverId string,
	seriesId string,
	stepOrder int,
	absentFromUnix int64,
	absentUntilUnix int64,
) (structs.ImproverAbsencePeriod, error) {
	absenceID := uuid.NewString()
	absence := structs.ImproverAbsencePeriod{}
	err := tx.QueryRow(ctx, `
		INSERT INTO workflow_improver_absences
			(
				id,
				improver_id,
				series_id,
				step_order,
				absent_from,
				absent_until
			)
		VALUES
			($1, $2, $3, $4, $5, $6)
		RETURNING
			id,
			improver_id,
			series_id,
			step_order,
			absent_from,
			absent_until,
			created_at,
			updated_at;
	`, absenceID, improverId, seriesId, stepOrder, absentFromUnix, absentUntilUnix).Scan(
		&absence.Id,
		&absence.ImproverId,
		&absence.SeriesId,
		&absence.StepOrder,
		&absence.AbsentFrom,
		&absence.AbsentUntil,
		&absence.CreatedAt,
		&absence.UpdatedAt,
	)
	if err != nil {
		return structs.ImproverAbsencePeriod{}, fmt.Errorf("error creating improver absence period: %s", err)
	}
	return absence, nil
}

func releaseAssignmentsForImproverAbsenceTx(
	ctx context.Context,
	tx pgx.Tx,
	improverId string,
	seriesId string,
	stepOrder int,
	absentFromUnix int64,
	absentUntilUnix int64,
) (int, int, error) {
	var targetedCount int
	err := tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_steps ws
		JOIN
			workflows w
		ON
			w.id = ws.workflow_id
		JOIN
			workflow_series sr
		ON
			sr.id = w.series_id
		WHERE
			ws.assigned_improver_id = $1
		AND
			w.series_id = $2
		AND
			COALESCE(NULLIF(TRIM(sr.recurrence), ''), 'one_time') <> 'one_time'
		AND
			ws.step_order = $3
		AND
			w.start_at >= $4
		AND
			w.start_at < $5
		AND
			w.status IN ('approved', 'blocked', 'in_progress')
		AND
			ws.status NOT IN ('completed', 'paid_out');
	`, improverId, seriesId, stepOrder, absentFromUnix, absentUntilUnix).Scan(&targetedCount)
	if err != nil {
		return 0, 0, fmt.Errorf("error counting absence target assignments: %s", err)
	}

	var releasedCount int
	err = tx.QueryRow(ctx, `
		WITH releasable AS (
			SELECT
				ws.id
			FROM
				workflow_steps ws
			JOIN
				workflows w
			ON
				w.id = ws.workflow_id
			JOIN
				workflow_series sr
			ON
				sr.id = w.series_id
			WHERE
				ws.assigned_improver_id = $1
			AND
				w.series_id = $2
			AND
				COALESCE(NULLIF(TRIM(sr.recurrence), ''), 'one_time') <> 'one_time'
			AND
				ws.step_order = $3
			AND
				w.start_at >= $4
			AND
				w.start_at < $5
			AND
				w.status IN ('approved', 'blocked', 'in_progress')
			AND
				ws.status IN ('locked', 'available')
			FOR UPDATE
		),
		released AS (
			UPDATE
				workflow_steps ws
			SET
				assigned_improver_id = NULL,
				updated_at = unix_now()
			WHERE
				ws.id IN (SELECT id FROM releasable)
			RETURNING
				ws.id
		),
		cleared_notifications AS (
			DELETE FROM workflow_step_notifications n
			WHERE
				n.step_id IN (SELECT id FROM released)
			AND
				n.user_id = $1
			AND
				n.notification_type = 'step_available'
			RETURNING
				n.step_id
		)
		SELECT
			COUNT(*)
		FROM
			released;
	`, improverId, seriesId, stepOrder, absentFromUnix, absentUntilUnix).Scan(&releasedCount)
	if err != nil {
		return 0, 0, fmt.Errorf("error releasing assignments for improver absence period: %s", err)
	}

	return targetedCount, releasedCount, nil
}

func countReplacementClaimsForImproverAbsenceTx(
	ctx context.Context,
	tx pgx.Tx,
	improverId string,
	seriesId string,
	stepOrder int,
	absentFromUnix int64,
	absentUntilUnix int64,
) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_steps ws
		JOIN
			workflows w
		ON
			w.id = ws.workflow_id
		JOIN
			workflow_series sr
		ON
			sr.id = w.series_id
		WHERE
			w.series_id = $1
		AND
			COALESCE(NULLIF(TRIM(sr.recurrence), ''), 'one_time') <> 'one_time'
		AND
			ws.step_order = $2
		AND
			w.start_at >= $3
		AND
			w.start_at < $4
		AND
			w.status IN ('approved', 'blocked', 'in_progress', 'completed', 'paid_out')
		AND
			ws.assigned_improver_id IS NOT NULL
		AND
			ws.assigned_improver_id <> $5;
	`, seriesId, stepOrder, absentFromUnix, absentUntilUnix, improverId).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error checking replacement claims for absence period: %s", err)
	}
	return count, nil
}

func hasWorkflowSeriesClaimMappingTx(
	ctx context.Context,
	tx pgx.Tx,
	seriesId string,
	stepOrder int,
	improverId string,
) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT
					1
				FROM
					workflow_series_step_claims
				WHERE
					series_id = $1
				AND
					step_order = $2
				AND
					improver_id = $3
			);
	`, seriesId, stepOrder, improverId).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking workflow series claim mapping: %s", err)
	}
	return exists, nil
}

func (a *AppDB) ClaimWorkflowStep(
	ctx context.Context,
	workflowId string,
	stepId string,
	improverId string,
) (*structs.Workflow, *structs.WorkflowStepAvailabilityNotification, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	var workflowStatus string
	var workflowStartAt int64
	var workflowTitle string
	var workflowSeriesId string
	var workflowRecurrence string
	var managerImproverID *string
	err = tx.QueryRow(ctx, `
				SELECT
					w.status,
					w.start_at,
					COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')),
					w.series_id,
					COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')),
					w.manager_improver_id
				FROM
					workflows w
				LEFT JOIN
					workflow_states st
				ON
					st.id = w.workflow_state_id
				LEFT JOIN
					workflow_series s
			ON
				s.id = w.series_id
			WHERE
				w.id = $1
			FOR UPDATE OF w;
	`, workflowId).Scan(&workflowStatus, &workflowStartAt, &workflowTitle, &workflowSeriesId, &workflowRecurrence, &managerImproverID)
	if err != nil {
		return nil, nil, err
	}
	if workflowStatus != "approved" && workflowStatus != "in_progress" {
		return nil, nil, fmt.Errorf("workflow is not available for claiming")
	}

	var claimedAssignments int
	err = tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		AND
			assigned_improver_id = $2;
	`, workflowId, improverId).Scan(&claimedAssignments)
	if err != nil {
		return nil, nil, err
	}
	if claimedAssignments > 0 {
		return nil, nil, fmt.Errorf("improver already assigned within this workflow")
	}
	if managerImproverID != nil && *managerImproverID == improverId {
		return nil, nil, fmt.Errorf("improver already assigned within this workflow")
	}

	var stepWorkflowId string
	var stepStatus string
	var stepTitle string
	var stepOrder int
	var roleId *string
	var assignedImproverId *string
	err = tx.QueryRow(ctx, `
		SELECT
			workflow_id,
			status,
			title,
			step_order,
			role_id,
			assigned_improver_id
		FROM
			workflow_steps
		WHERE
			id = $1
		FOR UPDATE;
	`, stepId).Scan(&stepWorkflowId, &stepStatus, &stepTitle, &stepOrder, &roleId, &assignedImproverId)
	if err != nil {
		return nil, nil, err
	}
	if stepWorkflowId != workflowId {
		return nil, nil, fmt.Errorf("step does not belong to workflow")
	}
	if assignedImproverId != nil {
		return nil, nil, fmt.Errorf("workflow step is already claimed")
	}
	if roleId == nil {
		return nil, nil, fmt.Errorf("workflow step is missing a role")
	}
	if stepStatus != "locked" && stepStatus != "available" {
		return nil, nil, fmt.Errorf("workflow step is not claimable")
	}
	if workflowRecurrence != "one_time" {
		isUnavailableForAbsence, err := hasImproverAbsenceCoverageTx(ctx, tx, improverId, workflowSeriesId, stepOrder, workflowStartAt)
		if err != nil {
			return nil, nil, err
		}
		if isUnavailableForAbsence {
			return nil, nil, fmt.Errorf("step is unavailable during your absence period")
		}
	}

	requiredRows, err := tx.Query(ctx, `
		SELECT
			credential_type
		FROM
			workflow_role_credentials
		WHERE
			role_id = $1;
	`, *roleId)
	if err != nil {
		return nil, nil, err
	}
	defer requiredRows.Close()

	requiredCredentials := []string{}
	for requiredRows.Next() {
		var credential string
		if err := requiredRows.Scan(&credential); err != nil {
			return nil, nil, err
		}
		requiredCredentials = append(requiredCredentials, strings.TrimSpace(credential))
	}
	if len(requiredCredentials) == 0 {
		return nil, nil, fmt.Errorf("workflow role has no credential requirements")
	}
	validCredentialTypes, err := getCredentialTypeSetTx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	for _, required := range requiredCredentials {
		if _, ok := validCredentialTypes[required]; !ok {
			return nil, nil, fmt.Errorf("workflow role references unknown credential type: %s", required)
		}
	}

	activeCredentials, err := getActiveCredentialTypesTx(ctx, tx, improverId)
	if err != nil {
		return nil, nil, err
	}
	activeSet := map[string]struct{}{}
	for _, credential := range activeCredentials {
		activeSet[credential] = struct{}{}
	}
	for _, required := range requiredCredentials {
		if _, ok := activeSet[required]; !ok {
			return nil, nil, fmt.Errorf("missing required credentials for workflow role")
		}
	}

	var postClaimStatus string
	err = tx.QueryRow(ctx, `
		UPDATE
			workflow_steps
		SET
			assigned_improver_id = $2,
			status = CASE
				WHEN status = 'locked' AND step_order = 1 AND $3 <= unix_now() THEN 'available'
				ELSE status
			END,
			updated_at = unix_now()
		WHERE
			id = $1
		RETURNING
			status;
		`, stepId, improverId, workflowStartAt).Scan(&postClaimStatus)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "workflow_single_assignment_per_improver_idx" {
			return nil, nil, fmt.Errorf("improver already assigned within this workflow")
		}
		return nil, nil, fmt.Errorf("error assigning workflow step: %s", err)
	}

	if workflowRecurrence != "one_time" {
		if err := ensureWorkflowSeriesClaimTx(ctx, tx, workflowSeriesId, stepOrder, improverId); err != nil {
			return nil, nil, err
		}
		if err := propagateWorkflowSeriesClaimTx(ctx, tx, workflowSeriesId, stepOrder, improverId, workflowStartAt); err != nil {
			return nil, nil, err
		}
	}

	var availabilityNotification *structs.WorkflowStepAvailabilityNotification
	if postClaimStatus == "available" {
		cmd, err := tx.Exec(ctx, `
			INSERT INTO workflow_step_notifications(step_id, user_id, notification_type)
			VALUES
				($1, $2, 'step_available')
			ON CONFLICT DO NOTHING;
		`, stepId, improverId)
		if err != nil {
			return nil, nil, fmt.Errorf("error recording workflow step notification after claim: %s", err)
		}
		if cmd.RowsAffected() > 0 {
			notification := structs.WorkflowStepAvailabilityNotification{
				WorkflowId:    workflowId,
				WorkflowTitle: workflowTitle,
				StepId:        stepId,
				StepTitle:     stepTitle,
				UserId:        improverId,
			}
			err = tx.QueryRow(ctx, `
				SELECT
					COALESCE(NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''), COALESCE(u.contact_name, '')),
					COALESCE(i.email, u.contact_email, '')
				FROM
					users u
				LEFT JOIN
					improvers i
				ON
					i.user_id = u.id
				WHERE
					u.id = $1;
			`, improverId).Scan(&notification.Name, &notification.Email)
			if err != nil {
				return nil, nil, err
			}
			availabilityNotification = &notification
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	workflow, err := a.GetWorkflowByID(ctx, workflowId)
	if err != nil {
		return nil, nil, err
	}
	return workflow, availabilityNotification, nil
}

func hasImproverAbsenceCoverageTx(
	ctx context.Context,
	tx pgx.Tx,
	improverId string,
	seriesId string,
	stepOrder int,
	workflowStartAt int64,
) (bool, error) {
	row := tx.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT
					1
				FROM
					workflow_improver_absences
				WHERE
					improver_id = $1
				AND
					series_id = $2
				AND
					step_order = $3
				AND
					$4 >= absent_from
				AND
					$4 < absent_until
			);
	`, improverId, seriesId, stepOrder, workflowStartAt)

	var covered bool
	if err := row.Scan(&covered); err != nil {
		return false, fmt.Errorf("error checking improver absence coverage: %s", err)
	}
	return covered, nil
}

func canStepTransitionToAvailableTx(ctx context.Context, tx pgx.Tx, workflowId string, stepOrder int, workflowStartAt int64) (bool, error) {
	if stepOrder <= 1 {
		return workflowStartAt <= time.Now().UTC().Unix(), nil
	}

	var previousStatus string
	err := tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		AND
			step_order = $2;
	`, workflowId, stepOrder-1).Scan(&previousStatus)
	if err != nil {
		return false, err
	}

	return previousStatus == "completed" || previousStatus == "paid_out", nil
}

func (a *AppDB) StartWorkflowStep(ctx context.Context, workflowId string, stepId string, improverId string) (*structs.Workflow, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var workflowStatus string
	var workflowStartAt int64
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			start_at
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&workflowStatus, &workflowStartAt)
	if err != nil {
		return nil, err
	}
	if workflowStatus != "approved" && workflowStatus != "in_progress" {
		return nil, fmt.Errorf("workflow is not active")
	}

	var stepWorkflowId string
	var stepOrder int
	var stepStatus string
	var assignedImproverId *string
	err = tx.QueryRow(ctx, `
		SELECT
			workflow_id,
			step_order,
			status,
			assigned_improver_id
		FROM
			workflow_steps
		WHERE
			id = $1
		FOR UPDATE;
	`, stepId).Scan(&stepWorkflowId, &stepOrder, &stepStatus, &assignedImproverId)
	if err != nil {
		return nil, err
	}
	if stepWorkflowId != workflowId {
		return nil, fmt.Errorf("step does not belong to workflow")
	}
	if assignedImproverId == nil || *assignedImproverId != improverId {
		return nil, fmt.Errorf("step is not assigned to this improver")
	}

	if stepStatus == "completed" || stepStatus == "paid_out" {
		return nil, fmt.Errorf("step has already been completed")
	}

	if stepStatus == "locked" {
		canUnlock, err := canStepTransitionToAvailableTx(ctx, tx, workflowId, stepOrder, workflowStartAt)
		if err != nil {
			return nil, err
		}
		if !canUnlock {
			return nil, fmt.Errorf("step is not available yet")
		}
		_, err = tx.Exec(ctx, `
			UPDATE
				workflow_steps
			SET
				status = 'available',
				updated_at = unix_now()
			WHERE
				id = $1;
		`, stepId)
		if err != nil {
			return nil, fmt.Errorf("error unlocking workflow step: %s", err)
		}
		stepStatus = "available"
	}

	if stepStatus == "available" {
		_, err = tx.Exec(ctx, `
			UPDATE
				workflow_steps
			SET
				status = 'in_progress',
				started_at = COALESCE(started_at, unix_now()),
				updated_at = unix_now()
			WHERE
				id = $1;
		`, stepId)
		if err != nil {
			return nil, fmt.Errorf("error starting workflow step: %s", err)
		}
	}

	if workflowStatus == "approved" {
		_, err = tx.Exec(ctx, `
			UPDATE
				workflows
			SET
				status = 'in_progress',
				updated_at = unix_now()
			WHERE
				id = $1;
		`, workflowId)
		if err != nil {
			return nil, fmt.Errorf("error updating workflow status to in_progress: %s", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a.GetWorkflowByID(ctx, workflowId)
}

var dropdownValueSanitizer = regexp.MustCompile(`[^a-z0-9]+`)
var workflowNotificationEmailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

const defaultWorkflowPhotoAspectRatio = "square"

func deriveDropdownValueFromLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	label = dropdownValueSanitizer.ReplaceAllString(label, "_")
	return strings.Trim(label, "_")
}

func normalizeWorkflowPhotoAspectRatio(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return defaultWorkflowPhotoAspectRatio, nil
	}
	switch normalized {
	case "vertical", "square", "horizontal":
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid photo aspect ratio")
	}
}

func normalizeEmailList(emails []string) []string {
	normalized := make([]string, 0, len(emails))
	seen := map[string]struct{}{}
	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		normalized = append(normalized, email)
	}
	return normalized
}

func normalizeValidatedWorkflowNotificationEmails(emails []string) ([]string, error) {
	normalized := make([]string, 0, len(emails))
	seen := map[string]struct{}{}
	for _, raw := range emails {
		email := strings.ToLower(strings.TrimSpace(raw))
		if email == "" {
			continue
		}
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed == nil || strings.TrimSpace(parsed.Address) == "" {
			return nil, fmt.Errorf("invalid notification email format")
		}
		if strings.ToLower(strings.TrimSpace(parsed.Address)) != email {
			return nil, fmt.Errorf("invalid notification email format")
		}
		if !workflowNotificationEmailPattern.MatchString(email) {
			return nil, fmt.Errorf("invalid notification email format")
		}
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		normalized = append(normalized, email)
	}
	return normalized, nil
}

const maxWorkflowPhotoUploadBytes = 2 * 1024 * 1024

type parsedWorkflowPhotoUpload struct {
	FileName    string
	ContentType string
	Data        []byte
}

func parseWorkflowPhotoUpload(upload structs.WorkflowPhotoUpload) (*parsedWorkflowPhotoUpload, error) {
	base64Payload := strings.TrimSpace(upload.DataBase64)
	if base64Payload == "" {
		return nil, fmt.Errorf("photo upload data is required")
	}
	if commaIdx := strings.Index(base64Payload, ","); commaIdx >= 0 {
		prefix := strings.ToLower(strings.TrimSpace(base64Payload[:commaIdx]))
		if strings.Contains(prefix, "base64") {
			base64Payload = strings.TrimSpace(base64Payload[commaIdx+1:])
		}
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Payload)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(base64Payload)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 image payload")
		}
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("photo upload payload is empty")
	}
	if len(decoded) > maxWorkflowPhotoUploadBytes {
		return nil, fmt.Errorf("photo upload exceeds maximum size of 2MB")
	}

	contentType := strings.ToLower(strings.TrimSpace(upload.ContentType))
	if contentType == "" {
		contentType = strings.ToLower(http.DetectContentType(decoded))
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("photo upload must be an image")
	}

	fileName := strings.TrimSpace(upload.FileName)
	if fileName != "" {
		fileName = filepath.Base(fileName)
	}
	if fileName == "" || fileName == "." || fileName == string(filepath.Separator) {
		fileName = "photo"
	}
	fileName = strings.ReplaceAll(fileName, "\x00", "")
	if len(fileName) > 180 {
		fileName = fileName[:180]
	}

	return &parsedWorkflowPhotoUpload{
		FileName:    fileName,
		ContentType: contentType,
		Data:        decoded,
	}, nil
}

func (a *AppDB) CompleteWorkflowStep(
	ctx context.Context,
	workflowId string,
	stepId string,
	improverId string,
	stepNotPossible bool,
	stepNotPossibleDetails *string,
	itemResponses []structs.WorkflowStepItemResponse,
) (*structs.WorkflowStepCompletionResult, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	result := &structs.WorkflowStepCompletionResult{
		AvailabilityNotifications: []structs.WorkflowStepAvailabilityNotification{},
		DropdownNotifications:     []structs.WorkflowDropdownNotification{},
	}

	var workflowStatus string
	var workflowStartAt int64
	var workflowTitle string
	err = tx.QueryRow(ctx, `
			SELECT
				w.status,
				w.start_at,
				COALESCE(
					(
						SELECT
							NULLIF(TRIM(ws.title), '')
						FROM
							workflow_series ws
						WHERE
							ws.id = w.series_id
					),
					''
				)
			FROM
				workflows w
			WHERE
				w.id = $1
			FOR UPDATE;
		`, workflowId).Scan(&workflowStatus, &workflowStartAt, &workflowTitle)
	if err != nil {
		return nil, err
	}
	if workflowStatus != "approved" && workflowStatus != "in_progress" {
		return nil, fmt.Errorf("workflow is not active")
	}

	var stepWorkflowId string
	var stepOrder int
	var stepStatus string
	var stepTitle string
	var allowStepNotPossible bool
	var assignedImproverId *string
	err = tx.QueryRow(ctx, `
		SELECT
			workflow_id,
			step_order,
			status,
			title,
			allow_step_not_possible,
			assigned_improver_id
		FROM
			workflow_steps
		WHERE
			id = $1
		FOR UPDATE;
	`, stepId).Scan(&stepWorkflowId, &stepOrder, &stepStatus, &stepTitle, &allowStepNotPossible, &assignedImproverId)
	if err != nil {
		return nil, err
	}
	if stepWorkflowId != workflowId {
		return nil, fmt.Errorf("step does not belong to workflow")
	}
	if assignedImproverId == nil || *assignedImproverId != improverId {
		return nil, fmt.Errorf("step is not assigned to this improver")
	}
	if stepStatus == "completed" || stepStatus == "paid_out" {
		return nil, fmt.Errorf("step has already been completed")
	}

	canUnlock, err := canStepTransitionToAvailableTx(ctx, tx, workflowId, stepOrder, workflowStartAt)
	if err != nil {
		return nil, err
	}
	if stepStatus == "locked" && !canUnlock {
		return nil, fmt.Errorf("step is not available yet")
	}

	stepNotPossibleDetailValue := ""
	if stepNotPossibleDetails != nil {
		stepNotPossibleDetailValue = strings.TrimSpace(*stepNotPossibleDetails)
	}
	if stepNotPossible {
		if !allowStepNotPossible {
			return nil, fmt.Errorf("invalid step_not_possible request: option is not enabled for this step")
		}
		if stepNotPossibleDetailValue == "" {
			return nil, fmt.Errorf("step_not_possible_details is required when step_not_possible is selected")
		}
		if len(itemResponses) > 0 {
			return nil, fmt.Errorf("invalid step_not_possible request: item responses are not allowed")
		}
	} else if stepNotPossibleDetailValue != "" {
		return nil, fmt.Errorf("invalid step_not_possible request: details are only allowed when step_not_possible is selected")
	}

	var stepNotPossibleDetailsNormalized *string
	if stepNotPossibleDetailValue != "" {
		stepNotPossibleDetailsNormalized = &stepNotPossibleDetailValue
	}

	serializedResponses := []structs.WorkflowStepItemResponse{}
	photoUploadsByItem := map[string][]parsedWorkflowPhotoUpload{}
	if !stepNotPossible {
		itemRows, err := tx.Query(ctx, `
			SELECT
				id,
				title,
				is_optional,
				requires_photo,
				camera_capture_only,
				photo_required_count,
				photo_allow_any_count,
				photo_aspect_ratio,
				requires_written_response,
				requires_dropdown,
				dropdown_options,
				dropdown_requires_written_response,
				notify_emails,
			notify_on_dropdown_values
		FROM
			workflow_step_items
		WHERE
			step_id = $1
		ORDER BY
			item_order ASC;
	`, stepId)
		if err != nil {
			return nil, fmt.Errorf("error querying workflow step items for completion: %s", err)
		}
		defer itemRows.Close()

		type stepItemMeta struct {
			Id                         string
			Title                      string
			Optional                   bool
			RequiresPhoto              bool
			CameraCaptureOnly          bool
			PhotoRequiredCount         int
			PhotoAllowAnyCount         bool
			PhotoAspectRatio           string
			RequiresWrittenResponse    bool
			RequiresDropdown           bool
			DropdownOptions            []structs.WorkflowDropdownOption
			DropdownRequiresWrittenMap map[string]bool
		}

		items := []stepItemMeta{}
		itemByID := map[string]stepItemMeta{}

		for itemRows.Next() {
			item := stepItemMeta{}
			var dropdownOptionsBytes []byte
			var dropdownRequiresBytes []byte
			var notifyEmailsBytes []byte
			var notifyValuesBytes []byte
			if err := itemRows.Scan(
				&item.Id,
				&item.Title,
				&item.Optional,
				&item.RequiresPhoto,
				&item.CameraCaptureOnly,
				&item.PhotoRequiredCount,
				&item.PhotoAllowAnyCount,
				&item.PhotoAspectRatio,
				&item.RequiresWrittenResponse,
				&item.RequiresDropdown,
				&dropdownOptionsBytes,
				&dropdownRequiresBytes,
				&notifyEmailsBytes,
				&notifyValuesBytes,
			); err != nil {
				return nil, fmt.Errorf("error scanning workflow step item metadata: %s", err)
			}

			item.DropdownOptions = []structs.WorkflowDropdownOption{}
			if item.PhotoRequiredCount <= 0 {
				item.PhotoRequiredCount = 1
			}
			normalizedAspect, aspectErr := normalizeWorkflowPhotoAspectRatio(item.PhotoAspectRatio)
			if aspectErr != nil {
				normalizedAspect = defaultWorkflowPhotoAspectRatio
			}
			item.PhotoAspectRatio = normalizedAspect
			if !item.RequiresPhoto {
				item.PhotoAllowAnyCount = false
			}
			if len(dropdownOptionsBytes) > 0 {
				if err := json.Unmarshal(dropdownOptionsBytes, &item.DropdownOptions); err != nil {
					return nil, fmt.Errorf("error unmarshalling workflow step item dropdown options: %s", err)
				}
			}
			for idx := range item.DropdownOptions {
				item.DropdownOptions[idx].NotifyEmails = normalizeEmailList(item.DropdownOptions[idx].NotifyEmails)
			}
			item.DropdownRequiresWrittenMap = map[string]bool{}
			if len(dropdownRequiresBytes) > 0 {
				if err := json.Unmarshal(dropdownRequiresBytes, &item.DropdownRequiresWrittenMap); err != nil {
					return nil, fmt.Errorf("error unmarshalling workflow step item dropdown requirement map: %s", err)
				}
			}

			legacyNotifyEmails := []string{}
			if len(notifyEmailsBytes) > 0 {
				if err := json.Unmarshal(notifyEmailsBytes, &legacyNotifyEmails); err != nil {
					return nil, fmt.Errorf("error unmarshalling workflow step item notification emails: %s", err)
				}
			}

			legacyNotifyValues := []string{}
			if len(notifyValuesBytes) > 0 {
				if err := json.Unmarshal(notifyValuesBytes, &legacyNotifyValues); err != nil {
					return nil, fmt.Errorf("error unmarshalling workflow step item notification values: %s", err)
				}
			}
			legacyNotifyEmails = normalizeEmailList(legacyNotifyEmails)
			if len(legacyNotifyEmails) > 0 && len(legacyNotifyValues) > 0 {
				legacyWatchValues := map[string]struct{}{}
				for _, value := range legacyNotifyValues {
					value = strings.TrimSpace(value)
					if value == "" {
						continue
					}
					legacyWatchValues[value] = struct{}{}
				}
				if len(legacyWatchValues) > 0 {
					for idx := range item.DropdownOptions {
						if len(item.DropdownOptions[idx].NotifyEmails) > 0 {
							continue
						}
						if _, ok := legacyWatchValues[item.DropdownOptions[idx].Value]; !ok {
							continue
						}
						item.DropdownOptions[idx].NotifyEmails = append([]string{}, legacyNotifyEmails...)
					}
				}
			}
			for idx := range item.DropdownOptions {
				item.DropdownOptions[idx].NotifyEmailCount = len(item.DropdownOptions[idx].NotifyEmails)
			}

			items = append(items, item)
			itemByID[item.Id] = item
		}

		responseMap := map[string]structs.WorkflowStepItemResponse{}
		for _, response := range itemResponses {
			itemId := strings.TrimSpace(response.ItemId)
			if itemId == "" {
				return nil, fmt.Errorf("item_id is required for step completion")
			}
			if _, exists := itemByID[itemId]; !exists {
				return nil, fmt.Errorf("workflow step response references unknown item_id: %s", itemId)
			}
			if _, exists := responseMap[itemId]; exists {
				return nil, fmt.Errorf("duplicate workflow step response item_id: %s", itemId)
			}

			cleanUploads := make([]parsedWorkflowPhotoUpload, 0, len(response.PhotoUploads))
			for _, upload := range response.PhotoUploads {
				parsedUpload, parseErr := parseWorkflowPhotoUpload(upload)
				if parseErr != nil {
					return nil, fmt.Errorf("invalid photo upload for item %s: %s", itemId, parseErr)
				}
				cleanUploads = append(cleanUploads, *parsedUpload)
			}
			photoUploadsByItem[itemId] = cleanUploads

			if response.WrittenResponse != nil {
				trimmed := strings.TrimSpace(*response.WrittenResponse)
				if trimmed == "" {
					response.WrittenResponse = nil
				} else {
					response.WrittenResponse = &trimmed
				}
			}
			if response.DropdownValue != nil {
				trimmed := strings.TrimSpace(*response.DropdownValue)
				if trimmed == "" {
					response.DropdownValue = nil
				} else {
					response.DropdownValue = &trimmed
				}
			}

			response.ItemId = itemId
			response.PhotoURLs = nil
			response.PhotoIDs = nil
			response.PhotoUploads = nil
			response.Photos = nil
			responseMap[itemId] = response
		}

		for _, item := range items {
			response, hasResponse := responseMap[item.Id]
			if !hasResponse {
				if item.Optional {
					continue
				}
				return nil, fmt.Errorf("required step item missing response: %s", item.Title)
			}

			photoUploads := photoUploadsByItem[item.Id]
			hasAnyResponse := len(photoUploads) > 0 || response.WrittenResponse != nil || response.DropdownValue != nil
			if item.Optional && !hasAnyResponse {
				continue
			}

			if item.RequiresPhoto {
				if item.PhotoAllowAnyCount {
					if len(photoUploads) == 0 {
						return nil, fmt.Errorf("step item requires photo evidence: %s", item.Title)
					}
				} else if len(photoUploads) != item.PhotoRequiredCount {
					return nil, fmt.Errorf("step item requires exactly %d photo(s): %s", item.PhotoRequiredCount, item.Title)
				}
			}
			if item.RequiresWrittenResponse && response.WrittenResponse == nil {
				return nil, fmt.Errorf("step item requires written response: %s", item.Title)
			}
			if item.RequiresDropdown {
				if response.DropdownValue == nil {
					return nil, fmt.Errorf("step item requires dropdown selection: %s", item.Title)
				}

				dropdownAllowed := map[string]struct{}{}
				var selectedOption *structs.WorkflowDropdownOption
				for _, option := range item.DropdownOptions {
					dropdownAllowed[option.Value] = struct{}{}
					if option.Value == *response.DropdownValue {
						opt := option
						selectedOption = &opt
					}
				}
				if _, ok := dropdownAllowed[*response.DropdownValue]; !ok {
					return nil, fmt.Errorf("invalid dropdown value for step item: %s", item.Title)
				}

				if requiredWritten, ok := item.DropdownRequiresWrittenMap[*response.DropdownValue]; ok && requiredWritten && response.WrittenResponse == nil {
					return nil, fmt.Errorf("dropdown selection requires written response for step item: %s", item.Title)
				}

				if selectedOption != nil {
					emails := normalizeEmailList(selectedOption.NotifyEmails)
					if len(emails) > 0 {
						result.DropdownNotifications = append(result.DropdownNotifications, structs.WorkflowDropdownNotification{
							WorkflowId:            workflowId,
							WorkflowTitle:         workflowTitle,
							StepId:                stepId,
							StepTitle:             stepTitle,
							ItemId:                item.Id,
							ItemTitle:             item.Title,
							DropdownValue:         *response.DropdownValue,
							Emails:                emails,
							SendPicturesWithEmail: selectedOption.SendPicturesWithEmail,
						})
					}
				}
			}

			serializedResponses = append(serializedResponses, response)
		}
	}

	var submissionId string
	err = tx.QueryRow(ctx, `
		INSERT INTO workflow_step_submissions
			(
				id,
				workflow_id,
				step_id,
				improver_id,
				step_not_possible,
				step_not_possible_details,
				item_responses,
				submitted_at,
				updated_at
			)
		VALUES
			($1, $2, $3, $4, $5, $6, '[]'::jsonb, unix_now(), unix_now())
		ON CONFLICT (step_id)
		DO UPDATE SET
			improver_id = EXCLUDED.improver_id,
			step_not_possible = EXCLUDED.step_not_possible,
			step_not_possible_details = EXCLUDED.step_not_possible_details,
			submitted_at = unix_now(),
			updated_at = unix_now()
		RETURNING
			id;
	`, uuid.NewString(), workflowId, stepId, improverId, stepNotPossible, stepNotPossibleDetailsNormalized).Scan(&submissionId)
	if err != nil {
		return nil, fmt.Errorf("error upserting workflow step submission: %s", err)
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM workflow_submission_photos
		WHERE submission_id = $1;
	`, submissionId)
	if err != nil {
		return nil, fmt.Errorf("error clearing workflow submission photos: %s", err)
	}

	for responseIndex := range serializedResponses {
		response := serializedResponses[responseIndex]
		uploads := photoUploadsByItem[response.ItemId]
		if len(uploads) == 0 {
			continue
		}

		photoIDs := make([]string, 0, len(uploads))
		for _, upload := range uploads {
			photoID := uuid.NewString()
			if _, err := tx.Exec(ctx, `
				INSERT INTO workflow_submission_photos
					(
						id,
						workflow_id,
						step_id,
						item_id,
						submission_id,
						file_name,
						content_type,
						photo_data,
						size_bytes
					)
				VALUES
					($1, $2, $3, $4, $5, $6, $7, $8, $9);
			`, photoID, workflowId, stepId, response.ItemId, submissionId, upload.FileName, upload.ContentType, upload.Data, len(upload.Data)); err != nil {
				return nil, fmt.Errorf("error inserting workflow submission photo: %s", err)
			}
			photoIDs = append(photoIDs, photoID)
		}

		response.PhotoIDs = photoIDs
		response.PhotoURLs = nil
		response.PhotoUploads = nil
		response.Photos = nil
		serializedResponses[responseIndex] = response
	}

	if len(result.DropdownNotifications) > 0 {
		allPhotoIDs := make([]string, 0)
		for _, response := range serializedResponses {
			if len(response.PhotoIDs) == 0 {
				continue
			}
			allPhotoIDs = append(allPhotoIDs, response.PhotoIDs...)
		}
		if len(allPhotoIDs) > 0 {
			for idx := range result.DropdownNotifications {
				if !result.DropdownNotifications[idx].SendPicturesWithEmail {
					continue
				}
				result.DropdownNotifications[idx].PhotoIDs = append([]string{}, allPhotoIDs...)
			}
		}
	}

	responsesJSON, err := json.Marshal(serializedResponses)
	if err != nil {
		return nil, fmt.Errorf("error marshalling workflow step responses: %s", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			workflow_step_submissions
		SET
			item_responses = $2::jsonb,
			submitted_at = unix_now(),
			updated_at = unix_now()
		WHERE
			id = $1;
	`, submissionId, string(responsesJSON))
	if err != nil {
		return nil, fmt.Errorf("error updating workflow step submission responses: %s", err)
	}

	if stepNotPossible {
		_, err = tx.Exec(ctx, `
			UPDATE
				workflow_steps
			SET
				status = CASE WHEN status = 'paid_out' THEN 'paid_out' ELSE 'completed' END,
				started_at = COALESCE(started_at, unix_now()),
				completed_at = COALESCE(completed_at, unix_now()),
				bounty = 0,
				payout_error = NULL,
				payout_last_try_at = NULL,
				payout_in_progress = false,
				retry_requested_at = NULL,
				retry_requested_by = NULL,
				updated_at = unix_now()
			WHERE
				workflow_id = $1;
		`, workflowId)
		if err != nil {
			return nil, fmt.Errorf("error marking workflow steps completed for step_not_possible: %s", err)
		}

		_, err = tx.Exec(ctx, `
			UPDATE
				workflows
			SET
				status = 'completed',
				total_bounty = 0,
				weekly_bounty_requirement = 0,
				manager_bounty = 0,
				manager_paid_out_at = NULL,
				manager_payout_error = NULL,
				manager_payout_last_try_at = NULL,
				manager_payout_in_progress = false,
				manager_retry_requested_at = NULL,
				manager_retry_requested_by = NULL,
				updated_at = unix_now()
			WHERE
				id = $1;
		`, workflowId)
		if err != nil {
			return nil, fmt.Errorf("error finalizing workflow after step_not_possible: %s", err)
		}
		if _, err := ensureRecurringWorkflowSuccessorTx(ctx, tx, workflowId); err != nil {
			return nil, err
		}

		result.WorkflowStatus = "completed"
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return result, nil
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			workflow_steps
		SET
			status = 'completed',
			started_at = COALESCE(started_at, unix_now()),
			completed_at = unix_now(),
			payout_in_progress = false,
			updated_at = unix_now()
		WHERE
			id = $1;
	`, stepId)
	if err != nil {
		return nil, fmt.Errorf("error marking workflow step completed: %s", err)
	}

	var nextStepId string
	var nextStepTitle string
	var nextStepStatus string
	var nextAssignedImproverId *string
	err = tx.QueryRow(ctx, `
		SELECT
			id,
			title,
			status,
			assigned_improver_id
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		AND
			step_order = $2
		FOR UPDATE;
	`, workflowId, stepOrder+1).Scan(&nextStepId, &nextStepTitle, &nextStepStatus, &nextAssignedImproverId)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	if err == nil && nextStepStatus == "locked" {
		_, err = tx.Exec(ctx, `
			UPDATE
				workflow_steps
			SET
				status = 'available',
				updated_at = unix_now()
			WHERE
				id = $1;
		`, nextStepId)
		if err != nil {
			return nil, fmt.Errorf("error unlocking next workflow step: %s", err)
		}

		if nextAssignedImproverId != nil {
			cmd, err := tx.Exec(ctx, `
				INSERT INTO workflow_step_notifications(step_id, user_id, notification_type)
				VALUES
					($1, $2, 'step_available')
				ON CONFLICT DO NOTHING;
			`, nextStepId, *nextAssignedImproverId)
			if err != nil {
				return nil, fmt.Errorf("error recording step availability notification: %s", err)
			}
			if cmd.RowsAffected() > 0 {
				notification := structs.WorkflowStepAvailabilityNotification{
					WorkflowId:    workflowId,
					WorkflowTitle: workflowTitle,
					StepId:        nextStepId,
					StepTitle:     nextStepTitle,
					UserId:        *nextAssignedImproverId,
				}
				err = tx.QueryRow(ctx, `
					SELECT
						COALESCE(NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''), COALESCE(u.contact_name, '')),
						COALESCE(i.email, u.contact_email, '')
					FROM
						users u
					LEFT JOIN
						improvers i
					ON
						i.user_id = u.id
					WHERE
						u.id = $1;
				`, *nextAssignedImproverId).Scan(&notification.Name, &notification.Email)
				if err != nil {
					return nil, err
				}
				result.AvailabilityNotifications = append(result.AvailabilityNotifications, notification)
			}
		}
	}

	var incompleteSteps int
	err = tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		AND
			status NOT IN ('completed', 'paid_out');
	`, workflowId).Scan(&incompleteSteps)
	if err != nil {
		return nil, err
	}

	if incompleteSteps == 0 {
		result.WorkflowStatus = "completed"
		_, err = tx.Exec(ctx, `
			UPDATE
				workflows
			SET
				status = 'completed',
				updated_at = unix_now()
			WHERE
				id = $1;
		`, workflowId)
		if err != nil {
			return nil, fmt.Errorf("error marking workflow completed: %s", err)
		}
		if _, err := ensureRecurringWorkflowSuccessorTx(ctx, tx, workflowId); err != nil {
			return nil, err
		}
	} else {
		result.WorkflowStatus = "in_progress"
		if workflowStatus == "approved" {
			_, err = tx.Exec(ctx, `
				UPDATE
					workflows
				SET
					status = 'in_progress',
					updated_at = unix_now()
				WHERE
					id = $1;
			`, workflowId)
			if err != nil {
				return nil, fmt.Errorf("error marking workflow in progress: %s", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (a *AppDB) GetWorkflowSeriesOrderedIDs(ctx context.Context, workflowId string) ([]string, error) {
	var seriesId string
	err := a.db.QueryRow(ctx, `
		SELECT
			series_id
		FROM
			workflows
		WHERE
			id = $1;
	`, workflowId).Scan(&seriesId)
	if err != nil {
		return nil, err
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			workflows
		WHERE
			series_id = $1
		ORDER BY
			start_at ASC,
			created_at ASC,
			id ASC;
	`, seriesId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow series order: %s", err)
	}
	defer rows.Close()

	workflowIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning workflow series id: %s", err)
		}
		workflowIDs = append(workflowIDs, id)
	}
	return workflowIDs, nil
}

func (a *AppDB) GetImproverUnpaidWorkflows(ctx context.Context, improverId string) ([]*structs.Workflow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			w.id
		FROM
			workflows w
		WHERE
			w.status = 'completed'
		AND
			(
				(
					EXISTS (
						SELECT
							1
						FROM
							workflow_steps ws
						WHERE
							ws.workflow_id = w.id
						AND
							ws.assigned_improver_id = $1
						AND
							ws.status = 'completed'
						AND
							ws.bounty > 0
					)
				)
				OR
				(
					w.manager_improver_id = $1
				AND
					w.manager_bounty > 0
				AND
					w.manager_paid_out_at IS NULL
				)
			)
		ORDER BY
			w.start_at ASC,
			w.created_at ASC
		LIMIT 300;
	`, improverId)
	if err != nil {
		return nil, fmt.Errorf("error querying improver unpaid workflows: %s", err)
	}
	defer rows.Close()

	workflowIDs := []string{}
	for rows.Next() {
		var workflowID string
		if err := rows.Scan(&workflowID); err != nil {
			return nil, fmt.Errorf("error scanning improver unpaid workflow id: %s", err)
		}
		workflowIDs = append(workflowIDs, workflowID)
	}

	workflows := make([]*structs.Workflow, 0, len(workflowIDs))
	for _, workflowID := range workflowIDs {
		workflow, err := a.GetWorkflowByID(ctx, workflowID)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, workflow)
	}
	return workflows, nil
}

func (a *AppDB) GetPreferredWorkflowPayoutAddressForUser(ctx context.Context, userId string, preferSupervisor bool) (string, error) {
	type rewardsSource struct {
		query string
		args  []any
	}
	sources := make([]rewardsSource, 0, 2)
	if preferSupervisor {
		sources = append(sources,
			rewardsSource{
				query: `
					SELECT
						primary_rewards_account
					FROM
						supervisors
					WHERE
						user_id = $1
					AND
						status = 'approved'
					AND
						TRIM(COALESCE(primary_rewards_account, '')) <> '';
				`,
				args: []any{userId},
			},
			rewardsSource{
				query: `
					SELECT
						primary_rewards_account
					FROM
						improvers
					WHERE
						user_id = $1
					AND
						status = 'approved'
					AND
						TRIM(COALESCE(primary_rewards_account, '')) <> '';
				`,
				args: []any{userId},
			},
		)
	} else {
		sources = append(sources,
			rewardsSource{
				query: `
					SELECT
						primary_rewards_account
					FROM
						improvers
					WHERE
						user_id = $1
					AND
						status = 'approved'
					AND
						TRIM(COALESCE(primary_rewards_account, '')) <> '';
				`,
				args: []any{userId},
			},
			rewardsSource{
				query: `
					SELECT
						primary_rewards_account
					FROM
						supervisors
					WHERE
						user_id = $1
					AND
						status = 'approved'
					AND
						TRIM(COALESCE(primary_rewards_account, '')) <> '';
				`,
				args: []any{userId},
			},
		)
	}

	defaultRewardsAccount, err := syncDefaultPrimaryRewardsAccountsForUser(ctx, a.db, userId)
	if err != nil {
		return "", fmt.Errorf("error syncing default rewards account: %s", err)
	}

	for _, source := range sources {
		var address string
		err := a.db.QueryRow(ctx, source.query, source.args...).Scan(&address)
		if err == pgx.ErrNoRows {
			continue
		}
		if err != nil {
			return "", err
		}
		address = strings.TrimSpace(address)
		if address != "" && common.IsHexAddress(address) {
			return common.HexToAddress(address).Hex(), nil
		}
	}

	if defaultRewardsAccount != "" {
		return defaultRewardsAccount, nil
	}

	row := a.db.QueryRow(ctx, `
		SELECT
			candidate.address
		FROM
			(
				SELECT
					CASE
						WHEN w.is_eoa = false AND TRIM(COALESCE(w.smart_address, '')) <> '' THEN TRIM(w.smart_address)
						WHEN w.is_eoa = true AND TRIM(COALESCE(w.eoa_address, '')) <> '' THEN TRIM(w.eoa_address)
						ELSE NULL
					END AS address,
					CASE
						WHEN w.is_eoa = false AND w.smart_index = 0 AND TRIM(COALESCE(w.smart_address, '')) <> '' THEN 0
						WHEN w.is_eoa = false AND TRIM(COALESCE(w.smart_address, '')) <> '' THEN 1
						WHEN w.is_eoa = true AND TRIM(COALESCE(w.eoa_address, '')) <> '' THEN 2
						ELSE 3
					END AS preference_rank,
					w.smart_index,
					w.id
				FROM
					wallets w
				WHERE
					w.owner = $1
			) candidate
		WHERE
			candidate.address IS NOT NULL
		ORDER BY
			candidate.preference_rank ASC,
			candidate.smart_index ASC NULLS LAST,
			candidate.id ASC
		LIMIT 1;
	`, userId)

	var address string
	if err := row.Scan(&address); err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("wallet address is not configured for user")
		}
		return "", err
	}
	if strings.TrimSpace(address) == "" {
		return "", fmt.Errorf("wallet address is not configured for user")
	}
	normalizedAddress := strings.TrimSpace(address)
	if !common.IsHexAddress(normalizedAddress) {
		return "", fmt.Errorf("wallet address is not configured for user")
	}
	return common.HexToAddress(normalizedAddress).Hex(), nil
}

func (a *AppDB) GetPreferredRedeemerWalletAddressForUser(ctx context.Context, userId string) (string, error) {
	return a.GetPreferredWorkflowPayoutAddressForUser(ctx, userId, false)
}

func truncateWorkflowPayoutErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 800 {
		return message
	}
	return message[:800]
}

func normalizeAdminWorkflowPayoutResolutionTargetType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "step", "supervisor":
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid payout resolution target type")
	}
}

func normalizeAdminWorkflowPayoutResolutionAction(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "mark_paid_out", "mark_failed":
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid payout resolution action")
	}
}

func (a *AppDB) ResolveWorkflowPayoutLockByAdmin(
	ctx context.Context,
	adminId string,
	workflowId string,
	req *structs.AdminWorkflowPayoutResolutionRequest,
) error {
	adminId = strings.TrimSpace(adminId)
	workflowId = strings.TrimSpace(workflowId)
	if adminId == "" {
		return fmt.Errorf("admin id is required")
	}
	if workflowId == "" {
		return fmt.Errorf("workflow id is required")
	}
	if req == nil {
		return fmt.Errorf("request is required")
	}

	targetType, err := normalizeAdminWorkflowPayoutResolutionTargetType(req.TargetType)
	if err != nil {
		return err
	}
	action, err := normalizeAdminWorkflowPayoutResolutionAction(req.Action)
	if err != nil {
		return err
	}
	stepId := strings.TrimSpace(req.StepId)
	errorMessage := truncateWorkflowPayoutErrorMessage(req.ErrorMessage)
	if action == "mark_failed" && errorMessage == "" {
		errorMessage = "admin manually marked payout as failed"
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	switch targetType {
	case "step":
		if stepId == "" {
			return fmt.Errorf("step id is required for step payout resolution")
		}

		var stepStatus string
		var payoutInProgress bool
		var stepBounty uint64
		err = tx.QueryRow(ctx, `
			SELECT
				status,
				payout_in_progress,
				bounty
			FROM
				workflow_steps
			WHERE
				id = $1
			AND
				workflow_id = $2
			FOR UPDATE;
		`, stepId, workflowId).Scan(&stepStatus, &payoutInProgress, &stepBounty)
		if err != nil {
			return err
		}

		if stepStatus != "completed" {
			return fmt.Errorf("workflow step payout resolution requires completed step status")
		}
		if stepBounty == 0 {
			return fmt.Errorf("workflow step payout resolution is not applicable for zero-bounty step")
		}
		if !payoutInProgress {
			return fmt.Errorf("workflow step payout is not currently locked in progress")
		}

		if action == "mark_paid_out" {
			_, err = tx.Exec(ctx, `
				UPDATE
					workflow_steps
				SET
					status = 'paid_out',
					payout_error = NULL,
					payout_last_try_at = unix_now(),
					payout_in_progress = false,
					retry_requested_at = NULL,
					retry_requested_by = NULL,
					updated_at = unix_now()
				WHERE
					id = $1
				AND
					workflow_id = $2;
			`, stepId, workflowId)
			if err != nil {
				return fmt.Errorf("error marking workflow step paid out during admin payout resolution: %s", err)
			}
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE
					workflow_steps
				SET
					payout_error = $3,
					payout_last_try_at = unix_now(),
					payout_in_progress = false,
					retry_requested_at = NULL,
					retry_requested_by = NULL,
					updated_at = unix_now()
				WHERE
					id = $1
				AND
					workflow_id = $2;
			`, stepId, workflowId, errorMessage)
			if err != nil {
				return fmt.Errorf("error marking workflow step payout failure during admin payout resolution: %s", err)
			}
		}

	case "supervisor":
		if stepId != "" {
			return fmt.Errorf("step id is not allowed for supervisor payout resolution")
		}

		var workflowStatus string
		var managerBounty uint64
		var managerImproverId *string
		var managerPaidOutAt *int64
		var managerPayoutInProgress bool
		err = tx.QueryRow(ctx, `
			SELECT
				status,
				manager_bounty,
				manager_improver_id,
				manager_paid_out_at,
				manager_payout_in_progress
			FROM
				workflows
			WHERE
				id = $1
			FOR UPDATE;
		`, workflowId).Scan(
			&workflowStatus,
			&managerBounty,
			&managerImproverId,
			&managerPaidOutAt,
			&managerPayoutInProgress,
		)
		if err != nil {
			return err
		}

		if workflowStatus != "completed" {
			return fmt.Errorf("workflow supervisor payout resolution requires completed workflow status")
		}
		if managerBounty == 0 || managerImproverId == nil || strings.TrimSpace(*managerImproverId) == "" {
			return fmt.Errorf("workflow supervisor payout resolution is not applicable for this workflow")
		}
		if managerPaidOutAt != nil {
			return fmt.Errorf("workflow supervisor payout is already marked paid out")
		}
		if !managerPayoutInProgress {
			return fmt.Errorf("workflow supervisor payout is not currently locked in progress")
		}

		if action == "mark_paid_out" {
			_, err = tx.Exec(ctx, `
				UPDATE
					workflows
				SET
					manager_paid_out_at = unix_now(),
					manager_payout_error = NULL,
					manager_payout_last_try_at = unix_now(),
					manager_payout_in_progress = false,
					manager_retry_requested_at = NULL,
					manager_retry_requested_by = NULL,
					updated_at = unix_now()
				WHERE
					id = $1;
			`, workflowId)
			if err != nil {
				return fmt.Errorf("error marking workflow supervisor paid out during admin payout resolution: %s", err)
			}
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE
					workflows
				SET
					manager_payout_error = $2,
					manager_payout_last_try_at = unix_now(),
					manager_payout_in_progress = false,
					manager_retry_requested_at = NULL,
					manager_retry_requested_by = NULL,
					updated_at = unix_now()
				WHERE
					id = $1;
			`, workflowId, errorMessage)
			if err != nil {
				return fmt.Errorf("error marking workflow supervisor payout failure during admin payout resolution: %s", err)
			}
		}
	}

	var stepIDValue any
	if targetType == "step" {
		stepIDValue = stepId
	} else {
		stepIDValue = nil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO workflow_payout_admin_actions(
			id,
			workflow_id,
			step_id,
			target_type,
			action,
			error_message,
			performed_by_user_id
		)
		VALUES
			($1, $2, $3, $4, $5, $6, $7);
	`, uuid.NewString(), workflowId, stepIDValue, targetType, action, errorMessage, adminId)
	if err != nil {
		return fmt.Errorf("error recording workflow payout admin action: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (a *AppDB) ClaimWorkflowStepPayoutAttempt(ctx context.Context, workflowId string, stepId string) (bool, error) {
	cmd, err := a.db.Exec(ctx, `
		UPDATE
			workflow_steps
		SET
			payout_in_progress = true,
			payout_error = NULL,
			payout_last_try_at = unix_now(),
			retry_requested_at = NULL,
			retry_requested_by = NULL,
			updated_at = unix_now()
		WHERE
			id = $1
		AND
			workflow_id = $2
		AND
			status = 'completed'
		AND
			bounty > 0
		AND
			payout_in_progress = false;
	`, stepId, workflowId)
	if err != nil {
		return false, fmt.Errorf("error claiming workflow step payout attempt: %s", err)
	}
	return cmd.RowsAffected() > 0, nil
}

func (a *AppDB) ClaimWorkflowManagerPayoutAttempt(ctx context.Context, workflowId string) (bool, error) {
	cmd, err := a.db.Exec(ctx, `
		UPDATE
			workflows
		SET
			manager_payout_in_progress = true,
			manager_payout_error = NULL,
			manager_payout_last_try_at = unix_now(),
			manager_retry_requested_at = NULL,
			manager_retry_requested_by = NULL,
			updated_at = unix_now()
		WHERE
			id = $1
		AND
			status = 'completed'
		AND
			manager_bounty > 0
		AND
			manager_improver_id IS NOT NULL
		AND
			manager_paid_out_at IS NULL
		AND
			manager_payout_in_progress = false;
	`, workflowId)
	if err != nil {
		return false, fmt.Errorf("error claiming workflow manager payout attempt: %s", err)
	}
	return cmd.RowsAffected() > 0, nil
}

func (a *AppDB) MarkWorkflowStepPayoutFailed(ctx context.Context, workflowId string, stepId string, errorMessage string) error {
	errorMessage = truncateWorkflowPayoutErrorMessage(errorMessage)
	cmd, err := a.db.Exec(ctx, `
		UPDATE
			workflow_steps
		SET
			payout_error = $3,
			payout_last_try_at = unix_now(),
			payout_in_progress = false,
			retry_requested_at = NULL,
			retry_requested_by = NULL,
			updated_at = unix_now()
		WHERE
			id = $1
		AND
			workflow_id = $2
		AND
			status = 'completed';
	`, stepId, workflowId, errorMessage)
	if err != nil {
		return fmt.Errorf("error recording workflow step payout failure: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("workflow step payout failure cannot be recorded")
	}
	return nil
}

func (a *AppDB) MarkWorkflowManagerPayoutFailed(ctx context.Context, workflowId string, errorMessage string) error {
	errorMessage = truncateWorkflowPayoutErrorMessage(errorMessage)
	cmd, err := a.db.Exec(ctx, `
		UPDATE
			workflows
		SET
			manager_payout_error = $2,
			manager_payout_last_try_at = unix_now(),
			manager_payout_in_progress = false,
			manager_retry_requested_at = NULL,
			manager_retry_requested_by = NULL,
			updated_at = unix_now()
		WHERE
			id = $1
		AND
			status = 'completed'
		AND
			manager_bounty > 0
		AND
			manager_improver_id IS NOT NULL
		AND
			manager_paid_out_at IS NULL;
	`, workflowId, errorMessage)
	if err != nil {
		return fmt.Errorf("error recording workflow manager payout failure: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("workflow manager payout failure cannot be recorded")
	}
	return nil
}

func (a *AppDB) MarkWorkflowStepPaidOut(ctx context.Context, workflowId string, stepId string) (bool, error) {
	cmd, err := a.db.Exec(ctx, `
		UPDATE
			workflow_steps
		SET
			status = 'paid_out',
			payout_error = NULL,
			payout_last_try_at = unix_now(),
			payout_in_progress = false,
			retry_requested_at = NULL,
			retry_requested_by = NULL,
			updated_at = unix_now()
		WHERE
			id = $1
		AND
			workflow_id = $2
		AND
			status = 'completed'
		AND
			(payout_in_progress = true OR bounty = 0);
	`, stepId, workflowId)
	if err != nil {
		return false, fmt.Errorf("error marking workflow step paid out: %s", err)
	}
	return cmd.RowsAffected() > 0, nil
}

func (a *AppDB) MarkWorkflowManagerPaidOut(ctx context.Context, workflowId string) (bool, error) {
	cmd, err := a.db.Exec(ctx, `
		UPDATE
			workflows
		SET
			manager_paid_out_at = unix_now(),
			manager_payout_error = NULL,
			manager_payout_last_try_at = unix_now(),
			manager_payout_in_progress = false,
			manager_retry_requested_at = NULL,
			manager_retry_requested_by = NULL,
			updated_at = unix_now()
		WHERE
			id = $1
		AND
			status IN ('completed', 'paid_out')
		AND
			manager_bounty > 0
		AND
			manager_improver_id IS NOT NULL
		AND
			manager_paid_out_at IS NULL
		AND
			manager_payout_in_progress = true;
	`, workflowId)
	if err != nil {
		return false, fmt.Errorf("error marking workflow manager paid out: %s", err)
	}
	return cmd.RowsAffected() > 0, nil
}

func (a *AppDB) FinalizeWorkflowPaidOutIfSettled(ctx context.Context, workflowId string) (bool, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var status string
	var managerBounty uint64
	var managerImproverID *string
	var managerPaidOutAt *int64
	var managerPayoutInProgress bool
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			manager_bounty,
			manager_improver_id,
			manager_paid_out_at,
			manager_payout_in_progress
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&status, &managerBounty, &managerImproverID, &managerPaidOutAt, &managerPayoutInProgress)
	if err != nil {
		return false, err
	}

	if status != "completed" && status != "paid_out" {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			workflow_steps
		SET
			status = 'paid_out',
			payout_error = NULL,
			payout_last_try_at = COALESCE(payout_last_try_at, unix_now()),
			payout_in_progress = false,
			retry_requested_at = NULL,
			retry_requested_by = NULL,
			updated_at = unix_now()
		WHERE
			workflow_id = $1
		AND
			status = 'completed'
		AND
			bounty = 0;
	`, workflowId)
	if err != nil {
		return false, fmt.Errorf("error auto-settling zero-bounty workflow steps: %s", err)
	}

	var pendingStepCount int
	err = tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		AND
			status <> 'paid_out';
	`, workflowId).Scan(&pendingStepCount)
	if err != nil {
		return false, fmt.Errorf("error checking pending workflow step payouts: %s", err)
	}
	if pendingStepCount > 0 {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}

	if managerBounty > 0 && managerImproverID != nil && (managerPaidOutAt == nil || managerPayoutInProgress) {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			workflows
		SET
			status = 'paid_out',
			is_start_blocked = false,
			blocked_by_workflow_id = NULL,
			updated_at = unix_now()
		WHERE
			id = $1;
	`, workflowId)
	if err != nil {
		return false, fmt.Errorf("error finalizing workflow paid_out status: %s", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			workflows
		SET
			is_start_blocked = false,
			blocked_by_workflow_id = NULL,
			status = CASE WHEN status = 'blocked' THEN 'approved' ELSE status END,
			updated_at = unix_now()
		WHERE
			status = 'blocked'
		AND
			blocked_by_workflow_id = $1;
	`, workflowId)
	if err != nil {
		return false, fmt.Errorf("error releasing blocked workflows after payout finalization: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (a *AppDB) RequestWorkflowStepPayoutRetry(ctx context.Context, workflowId string, stepId string, improverId string) error {
	cmd, err := a.db.Exec(ctx, `
		UPDATE
			workflow_steps
		SET
			retry_requested_at = unix_now(),
			retry_requested_by = $3,
			updated_at = unix_now()
		WHERE
			id = $1
		AND
			workflow_id = $2
		AND
			assigned_improver_id = $3
		AND
			status = 'completed'
		AND
			bounty > 0
		AND
			payout_in_progress = false
		AND
			COALESCE(NULLIF(TRIM(payout_error), ''), '') <> '';
	`, stepId, workflowId, improverId)
	if err != nil {
		return fmt.Errorf("error requesting workflow step payout retry: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("no failed step payout found for retry")
	}
	return nil
}

func (a *AppDB) RequestWorkflowManagerPayoutRetry(ctx context.Context, workflowId string, improverId string) error {
	cmd, err := a.db.Exec(ctx, `
		UPDATE
			workflows
		SET
			manager_retry_requested_at = unix_now(),
			manager_retry_requested_by = $2,
			updated_at = unix_now()
		WHERE
			id = $1
		AND
			status = 'completed'
		AND
			manager_bounty > 0
		AND
			manager_improver_id = $2
		AND
			manager_paid_out_at IS NULL
		AND
			manager_payout_in_progress = false
		AND
			COALESCE(NULLIF(TRIM(manager_payout_error), ''), '') <> '';
	`, workflowId, improverId)
	if err != nil {
		return fmt.Errorf("error requesting workflow manager payout retry: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("no failed manager payout found for retry")
	}
	return nil
}

func (a *AppDB) CountEligibleVoters(ctx context.Context) (int, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			users
		WHERE
			is_voter = true
		OR
			is_admin = true;
	`)
	var total int
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func quorumVotesRequired(totalVoters int) int {
	if totalVoters <= 0 {
		return 0
	}
	return (totalVoters + 1) / 2
}

func possibleBodyMajority(totalVoters int) int {
	if totalVoters <= 0 {
		return 0
	}
	return (totalVoters / 2) + 1
}

func (a *AppDB) GetWorkflowVotes(ctx context.Context, workflowId string) (*structs.WorkflowVotes, error) {
	return a.getWorkflowVotesInternal(ctx, workflowId, nil)
}

func (a *AppDB) GetWorkflowVotesForUser(ctx context.Context, workflowId string, userId string) (*structs.WorkflowVotes, error) {
	return a.getWorkflowVotesInternal(ctx, workflowId, &userId)
}

func (a *AppDB) getWorkflowVotesInternal(ctx context.Context, workflowId string, userId *string) (*structs.WorkflowVotes, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE decision = 'approve'),
			COUNT(*) FILTER (WHERE decision = 'deny')
		FROM
			workflow_votes
		WHERE
			workflow_id = $1;
	`, workflowId)

	votes := &structs.WorkflowVotes{}
	if err := row.Scan(&votes.Approve, &votes.Deny); err != nil {
		return nil, err
	}

	totalVoters, err := a.CountEligibleVoters(ctx)
	if err != nil {
		return nil, err
	}
	votes.TotalVoters = totalVoters
	votes.VotesCast = votes.Approve + votes.Deny
	votes.QuorumThreshold = quorumVotesRequired(totalVoters)
	votes.QuorumReached = votes.VotesCast >= votes.QuorumThreshold && totalVoters > 0

	row = a.db.QueryRow(ctx, `
		SELECT
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at,
			vote_decision
		FROM
			workflows
		WHERE
			id = $1;
	`, workflowId)
	if err := row.Scan(&votes.QuorumReachedAt, &votes.FinalizeAt, &votes.FinalizedAt, &votes.Decision); err != nil {
		return nil, err
	}

	if userId != nil {
		voteRow := a.db.QueryRow(ctx, `
			SELECT
				decision
			FROM
				workflow_votes
			WHERE
				workflow_id = $1
			AND
				voter_id = $2;
		`, workflowId, *userId)
		var decision string
		err := voteRow.Scan(&decision)
		if err == nil {
			votes.MyDecision = &decision
		} else if err != pgx.ErrNoRows {
			return nil, err
		}
	}

	return votes, nil
}

func (a *AppDB) RecordWorkflowVote(ctx context.Context, workflowId string, voterId string, decision string, comment string) (*structs.WorkflowVotes, error) {
	_, err := a.db.Exec(ctx, `
		INSERT INTO workflow_votes
			(workflow_id, voter_id, decision, comment)
		VALUES
			($1, $2, $3, $4)
		ON CONFLICT (workflow_id, voter_id)
		DO UPDATE SET
			decision = EXCLUDED.decision,
			comment = EXCLUDED.comment,
			updated_at = unix_now();
	`, workflowId, voterId, decision, comment)
	if err != nil {
		return nil, fmt.Errorf("error recording workflow vote: %s", err)
	}
	return a.GetWorkflowVotesForUser(ctx, workflowId, voterId)
}

func (a *AppDB) GetWorkflowForApproval(ctx context.Context, workflowId string) (*structs.Workflow, error) {
	return a.GetWorkflowByID(ctx, workflowId)
}

func (a *AppDB) ExpireStaleWorkflowProposals(ctx context.Context) ([]structs.WorkflowProposalExpiryNotice, error) {
	rows, err := a.db.Query(ctx, `
			WITH expired AS (
				UPDATE
				workflows w
			SET
				status = 'expired',
				vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, unix_now()),
				vote_finalize_at = COALESCE(vote_finalize_at, unix_now()),
				vote_finalized_at = COALESCE(vote_finalized_at, unix_now()),
				updated_at = unix_now()
			WHERE
				w.status = 'pending'
				AND
					w.created_at <= (unix_now() - (14 * 24 * 60 * 60))
				RETURNING
					w.id,
					w.series_id,
					w.workflow_state_id,
					w.proposer_id
			)
			SELECT
				e.id,
				COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')),
				e.proposer_id,
				COALESCE(NULLIF(TRIM(p.email), ''), COALESCE(u.contact_email, ''))
			FROM
				expired e
			LEFT JOIN
				workflow_states st
			ON
				st.id = e.workflow_state_id
			LEFT JOIN
				workflow_series s
			ON
				s.id = e.series_id
			LEFT JOIN
				proposers p
			ON
				p.user_id = e.proposer_id
		LEFT JOIN
			users u
		ON
			u.id = e.proposer_id;
	`)
	if err != nil {
		return nil, fmt.Errorf("error expiring stale workflow proposals: %s", err)
	}
	defer rows.Close()

	notifications := []structs.WorkflowProposalExpiryNotice{}
	for rows.Next() {
		notice := structs.WorkflowProposalExpiryNotice{}
		if err := rows.Scan(&notice.WorkflowId, &notice.WorkflowTitle, &notice.ProposerUserId, &notice.ProposerEmail); err != nil {
			return nil, fmt.Errorf("error scanning expired workflow notice: %s", err)
		}
		notice.ProposerEmail = strings.TrimSpace(notice.ProposerEmail)
		notifications = append(notifications, notice)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating expired workflow notices: %s", err)
	}

	return notifications, nil
}

func (a *AppDB) GetWorkflowProposalOutcomeNotification(ctx context.Context, workflowId string) (*structs.WorkflowProposalOutcomeNotification, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			w.id,
			COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')),
			CASE
				WHEN w.status IN ('approved', 'blocked') THEN 'approved'
				WHEN w.status = 'rejected' THEN 'rejected'
				ELSE ''
			END,
			w.proposer_id,
			COALESCE(NULLIF(TRIM(p.email), ''), COALESCE(u.contact_email, ''))
		FROM
			workflows w
		LEFT JOIN
			workflow_states st
		ON
			st.id = w.workflow_state_id
		LEFT JOIN
			workflow_series s
		ON
			s.id = w.series_id
		LEFT JOIN
			proposers p
		ON
			p.user_id = w.proposer_id
		LEFT JOIN
			users u
		ON
			u.id = w.proposer_id
		WHERE
			w.id = $1;
	`, workflowId)

	notification := structs.WorkflowProposalOutcomeNotification{}
	if err := row.Scan(
		&notification.WorkflowId,
		&notification.WorkflowTitle,
		&notification.Decision,
		&notification.ProposerUserId,
		&notification.ProposerEmail,
	); err != nil {
		return nil, err
	}

	notification.ProposerEmail = strings.TrimSpace(notification.ProposerEmail)
	if notification.Decision == "" {
		return nil, fmt.Errorf("workflow outcome is not finalized")
	}
	return &notification, nil
}

func (a *AppDB) EvaluateWorkflowVoteState(ctx context.Context, workflowId string) (*structs.Workflow, error) {
	return a.EvaluateWorkflowVoteStateWithApproval(ctx, workflowId, true)
}

func (a *AppDB) EvaluateWorkflowVoteStateWithApproval(ctx context.Context, workflowId string, allowApproval bool) (*structs.Workflow, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	type workflowVoteState struct {
		Status          string
		IsStartBlocked  bool
		QuorumReachedAt *int64
		FinalizeAt      *int64
		FinalizedAt     *int64
	}

	state := workflowVoteState{}
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			is_start_blocked,
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(
		&state.Status,
		&state.IsStartBlocked,
		&state.QuorumReachedAt,
		&state.FinalizeAt,
		&state.FinalizedAt,
	)
	if err != nil {
		return nil, err
	}

	if state.Status != "pending" {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return a.GetWorkflowByID(ctx, workflowId)
	}

	totalVoters, err := countEligibleVotersTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	approveCount, denyCount, err := countWorkflowVotesTx(ctx, tx, workflowId)
	if err != nil {
		return nil, err
	}
	votesCast := approveCount + denyCount
	quorumThreshold := quorumVotesRequired(totalVoters)
	quorumReached := totalVoters > 0 && votesCast >= quorumThreshold
	nowUnix := time.Now().UTC().Unix()

	if quorumReached && state.QuorumReachedAt == nil {
		quorumReachedAt := nowUnix
		finalizeAt := nowUnix + int64((24 * time.Hour).Seconds())
		_, err = tx.Exec(ctx, `
			UPDATE
				workflows
			SET
				vote_quorum_reached_at = $2,
				vote_finalize_at = $3,
				updated_at = unix_now()
			WHERE
				id = $1;
		`, workflowId, quorumReachedAt, finalizeAt)
		if err != nil {
			return nil, fmt.Errorf("error setting vote quorum countdown: %s", err)
		}
		state.QuorumReachedAt = &quorumReachedAt
		state.FinalizeAt = &finalizeAt
	}

	majorityThreshold := possibleBodyMajority(totalVoters)
	outcome := ""
	if totalVoters > 0 && approveCount >= majorityThreshold {
		outcome = "approve"
	} else if totalVoters > 0 && denyCount >= majorityThreshold {
		outcome = "deny"
	} else if quorumReached && state.FinalizeAt != nil && nowUnix >= *state.FinalizeAt {
		if approveCount > denyCount {
			outcome = "approve"
		} else {
			outcome = "deny"
		}
	}

	if outcome == "approve" && !allowApproval {
		outcome = ""
	}

	if outcome == "approve" {
		if err := finalizeWorkflowApprovalTx(ctx, tx, workflowId, state.IsStartBlocked, nil, "approve"); err != nil {
			return nil, err
		}
	}
	if outcome == "deny" {
		if err := finalizeWorkflowRejectionTx(ctx, tx, workflowId, "deny", nil); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetWorkflowByID(ctx, workflowId)
}

func (a *AppDB) ApproveWorkflow(ctx context.Context, workflowId string, approverId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var isStartBlocked bool
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			is_start_blocked
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&status, &isStartBlocked)
	if err != nil {
		return err
	}

	if status == "approved" || status == "blocked" || status == "in_progress" || status == "completed" || status == "paid_out" {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	if status != "pending" {
		return fmt.Errorf("workflow is not pending")
	}

	if err := finalizeWorkflowApprovalTx(ctx, tx, workflowId, isStartBlocked, &approverId, "approve"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (a *AppDB) ForceApproveWorkflowAsAdmin(ctx context.Context, workflowId string, adminId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var isStartBlocked bool
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			is_start_blocked
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&status, &isStartBlocked)
	if err != nil {
		return err
	}

	if status == "approved" || status == "blocked" || status == "in_progress" || status == "completed" || status == "paid_out" {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	if status != "pending" {
		return fmt.Errorf("workflow is not pending")
	}

	if err := finalizeWorkflowApprovalTx(ctx, tx, workflowId, isStartBlocked, &adminId, "admin_approve"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (a *AppDB) RejectWorkflow(ctx context.Context, workflowId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&status)
	if err != nil {
		return err
	}

	if status == "rejected" {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	if status != "pending" {
		return fmt.Errorf("approved or active workflows cannot be rejected")
	}

	if err := finalizeWorkflowRejectionTx(ctx, tx, workflowId, "deny", nil); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func finalizeWorkflowApprovalTx(
	ctx context.Context,
	tx pgx.Tx,
	workflowId string,
	_ bool,
	actorUserId *string,
	decision string,
) error {
	nextStatus := "approved"

	_, err := tx.Exec(ctx, `
		UPDATE
			workflows
		SET
			status = $2,
			is_start_blocked = false,
			blocked_by_workflow_id = NULL,
			approved_at = COALESCE(approved_at, unix_now()),
			approved_by_user_id = COALESCE($3, approved_by_user_id),
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, unix_now()),
			vote_finalize_at = COALESCE(vote_finalize_at, unix_now()),
			vote_finalized_at = COALESCE(vote_finalized_at, unix_now()),
			vote_finalized_by_user_id = COALESCE($4, vote_finalized_by_user_id),
			vote_decision = $5,
			updated_at = unix_now()
		WHERE
			id = $1;
	`, workflowId, nextStatus, actorUserId, actorUserId, decision)
	if err != nil {
		return fmt.Errorf("error approving workflow: %s", err)
	}

	// If the start time has already elapsed by approval time, unlock step 1 immediately.
	_, err = tx.Exec(ctx, `
		UPDATE
			workflow_steps ws
		SET
			status = 'available',
			updated_at = unix_now()
		FROM
			workflows w
		WHERE
			ws.workflow_id = w.id
		AND
			w.id = $1
		AND
			ws.step_order = 1
		AND
			ws.status = 'locked'
		AND
			w.status IN ('approved', 'in_progress')
		AND
			w.start_at <= unix_now();
	`, workflowId)
	if err != nil {
		return fmt.Errorf("error unlocking initial workflow step on approval: %s", err)
	}

	return nil
}

func finalizeWorkflowRejectionTx(
	ctx context.Context,
	tx pgx.Tx,
	workflowId string,
	decision string,
	actorUserId *string,
) error {
	_, err := tx.Exec(ctx, `
		UPDATE
			workflows
		SET
			status = 'rejected',
			budget_weekly_deducted = 0,
			budget_one_time_deducted = 0,
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, unix_now()),
			vote_finalize_at = COALESCE(vote_finalize_at, unix_now()),
			vote_finalized_at = COALESCE(vote_finalized_at, unix_now()),
			vote_finalized_by_user_id = COALESCE($3, vote_finalized_by_user_id),
			vote_decision = $2,
			updated_at = unix_now()
		WHERE
			id = $1;
	`, workflowId, decision, actorUserId)
	if err != nil {
		return fmt.Errorf("error updating rejected workflow: %s", err)
	}

	return nil
}

func countEligibleVotersTx(ctx context.Context, tx pgx.Tx) (int, error) {
	row := tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			users
		WHERE
			is_voter = true
		OR
			is_admin = true;
	`)
	var total int
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func countWorkflowVotesTx(ctx context.Context, tx pgx.Tx, workflowId string) (int, int, error) {
	row := tx.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE decision = 'approve'),
			COUNT(*) FILTER (WHERE decision = 'deny')
		FROM
			workflow_votes
		WHERE
			workflow_id = $1;
	`, workflowId)
	var approve int
	var deny int
	if err := row.Scan(&approve, &deny); err != nil {
		return 0, 0, err
	}
	return approve, deny, nil
}

func (a *AppDB) GetWorkflowByIDAndProposer(ctx context.Context, workflowId string, proposerId string) (*structs.Workflow, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id
		FROM
			workflows
		WHERE
			id = $1
		AND
			proposer_id = $2;
	`, workflowId, proposerId)

	var id string
	err := row.Scan(&id)
	if err != nil {
		return nil, err
	}

	return a.GetWorkflowByID(ctx, workflowId)
}

func (a *AppDB) GetVoterWorkflows(ctx context.Context, voterId string) ([]*structs.Workflow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			workflows
		WHERE
			status = 'pending'
		ORDER BY
			created_at DESC
		LIMIT 200;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying voter workflows: %s", err)
	}
	defer rows.Close()

	workflowIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning voter workflow id: %s", err)
		}
		workflowIDs = append(workflowIDs, id)
	}

	workflows := make([]*structs.Workflow, 0, len(workflowIDs))
	for _, workflowId := range workflowIDs {
		workflow, err := a.GetWorkflowByID(ctx, workflowId)
		if err != nil {
			return nil, err
		}
		votes, err := a.GetWorkflowVotesForUser(ctx, workflowId, voterId)
		if err != nil {
			return nil, err
		}
		workflow.Votes = *votes
		workflows = append(workflows, workflow)
	}

	return workflows, nil
}

func (a *AppDB) GetActiveWorkflows(ctx context.Context) ([]*structs.ActiveWorkflowListItem, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			w.id,
			w.series_id,
			w.workflow_state_id,
			w.proposer_id,
			COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')),
			COALESCE(st.description, s.description, ''),
			COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')),
			COALESCE(st.recurrence_end_at, s.recurrence_end_at),
			w.start_at,
			w.status,
			w.is_start_blocked,
			w.blocked_by_workflow_id,
			w.total_bounty,
			w.weekly_bounty_requirement,
			w.created_at,
			w.updated_at,
			w.vote_decision,
			w.approved_at
		FROM
			workflows w
		LEFT JOIN
			workflow_states st
		ON
			st.id = w.workflow_state_id
		LEFT JOIN
			workflow_series s
		ON
			s.id = w.series_id
		WHERE
			w.status IN ('approved', 'blocked', 'in_progress', 'completed')
		ORDER BY
			w.start_at ASC,
			w.created_at DESC
		LIMIT 500;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying active workflows: %s", err)
	}
	defer rows.Close()

	results := []*structs.ActiveWorkflowListItem{}
	for rows.Next() {
		workflow := &structs.ActiveWorkflowListItem{}
		if err := rows.Scan(
			&workflow.Id,
			&workflow.SeriesId,
			&workflow.WorkflowStateId,
			&workflow.ProposerId,
			&workflow.Title,
			&workflow.Description,
			&workflow.Recurrence,
			&workflow.RecurrenceEndAt,
			&workflow.StartAt,
			&workflow.Status,
			&workflow.IsStartBlocked,
			&workflow.BlockedByWorkflowId,
			&workflow.TotalBounty,
			&workflow.WeeklyBountyRequirement,
			&workflow.CreatedAt,
			&workflow.UpdatedAt,
			&workflow.VoteDecision,
			&workflow.ApprovedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning active workflow: %s", err)
		}
		results = append(results, workflow)
	}

	return results, nil
}

func (a *AppDB) GetAdminWorkflows(ctx context.Context, search string, page, count int, includeArchived bool) (*structs.AdminWorkflowListResponse, error) {
	if page < 0 {
		page = 0
	}
	if count <= 0 {
		count = 20
	}
	if count > 200 {
		count = 200
	}

	trimmedSearch := strings.TrimSpace(search)
	likeSearch := "%" + trimmedSearch + "%"
	offset := page * count

	baseCTE := `
			WITH assigned AS (
			SELECT
				ws.workflow_id,
				ARRAY_REMOVE(
					ARRAY_AGG(
						DISTINCT NULLIF(TRIM(COALESCE(i.email, u.contact_email, '')), '')
					),
					NULL
				) AS assigned_improver_emails
			FROM
				workflow_steps ws
			LEFT JOIN
				improvers i
			ON
				i.user_id = ws.assigned_improver_id
			LEFT JOIN
				users u
			ON
				u.id = ws.assigned_improver_id
			WHERE
				ws.assigned_improver_id IS NOT NULL
			GROUP BY
				ws.workflow_id
		),
			base AS (
				SELECT
					w.id,
					w.series_id,
					COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(s.title), ''), '')) AS title,
					COALESCE(st.description, s.description, '') AS description,
					COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time')) AS recurrence,
					w.status,
					w.start_at,
					w.created_at,
					w.updated_at,
					COALESCE(a.assigned_improver_emails, ARRAY[]::text[]) AS assigned_improver_emails
				FROM
					workflows w
				LEFT JOIN
					workflow_states st
				ON
					st.id = w.workflow_state_id
				LEFT JOIN
					workflow_series s
				ON
					s.id = w.series_id
				LEFT JOIN
					assigned a
				ON
				a.workflow_id = w.id
			WHERE
				(
					w.status IN ('approved', 'blocked', 'in_progress', 'completed', 'paid_out', 'failed', 'skipped')
				OR
					($1 AND w.status = 'deleted')
				)
		)
	`

	whereClause := `
		(
			$2 = ''
			OR base.title ILIKE $3
			OR EXISTS (
				SELECT
					1
				FROM
					UNNEST(base.assigned_improver_emails) AS email
				WHERE
					email ILIKE $3
			)
		)
	`

	var total int
	countQuery := baseCTE + fmt.Sprintf(`
		,
		matching_series AS (
			SELECT DISTINCT
				base.series_id
			FROM
				base
			WHERE
				%s
		)
		SELECT
			COUNT(*)
		FROM
			matching_series;
	`, whereClause)
	if err := a.db.QueryRow(ctx, countQuery, includeArchived, trimmedSearch, likeSearch).Scan(&total); err != nil {
		return nil, fmt.Errorf("error counting admin workflows: %s", err)
	}

	listQuery := baseCTE + fmt.Sprintf(`
		,
		matching_series AS (
			SELECT DISTINCT
				base.series_id
			FROM
				base
			WHERE
				%s
		),
		series_ranked AS (
			SELECT
				b.series_id,
				MAX(b.start_at) AS latest_start_at,
				MAX(b.created_at) AS latest_created_at
			FROM
				base b
			INNER JOIN
				matching_series ms
			ON
				ms.series_id = b.series_id
			GROUP BY
				b.series_id
		),
		selected_series AS (
			SELECT
				sr.series_id,
				sr.latest_start_at,
				sr.latest_created_at
			FROM
				series_ranked sr
			ORDER BY
				sr.latest_start_at DESC,
				sr.latest_created_at DESC,
				sr.series_id DESC
			LIMIT $4
			OFFSET $5
		)
		SELECT
			b.id,
			b.series_id,
			b.title,
			b.description,
			b.recurrence,
			b.status,
			b.start_at,
			b.created_at,
			b.updated_at,
			b.assigned_improver_emails
		FROM
			base b
		INNER JOIN
			selected_series ss
		ON
			ss.series_id = b.series_id
		ORDER BY
			ss.latest_start_at DESC,
			ss.latest_created_at DESC,
			ss.series_id DESC,
			b.start_at ASC,
			b.created_at ASC,
			b.id ASC;
	`, whereClause)

	rows, err := a.db.Query(ctx, listQuery, includeArchived, trimmedSearch, likeSearch, count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying admin workflows: %s", err)
	}
	defer rows.Close()

	items := []*structs.AdminWorkflowListItem{}
	for rows.Next() {
		item := &structs.AdminWorkflowListItem{}
		if err := rows.Scan(
			&item.Id,
			&item.SeriesId,
			&item.Title,
			&item.Description,
			&item.Recurrence,
			&item.Status,
			&item.StartAt,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.AssignedImproverEmails,
		); err != nil {
			return nil, fmt.Errorf("error scanning admin workflow list item: %s", err)
		}
		if item.AssignedImproverEmails == nil {
			item.AssignedImproverEmails = []string{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating admin workflows: %s", err)
	}

	return &structs.AdminWorkflowListResponse{
		Items: items,
		Total: total,
		Page:  page,
		Count: count,
	}, nil
}

func (a *AppDB) GetWorkflowSeriesClaimants(ctx context.Context, seriesId string) ([]*structs.WorkflowSeriesClaimant, error) {
	seriesId = strings.TrimSpace(seriesId)
	if seriesId == "" {
		return nil, fmt.Errorf("series_id is required")
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			ws.assigned_improver_id,
			COALESCE(NULLIF(TRIM(i.email), ''), NULLIF(TRIM(u.contact_email), ''), ''),
			COALESCE(
				NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''),
				NULLIF(TRIM(u.contact_name), ''),
				ws.assigned_improver_id
			),
			COUNT(*)
		FROM
			workflow_steps ws
		JOIN
			workflows w
		ON
			w.id = ws.workflow_id
		LEFT JOIN
			improvers i
		ON
			i.user_id = ws.assigned_improver_id
		LEFT JOIN
			users u
		ON
			u.id = ws.assigned_improver_id
		WHERE
			w.series_id = $1
		AND
			ws.assigned_improver_id IS NOT NULL
		AND
			w.status IN ('approved', 'blocked', 'in_progress', 'completed', 'paid_out')
		GROUP BY
			ws.assigned_improver_id,
			i.email,
			u.contact_email,
			i.first_name,
			i.last_name,
			u.contact_name
		ORDER BY
			COALESCE(NULLIF(TRIM(i.email), ''), NULLIF(TRIM(u.contact_email), ''), ws.assigned_improver_id) ASC;
	`, seriesId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow series claimants: %s", err)
	}
	defer rows.Close()

	claimants := []*structs.WorkflowSeriesClaimant{}
	for rows.Next() {
		claimant := &structs.WorkflowSeriesClaimant{}
		if err := rows.Scan(
			&claimant.UserId,
			&claimant.Email,
			&claimant.Name,
			&claimant.ClaimCount,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow series claimant: %s", err)
		}
		claimants = append(claimants, claimant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating workflow series claimants: %s", err)
	}
	return claimants, nil
}

func (a *AppDB) UnclaimImproverWorkflowSeriesStep(
	ctx context.Context,
	improverId string,
	seriesId string,
	stepOrder int,
) (*structs.ImproverWorkflowSeriesUnclaimResult, error) {
	improverId = strings.TrimSpace(improverId)
	seriesId = strings.TrimSpace(seriesId)
	if improverId == "" {
		return nil, fmt.Errorf("improver_id is required")
	}
	if seriesId == "" {
		return nil, fmt.Errorf("series_id is required")
	}
	if stepOrder <= 0 {
		return nil, fmt.Errorf("step_order must be greater than zero")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var targetedCount int
	err = tx.QueryRow(ctx, `
			SELECT
				COUNT(*)
			FROM
				workflow_steps ws
			JOIN
				workflows w
			ON
				w.id = ws.workflow_id
			JOIN
				workflow_series sr
			ON
				sr.id = w.series_id
			WHERE
				w.series_id = $1
			AND
				COALESCE(NULLIF(TRIM(sr.recurrence), ''), 'one_time') <> 'one_time'
			AND
				ws.step_order = $2
		AND
			ws.assigned_improver_id = $3
		AND
			w.status IN ('approved', 'blocked', 'in_progress')
		AND
			ws.status IN ('locked', 'available', 'in_progress');
	`, seriesId, stepOrder, improverId).Scan(&targetedCount)
	if err != nil {
		return nil, fmt.Errorf("error counting improver workflow series claims: %s", err)
	}
	if targetedCount == 0 {
		return nil, fmt.Errorf("no claimed recurring workpiece found for this series and step")
	}

	var releasedCount int
	err = tx.QueryRow(ctx, `
			WITH releasable AS (
				SELECT
					ws.id
				FROM
					workflow_steps ws
				JOIN
					workflows w
				ON
					w.id = ws.workflow_id
				JOIN
					workflow_series sr
				ON
					sr.id = w.series_id
				WHERE
					w.series_id = $1
				AND
					COALESCE(NULLIF(TRIM(sr.recurrence), ''), 'one_time') <> 'one_time'
				AND
					ws.step_order = $2
			AND
				ws.assigned_improver_id = $3
			AND
				w.status IN ('approved', 'blocked', 'in_progress')
			AND
				ws.status IN ('locked', 'available')
			FOR UPDATE
		),
		released AS (
			UPDATE
				workflow_steps ws
			SET
				assigned_improver_id = NULL,
				updated_at = unix_now()
			WHERE
				ws.id IN (SELECT id FROM releasable)
			RETURNING
				ws.id
		),
		cleared_notifications AS (
			DELETE FROM workflow_step_notifications n
			WHERE
				n.step_id IN (SELECT id FROM released)
			AND
				n.user_id = $3
			AND
				n.notification_type = 'step_available'
		)
		SELECT
			COUNT(*)
		FROM
			released;
	`, seriesId, stepOrder, improverId).Scan(&releasedCount)
	if err != nil {
		return nil, fmt.Errorf("error releasing workflow series claims: %s", err)
	}
	if releasedCount == 0 {
		return nil, fmt.Errorf("no claimable recurring assignments found to release")
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM workflow_series_step_claims
		WHERE
			series_id = $1
		AND
			step_order = $2
		AND
			improver_id = $3;
	`, seriesId, stepOrder, improverId)
	if err != nil {
		return nil, fmt.Errorf("error removing workflow series claim mapping: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	result := &structs.ImproverWorkflowSeriesUnclaimResult{
		SeriesId:      seriesId,
		StepOrder:     stepOrder,
		ReleasedCount: releasedCount,
		SkippedCount:  targetedCount - releasedCount,
	}
	if result.SkippedCount < 0 {
		result.SkippedCount = 0
	}
	return result, nil
}

func (a *AppDB) AdminRevokeWorkflowSeriesImproverClaims(
	ctx context.Context,
	seriesId string,
	improverUserId string,
) (*structs.WorkflowSeriesClaimRevokeResult, error) {
	seriesId = strings.TrimSpace(seriesId)
	improverUserId = strings.TrimSpace(improverUserId)
	if seriesId == "" {
		return nil, fmt.Errorf("series_id is required")
	}
	if improverUserId == "" {
		return nil, fmt.Errorf("improver_user_id is required")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var targetedCount int
	err = tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_steps ws
		JOIN
			workflows w
		ON
			w.id = ws.workflow_id
		WHERE
			w.series_id = $1
		AND
			ws.assigned_improver_id = $2
		AND
			w.status IN ('approved', 'blocked', 'in_progress')
		AND
			ws.status IN ('locked', 'available', 'in_progress');
	`, seriesId, improverUserId).Scan(&targetedCount)
	if err != nil {
		return nil, fmt.Errorf("error counting admin workflow claim targets: %s", err)
	}
	if targetedCount == 0 {
		return nil, fmt.Errorf("no claimed workflow assignments found for selected improver in this series")
	}

	var releasedCount int
	err = tx.QueryRow(ctx, `
		WITH releasable AS (
			SELECT
				ws.id
			FROM
				workflow_steps ws
			JOIN
				workflows w
			ON
				w.id = ws.workflow_id
			WHERE
				w.series_id = $1
			AND
				ws.assigned_improver_id = $2
			AND
				w.status IN ('approved', 'blocked', 'in_progress')
			AND
				ws.status IN ('locked', 'available')
			FOR UPDATE
		),
		released AS (
			UPDATE
				workflow_steps ws
			SET
				assigned_improver_id = NULL,
				updated_at = unix_now()
			WHERE
				ws.id IN (SELECT id FROM releasable)
			RETURNING
				ws.id
		),
		cleared_notifications AS (
			DELETE FROM workflow_step_notifications n
			WHERE
				n.step_id IN (SELECT id FROM released)
			AND
				n.user_id = $2
			AND
				n.notification_type = 'step_available'
		)
		SELECT
			COUNT(*)
		FROM
			released;
	`, seriesId, improverUserId).Scan(&releasedCount)
	if err != nil {
		return nil, fmt.Errorf("error revoking workflow claims from admin action: %s", err)
	}
	if releasedCount == 0 {
		return nil, fmt.Errorf("no claimable assignments found to revoke")
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM workflow_series_step_claims
		WHERE
			series_id = $1
		AND
			improver_id = $2;
	`, seriesId, improverUserId)
	if err != nil {
		return nil, fmt.Errorf("error clearing workflow series claim mappings from admin revocation: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	result := &structs.WorkflowSeriesClaimRevokeResult{
		SeriesId:       seriesId,
		ImproverUserId: improverUserId,
		ReleasedCount:  releasedCount,
		SkippedCount:   targetedCount - releasedCount,
	}
	if result.SkippedCount < 0 {
		result.SkippedCount = 0
	}
	return result, nil
}

func (a *AppDB) CreateWorkflowEditProposal(
	ctx context.Context,
	requesterId string,
	targetWorkflowID string,
	req *structs.WorkflowEditProposalCreateRequest,
) (*structs.WorkflowEditProposal, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	targetWorkflowID = strings.TrimSpace(targetWorkflowID)
	if targetWorkflowID == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}

	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 2000 {
		return nil, fmt.Errorf("reason is too long")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var seriesID string
	var proposerID string
	var targetWorkflowStatus string
	var seriesRecurrence string
	var seriesRecurrenceEndAt *int64
	err = tx.QueryRow(ctx, `
		SELECT
			w.series_id,
			s.proposer_id,
			w.status,
			s.recurrence,
			s.recurrence_end_at
		FROM
			workflows w
		JOIN
			workflow_series s
		ON
			s.id = w.series_id
		WHERE
			w.id = $1
		FOR UPDATE OF w, s;
	`, targetWorkflowID).Scan(&seriesID, &proposerID, &targetWorkflowStatus, &seriesRecurrence, &seriesRecurrenceEndAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("workflow not found")
		}
		return nil, fmt.Errorf("error loading workflow series for edit proposal: %s", err)
	}

	if strings.TrimSpace(proposerID) != strings.TrimSpace(requesterId) {
		return nil, fmt.Errorf("only the original proposer can propose workflow edits")
	}
	switch targetWorkflowStatus {
	case "approved", "blocked", "in_progress", "completed", "paid_out", "failed", "skipped":
	default:
		return nil, fmt.Errorf("workflow edits can only be proposed for active or finalized workflows")
	}

	var pendingCount int
	if err := tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_edit_proposals
		WHERE
			series_id = $1
		AND
			status = 'pending';
	`, seriesID).Scan(&pendingCount); err != nil {
		return nil, fmt.Errorf("error checking pending workflow edit proposals: %s", err)
	}
	if pendingCount > 0 {
		return nil, fmt.Errorf("a pending workflow edit vote already exists for this series")
	}

	validCredentialTypes, err := a.getValidCredentialTypeSet(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading credential types: %s", err)
	}

	var recurrenceEndAt *time.Time
	if seriesRecurrenceEndAt != nil {
		endAt := time.Unix(*seriesRecurrenceEndAt, 0).UTC()
		recurrenceEndAt = &endAt
	}

	definition, err := normalizeWorkflowDefinitionData(
		req.Title,
		req.Description,
		seriesRecurrence,
		recurrenceEndAt,
		req.Supervisor,
		req.SupervisorDataFields,
		req.Roles,
		req.Steps,
		validCredentialTypes,
	)
	if err != nil {
		return nil, err
	}

	if definition.SupervisorUserId != nil {
		var isSupervisor bool
		var supervisorStatus string
		err = tx.QueryRow(ctx, `
			SELECT
				u.is_supervisor,
				COALESCE(
					(
						SELECT
							s.status
						FROM
							supervisors s
						WHERE
							s.user_id = u.id
					),
					''
				)
			FROM
				users u
			WHERE
				u.id = $1
			FOR UPDATE;
		`, *definition.SupervisorUserId).Scan(&isSupervisor, &supervisorStatus)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, fmt.Errorf("workflow supervisor user not found")
			}
			return nil, fmt.Errorf("error validating workflow supervisor: %s", err)
		}
		if !isSupervisor || strings.TrimSpace(supervisorStatus) != "approved" {
			return nil, fmt.Errorf("workflow supervisor must be approved")
		}
	}

	proposedByID := requesterId
	sourceWorkflowID := targetWorkflowID
	proposedStateID, err := upsertWorkflowStateVersionTx(ctx, tx, seriesID, proposerID, definition, &sourceWorkflowID, &proposedByID)
	if err != nil {
		return nil, err
	}

	proposalID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO workflow_edit_proposals(
			id,
			series_id,
			target_workflow_id,
			proposed_state_id,
			requested_by_user_id,
			reason
		)
		VALUES
			($1, $2, $3, $4, $5, $6);
	`, proposalID, seriesID, targetWorkflowID, proposedStateID, requesterId, reason)
	if err != nil {
		return nil, fmt.Errorf("error creating workflow edit proposal: %s", err)
	}

	autoApproveWithoutVote := definition.TotalBounty == 0 && definition.SupervisorUserId != nil && *definition.SupervisorUserId == proposerID
	if autoApproveWithoutVote {
		if err := finalizeWorkflowEditApprovalTx(ctx, tx, proposalID, seriesID, proposedStateID, nil, "approve"); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a.GetWorkflowEditProposalByIDForUser(ctx, proposalID, nil)
}

func (a *AppDB) GetWorkflowEditProposalByIDForUser(ctx context.Context, proposalID string, voterID *string) (*structs.WorkflowEditProposal, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			p.id,
			p.series_id,
			p.target_workflow_id,
			p.proposed_state_id,
			p.requested_by_user_id,
			p.reason,
			p.status,
			p.vote_quorum_reached_at,
			p.vote_finalize_at,
			p.vote_finalized_at,
			p.vote_finalized_by_user_id,
			p.vote_decision,
			p.created_at,
			p.updated_at,
			COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(sr.title), ''), '')),
			COALESCE(st.description, sr.description, ''),
			COALESCE(tw.start_at, 0),
			COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(sr.recurrence), ''), 'one_time')),
			COALESCE(st.recurrence_end_at, sr.recurrence_end_at),
			st.supervisor_user_id,
			COALESCE(st.supervisor_bounty, 0),
			COALESCE(st.roles_json, '[]'::jsonb),
			COALESCE(st.steps_json, '[]'::jsonb)
		FROM
			workflow_edit_proposals p
		LEFT JOIN
			workflows tw
		ON
			tw.id = p.target_workflow_id
		LEFT JOIN
			workflow_series sr
		ON
			sr.id = p.series_id
		LEFT JOIN
			workflow_states st
		ON
			st.id = p.proposed_state_id
		WHERE
			p.id = $1;
	`, proposalID)

	proposal := &structs.WorkflowEditProposal{}
	var rolesJSON []byte
	var stepsJSON []byte
	if err := row.Scan(
		&proposal.Id,
		&proposal.SeriesId,
		&proposal.TargetWorkflowId,
		&proposal.ProposedStateId,
		&proposal.RequestedByUserId,
		&proposal.Reason,
		&proposal.Status,
		&proposal.VoteQuorumReachedAt,
		&proposal.VoteFinalizeAt,
		&proposal.VoteFinalizedAt,
		&proposal.VoteFinalizedBy,
		&proposal.VoteDecision,
		&proposal.CreatedAt,
		&proposal.UpdatedAt,
		&proposal.WorkflowTitle,
		&proposal.WorkflowDescription,
		&proposal.WorkflowStartAt,
		&proposal.Recurrence,
		&proposal.RecurrenceEndAt,
		&proposal.SupervisorUserId,
		&proposal.SupervisorBounty,
		&rolesJSON,
		&stepsJSON,
	); err != nil {
		return nil, err
	}

	proposal.SupervisorRequired = proposal.SupervisorUserId != nil && strings.TrimSpace(*proposal.SupervisorUserId) != ""
	roles := []structs.WorkflowRoleCreateInput{}
	if len(rolesJSON) > 0 {
		if err := json.Unmarshal(rolesJSON, &roles); err != nil {
			return nil, fmt.Errorf("error decoding workflow edit proposal roles: %s", err)
		}
	}
	steps := []structs.WorkflowStepCreateInput{}
	if len(stepsJSON) > 0 {
		if err := json.Unmarshal(stepsJSON, &steps); err != nil {
			return nil, fmt.Errorf("error decoding workflow edit proposal steps: %s", err)
		}
	}
	proposal.Roles = roles
	proposal.Steps = sanitizeWorkflowEditProposalPreviewSteps(steps)
	totalBounty := proposal.SupervisorBounty
	for _, step := range steps {
		totalBounty += step.Bounty
	}
	proposal.TotalBounty = totalBounty
	proposal.WeeklyRequirement = weeklyBountyRequirement(totalBounty, proposal.Recurrence)

	votes, err := a.getWorkflowEditVotesInternal(ctx, proposalID, voterID)
	if err != nil {
		return nil, err
	}
	proposal.Votes = *votes
	return proposal, nil
}

func sanitizeWorkflowEditProposalPreviewSteps(steps []structs.WorkflowStepCreateInput) []structs.WorkflowStepCreateInput {
	if len(steps) == 0 {
		return []structs.WorkflowStepCreateInput{}
	}

	sanitized := make([]structs.WorkflowStepCreateInput, len(steps))
	for stepIdx, step := range steps {
		sanitizedStep := step
		if len(step.WorkItems) > 0 {
			sanitizedStep.WorkItems = make([]structs.WorkflowWorkItemCreateInput, len(step.WorkItems))
			for itemIdx, item := range step.WorkItems {
				sanitizedItem := item
				if len(item.DropdownOptions) > 0 {
					sanitizedItem.DropdownOptions = make([]structs.WorkflowDropdownOptionCreateInput, len(item.DropdownOptions))
					for optionIdx, option := range item.DropdownOptions {
						sanitizedOption := option
						notifyEmailCount := sanitizedOption.NotifyEmailCount
						if len(sanitizedOption.NotifyEmails) > notifyEmailCount {
							notifyEmailCount = len(sanitizedOption.NotifyEmails)
						}
						sanitizedOption.NotifyEmailCount = notifyEmailCount
						sanitizedOption.NotifyEmails = nil
						sanitizedItem.DropdownOptions[optionIdx] = sanitizedOption
					}
				} else {
					sanitizedItem.DropdownOptions = []structs.WorkflowDropdownOptionCreateInput{}
				}
				sanitizedStep.WorkItems[itemIdx] = sanitizedItem
			}
		} else {
			sanitizedStep.WorkItems = []structs.WorkflowWorkItemCreateInput{}
		}
		sanitized[stepIdx] = sanitizedStep
	}

	return sanitized
}

func (a *AppDB) GetWorkflowEditProposalsForVoter(ctx context.Context, voterID string) ([]*structs.WorkflowEditProposal, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			workflow_edit_proposals
		WHERE
			status = 'pending'
		ORDER BY
			created_at DESC
		LIMIT 200;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow edit proposals: %s", err)
	}
	defer rows.Close()

	proposalIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning workflow edit proposal id: %s", err)
		}
		proposalIDs = append(proposalIDs, id)
	}

	proposals := make([]*structs.WorkflowEditProposal, 0, len(proposalIDs))
	for _, proposalID := range proposalIDs {
		proposal, err := a.GetWorkflowEditProposalByIDForUser(ctx, proposalID, &voterID)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

func (a *AppDB) getWorkflowEditVotesInternal(ctx context.Context, proposalID string, voterID *string) (*structs.WorkflowVotes, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE decision = 'approve'),
			COUNT(*) FILTER (WHERE decision = 'deny')
		FROM
			workflow_edit_votes
		WHERE
			proposal_id = $1;
	`, proposalID)

	votes := &structs.WorkflowVotes{}
	if err := row.Scan(&votes.Approve, &votes.Deny); err != nil {
		return nil, err
	}

	totalVoters, err := a.CountEligibleVoters(ctx)
	if err != nil {
		return nil, err
	}
	votes.TotalVoters = totalVoters
	votes.VotesCast = votes.Approve + votes.Deny
	votes.QuorumThreshold = quorumVotesRequired(totalVoters)
	votes.QuorumReached = votes.VotesCast >= votes.QuorumThreshold && totalVoters > 0

	row = a.db.QueryRow(ctx, `
		SELECT
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at,
			vote_decision
		FROM
			workflow_edit_proposals
		WHERE
			id = $1;
	`, proposalID)
	if err := row.Scan(&votes.QuorumReachedAt, &votes.FinalizeAt, &votes.FinalizedAt, &votes.Decision); err != nil {
		return nil, err
	}

	if voterID != nil {
		voteRow := a.db.QueryRow(ctx, `
			SELECT
				decision
			FROM
				workflow_edit_votes
			WHERE
				proposal_id = $1
			AND
				voter_id = $2;
		`, proposalID, *voterID)
		var decision string
		err := voteRow.Scan(&decision)
		if err == nil {
			votes.MyDecision = &decision
		} else if err != pgx.ErrNoRows {
			return nil, err
		}
	}

	return votes, nil
}

func (a *AppDB) RecordWorkflowEditVote(ctx context.Context, proposalID string, voterID string, decision string, comment string) (*structs.WorkflowVotes, error) {
	_, err := a.db.Exec(ctx, `
		INSERT INTO workflow_edit_votes
			(proposal_id, voter_id, decision, comment)
		VALUES
			($1, $2, $3, $4)
		ON CONFLICT (proposal_id, voter_id)
		DO UPDATE SET
			decision = EXCLUDED.decision,
			comment = EXCLUDED.comment,
			updated_at = unix_now();
	`, proposalID, voterID, decision, comment)
	if err != nil {
		return nil, fmt.Errorf("error recording workflow edit vote: %s", err)
	}
	return a.getWorkflowEditVotesInternal(ctx, proposalID, &voterID)
}

func countWorkflowEditVotesTx(ctx context.Context, tx pgx.Tx, proposalID string) (int, int, error) {
	row := tx.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE decision = 'approve'),
			COUNT(*) FILTER (WHERE decision = 'deny')
		FROM
			workflow_edit_votes
		WHERE
			proposal_id = $1;
	`, proposalID)
	var approve int
	var deny int
	if err := row.Scan(&approve, &deny); err != nil {
		return 0, 0, err
	}
	return approve, deny, nil
}

func finalizeWorkflowEditApprovalTx(
	ctx context.Context,
	tx pgx.Tx,
	proposalID string,
	seriesID string,
	proposedStateID string,
	actorUserID *string,
	decision string,
) error {
	if err := applyWorkflowStateVersionToSeriesTx(ctx, tx, seriesID, proposedStateID); err != nil {
		return err
	}
	if err := syncWorkflowLinkedStatePresentationFieldsTx(ctx, tx, seriesID, proposedStateID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE
			workflow_edit_proposals
		SET
			status = 'approved',
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, unix_now()),
			vote_finalize_at = COALESCE(vote_finalize_at, unix_now()),
			vote_finalized_at = COALESCE(vote_finalized_at, unix_now()),
			vote_finalized_by_user_id = COALESCE($3, vote_finalized_by_user_id),
			vote_decision = $2,
			updated_at = unix_now()
		WHERE
			id = $1;
	`, proposalID, decision, actorUserID)
	if err != nil {
		return fmt.Errorf("error finalizing approved workflow edit proposal: %s", err)
	}
	return nil
}

func finalizeWorkflowEditDenialTx(
	ctx context.Context,
	tx pgx.Tx,
	proposalID string,
	actorUserID *string,
	decision string,
) error {
	_, err := tx.Exec(ctx, `
		UPDATE
			workflow_edit_proposals
		SET
			status = 'denied',
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, unix_now()),
			vote_finalize_at = COALESCE(vote_finalize_at, unix_now()),
			vote_finalized_at = COALESCE(vote_finalized_at, unix_now()),
			vote_finalized_by_user_id = COALESCE($3, vote_finalized_by_user_id),
			vote_decision = $2,
			updated_at = unix_now()
		WHERE
			id = $1;
	`, proposalID, decision, actorUserID)
	if err != nil {
		return fmt.Errorf("error finalizing denied workflow edit proposal: %s", err)
	}
	return nil
}

func (a *AppDB) EvaluateWorkflowEditVoteState(ctx context.Context, proposalID string) (*structs.WorkflowEditProposal, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	type editVoteState struct {
		Status          string
		SeriesID        string
		ProposedStateID string
		QuorumReachedAt *int64
		FinalizeAt      *int64
		FinalizedAt     *int64
	}

	state := editVoteState{}
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			series_id,
			proposed_state_id,
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at
		FROM
			workflow_edit_proposals
		WHERE
			id = $1
		FOR UPDATE;
	`, proposalID).Scan(
		&state.Status,
		&state.SeriesID,
		&state.ProposedStateID,
		&state.QuorumReachedAt,
		&state.FinalizeAt,
		&state.FinalizedAt,
	)
	if err != nil {
		return nil, err
	}
	if state.Status != "pending" {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return a.GetWorkflowEditProposalByIDForUser(ctx, proposalID, nil)
	}

	totalVoters, err := countEligibleVotersTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	approveCount, denyCount, err := countWorkflowEditVotesTx(ctx, tx, proposalID)
	if err != nil {
		return nil, err
	}
	votesCast := approveCount + denyCount
	quorumThreshold := quorumVotesRequired(totalVoters)
	quorumReached := totalVoters > 0 && votesCast >= quorumThreshold
	nowUnix := time.Now().UTC().Unix()

	if quorumReached && state.QuorumReachedAt == nil {
		quorumReachedAt := nowUnix
		finalizeAt := nowUnix + int64((24 * time.Hour).Seconds())
		_, err = tx.Exec(ctx, `
			UPDATE
				workflow_edit_proposals
			SET
				vote_quorum_reached_at = $2,
				vote_finalize_at = $3,
				updated_at = unix_now()
			WHERE
				id = $1;
		`, proposalID, quorumReachedAt, finalizeAt)
		if err != nil {
			return nil, fmt.Errorf("error setting workflow edit vote quorum countdown: %s", err)
		}
		state.QuorumReachedAt = &quorumReachedAt
		state.FinalizeAt = &finalizeAt
	}

	majorityThreshold := possibleBodyMajority(totalVoters)
	outcome := ""
	if totalVoters > 0 && approveCount >= majorityThreshold {
		outcome = "approve"
	} else if totalVoters > 0 && denyCount >= majorityThreshold {
		outcome = "deny"
	} else if quorumReached && state.FinalizeAt != nil && nowUnix >= *state.FinalizeAt {
		if approveCount > denyCount {
			outcome = "approve"
		} else {
			outcome = "deny"
		}
	}

	if outcome == "approve" {
		if err := finalizeWorkflowEditApprovalTx(ctx, tx, proposalID, state.SeriesID, state.ProposedStateID, nil, "approve"); err != nil {
			return nil, err
		}
	}
	if outcome == "deny" {
		if err := finalizeWorkflowEditDenialTx(ctx, tx, proposalID, nil, "deny"); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a.GetWorkflowEditProposalByIDForUser(ctx, proposalID, nil)
}

func (a *AppDB) ExpireStaleWorkflowEditProposals(ctx context.Context) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			workflow_edit_proposals
		SET
			status = 'expired',
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, unix_now()),
			vote_finalize_at = COALESCE(vote_finalize_at, unix_now()),
			vote_finalized_at = COALESCE(vote_finalized_at, unix_now()),
			updated_at = unix_now()
		WHERE
			status = 'pending'
		AND
			created_at <= (unix_now() - (14 * 24 * 60 * 60));
	`)
	if err != nil {
		return fmt.Errorf("error expiring stale workflow edit proposals: %s", err)
	}
	return nil
}

func (a *AppDB) CreateWorkflowDeletionProposal(
	ctx context.Context,
	requesterId string,
	req *structs.WorkflowDeletionProposalCreateRequest,
) (*structs.WorkflowDeletionProposal, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	workflowId := strings.TrimSpace(req.WorkflowId)
	if workflowId == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}

	targetType := strings.TrimSpace(req.TargetType)
	if targetType == "" {
		targetType = "workflow"
	}
	if targetType != "workflow" && targetType != "series" {
		return nil, fmt.Errorf("invalid target_type")
	}

	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 2000 {
		return nil, fmt.Errorf("reason is too long")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var isVoter bool
	var isAdmin bool
	var proposerStatus *string
	err = tx.QueryRow(ctx, `
		SELECT
			u.is_voter,
			u.is_admin,
			p.status
		FROM
			users u
		LEFT JOIN
			proposers p
		ON
			p.user_id = u.id
		WHERE
			u.id = $1;
	`, requesterId).Scan(&isVoter, &isAdmin, &proposerStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("requester not found")
		}
		return nil, err
	}

	isApprovedProposer := proposerStatus != nil && strings.TrimSpace(*proposerStatus) == "approved"
	if !isApprovedProposer && !isVoter && !isAdmin {
		return nil, fmt.Errorf("requester is not authorized to propose workflow deletion")
	}

	var seriesId string
	var workflowStatus string
	var recurrence string
	err = tx.QueryRow(ctx, `
		SELECT
			w.series_id,
			w.status,
			COALESCE(NULLIF(TRIM(st.recurrence), ''), COALESCE(NULLIF(TRIM(s.recurrence), ''), 'one_time'))
		FROM
			workflows w
		LEFT JOIN
			workflow_states st
		ON
			st.id = w.workflow_state_id
		LEFT JOIN
			workflow_series s
		ON
			s.id = w.series_id
		WHERE
			w.id = $1
		FOR UPDATE OF w;
	`, workflowId).Scan(&seriesId, &workflowStatus, &recurrence)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("workflow not found")
		}
		return nil, err
	}

	switch workflowStatus {
	case "approved", "blocked", "in_progress", "completed":
	default:
		return nil, fmt.Errorf("workflow is not active")
	}

	if targetType == "workflow" {
		isSeriesWorkflow := recurrence != "one_time"
		if !isSeriesWorkflow {
			var seriesCount int
			err = tx.QueryRow(ctx, `
				SELECT
					COUNT(*)
				FROM
					workflows
				WHERE
					series_id = $1;
			`, seriesId).Scan(&seriesCount)
			if err != nil {
				return nil, err
			}
			isSeriesWorkflow = seriesCount > 1
		}
		if isSeriesWorkflow {
			return nil, fmt.Errorf("individual deletion is not allowed for workflows in a series; propose series deletion")
		}

		var pendingCount int
		err = tx.QueryRow(ctx, `
			SELECT
				COUNT(*)
			FROM
				workflow_deletion_proposals
			WHERE
				target_type = 'workflow'
			AND
				target_workflow_id = $1
			AND
				status = 'pending';
		`, workflowId).Scan(&pendingCount)
		if err != nil {
			return nil, err
		}
		if pendingCount > 0 {
			return nil, fmt.Errorf("a pending deletion vote already exists for this workflow")
		}
	} else {
		var pendingCount int
		err = tx.QueryRow(ctx, `
			SELECT
				COUNT(*)
			FROM
				workflow_deletion_proposals
			WHERE
				target_type = 'series'
			AND
				target_series_id = $1
			AND
				status = 'pending';
		`, seriesId).Scan(&pendingCount)
		if err != nil {
			return nil, err
		}
		if pendingCount > 0 {
			return nil, fmt.Errorf("a pending deletion vote already exists for this series")
		}
	}

	proposalId := uuid.NewString()
	var targetWorkflowID *string
	var targetSeriesID *string
	if targetType == "workflow" {
		targetWorkflowID = &workflowId
	} else {
		targetSeriesID = &seriesId
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO workflow_deletion_proposals
			(
				id,
				target_type,
				target_workflow_id,
				target_series_id,
				requested_by_user_id,
				reason
			)
		VALUES
			($1, $2, $3, $4, $5, $6);
	`, proposalId, targetType, targetWorkflowID, targetSeriesID, requesterId, reason)
	if err != nil {
		return nil, fmt.Errorf("error creating workflow deletion proposal: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetWorkflowDeletionProposalByIDForUser(ctx, proposalId, nil)
}

func (a *AppDB) GetWorkflowDeletionProposalByIDForUser(ctx context.Context, proposalId string, voterId *string) (*structs.WorkflowDeletionProposal, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			p.id,
			p.target_type,
			p.target_workflow_id,
			COALESCE(p.target_workflow_id, pw.id),
			CASE
				WHEN p.target_type = 'workflow' THEN COALESCE(NULLIF(TRIM(st.title), ''), COALESCE(NULLIF(TRIM(sr.title), ''), ''))
				WHEN p.target_type = 'series' THEN COALESCE(NULLIF(TRIM(tsr.title), ''), '')
				ELSE NULL
			END,
			p.target_series_id,
			p.reason,
			p.status,
			p.requested_by_user_id,
			p.vote_quorum_reached_at,
			p.vote_finalize_at,
			p.vote_finalized_at,
			p.vote_finalized_by_user_id,
			p.vote_decision,
			p.created_at,
			p.updated_at
		FROM
			workflow_deletion_proposals p
		LEFT JOIN
			workflows w
		ON
			w.id = p.target_workflow_id
		LEFT JOIN
			workflow_states st
		ON
			st.id = w.workflow_state_id
		LEFT JOIN
			workflow_series sr
		ON
			sr.id = w.series_id
		LEFT JOIN
			workflow_series tsr
		ON
			tsr.id = p.target_series_id
		LEFT JOIN LATERAL (
			SELECT
				w2.id
			FROM
				workflows w2
			WHERE
				w2.series_id = p.target_series_id
			AND
				w2.status <> 'deleted'
			ORDER BY
				CASE
					WHEN w2.status IN ('approved', 'blocked', 'in_progress', 'completed') THEN 0
					ELSE 1
				END,
				w2.start_at ASC,
				w2.created_at DESC,
				w2.id DESC
			LIMIT 1
		) pw
		ON
			TRUE
		WHERE
			p.id = $1;
	`, proposalId)

	proposal := &structs.WorkflowDeletionProposal{}
	if err := row.Scan(
		&proposal.Id,
		&proposal.TargetType,
		&proposal.TargetWorkflowId,
		&proposal.PreviewWorkflowId,
		&proposal.TargetWorkflowTitle,
		&proposal.TargetSeriesId,
		&proposal.Reason,
		&proposal.Status,
		&proposal.RequestedByUserId,
		&proposal.VoteQuorumReachedAt,
		&proposal.VoteFinalizeAt,
		&proposal.VoteFinalizedAt,
		&proposal.VoteFinalizedBy,
		&proposal.VoteDecision,
		&proposal.CreatedAt,
		&proposal.UpdatedAt,
	); err != nil {
		return nil, err
	}

	votes, err := a.getWorkflowDeletionVotesInternal(ctx, proposalId, voterId)
	if err != nil {
		return nil, err
	}
	proposal.Votes = *votes

	return proposal, nil
}

func (a *AppDB) GetWorkflowDeletionProposalsForVoter(ctx context.Context, voterId string) ([]*structs.WorkflowDeletionProposal, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			workflow_deletion_proposals
		WHERE
			status = 'pending'
		ORDER BY
			created_at DESC
		LIMIT 300;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow deletion proposals: %s", err)
	}
	defer rows.Close()

	proposalIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning workflow deletion proposal id: %s", err)
		}
		proposalIDs = append(proposalIDs, id)
	}

	proposals := make([]*structs.WorkflowDeletionProposal, 0, len(proposalIDs))
	for _, proposalID := range proposalIDs {
		proposal, err := a.GetWorkflowDeletionProposalByIDForUser(ctx, proposalID, &voterId)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

func (a *AppDB) ForceApproveWorkflowEditProposalAsAdmin(ctx context.Context, proposalID string, adminID string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var seriesID string
	var proposedStateID string
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			series_id,
			proposed_state_id
		FROM
			workflow_edit_proposals
		WHERE
			id = $1
		FOR UPDATE;
	`, proposalID).Scan(&status, &seriesID, &proposedStateID)
	if err != nil {
		return err
	}

	if status == "approved" {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	if status != "pending" {
		return fmt.Errorf("workflow edit proposal is not pending")
	}

	if err := finalizeWorkflowEditApprovalTx(ctx, tx, proposalID, seriesID, proposedStateID, &adminID, "admin_approve"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (a *AppDB) getWorkflowDeletionVotesInternal(ctx context.Context, proposalId string, voterId *string) (*structs.WorkflowVotes, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE decision = 'approve'),
			COUNT(*) FILTER (WHERE decision = 'deny')
		FROM
			workflow_deletion_votes
		WHERE
			proposal_id = $1;
	`, proposalId)

	votes := &structs.WorkflowVotes{}
	if err := row.Scan(&votes.Approve, &votes.Deny); err != nil {
		return nil, err
	}

	totalVoters, err := a.CountEligibleVoters(ctx)
	if err != nil {
		return nil, err
	}
	votes.TotalVoters = totalVoters
	votes.VotesCast = votes.Approve + votes.Deny
	votes.QuorumThreshold = quorumVotesRequired(totalVoters)
	votes.QuorumReached = votes.VotesCast >= votes.QuorumThreshold && totalVoters > 0

	row = a.db.QueryRow(ctx, `
		SELECT
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at,
			vote_decision
		FROM
			workflow_deletion_proposals
		WHERE
			id = $1;
	`, proposalId)
	if err := row.Scan(&votes.QuorumReachedAt, &votes.FinalizeAt, &votes.FinalizedAt, &votes.Decision); err != nil {
		return nil, err
	}

	if voterId != nil {
		voteRow := a.db.QueryRow(ctx, `
			SELECT
				decision
			FROM
				workflow_deletion_votes
			WHERE
				proposal_id = $1
			AND
				voter_id = $2;
		`, proposalId, *voterId)
		var decision string
		err := voteRow.Scan(&decision)
		if err == nil {
			votes.MyDecision = &decision
		} else if err != pgx.ErrNoRows {
			return nil, err
		}
	}

	return votes, nil
}

func (a *AppDB) RecordWorkflowDeletionVote(ctx context.Context, proposalId string, voterId string, decision string, comment string) (*structs.WorkflowVotes, error) {
	_, err := a.db.Exec(ctx, `
		INSERT INTO workflow_deletion_votes
			(proposal_id, voter_id, decision, comment)
		VALUES
			($1, $2, $3, $4)
		ON CONFLICT (proposal_id, voter_id)
		DO UPDATE SET
			decision = EXCLUDED.decision,
			comment = EXCLUDED.comment,
			updated_at = unix_now();
	`, proposalId, voterId, decision, comment)
	if err != nil {
		return nil, fmt.Errorf("error recording workflow deletion vote: %s", err)
	}
	return a.getWorkflowDeletionVotesInternal(ctx, proposalId, &voterId)
}

func (a *AppDB) EvaluateWorkflowDeletionVoteState(ctx context.Context, proposalId string) (*structs.WorkflowDeletionProposal, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	type deletionVoteState struct {
		Status          string
		TargetType      string
		TargetWorkflow  *string
		TargetSeries    *string
		QuorumReachedAt *int64
		FinalizeAt      *int64
		FinalizedAt     *int64
	}

	state := deletionVoteState{}
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			target_type,
			target_workflow_id,
			target_series_id,
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at
		FROM
			workflow_deletion_proposals
		WHERE
			id = $1
		FOR UPDATE;
	`, proposalId).Scan(
		&state.Status,
		&state.TargetType,
		&state.TargetWorkflow,
		&state.TargetSeries,
		&state.QuorumReachedAt,
		&state.FinalizeAt,
		&state.FinalizedAt,
	)
	if err != nil {
		return nil, err
	}

	if state.Status != "pending" {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return a.GetWorkflowDeletionProposalByIDForUser(ctx, proposalId, nil)
	}

	totalVoters, err := countEligibleVotersTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	approveCount, denyCount, err := countWorkflowDeletionVotesTx(ctx, tx, proposalId)
	if err != nil {
		return nil, err
	}
	votesCast := approveCount + denyCount
	quorumThreshold := quorumVotesRequired(totalVoters)
	quorumReached := totalVoters > 0 && votesCast >= quorumThreshold
	nowUnix := time.Now().UTC().Unix()

	if quorumReached && state.QuorumReachedAt == nil {
		quorumReachedAt := nowUnix
		finalizeAt := nowUnix + int64((24 * time.Hour).Seconds())
		_, err = tx.Exec(ctx, `
			UPDATE
				workflow_deletion_proposals
			SET
				vote_quorum_reached_at = $2,
				vote_finalize_at = $3,
				updated_at = unix_now()
			WHERE
				id = $1;
		`, proposalId, quorumReachedAt, finalizeAt)
		if err != nil {
			return nil, fmt.Errorf("error setting deletion vote quorum countdown: %s", err)
		}
		state.QuorumReachedAt = &quorumReachedAt
		state.FinalizeAt = &finalizeAt
	}

	majorityThreshold := possibleBodyMajority(totalVoters)
	outcome := ""
	if totalVoters > 0 && approveCount >= majorityThreshold {
		outcome = "approve"
	} else if totalVoters > 0 && denyCount >= majorityThreshold {
		outcome = "deny"
	} else if quorumReached && state.FinalizeAt != nil && nowUnix >= *state.FinalizeAt {
		if approveCount > denyCount {
			outcome = "approve"
		} else {
			outcome = "deny"
		}
	}

	if outcome == "approve" {
		if err := finalizeWorkflowDeletionApprovalTx(ctx, tx, proposalId, state.TargetType, state.TargetWorkflow, state.TargetSeries, nil, "approve"); err != nil {
			return nil, err
		}
	}
	if outcome == "deny" {
		if err := finalizeWorkflowDeletionDenialTx(ctx, tx, proposalId, nil, "deny"); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetWorkflowDeletionProposalByIDForUser(ctx, proposalId, nil)
}

func (a *AppDB) ForceApproveWorkflowDeletionProposalAsAdmin(ctx context.Context, proposalId string, adminId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var targetType string
	var targetWorkflowId *string
	var targetSeriesId *string
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			target_type,
			target_workflow_id,
			target_series_id
		FROM
			workflow_deletion_proposals
		WHERE
			id = $1
		FOR UPDATE;
	`, proposalId).Scan(&status, &targetType, &targetWorkflowId, &targetSeriesId)
	if err != nil {
		return err
	}

	if status == "approved" {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	if status != "pending" {
		return fmt.Errorf("workflow deletion proposal is not pending")
	}

	if err := finalizeWorkflowDeletionApprovalTx(ctx, tx, proposalId, targetType, targetWorkflowId, targetSeriesId, &adminId, "admin_approve"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func countWorkflowDeletionVotesTx(ctx context.Context, tx pgx.Tx, proposalId string) (int, int, error) {
	row := tx.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE decision = 'approve'),
			COUNT(*) FILTER (WHERE decision = 'deny')
		FROM
			workflow_deletion_votes
		WHERE
			proposal_id = $1;
	`, proposalId)
	var approve int
	var deny int
	if err := row.Scan(&approve, &deny); err != nil {
		return 0, 0, err
	}
	return approve, deny, nil
}

func finalizeWorkflowDeletionApprovalTx(
	ctx context.Context,
	tx pgx.Tx,
	proposalId string,
	targetType string,
	targetWorkflowId *string,
	targetSeriesId *string,
	actorUserId *string,
	decision string,
) error {
	if targetType == "workflow" && targetWorkflowId != nil {
		_, err := tx.Exec(ctx, `
			WITH deleted AS (
				UPDATE
					workflows
				SET
					status = 'deleted',
					updated_at = unix_now()
				WHERE
					id = $1
				AND
					status <> 'deleted'
				RETURNING
					id
			)
			UPDATE
				workflows
			SET
				is_start_blocked = false,
				blocked_by_workflow_id = NULL,
				status = CASE WHEN status = 'blocked' THEN 'approved' ELSE status END,
				updated_at = unix_now()
			WHERE
				status = 'blocked'
			AND
				blocked_by_workflow_id IN (SELECT id FROM deleted);
		`, *targetWorkflowId)
		if err != nil {
			return fmt.Errorf("error archiving workflow from approved deletion vote: %s", err)
		}
	}

	if targetType == "series" && targetSeriesId != nil {
		_, err := tx.Exec(ctx, `
			WITH deleted AS (
				UPDATE
					workflows
				SET
					status = 'deleted',
					updated_at = unix_now()
				WHERE
					series_id = $1
				AND
					status <> 'deleted'
				RETURNING
					id
			)
			UPDATE
				workflows
			SET
				is_start_blocked = false,
				blocked_by_workflow_id = NULL,
				status = CASE WHEN status = 'blocked' THEN 'approved' ELSE status END,
				updated_at = unix_now()
			WHERE
				status = 'blocked'
			AND
				blocked_by_workflow_id IN (SELECT id FROM deleted);
		`, *targetSeriesId)
		if err != nil {
			return fmt.Errorf("error archiving workflow series from approved deletion vote: %s", err)
		}
	}

	_, err := tx.Exec(ctx, `
		UPDATE
			workflow_deletion_proposals
		SET
			status = 'approved',
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, unix_now()),
			vote_finalize_at = COALESCE(vote_finalize_at, unix_now()),
			vote_finalized_at = COALESCE(vote_finalized_at, unix_now()),
			vote_finalized_by_user_id = COALESCE($3, vote_finalized_by_user_id),
			vote_decision = $2,
			updated_at = unix_now()
		WHERE
			id = $1;
	`, proposalId, decision, actorUserId)
	if err != nil {
		return fmt.Errorf("error finalizing approved deletion vote: %s", err)
	}

	return nil
}

func finalizeWorkflowDeletionDenialTx(
	ctx context.Context,
	tx pgx.Tx,
	proposalId string,
	actorUserId *string,
	decision string,
) error {
	_, err := tx.Exec(ctx, `
		UPDATE
			workflow_deletion_proposals
		SET
			status = 'denied',
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, unix_now()),
			vote_finalize_at = COALESCE(vote_finalize_at, unix_now()),
			vote_finalized_at = COALESCE(vote_finalized_at, unix_now()),
			vote_finalized_by_user_id = COALESCE($3, vote_finalized_by_user_id),
			vote_decision = $2,
			updated_at = unix_now()
		WHERE
			id = $1;
	`, proposalId, decision, actorUserId)
	if err != nil {
		return fmt.Errorf("error finalizing denied deletion vote: %s", err)
	}
	return nil
}

func (a *AppDB) GetIssuersWithScopes(ctx context.Context, search string, page, count int) ([]*structs.IssuerWithScopes, error) {
	if count <= 0 {
		count = 20
	}
	offset := page * count
	likeSearch := "%" + search + "%"
	rows, err := a.db.Query(ctx, `
		SELECT
			u.id,
			u.is_issuer,
			COALESCE(i.organization, '') AS organization,
			i.nickname
		FROM
			users u
		LEFT JOIN
			issuers i ON i.user_id = u.id
		WHERE
			u.is_issuer = true
		AND
			(COALESCE(i.organization, '') ILIKE $1 OR COALESCE(i.nickname, '') ILIKE $1)
		ORDER BY
			u.id ASC
		LIMIT $2
		OFFSET $3;
	`, likeSearch, count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying issuers: %s", err)
	}
	defer rows.Close()

	results := []*structs.IssuerWithScopes{}
	for rows.Next() {
		issuer := structs.IssuerWithScopes{}
		if err := rows.Scan(&issuer.UserId, &issuer.IsIssuer, &issuer.Organization, &issuer.Nickname); err != nil {
			return nil, fmt.Errorf("error scanning issuer: %s", err)
		}
		issuer.AllowedCredentials = []string{}
		results = append(results, &issuer)
	}

	for _, issuer := range results {
		scopes, err := a.GetIssuerScopeCredentials(ctx, issuer.UserId)
		if err != nil {
			return nil, err
		}
		issuer.AllowedCredentials = scopes
	}

	return results, nil
}

func (a *AppDB) GetIssuerScopeCredentials(ctx context.Context, issuerId string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			credential_type
		FROM
			issuer_credential_scopes
		WHERE
			issuer_id = $1
		ORDER BY
			credential_type ASC;
	`, issuerId)
	if err != nil {
		return nil, fmt.Errorf("error querying issuer scopes: %s", err)
	}
	defer rows.Close()

	credentials := []string{}
	for rows.Next() {
		var credential string
		if err := rows.Scan(&credential); err != nil {
			return nil, fmt.Errorf("error scanning issuer scope credential: %s", err)
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func (a *AppDB) SetIssuerScopes(ctx context.Context, adminId string, req *structs.IssuerScopeUpdateRequest) (*structs.IssuerWithScopes, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	req.UserId = strings.TrimSpace(req.UserId)
	if req.UserId == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	normalized := make([]string, 0, len(req.AllowedCredentials))
	seen := map[string]struct{}{}
	for _, credential := range req.AllowedCredentials {
		credential = strings.TrimSpace(credential)
		if credential == "" {
			continue
		}
		valid, err := a.IsGlobalCredentialType(ctx, credential)
		if err != nil {
			return nil, fmt.Errorf("error validating credential type: %s", err)
		}
		if !valid {
			return nil, fmt.Errorf("invalid credential type: %s", credential)
		}
		if _, exists := seen[credential]; exists {
			continue
		}
		seen[credential] = struct{}{}
		normalized = append(normalized, credential)
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT
			id
		FROM
			users
		WHERE
			id = $1;
	`, req.UserId)
	var userId string
	if err := row.Scan(&userId); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("issuer user not found")
		}
		return nil, err
	}

	makeIssuer := true
	if req.MakeIssuer != nil {
		makeIssuer = *req.MakeIssuer
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			users
		SET
			is_issuer = $2
		WHERE
			id = $1;
	`, req.UserId, makeIssuer)
	if err != nil {
		return nil, fmt.Errorf("error updating issuer role: %s", err)
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM issuer_credential_scopes WHERE issuer_id = $1;
	`, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("error resetting issuer scopes: %s", err)
	}

	for _, credential := range normalized {
		_, err = tx.Exec(ctx, `
			INSERT INTO issuer_credential_scopes
				(issuer_id, credential_type, created_by)
			VALUES
				($1, $2, $3);
		`, req.UserId, credential, adminId)
		if err != nil {
			return nil, fmt.Errorf("error inserting issuer scope: %s", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	scope := &structs.IssuerWithScopes{
		UserId:             req.UserId,
		IsIssuer:           makeIssuer,
		AllowedCredentials: normalized,
	}
	return scope, nil
}

func (a *AppDB) IssueCredential(ctx context.Context, issuerId string, req *structs.CredentialIssueRequest) (*structs.UserCredential, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	req.UserId = strings.TrimSpace(req.UserId)
	req.CredentialType = strings.TrimSpace(req.CredentialType)

	if req.UserId == "" || req.CredentialType == "" {
		return nil, fmt.Errorf("user_id and credential_type are required")
	}
	valid, err := a.IsGlobalCredentialType(ctx, req.CredentialType)
	if err != nil {
		return nil, fmt.Errorf("error validating credential type: %s", err)
	}
	if !valid {
		return nil, fmt.Errorf("invalid credential type")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	credential, err := a.issueCredentialTx(ctx, tx, issuerId, req)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return credential, nil
}

func ensureIssuerCanManageCredentialTx(ctx context.Context, tx pgx.Tx, issuerId string, credentialType string, action string) error {
	var issuerIsAdmin bool
	var issuerIsIssuer bool
	err := tx.QueryRow(ctx, `
		SELECT
			is_admin,
			is_issuer
		FROM
			users
		WHERE
			id = $1;
	`, issuerId).Scan(&issuerIsAdmin, &issuerIsIssuer)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("issuer user not found")
		}
		return err
	}

	if issuerIsAdmin {
		return nil
	}
	if !issuerIsIssuer {
		return fmt.Errorf("issuer role required")
	}

	var scopeCount int
	err = tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			issuer_credential_scopes
		WHERE
			issuer_id = $1
		AND
			credential_type = $2;
	`, issuerId, credentialType).Scan(&scopeCount)
	if err != nil {
		return err
	}
	if scopeCount == 0 {
		if action == "revoke" {
			return fmt.Errorf("issuer is not allowed to revoke this credential")
		}
		return fmt.Errorf("issuer is not allowed to grant this credential")
	}

	return nil
}

func (a *AppDB) issueCredentialTx(ctx context.Context, tx pgx.Tx, issuerId string, req *structs.CredentialIssueRequest) (*structs.UserCredential, error) {
	if err := ensureIssuerCanManageCredentialTx(ctx, tx, issuerId, req.CredentialType, "grant"); err != nil {
		return nil, err
	}

	var targetUserId string
	if err := tx.QueryRow(ctx, `
		SELECT
			id
		FROM
			users
		WHERE
			id = $1;
	`, req.UserId).Scan(&targetUserId); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("target user not found")
		}
		return nil, err
	}

	_, err := tx.Exec(ctx, `
		UPDATE
			user_credentials
		SET
			is_revoked = false,
			revoked_at = NULL,
			issued_by = $3,
			issued_at = NOW()
		WHERE
			user_id = $1
		AND
			credential_type = $2
		AND
			is_revoked = true;
	`, req.UserId, req.CredentialType, issuerId)
	if err != nil {
		return nil, fmt.Errorf("error reactivating credential: %s", err)
	}

	row := tx.QueryRow(ctx, `
		SELECT
			id,
			user_id,
			credential_type,
			issued_by,
			issued_at,
			is_revoked,
			revoked_at
		FROM
			user_credentials
		WHERE
			user_id = $1
		AND
			credential_type = $2
		AND
			is_revoked = false
		LIMIT 1;
	`, req.UserId, req.CredentialType)

	credential := &structs.UserCredential{}
	err = row.Scan(
		&credential.Id,
		&credential.UserId,
		&credential.CredentialType,
		&credential.IssuedBy,
		&credential.IssuedAt,
		&credential.IsRevoked,
		&credential.RevokedAt,
	)
	if err == pgx.ErrNoRows {
		row = tx.QueryRow(ctx, `
			INSERT INTO user_credentials
				(user_id, credential_type, issued_by)
			VALUES
				($1, $2, $3)
			RETURNING
				id,
				user_id,
				credential_type,
				issued_by,
				issued_at,
				is_revoked,
				revoked_at;
		`, req.UserId, req.CredentialType, issuerId)
		err = row.Scan(
			&credential.Id,
			&credential.UserId,
			&credential.CredentialType,
			&credential.IssuedBy,
			&credential.IssuedAt,
			&credential.IsRevoked,
			&credential.RevokedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error issuing credential: %s", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("error checking existing credential: %s", err)
	}

	return credential, nil
}

func (a *AppDB) RevokeCredential(ctx context.Context, issuerId string, req *structs.CredentialIssueRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	req.UserId = strings.TrimSpace(req.UserId)
	req.CredentialType = strings.TrimSpace(req.CredentialType)

	if req.UserId == "" || req.CredentialType == "" {
		return fmt.Errorf("user_id and credential_type are required")
	}
	validR, errR := a.IsGlobalCredentialType(ctx, req.CredentialType)
	if errR != nil {
		return fmt.Errorf("error validating credential type: %s", errR)
	}
	if !validR {
		return fmt.Errorf("invalid credential type")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := ensureIssuerCanManageCredentialTx(ctx, tx, issuerId, req.CredentialType, "revoke"); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			user_credentials
		SET
			is_revoked = true,
			revoked_at = NOW()
		WHERE
			user_id = $1
		AND
			credential_type = $2
		AND
			is_revoked = false;
	`, req.UserId, req.CredentialType)
	if err != nil {
		return fmt.Errorf("error revoking credential: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (a *AppDB) GetUserCredentials(ctx context.Context, userId string) ([]*structs.UserCredential, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			user_id,
			credential_type,
			issued_by,
			issued_at,
			is_revoked,
			revoked_at
		FROM
			user_credentials
		WHERE
			user_id = $1
		ORDER BY
			issued_at DESC;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error querying user credentials: %s", err)
	}
	defer rows.Close()

	credentials := []*structs.UserCredential{}
	for rows.Next() {
		credential := structs.UserCredential{}
		if err := rows.Scan(
			&credential.Id,
			&credential.UserId,
			&credential.CredentialType,
			&credential.IssuedBy,
			&credential.IssuedAt,
			&credential.IsRevoked,
			&credential.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning user credential: %s", err)
		}
		credentials = append(credentials, &credential)
	}

	return credentials, nil
}

func scanCredentialRequest(row interface {
	Scan(...any) error
}) (*structs.CredentialRequest, error) {
	req := structs.CredentialRequest{}
	if err := row.Scan(
		&req.Id,
		&req.UserId,
		&req.CredentialType,
		&req.Status,
		&req.RequestedAt,
		&req.ResolvedAt,
		&req.ResolvedBy,
		&req.CreatedAt,
		&req.UpdatedAt,
		&req.RequesterName,
		&req.RequesterFirstName,
		&req.RequesterLastName,
		&req.RequesterEmail,
	); err != nil {
		return nil, err
	}
	return &req, nil
}

func getCredentialRequestByIDTx(ctx context.Context, tx pgx.Tx, requestId string) (*structs.CredentialRequest, error) {
	row := tx.QueryRow(ctx, `
		SELECT
			cr.id,
			cr.user_id,
			cr.credential_type,
			cr.status,
			cr.requested_at,
			cr.resolved_at,
			cr.resolved_by,
			cr.created_at,
			cr.updated_at,
			COALESCE(NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''), COALESCE(u.contact_name, ''), cr.user_id),
			COALESCE(i.first_name, ''),
			COALESCE(i.last_name, ''),
			COALESCE(NULLIF(i.email, ''), COALESCE(u.contact_email, ''))
		FROM
			credential_requests cr
		LEFT JOIN
			improvers i
		ON
			i.user_id = cr.user_id
		LEFT JOIN
			users u
		ON
			u.id = cr.user_id
		WHERE
			cr.id = $1;
	`, requestId)
	return scanCredentialRequest(row)
}

func (a *AppDB) CreateCredentialRequest(ctx context.Context, userId string, credentialType string, allowUnlisted bool) (*structs.CredentialRequest, error) {
	userId = strings.TrimSpace(userId)
	credentialType = strings.TrimSpace(credentialType)
	if userId == "" || credentialType == "" {
		return nil, fmt.Errorf("user_id and credential_type are required")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var visibilityRaw string
	if err := tx.QueryRow(ctx, `
		SELECT
			visibility
		FROM
			credential_type_definitions
		WHERE
			value = $1;
	`, credentialType).Scan(&visibilityRaw); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("invalid credential type")
		}
		return nil, fmt.Errorf("error validating credential type: %s", err)
	}

	visibility, err := normalizeCredentialVisibility(visibilityRaw)
	if err != nil {
		return nil, fmt.Errorf("error validating credential visibility: %s", err)
	}
	if visibility == string(structs.CredentialVisibilityPrivate) {
		return nil, fmt.Errorf("credential type is not requestable")
	}
	if visibility == string(structs.CredentialVisibilityUnlisted) && !allowUnlisted {
		return nil, fmt.Errorf("credential type is not requestable")
	}

	var existingUser string
	if err := tx.QueryRow(ctx, `
		SELECT
			id
		FROM
			users
		WHERE
			id = $1;
	`, userId).Scan(&existingUser); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("target user not found")
		}
		return nil, err
	}

	var activeCredentialCount int
	if err := tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			user_credentials
		WHERE
			user_id = $1
		AND
			credential_type = $2
		AND
			is_revoked = false;
	`, userId, credentialType).Scan(&activeCredentialCount); err != nil {
		return nil, fmt.Errorf("error checking active credential state: %s", err)
	}
	if activeCredentialCount > 0 {
		return nil, fmt.Errorf("credential already active")
	}

	requestId := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO credential_requests
			(
				id,
				user_id,
				credential_type,
				status,
				requested_at
			)
		VALUES
			($1, $2, $3, 'pending', NOW());
	`, requestId, userId, credentialType)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "credential_requests_pending_unique" {
			return nil, fmt.Errorf("pending credential request already exists")
		}
		return nil, fmt.Errorf("error creating credential request: %s", err)
	}

	request, err := getCredentialRequestByIDTx(ctx, tx, requestId)
	if err != nil {
		return nil, fmt.Errorf("error loading created credential request: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return request, nil
}

func (a *AppDB) GetCredentialRequestsByUser(ctx context.Context, userId string) ([]*structs.CredentialRequest, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			cr.id,
			cr.user_id,
			cr.credential_type,
			cr.status,
			cr.requested_at,
			cr.resolved_at,
			cr.resolved_by,
			cr.created_at,
			cr.updated_at,
			COALESCE(NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''), COALESCE(u.contact_name, ''), cr.user_id),
			COALESCE(i.first_name, ''),
			COALESCE(i.last_name, ''),
			COALESCE(NULLIF(i.email, ''), COALESCE(u.contact_email, ''))
		FROM
			credential_requests cr
		LEFT JOIN
			improvers i
		ON
			i.user_id = cr.user_id
		LEFT JOIN
			users u
		ON
			u.id = cr.user_id
		WHERE
			cr.user_id = $1
		ORDER BY
			CASE WHEN cr.status = 'pending' THEN 0 ELSE 1 END,
			cr.requested_at DESC,
			cr.created_at DESC
		LIMIT 300;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error querying credential requests for user: %s", err)
	}
	defer rows.Close()

	results := []*structs.CredentialRequest{}
	for rows.Next() {
		request, err := scanCredentialRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning credential request: %s", err)
		}
		results = append(results, request)
	}
	return results, nil
}

func (a *AppDB) GetCredentialRequestsForIssuer(ctx context.Context, issuerId, search string, page, count int) ([]*structs.CredentialRequest, error) {
	issuerId = strings.TrimSpace(issuerId)
	if issuerId == "" {
		return nil, fmt.Errorf("issuer user_id is required")
	}
	if count <= 0 {
		count = 20
	}
	offset := page * count
	likeSearch := "%" + search + "%"

	var isAdmin bool
	var isIssuer bool
	if err := a.db.QueryRow(ctx, `
		SELECT
			is_admin,
			is_issuer
		FROM
			users
		WHERE
			id = $1;
	`, issuerId).Scan(&isAdmin, &isIssuer); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("issuer user not found")
		}
		return nil, err
	}
	if !isAdmin && !isIssuer {
		return nil, fmt.Errorf("issuer role required")
	}

	var rows pgx.Rows
	var errQuery error
	if isAdmin {
		rows, errQuery = a.db.Query(ctx, `
			SELECT
				cr.id,
				cr.user_id,
				cr.credential_type,
				cr.status,
				cr.requested_at,
				cr.resolved_at,
				cr.resolved_by,
				cr.created_at,
				cr.updated_at,
				COALESCE(NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''), COALESCE(u.contact_name, ''), cr.user_id),
				COALESCE(i.first_name, ''),
				COALESCE(i.last_name, ''),
				COALESCE(NULLIF(i.email, ''), COALESCE(u.contact_email, ''))
			FROM
				credential_requests cr
			LEFT JOIN
				improvers i
			ON
				i.user_id = cr.user_id
			LEFT JOIN
				users u
			ON
				u.id = cr.user_id
			WHERE
				(
					COALESCE(i.first_name, '') ILIKE $1
					OR COALESCE(i.last_name, '') ILIKE $1
					OR COALESCE(i.email, '') ILIKE $1
					OR COALESCE(u.contact_name, '') ILIKE $1
					OR COALESCE(u.contact_email, '') ILIKE $1
				)
			ORDER BY
				CASE WHEN cr.status = 'pending' THEN 0 ELSE 1 END,
				cr.requested_at DESC,
				cr.created_at DESC
			LIMIT $2
			OFFSET $3;
		`, likeSearch, count, offset)
	} else {
		rows, errQuery = a.db.Query(ctx, `
			SELECT
				cr.id,
				cr.user_id,
				cr.credential_type,
				cr.status,
				cr.requested_at,
				cr.resolved_at,
				cr.resolved_by,
				cr.created_at,
				cr.updated_at,
				COALESCE(NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''), COALESCE(u.contact_name, ''), cr.user_id),
				COALESCE(i.first_name, ''),
				COALESCE(i.last_name, ''),
				COALESCE(NULLIF(i.email, ''), COALESCE(u.contact_email, ''))
			FROM
				credential_requests cr
			JOIN
				issuer_credential_scopes scope
			ON
				scope.credential_type = cr.credential_type
			AND
				scope.issuer_id = $2
			LEFT JOIN
				improvers i
			ON
				i.user_id = cr.user_id
			LEFT JOIN
				users u
			ON
				u.id = cr.user_id
			WHERE
				(
					COALESCE(i.first_name, '') ILIKE $1
					OR COALESCE(i.last_name, '') ILIKE $1
					OR COALESCE(i.email, '') ILIKE $1
					OR COALESCE(u.contact_name, '') ILIKE $1
					OR COALESCE(u.contact_email, '') ILIKE $1
				)
			ORDER BY
				CASE WHEN cr.status = 'pending' THEN 0 ELSE 1 END,
				cr.requested_at DESC,
				cr.created_at DESC
			LIMIT $3
			OFFSET $4;
		`, likeSearch, issuerId, count, offset)
	}
	if errQuery != nil {
		return nil, fmt.Errorf("error querying credential requests for issuer: %s", errQuery)
	}
	defer rows.Close()

	results := []*structs.CredentialRequest{}
	for rows.Next() {
		request, err := scanCredentialRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning issuer credential request: %s", err)
		}
		results = append(results, request)
	}
	return results, nil
}

func normalizeCredentialRequestStatusInput(input string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(input))
	switch normalized {
	case "approve", "approved":
		return "approved", nil
	case "reject", "rejected", "deny", "denied":
		return "rejected", nil
	case "pending":
		return "pending", nil
	default:
		return "", fmt.Errorf("invalid decision")
	}
}

func revokeCredentialTx(ctx context.Context, tx pgx.Tx, issuerId string, userId string, credentialType string) error {
	if err := ensureIssuerCanManageCredentialTx(ctx, tx, issuerId, credentialType, "revoke"); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE
			user_credentials
		SET
			is_revoked = true,
			revoked_at = NOW()
		WHERE
			user_id = $1
		AND
			credential_type = $2
		AND
			is_revoked = false;
	`, userId, credentialType); err != nil {
		return fmt.Errorf("error revoking credential: %s", err)
	}

	return nil
}

func (a *AppDB) ResolveCredentialRequest(ctx context.Context, issuerId string, requestId string, decision string) (*structs.CredentialRequest, error) {
	issuerId = strings.TrimSpace(issuerId)
	requestId = strings.TrimSpace(requestId)
	targetStatus, err := normalizeCredentialRequestStatusInput(decision)
	if issuerId == "" || requestId == "" {
		return nil, fmt.Errorf("issuer_id and request_id are required")
	}
	if err != nil {
		return nil, err
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var requestUserId string
	var requestCredential string
	err = tx.QueryRow(ctx, `
		SELECT
			user_id,
			credential_type
		FROM
			credential_requests
		WHERE
			id = $1
		FOR UPDATE;
	`, requestId).Scan(&requestUserId, &requestCredential)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("credential request not found")
		}
		return nil, err
	}

	if targetStatus == "approved" {
		if _, err := a.issueCredentialTx(ctx, tx, issuerId, &structs.CredentialIssueRequest{
			UserId:         requestUserId,
			CredentialType: requestCredential,
		}); err != nil {
			return nil, err
		}
	} else {
		if err := revokeCredentialTx(ctx, tx, issuerId, requestUserId, requestCredential); err != nil {
			return nil, err
		}
	}

	if targetStatus == "pending" {
		_, err = tx.Exec(ctx, `
			UPDATE
				credential_requests
			SET
				status = 'pending',
				resolved_by = NULL,
				resolved_at = NULL,
				updated_at = NOW()
			WHERE
				id = $1;
		`, requestId)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "credential_requests_pending_unique" {
				return nil, fmt.Errorf("pending credential request already exists")
			}
			return nil, fmt.Errorf("error updating credential request status: %s", err)
		}
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE
				credential_requests
			SET
				status = $2,
				resolved_by = $3,
				resolved_at = NOW(),
				updated_at = NOW()
			WHERE
				id = $1;
		`, requestId, targetStatus, issuerId)
		if err != nil {
			return nil, fmt.Errorf("error updating credential request status: %s", err)
		}
	}

	request, err := getCredentialRequestByIDTx(ctx, tx, requestId)
	if err != nil {
		return nil, fmt.Errorf("error loading updated credential request: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return request, nil
}

func (a *AppDB) GetIssuersAllowedForCredential(ctx context.Context, credentialType string) ([]*structs.CredentialRequestIssuerRecipient, error) {
	credentialType = strings.TrimSpace(credentialType)
	if credentialType == "" {
		return nil, fmt.Errorf("credential_type is required")
	}

	rows, err := a.db.Query(ctx, `
		SELECT DISTINCT
			u.id,
			COALESCE(NULLIF(i.nickname, ''), NULLIF(i.organization, ''), NULLIF(u.contact_name, ''), u.id),
			COALESCE(NULLIF(i.email, ''), COALESCE(u.contact_email, ''))
		FROM
			users u
		JOIN
			issuer_credential_scopes scope
		ON
			scope.issuer_id = u.id
		AND
			scope.credential_type = $1
		LEFT JOIN
			issuers i
		ON
			i.user_id = u.id
		WHERE
			u.is_issuer = true
		AND
			(i.status = 'approved' OR i.status IS NULL)
		ORDER BY
			u.id ASC;
	`, credentialType)
	if err != nil {
		return nil, fmt.Errorf("error querying issuers for credential notifications: %s", err)
	}
	defer rows.Close()

	recipients := []*structs.CredentialRequestIssuerRecipient{}
	for rows.Next() {
		recipient := &structs.CredentialRequestIssuerRecipient{}
		if err := rows.Scan(&recipient.UserId, &recipient.Name, &recipient.Email); err != nil {
			return nil, fmt.Errorf("error scanning issuer recipient: %s", err)
		}
		recipients = append(recipients, recipient)
	}
	return recipients, nil
}

// ── Credential Type Definitions ──────────────────────────────────────────────

func (a *AppDB) getValidCredentialTypeSet(ctx context.Context) (map[string]struct{}, error) {
	types, err := a.GetGlobalCredentialTypes(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(types))
	for _, t := range types {
		set[t.Value] = struct{}{}
	}
	return set, nil
}

func (a *AppDB) IsGlobalCredentialType(ctx context.Context, value string) (bool, error) {
	var count int
	err := a.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM credential_type_definitions WHERE value = $1;
	`, value).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("error checking credential type: %s", err)
	}
	return count > 0, nil
}

func normalizeCredentialVisibility(input string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return string(structs.CredentialVisibilityPublic), nil
	}
	switch normalized {
	case string(structs.CredentialVisibilityPublic),
		string(structs.CredentialVisibilityPrivate),
		string(structs.CredentialVisibilityUnlisted):
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid credential visibility")
	}
}

const maxCredentialBadgeUploadBytes = 2 * 1024 * 1024

type parsedCredentialBadgeUpload struct {
	ContentType string
	Data        []byte
}

func parseCredentialBadgeUpload(contentTypeInput, base64PayloadInput string) (*parsedCredentialBadgeUpload, error) {
	base64Payload := strings.TrimSpace(base64PayloadInput)
	if base64Payload == "" {
		return nil, fmt.Errorf("badge upload data is required")
	}
	if commaIdx := strings.Index(base64Payload, ","); commaIdx >= 0 {
		prefix := strings.ToLower(strings.TrimSpace(base64Payload[:commaIdx]))
		if strings.Contains(prefix, "base64") {
			base64Payload = strings.TrimSpace(base64Payload[commaIdx+1:])
		}
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Payload)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(base64Payload)
		if err != nil {
			return nil, fmt.Errorf("invalid badge image payload")
		}
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("badge upload payload is empty")
	}
	if len(decoded) > maxCredentialBadgeUploadBytes {
		return nil, fmt.Errorf("badge upload exceeds maximum size of 2MB")
	}

	contentType := strings.ToLower(strings.TrimSpace(contentTypeInput))
	if contentType == "" {
		contentType = strings.ToLower(http.DetectContentType(decoded))
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("badge upload must be an image")
	}

	return &parsedCredentialBadgeUpload{
		ContentType: contentType,
		Data:        decoded,
	}, nil
}

func (a *AppDB) GetGlobalCredentialTypes(ctx context.Context) ([]*structs.GlobalCredentialType, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			value,
			label,
			visibility,
			badge_content_type,
			CASE
				WHEN badge_data IS NULL THEN NULL
				ELSE ENCODE(badge_data, 'base64')
			END AS badge_data_base64,
			created_at,
			updated_at
		FROM credential_type_definitions
		ORDER BY created_at ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying credential types: %s", err)
	}
	defer rows.Close()

	results := []*structs.GlobalCredentialType{}
	for rows.Next() {
		t := structs.GlobalCredentialType{}
		if err := rows.Scan(
			&t.Value,
			&t.Label,
			&t.Visibility,
			&t.BadgeContentType,
			&t.BadgeDataBase64,
			&t.CreatedAt,
			&t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning credential type: %s", err)
		}
		results = append(results, &t)
	}
	return results, nil
}

func (a *AppDB) CreateGlobalCredentialType(ctx context.Context, value, label, visibility string) (*structs.GlobalCredentialType, error) {
	value = strings.TrimSpace(value)
	label = strings.TrimSpace(label)
	if value == "" || label == "" {
		return nil, fmt.Errorf("value and label are required")
	}

	normalizedVisibility, err := normalizeCredentialVisibility(visibility)
	if err != nil {
		return nil, err
	}

	t := structs.GlobalCredentialType{}
	err = a.db.QueryRow(ctx, `
		INSERT INTO credential_type_definitions (value, label, visibility)
		VALUES ($1, $2, $3)
		RETURNING
			value,
			label,
			visibility,
			badge_content_type,
			CASE
				WHEN badge_data IS NULL THEN NULL
				ELSE ENCODE(badge_data, 'base64')
			END AS badge_data_base64,
			created_at,
			updated_at;
	`, value, label, normalizedVisibility).Scan(
		&t.Value,
		&t.Label,
		&t.Visibility,
		&t.BadgeContentType,
		&t.BadgeDataBase64,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return nil, fmt.Errorf("credential type already exists")
		}
		return nil, fmt.Errorf("error creating credential type: %s", err)
	}
	return &t, nil
}

func (a *AppDB) UpdateGlobalCredentialType(
	ctx context.Context,
	value string,
	label string,
	visibility *string,
	badgeContentType *string,
	badgeDataBase64 *string,
	clearBadge bool,
) (*structs.GlobalCredentialType, error) {
	value = strings.TrimSpace(value)
	label = strings.TrimSpace(label)
	if value == "" {
		return nil, fmt.Errorf("value is required")
	}
	if label == "" {
		return nil, fmt.Errorf("label is required")
	}

	var normalizedVisibility string
	hasVisibility := false
	if visibility != nil {
		parsedVisibility, err := normalizeCredentialVisibility(*visibility)
		if err != nil {
			return nil, err
		}
		normalizedVisibility = parsedVisibility
		hasVisibility = true
	}

	normalizedBadgeContentType := ""
	if badgeContentType != nil {
		normalizedBadgeContentType = strings.TrimSpace(*badgeContentType)
	}
	normalizedBadgeDataBase64 := ""
	if badgeDataBase64 != nil {
		normalizedBadgeDataBase64 = strings.TrimSpace(*badgeDataBase64)
	}
	hasBadgePayload := normalizedBadgeContentType != "" || normalizedBadgeDataBase64 != ""

	if clearBadge && hasBadgePayload {
		return nil, fmt.Errorf("cannot upload and clear badge in the same request")
	}

	var badgeData []byte
	var parsedContentType string
	if hasBadgePayload {
		if normalizedBadgeDataBase64 == "" {
			return nil, fmt.Errorf("badge_data_base64 is required when uploading a badge")
		}
		parsedBadge, err := parseCredentialBadgeUpload(normalizedBadgeContentType, normalizedBadgeDataBase64)
		if err != nil {
			return nil, err
		}
		badgeData = parsedBadge.Data
		parsedContentType = parsedBadge.ContentType
	}

	contentTypeParam := parsedContentType

	t := structs.GlobalCredentialType{}
	err := a.db.QueryRow(ctx, `
			UPDATE credential_type_definitions
			SET
				label = $2,
				visibility = CASE
					WHEN $3 THEN $4
					ELSE visibility
				END,
				badge_data = CASE
					WHEN $5 THEN NULL
					WHEN $8 THEN $6
					ELSE badge_data
				END,
				badge_content_type = CASE
					WHEN $5 THEN NULL
					WHEN $8 THEN $7
					ELSE badge_content_type
				END,
				updated_at = NOW()
		WHERE value = $1
		RETURNING
			value,
			label,
			visibility,
			badge_content_type,
			CASE
				WHEN badge_data IS NULL THEN NULL
				ELSE ENCODE(badge_data, 'base64')
				END AS badge_data_base64,
				created_at,
				updated_at;
		`, value, label, hasVisibility, normalizedVisibility, clearBadge, badgeData, contentTypeParam, hasBadgePayload).Scan(
		&t.Value,
		&t.Label,
		&t.Visibility,
		&t.BadgeContentType,
		&t.BadgeDataBase64,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("credential type not found")
		}
		return nil, fmt.Errorf("error updating credential type: %s", err)
	}

	return &t, nil
}

func (a *AppDB) DeleteGlobalCredentialType(ctx context.Context, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("value is required")
	}
	result, err := a.db.Exec(ctx, `
		DELETE FROM credential_type_definitions WHERE value = $1;
	`, value)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return fmt.Errorf("credential type is in use")
		}
		return fmt.Errorf("error deleting credential type: %s", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("credential type not found")
	}
	return nil
}

// ── Issuer Requests ───────────────────────────────────────────────────────────

func scanIssuer(row interface {
	Scan(...any) error
}) (*structs.Issuer, error) {
	i := structs.Issuer{}
	if err := row.Scan(&i.UserId, &i.Organization, &i.Email, &i.Nickname, &i.Status, &i.CreatedAt, &i.UpdatedAt); err != nil {
		return nil, err
	}
	return &i, nil
}

func (a *AppDB) UpsertIssuerRequest(ctx context.Context, userId, organization, email string) (*structs.Issuer, error) {
	organization = strings.TrimSpace(organization)
	email = strings.ToLower(strings.TrimSpace(email))
	if organization == "" {
		return nil, fmt.Errorf("organization is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	isVerified, err := a.IsVerifiedEmailForUser(ctx, userId, email)
	if err != nil {
		return nil, err
	}
	if !isVerified {
		return nil, fmt.Errorf("email must be verified before requesting issuer status")
	}

	var existingStatus string
	err = a.db.QueryRow(ctx, `SELECT status FROM issuers WHERE user_id = $1;`, userId).Scan(&existingStatus)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("error checking issuer request: %s", err)
	}
	if existingStatus == "approved" {
		return nil, fmt.Errorf("issuer already approved")
	}

	row := a.db.QueryRow(ctx, `
		INSERT INTO issuers (user_id, organization, email, status)
		VALUES ($1, $2, $3, 'pending')
		ON CONFLICT (user_id) DO UPDATE
			SET organization = EXCLUDED.organization,
			    email        = EXCLUDED.email,
			    status       = CASE WHEN issuers.status = 'approved' THEN 'approved' ELSE 'pending' END,
			    updated_at   = NOW()
		RETURNING user_id, organization, email, nickname, status, created_at, updated_at;
	`, userId, organization, email)
	return scanIssuer(row)
}

func (a *AppDB) GetIssuerByUser(ctx context.Context, userId string) (*structs.Issuer, error) {
	row := a.db.QueryRow(ctx, `
		SELECT user_id, organization, email, nickname, status, created_at, updated_at
		FROM issuers WHERE user_id = $1;
	`, userId)
	i, err := scanIssuer(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error getting issuer: %s", err)
	}
	return i, nil
}

func (a *AppDB) GetIssuerRequests(ctx context.Context, search string, page, count int) ([]*structs.Issuer, error) {
	if count <= 0 {
		count = 20
	}
	offset := page * count
	likeSearch := "%" + search + "%"
	rows, err := a.db.Query(ctx, `
		SELECT user_id, organization, email, nickname, status, created_at, updated_at
		FROM issuers
		WHERE (organization ILIKE $1 OR email ILIKE $1 OR COALESCE(nickname, '') ILIKE $1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3;
	`, likeSearch, count, offset)
	if err != nil {
		return nil, fmt.Errorf("error querying issuer requests: %s", err)
	}
	defer rows.Close()

	results := []*structs.Issuer{}
	for rows.Next() {
		i, err := scanIssuer(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning issuer: %s", err)
		}
		results = append(results, i)
	}
	return results, nil
}

func (a *AppDB) UpdateIssuerRequest(ctx context.Context, req *structs.IssuerUpdateRequest) (*structs.Issuer, error) {
	if req == nil || strings.TrimSpace(req.UserId) == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		switch status {
		case "approved", "rejected", "pending":
		default:
			return nil, fmt.Errorf("invalid status")
		}
		_, err = tx.Exec(ctx, `UPDATE issuers SET status = $2, updated_at = NOW() WHERE user_id = $1;`, req.UserId, status)
		if err != nil {
			return nil, fmt.Errorf("error updating issuer status: %s", err)
		}
		if status == "approved" {
			_, err = tx.Exec(ctx, `UPDATE users SET is_issuer = true WHERE id = $1;`, req.UserId)
			if err != nil {
				return nil, fmt.Errorf("error granting issuer role: %s", err)
			}
		}
	}

	if req.Nickname != nil {
		_, err = tx.Exec(ctx, `UPDATE issuers SET nickname = $2, updated_at = NOW() WHERE user_id = $1;`, req.UserId, req.Nickname)
		if err != nil {
			return nil, fmt.Errorf("error updating issuer nickname: %s", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetIssuerByUser(ctx, req.UserId)
}

// ── User by Address ───────────────────────────────────────────────────────────

func (a *AppDB) GetUserByAddress(ctx context.Context, address string) (*structs.User, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}
	row := a.db.QueryRow(ctx, `
		SELECT id, is_admin, is_merchant, is_organizer, is_improver, is_proposer,
		       is_voter, is_issuer, is_supervisor, is_affiliate, contact_email, contact_phone,
		       contact_name, paypal_eth, last_redemption
		FROM users WHERE LOWER(paypal_eth) = LOWER($1);
	`, address)
	u := structs.User{Exists: true}
	err := row.Scan(
		&u.Id, &u.IsAdmin, &u.IsMerchant, &u.IsOrganizer, &u.IsImprover, &u.IsProposer,
		&u.IsVoter, &u.IsIssuer, &u.IsSupervisor, &u.IsAffiliate, &u.Email, &u.Phone,
		&u.Name, &u.PayPalEth, &u.LastRedemption,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("error looking up user by address: %s", err)
	}
	return &u, nil
}
