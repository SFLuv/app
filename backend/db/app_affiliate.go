package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func (a *AppDB) UpsertAffiliateRequest(ctx context.Context, userId string, organization string) (*structs.Affiliate, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	row := tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			affiliates
		WHERE
			user_id = $1;
	`, userId)
	err = row.Scan(&status)
	if err == pgx.ErrNoRows {
		_, err = tx.Exec(ctx, `
			INSERT INTO affiliates
				(user_id, organization, status)
			VALUES
				($1, $2, 'pending');
		`, userId, organization)
		if err != nil {
			return nil, fmt.Errorf("error inserting affiliate request: %s", err)
		}
	} else if err != nil {
		return nil, err
	} else {
		if status == "approved" {
			return nil, fmt.Errorf("affiliate already approved")
		}
		_, err = tx.Exec(ctx, `
			UPDATE
				affiliates
			SET
				organization = $2,
				status = 'pending',
				updated_at = NOW()
			WHERE
				user_id = $1;
		`, userId, organization)
		if err != nil {
			return nil, fmt.Errorf("error updating affiliate request: %s", err)
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			users
		SET
			is_affiliate = false
		WHERE
			id = $1;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error resetting affiliate status: %s", err)
	}

	affiliate, err := getAffiliateByUser(ctx, tx, userId)
	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	return affiliate, nil
}

func (a *AppDB) GetAffiliateByUser(ctx context.Context, userId string) (*structs.Affiliate, error) {
	return getAffiliateByUser(ctx, a.db, userId)
}

func (a *AppDB) GetAffiliates(ctx context.Context) ([]*structs.Affiliate, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			user_id,
			organization,
			nickname,
			status,
			affiliate_logo,
			weekly_allocation,
			weekly_balance,
			one_time_balance
		FROM
			affiliates
		ORDER BY
			created_at DESC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying affiliates: %s", err)
	}
	defer rows.Close()

	results := []*structs.Affiliate{}
	for rows.Next() {
		affiliate := structs.Affiliate{}
		err = rows.Scan(
			&affiliate.UserId,
			&affiliate.Organization,
			&affiliate.Nickname,
			&affiliate.Status,
			&affiliate.AffiliateLogo,
			&affiliate.WeeklyAllocation,
			&affiliate.WeeklyBalance,
			&affiliate.OneTimeBalance,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning affiliate: %s", err)
		}
		results = append(results, &affiliate)
	}

	return results, nil
}

func (a *AppDB) GetAffiliateWeeklyConfigs(ctx context.Context) ([]*structs.AffiliateWeeklyConfig, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			user_id,
			weekly_allocation
		FROM
			affiliates
		WHERE
			status = 'approved';
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying affiliate weekly configs: %s", err)
	}
	defer rows.Close()

	results := []*structs.AffiliateWeeklyConfig{}
	for rows.Next() {
		cfg := structs.AffiliateWeeklyConfig{}
		err = rows.Scan(&cfg.UserId, &cfg.WeeklyAllocation)
		if err != nil {
			return nil, fmt.Errorf("error scanning affiliate weekly config: %s", err)
		}
		results = append(results, &cfg)
	}

	return results, nil
}

func (a *AppDB) SetAffiliateWeeklyBalance(ctx context.Context, userId string, balance uint64) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			affiliates
		SET
			weekly_balance = $1,
			updated_at = NOW()
		WHERE
			user_id = $2;
	`, balance, userId)
	if err != nil {
		return fmt.Errorf("error updating affiliate weekly balance: %s", err)
	}
	return nil
}

func (a *AppDB) AddAffiliateWeeklyBalance(ctx context.Context, userId string, amount uint64) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			affiliates
		SET
			weekly_balance = weekly_balance + $1,
			updated_at = NOW()
		WHERE
			user_id = $2;
	`, amount, userId)
	if err != nil {
		return fmt.Errorf("error adding affiliate weekly balance: %s", err)
	}
	return nil
}

func (a *AppDB) UpdateAffiliateLogo(ctx context.Context, userId string, logo *string) (*structs.Affiliate, error) {
	var logoValue any
	if logo != nil {
		trimmed := strings.TrimSpace(*logo)
		if trimmed != "" {
			logoValue = trimmed
		}
	}

	cmd, err := a.db.Exec(ctx, `
		UPDATE
			affiliates
		SET
			affiliate_logo = $2,
			updated_at = NOW()
		WHERE
			user_id = $1;
	`, userId, logoValue)
	if err != nil {
		return nil, fmt.Errorf("error updating affiliate logo: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, fmt.Errorf("affiliate not found")
	}

	return getAffiliateByUser(ctx, a.db, userId)
}

func (a *AppDB) ReserveAffiliateBalance(ctx context.Context, userId string, amount uint64) (*structs.BalanceReservation, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var weekly uint64
	var oneTime uint64
	row := tx.QueryRow(ctx, `
		SELECT
			weekly_balance,
			one_time_balance
		FROM
			affiliates
		WHERE
			user_id = $1
		FOR UPDATE;
	`, userId)
	if err := row.Scan(&weekly, &oneTime); err != nil {
		return nil, fmt.Errorf("error getting affiliate balances: %s", err)
	}

	total := weekly + oneTime
	if total < amount {
		return nil, fmt.Errorf("insufficient affiliate balance")
	}

	deductOneTime := amount
	if oneTime < amount {
		deductOneTime = oneTime
	}
	remainder := amount - deductOneTime
	deductWeekly := remainder

	_, err = tx.Exec(ctx, `
		UPDATE
			affiliates
		SET
			weekly_balance = $2,
			one_time_balance = $3,
			updated_at = NOW()
		WHERE
			user_id = $1;
	`, userId, weekly-deductWeekly, oneTime-deductOneTime)
	if err != nil {
		return nil, fmt.Errorf("error reserving affiliate balance: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("error committing affiliate balance reservation: %s", err)
	}

	return &structs.BalanceReservation{
		WeeklyDeducted:  deductWeekly,
		OneTimeDeducted: deductOneTime,
	}, nil
}

func (a *AppDB) RefundAffiliateBalance(ctx context.Context, userId string, weeklyAmount uint64, oneTimeAmount uint64) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			affiliates
		SET
			weekly_balance = weekly_balance + $1,
			one_time_balance = one_time_balance + $2,
			updated_at = NOW()
		WHERE
			user_id = $3;
	`, weeklyAmount, oneTimeAmount, userId)
	if err != nil {
		return fmt.Errorf("error refunding affiliate balance: %s", err)
	}
	return nil
}

func (a *AppDB) UpdateAffiliate(ctx context.Context, req *structs.AffiliateUpdateRequest) (*structs.Affiliate, error) {
	if req.UserId == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	if req.Status != nil {
		switch *req.Status {
		case "pending", "approved", "rejected":
		default:
			return nil, fmt.Errorf("invalid affiliate status")
		}
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var currentAllocation uint64
	var currentWeekly uint64
	row := tx.QueryRow(ctx, `
		SELECT
			weekly_allocation,
			weekly_balance
		FROM
			affiliates
		WHERE
			user_id = $1;
	`, req.UserId)
	if err := row.Scan(&currentAllocation, &currentWeekly); err != nil {
		return nil, fmt.Errorf("error getting affiliate balances: %s", err)
	}

	newAllocation := currentAllocation
	newWeekly := currentWeekly
	if req.WeeklyBalance != nil {
		newAllocation = *req.WeeklyBalance
		if newAllocation >= currentAllocation {
			newWeekly = currentWeekly + (newAllocation - currentAllocation)
		} else {
			delta := currentAllocation - newAllocation
			if currentWeekly > delta {
				newWeekly = currentWeekly - delta
			} else {
				newWeekly = 0
			}
		}
	}

	cmd, err := tx.Exec(ctx, `
		UPDATE
			affiliates
		SET
			nickname = COALESCE($2, nickname),
			weekly_allocation = $3,
			weekly_balance = $4,
			one_time_balance = one_time_balance + COALESCE($5, 0),
			status = COALESCE($6, status),
			updated_at = NOW()
		WHERE
			user_id = $1;
	`, req.UserId, req.Nickname, newAllocation, newWeekly, req.OneTimeBonus, req.Status)
	if err != nil {
		return nil, fmt.Errorf("error updating affiliate: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, fmt.Errorf("affiliate not found")
	}

	if req.Status != nil {
		isAffiliate := *req.Status == "approved"
		_, err = tx.Exec(ctx, `
			UPDATE
				users
			SET
				is_affiliate = $1
			WHERE
				id = $2;
		`, isAffiliate, req.UserId)
		if err != nil {
			return nil, fmt.Errorf("error updating user affiliate flag: %s", err)
		}
	}

	affiliate, err := getAffiliateByUser(ctx, tx, req.UserId)
	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	return affiliate, nil
}

func (a *AppDB) IsAffiliate(ctx context.Context, id string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			is_affiliate
		FROM
			users
		WHERE
			id = $1;
	`, id)
	var isAffiliate bool
	err := row.Scan(&isAffiliate)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return isAffiliate, nil
}

func (a *AppDB) GetFirstAdminId(ctx context.Context) (string, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id
		FROM
			users
		WHERE
			is_admin = true
		ORDER BY
			id
		LIMIT 1;
	`)
	var id string
	err := row.Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

func getAffiliateByUser(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userId string) (*structs.Affiliate, error) {
	row := querier.QueryRow(ctx, `
		SELECT
			user_id,
			organization,
			nickname,
			status,
			affiliate_logo,
			weekly_allocation,
			weekly_balance,
			one_time_balance
		FROM
			affiliates
		WHERE
			user_id = $1;
	`, userId)

	affiliate := structs.Affiliate{}
	err := row.Scan(
		&affiliate.UserId,
		&affiliate.Organization,
		&affiliate.Nickname,
		&affiliate.Status,
		&affiliate.AffiliateLogo,
		&affiliate.WeeklyAllocation,
		&affiliate.WeeklyBalance,
		&affiliate.OneTimeBalance,
	)
	if err != nil {
		return nil, err
	}

	return &affiliate, nil
}
