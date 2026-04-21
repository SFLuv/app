package db

import (
	"context"
	"fmt"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) AddContact(ctx context.Context, c *structs.Contact, userId string) (*structs.Contact, error) {
	row := a.db.QueryRow(ctx, `
		INSERT INTO contacts(
			owner,
			name,
			address
		) VALUES (
			$1,
			$2,
			$3
		)
		ON CONFLICT (owner, address) WHERE active = TRUE
		DO UPDATE
		SET
			name = EXCLUDED.name,
			address = EXCLUDED.address
		RETURNING id;
	`, userId, c.Name, c.Address)

	err := row.Scan(&c.Id)
	if err != nil {
		return nil, err
	}

	c.Owner = userId
	return c, nil
}

func (a *AppDB) UpdateContact(ctx context.Context, c *structs.Contact, userId string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE contacts
		SET
			name = $1,
			address = $2,
			is_favorite = $3
		FROM
			contacts c INNER JOIN users u ON c.owner = u.id
		WHERE
			(contacts.id = $4 AND u.id = $5 AND contacts.active = TRUE AND u.active = TRUE);
	`, c.Name, c.Address, c.IsFavorite, c.Id, userId)

	return err
}

func (a *AppDB) GetContacts(ctx context.Context, userId string) ([]*structs.Contact, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			c.id,
			c.name,
			c.address,
			c.is_favorite
		FROM
			contacts AS c
		WHERE
			c.owner = $1
		AND
			c.active = TRUE
		ORDER BY
			c.is_favorite DESC,
			c.id ASC
		LIMIT 500;
	`, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	contacts := []*structs.Contact{}
	for rows.Next() {
		var c structs.Contact

		err = rows.Scan(
			&c.Id,
			&c.Name,
			&c.Address,
			&c.IsFavorite,
		)
		if err != nil {
			fmt.Printf("error scanning contact %d: %s\n", c.Id, err)
			continue
		}

		c.Owner = userId
		contacts = append(contacts, &c)
	}

	return contacts, nil
}

func (a *AppDB) DeleteContact(ctx context.Context, contactId int, userId string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			contacts
		SET
			active = FALSE,
			delete_date = $3,
			delete_reason = $4
		WHERE
			contacts.id = $1
		AND
			contacts.owner = $2
		AND
			contacts.active = TRUE;
	`, contactId, userId, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonContactDelete)

	return err
}
