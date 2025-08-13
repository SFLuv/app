package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) AddContact(c *structs.Contact, userId string) (*structs.Contact, error) {
	fmt.Println("add", c.Address)
	fmt.Println("add", c.Id)
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
	fmt.Println("update", c.Address)
	fmt.Println("update", c.Id)
	_, err := a.db.Exec(context.Background(), `
		UPDATE contacts
		SET
			name = $1,
			address = $2,
			is_favorite = $3
		FROM
			contacts AS c JOIN users as u ON c.owner = u.id
		WHERE
			(c.id = $4 AND u.id = $5);
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

func (a *AppDB) DeleteContact(userId string, contactId int) error {
	_, err := a.db.Exec(context.Background(), `
		DELETE FROM
			contacts JOIN users ON contacts.owner = users.id
		WHERE
			(users.id = $1 AND contacts.id = $2);
	`, userId, contactId)

	return err
}
