package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func (a *AppDB) AddUser(id string) error {
	_, err := a.db.Exec(context.Background(), `
		INSERT INTO users
			(id)
		VALUES
			($1)
		ON CONFLICT
			(id)
		DO NOTHING;
	`, id)
	if err != nil {
		return err
	}

	return nil
}

func (a *AppDB) UpdateUserInfo(user *structs.User) error {
	_, err := a.db.Exec(context.Background(), `
		UPDATE
			users
		SET
			contact_email = $1,
			contact_phone = $2,
			contact_name = $3
		WHERE
			id = $4;
	`, user.Email, user.Phone, user.Name, user.Id)
	if err != nil {
		return err
	}

	return nil
}

func (a *AppDB) UpdateUserRole(userId string, role string, value bool) error {
	roles := map[string]string{
		"admin":     "is_admin",
		"merchant=": "is_merchant",
		"organizer": "is_organizer",
		"improver":  "is_improver",
	}

	role, ok := roles[role]
	if !ok {
		return fmt.Errorf("invalid role column name")
	}

	_, err := a.db.Exec(context.Background(), fmt.Sprintf(`
		UPDATE
			users
		SET
			%s = $1
		WHERE
			id = $2;
	`, role), value, userId)
	if err != nil {
		return fmt.Errorf("error updating user: %s", err)
	}

	return nil
}

func (a *AppDB) GetUsers(page int, count int) ([]*structs.User, error) {
	var users []*structs.User
	offset := page * count

	rows, err := a.db.Query(context.Background(), `
		SELECT
			id,
			is_admin,
			is_merchant,
			is_organizer,
			is_improver,
			contact_email,
			contact_phone,
			contact_name
		FROM
			users
		LIMIT $1
		OFFSET $2;
	`, count, offset)
	if err == pgx.ErrNoRows {
		return users, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error getting users: %s", err)
	}

	var scanError error
	for rows.Next() {
		user := structs.User{}
		user.Exists = true
		err = rows.Scan(
			&user.Id,
			&user.IsAdmin,
			&user.IsMerchant,
			&user.IsOrganizer,
			&user.IsImprover,
			&user.Email,
			&user.Phone,
			&user.Name,
		)
		if err != nil {
			fmt.Println(err)
			scanError = err
			continue
		}

		users = append(users, &user)
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("error while scanning all rows: %s", scanError)
	}

	return users, nil
}

func (a *AppDB) GetUserById(userId string) (*structs.User, error) {
	var user structs.User
	row := a.db.QueryRow(context.Background(), `
		SELECT
			id,
			is_admin,
			is_merchant,
			is_organizer,
			is_improver,
			contact_email,
			contact_phone,
			contact_name
		FROM
			users
		WHERE
			id = $1;
	`, userId)
	err := row.Scan(
		&user.Id,
		&user.IsAdmin,
		&user.IsMerchant,
		&user.IsOrganizer,
		&user.IsImprover,
		&user.Email,
		&user.Phone,
		&user.Name,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}
