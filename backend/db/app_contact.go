package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) AddContact(c *structs.Contact, userId string) (*structs.Contact, error) {
	row := a.db.QueryRow(context.Background(), `
		INSERT INTO contacts(
			owner,
			name,
			address
		) VALUES (
			$1,
			$2,
			$3
		)
		ON CONFLICT (owner, address)
		DO NOTHING
		RETURNING id;
	`, userId, c.Name, c.Address)

	err := row.Scan(&c.Id)
	if err != nil {
		return nil, err
	}

	c.Owner = userId
	return c, nil
}

func (a *AppDB) UpdateContact(c *structs.Contact, userId string) error {
	_, err := a.db.Exec(context.Background(), `
		UPDATE contacts
		SET
			name = $1,
			address = $2,
			is_favorite = $3
		FROM
			contacts c INNER JOIN users u ON c.owner = u.id
		WHERE
			(contacts.id = $4 AND u.id = $5);
	`, c.Name, c.Address, c.IsFavorite, c.Id, userId)

	return err
}

func (a *AppDB) GetContacts(userId string) ([]*structs.Contact, error) {
	rows, err := a.db.Query(context.Background(), `
		SELECT
			c.id,
			c.name,
			c.address,
			c.is_favorite
		FROM
			contacts AS c JOIN users AS u ON c.owner = u.id
		WHERE
			u.id = $1;
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

func (a *AppDB) DeleteContact(contactId int, userId string) error {
	_, err := a.db.Exec(context.Background(), `
		DELETE FROM
			contacts
		USING
			users
		WHERE
			(contacts.id = $1 AND owner = users.id AND users.id = $2);
	`, contactId, userId)

	return err
}
