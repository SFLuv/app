package db

import (
	"context"
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

func (a *AppDB) HasApprovedLocations(ctx context.Context, id string) (bool, error) {
	var hasApprovedLocations = false
	var isApproved = false

	rows, err := a.db.Query(ctx, `
		SELECT
			l.approval
		FROM
			locations l
		INNER JOIN users u ON l.owner_id = u.id
		WHERE u.id = $1;
	`, id)

	if err != nil {
		return false, err
	}

	for rows.Next() {
		fmt.Println("iterating through row")
		err = rows.Scan(&isApproved)
		if err != nil {
			return false, err
		}
		if isApproved {
			fmt.Println("found approved location")
			hasApprovedLocations = true
		}

	}
	fmt.Println(hasApprovedLocations)
	return hasApprovedLocations, nil
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

	/*rows, err := tx.Query(ctx, `
		SELECT
			l.approval
		FROM
			locations l
		INNER JOIN users u ON l.owner_id = u.id
		WHERE u.id = $1;
	`, owner_id)
	if err != nil {
		return fmt.Errorf("error getting approval statuses for merchant %s: %s", owner_id, err)
	}

	for rows.Next() {
		var a bool
		err = rows.Scan(&a)
		if err != nil {
			return fmt.Errorf("error scanning approval status for merchant %s: %s", owner_id, err)
		}
		fmt.Println("value of a:")
		fmt.Println(a)
		if a {
			fmt.Println("merchant approval status set to true")
			merchantApproval = true
			rows.Close()
			break
		}
	}
	*/

	hasApprovedLocations, _ := a.HasApprovedLocations(ctx, owner_id)
	if hasApprovedLocations {
		fmt.Println("owner has approved locations")
	} else {
		fmt.Println("no approved locations found for owner")
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
