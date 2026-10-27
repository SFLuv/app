package db

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ErrAvatarNotFound means the user has no picture.
var ErrAvatarNotFound = errors.New("no avatar")

// SetProfileAvatar stores or replaces the caller's profile picture.
func (a *AppDB) SetProfileAvatar(ctx context.Context, userId string, contentType string, data []byte) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO profile_avatars(user_id, content_type, image_data, size_bytes)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			content_type = EXCLUDED.content_type,
			image_data = EXCLUDED.image_data,
			size_bytes = EXCLUDED.size_bytes,
			updated_at = NOW();
	`, userId, contentType, data, len(data))
	return err
}

// GetProfileAvatar reads a user's profile picture.
//
// Readable by anybody signed in, by design: it is the picture attached to the
// public name that already appears on posts and in threads, so gating it more
// tightly than the name it sits next to would protect nothing.
func (a *AppDB) GetProfileAvatar(ctx context.Context, userId string) (string, []byte, error) {
	var contentType string
	var data []byte
	err := a.db.QueryRow(ctx, `
		SELECT content_type, image_data FROM profile_avatars WHERE user_id = $1;
	`, userId).Scan(&contentType, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrAvatarNotFound
	}
	return contentType, data, err
}

// DeleteProfileAvatar removes the caller's picture.
func (a *AppDB) DeleteProfileAvatar(ctx context.Context, userId string) error {
	_, err := a.db.Exec(ctx, `DELETE FROM profile_avatars WHERE user_id = $1;`, userId)
	return err
}

// UsersWithAvatars narrows a set of ids to those that have a picture, so a list
// of people can be rendered without one request per person who has none.
func (a *AppDB) UsersWithAvatars(ctx context.Context, userIds []string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if len(userIds) == 0 {
		return out, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT user_id FROM profile_avatars WHERE user_id = ANY($1);
	`, userIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// SearchUsersViaContacts finds accounts the caller has saved as contacts, by the
// name they saved them under.
//
// ── Why this is a separate search, and why it ignores discoverability ────────
//
// SearchSFLuvUsers deliberately only finds people who opted into being findable,
// or whose exact handle you already know. That is the right rule for searching
// strangers: being in the app is not consent to being looked up by name.
//
// A saved contact is a different relationship. You have their wallet address —
// you scanned their code or they sent you a link — and you chose a name for
// them. Searching that name is searching your own address book, and refusing to
// find them because they have not made themselves public to everybody would be
// protecting them from the person they already gave their address to.
//
// So this returns them regardless of up.discoverable, and only ever to the
// contact's owner. Blocks still apply: blocking someone overrides having saved
// them.
//
// It returns the contact's saved name alongside, because that is what the
// searcher typed and what they expect to see in the result — not the public name
// the account chose, which they may never have seen.
func (a *AppDB) SearchUsersViaContacts(ctx context.Context, viewerId string, query string) (map[string]string, error) {
	out := map[string]string{}
	query = strings.ToLower(strings.TrimSpace(query))
	query = strings.TrimPrefix(query, "@")
	if query == "" {
		return out, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT DISTINCT ON (c.user_id) c.user_id, c.name
		FROM contacts c
		JOIN users u ON u.id = c.user_id AND u.active = TRUE
		WHERE c.owner = $1
			AND c.active = TRUE
			AND c.user_id IS NOT NULL
			AND c.user_id <> $1
			AND LOWER(c.name) LIKE '%' || $2 ESCAPE '\'
			AND NOT EXISTS (
				SELECT 1 FROM user_blocks b
				WHERE (b.blocker_id = $1 AND b.blocked_id = c.user_id)
					OR (b.blocker_id = c.user_id AND b.blocked_id = $1)
			)
		ORDER BY c.user_id, c.is_favorite DESC, c.id
		LIMIT 20;
	`, viewerId, escapeLike(query)+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var userId, name string
		if err := rows.Scan(&userId, &name); err != nil {
			return nil, err
		}
		out[userId] = name
	}
	return out, rows.Err()
}
