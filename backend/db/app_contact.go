package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// ErrContactNotFound means no such contact, or not the caller's.
var ErrContactNotFound = errors.New("contact not found")

// AddContact saves a contact, and links it to an SFLuv account when the address
// belongs to one.
//
// The link is resolved here, in the same statement, rather than in the handler:
// a contact saved while the wallet lookup was briefly failing would otherwise
// stay unlinked forever, because nothing re-runs the resolution. Doing it in the
// INSERT means it is resolved exactly when the address is written, every time,
// including on the conflict path where an existing contact's address changes.
//
// Nothing about this is shown to the person saving the contact. What it buys is
// that searching the name they chose in the new-message picker finds the
// account — see the comment on structs.Contact.UserId.
func (a *AppDB) AddContact(ctx context.Context, c *structs.Contact, userId string) (*structs.Contact, error) {
	row := a.db.QueryRow(ctx, `
		INSERT INTO contacts(
			owner,
			name,
			address,
			notes,
			user_id
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			(
				SELECT w.owner FROM wallets w
				WHERE LOWER(w.eoa_address) = LOWER($3)
					OR LOWER(w.smart_address) = LOWER($3)
				LIMIT 1
			)
		)
		ON CONFLICT (owner, address) WHERE active = TRUE
		DO UPDATE
		SET
			name = EXCLUDED.name,
			address = EXCLUDED.address,
			notes = EXCLUDED.notes,
			user_id = EXCLUDED.user_id
		RETURNING id, user_id;
	`, userId, c.Name, c.Address, c.Notes)

	err := row.Scan(&c.Id, &c.UserId)
	if err != nil {
		return nil, err
	}

	c.Owner = userId
	return c, nil
}

// GetContact loads one contact card for its owner.
func (a *AppDB) GetContact(ctx context.Context, contactId int, userId string) (*structs.Contact, error) {
	var c structs.Contact
	err := a.db.QueryRow(ctx, `
		SELECT
			c.id, c.name, c.address, c.is_favorite, c.notes, c.user_id,
			EXISTS (SELECT 1 FROM contact_avatars av WHERE av.contact_id = c.id)
		FROM contacts c
		WHERE c.id = $1 AND c.owner = $2 AND c.active = TRUE;
	`, contactId, userId).Scan(&c.Id, &c.Name, &c.Address, &c.IsFavorite, &c.Notes, &c.UserId, &c.HasAvatar)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrContactNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Owner = userId
	return &c, nil
}

// PatchContact applies a partial update, re-resolving the account link if the
// address changed.
func (a *AppDB) PatchContact(ctx context.Context, contactId int, userId string, patch *structs.ContactPatch) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE contacts
		SET
			name = COALESCE($3, name),
			address = COALESCE($4, address),
			is_favorite = COALESCE($5, is_favorite),
			notes = COALESCE($6, notes),
			user_id = CASE
				WHEN $4::text IS NULL THEN user_id
				ELSE (
					SELECT w.owner FROM wallets w
					WHERE LOWER(w.eoa_address) = LOWER($4)
						OR LOWER(w.smart_address) = LOWER($4)
					LIMIT 1
				)
			END
		WHERE id = $1 AND owner = $2 AND active = TRUE;
	`, contactId, userId, patch.Name, patch.Address, patch.IsFavorite, patch.Notes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrContactNotFound
	}
	return nil
}

// SetContactAvatar stores or replaces a contact's picture.
func (a *AppDB) SetContactAvatar(ctx context.Context, contactId int, userId string, contentType string, data []byte) error {
	// The ownership test is a subquery rather than a prior read, so a contact id
	// belonging to somebody else cannot be written to even if the two steps
	// would have raced.
	tag, err := a.db.Exec(ctx, `
		INSERT INTO contact_avatars(contact_id, content_type, image_data, size_bytes)
		SELECT c.id, $3, $4, $5
		FROM contacts c
		WHERE c.id = $1 AND c.owner = $2 AND c.active = TRUE
		ON CONFLICT (contact_id) DO UPDATE SET
			content_type = EXCLUDED.content_type,
			image_data = EXCLUDED.image_data,
			size_bytes = EXCLUDED.size_bytes,
			updated_at = NOW();
	`, contactId, userId, contentType, data, len(data))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrContactNotFound
	}
	return nil
}

// GetContactAvatar reads a contact's picture, for its owner only.
func (a *AppDB) GetContactAvatar(ctx context.Context, contactId int, userId string) (string, []byte, error) {
	var contentType string
	var data []byte
	err := a.db.QueryRow(ctx, `
		SELECT av.content_type, av.image_data
		FROM contact_avatars av
		JOIN contacts c ON c.id = av.contact_id
		WHERE av.contact_id = $1 AND c.owner = $2 AND c.active = TRUE;
	`, contactId, userId).Scan(&contentType, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrContactNotFound
	}
	return contentType, data, err
}

// DeleteContactAvatar removes a contact's picture.
func (a *AppDB) DeleteContactAvatar(ctx context.Context, contactId int, userId string) error {
	_, err := a.db.Exec(ctx, `
		DELETE FROM contact_avatars av
		USING contacts c
		WHERE av.contact_id = $1 AND c.id = av.contact_id AND c.owner = $2;
	`, contactId, userId)
	return err
}

// SearchOwnContacts finds the caller's contacts by name, for the new-message
// picker, and reports which of them are reachable accounts.
//
// Only contacts with a resolved user_id can be messaged, so the caller filters
// on that — but the search runs over all of them, because a name that matches a
// contact with no account should still not appear as "no results".
func (a *AppDB) SearchOwnContacts(ctx context.Context, userId string, query string, limit int) ([]*structs.Contact, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []*structs.Contact{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			c.id, c.name, c.address, c.is_favorite, c.notes, c.user_id,
			EXISTS (SELECT 1 FROM contact_avatars av WHERE av.contact_id = c.id)
		FROM contacts c
		WHERE c.owner = $1
			AND c.active = TRUE
			AND LOWER(c.name) LIKE '%' || LOWER($2) || '%'
		ORDER BY (c.user_id IS NOT NULL) DESC, c.is_favorite DESC, c.name
		LIMIT $3;
	`, userId, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*structs.Contact{}
	for rows.Next() {
		var c structs.Contact
		if err := rows.Scan(&c.Id, &c.Name, &c.Address, &c.IsFavorite, &c.Notes, &c.UserId, &c.HasAvatar); err != nil {
			return nil, err
		}
		c.Owner = userId
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (a *AppDB) UpdateContact(ctx context.Context, c *structs.Contact, userId string) error {
	// The account link is re-resolved because the address may have changed. An
	// address that no longer belongs to anybody clears it, which is correct: the
	// contact is no longer that person.
	_, err := a.db.Exec(ctx, `
		UPDATE contacts
		SET
			name = $1,
			address = $2,
			is_favorite = $3,
			notes = $6,
			user_id = (
				SELECT w.owner FROM wallets w
				WHERE LOWER(w.eoa_address) = LOWER($2)
					OR LOWER(w.smart_address) = LOWER($2)
				LIMIT 1
			)
		WHERE
			contacts.id = $4
		AND
			contacts.owner = $5
		AND
			contacts.active = TRUE;
	`, c.Name, c.Address, c.IsFavorite, c.Id, userId, c.Notes)

	return err
}

func (a *AppDB) GetContacts(ctx context.Context, userId string) ([]*structs.Contact, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			c.id,
			c.name,
			c.address,
			c.is_favorite,
			c.notes,
			c.user_id,
			EXISTS (SELECT 1 FROM contact_avatars av WHERE av.contact_id = c.id)
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
			&c.Notes,
			&c.UserId,
			&c.HasAvatar,
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
