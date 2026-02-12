package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (a *AppDB) IsAdmin(ctx context.Context, id string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			is_admin
		FROM
			users
		WHERE
			id = $1;
	`, id)
	var isAdmin bool
	err := row.Scan(&isAdmin)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return isAdmin, nil
}

func (a *AppDB) UpdateLocationApproval(ctx context.Context, id uint, approval bool) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
			SELECT
				owner_id
			FROM
				locations
			WHERE
				id = $1;
	`, id)

	var owner_id string
	err = row.Scan(&owner_id)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			locations
		SET
			approval = $1
		WHERE
			id = $2;
	`, approval, id)
	if err != nil {
		return fmt.Errorf("error updating approval for location %d: %s", id, err)
	}

	if approval {
		_, err = tx.Exec(ctx, `
		UPDATE
			users
		SET
			is_merchant = $1
		WHERE
			id = $2;
	`, approval, owner_id)
		if err != nil {
			return fmt.Errorf("error updating owner merchant status for user %s: %s", owner_id, err)
		}
	}

	tx.Commit(ctx)
	return nil
}

func (a *AppDB) GetLocationOwnerAndApproval(ctx context.Context, id uint) (string, bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			owner_id,
			approval
		FROM
			locations
		WHERE
			id = $1;
	`, id)

	var ownerID string
	var approval sql.NullBool
	if err := row.Scan(&ownerID, &approval); err != nil {
		return "", false, err
	}

	return ownerID, approval.Valid && approval.Bool, nil
}

func (a *AppDB) OwnerHasApprovedLocationExcluding(ctx context.Context, ownerID string, excludedLocationID uint) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1
				FROM locations
				WHERE owner_id = $1
				AND approval = TRUE
				AND id <> $2
			);
	`, ownerID, excludedLocationID)

	var hasApproved bool
	if err := row.Scan(&hasApproved); err != nil {
		return false, err
	}

	return hasApproved, nil
}

func (a *AppDB) UserHasAnyApprovedLocation(ctx context.Context, ownerID string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1
				FROM locations
				WHERE owner_id = $1
				AND approval = TRUE
			);
	`, ownerID)

	var hasApproved bool
	if err := row.Scan(&hasApproved); err != nil {
		return false, err
	}

	return hasApproved, nil
}

func (a *AppDB) GetOwnersWithApprovedLocations(ctx context.Context) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT DISTINCT owner_id
		FROM locations
		WHERE approval = TRUE
		AND owner_id IS NOT NULL
		ORDER BY owner_id;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ownerIDs := make([]string, 0)
	for rows.Next() {
		var ownerID string
		if err := rows.Scan(&ownerID); err != nil {
			return nil, err
		}
		ownerIDs = append(ownerIDs, ownerID)
	}

	return ownerIDs, nil
}
