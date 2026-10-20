package db

import (
	"context"
	"errors"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// ErrConversationNotFound covers both "no such conversation" and "not a member",
// so membership cannot be probed.
var ErrConversationNotFound = errors.New("conversation not found")

// ErrPaymentAlreadyAttached means the transfer is already shown in a thread.
var ErrPaymentAlreadyAttached = errors.New("payment already attached")

// GetOrCreateConversation returns the thread between two users about a post
// (or their plain thread when postId is nil), creating it if needed. The
// caller is always made an active member again, so a thread they archived
// comes back when they reopen it.
func (a *AppDB) GetOrCreateConversation(ctx context.Context, me, other string, postId *int64) (*structs.ConversationRow, bool, error) {
	low, high := orderedPair(me, other)

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	created := false
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO conversations(user_low, user_high, post_id, started_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_low, user_high, COALESCE(post_id, 0)) DO NOTHING
		RETURNING id;
	`, low, high, postId, me).Scan(&id)
	if err == nil {
		created = true
	} else if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `
			SELECT id FROM conversations
			WHERE user_low = $1 AND user_high = $2 AND COALESCE(post_id, 0) = COALESCE($3, 0);
		`, low, high, postId).Scan(&id); err != nil {
			return nil, false, err
		}
	} else {
		return nil, false, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO conversation_members(conversation_id, user_id)
		VALUES ($1, $2), ($1, $3)
		ON CONFLICT (conversation_id, user_id) DO NOTHING;
	`, id, low, high); err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE conversation_members SET archived_at = NULL
		WHERE conversation_id = $1 AND user_id = $2;
	`, id, me); err != nil {
		return nil, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}

	row, err := a.GetConversationForMember(ctx, id, me)
	return row, created, err
}

func (a *AppDB) GetConversationForMember(ctx context.Context, conversationId int64, userId string) (*structs.ConversationRow, error) {
	var c structs.ConversationRow
	err := a.db.QueryRow(ctx, `
		SELECT c.id, c.user_low, c.user_high, c.post_id, c.last_message_at, m.last_read_message_id
		FROM conversations c
		JOIN conversation_members m ON m.conversation_id = c.id AND m.user_id = $2
		WHERE c.id = $1;
	`, conversationId, userId).Scan(&c.Id, &c.UserLow, &c.UserHigh, &c.PostId, &c.LastMessageAt, &c.LastReadId)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConversationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CountConversationsStartedSince is the rate-limit input for opening new threads.
func (a *AppDB) CountConversationsStartedSince(ctx context.Context, userId string, since time.Time) (int, error) {
	var n int
	err := a.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM conversations WHERE started_by = $1 AND created_at >= $2;
	`, userId, since).Scan(&n)
	return n, err
}

const messageColumns = `
	x.id, x.conversation_id, x.sender_id, x.kind, x.body, x.tx_hash,
	x.amount_sfluv_base::text, x.payment_verified, x.created_at`

func scanMessage(row pgx.Row, m *structs.Message) error {
	return row.Scan(&m.Id, &m.ConversationId, &m.SenderId, &m.Kind, &m.Body, &m.TxHash,
		&m.AmountSfluvBase, &m.PaymentVerified, &m.CreatedAt)
}

type InboxRow struct {
	Conversation structs.ConversationRow
	Unread       int
	Last         *structs.Message
}

// ListConversations is the inbox: threads that have at least one message,
// newest activity first, with the latest message and the unread count.
func (a *AppDB) ListConversations(ctx context.Context, userId string) ([]InboxRow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			c.id, c.user_low, c.user_high, c.post_id, c.last_message_at, m.last_read_message_id,
			(
				SELECT COUNT(*) FROM messages u
				WHERE u.conversation_id = c.id
					AND u.id > m.last_read_message_id
					AND u.sender_id <> $1
					AND u.deleted_at IS NULL
			),
			lm.id, lm.conversation_id, lm.sender_id, lm.kind, lm.body, lm.tx_hash,
			lm.amount_sfluv_base::text, lm.payment_verified, lm.created_at
		FROM conversation_members m
		JOIN conversations c ON c.id = m.conversation_id
		JOIN LATERAL (
			SELECT * FROM messages x
			WHERE x.conversation_id = c.id AND x.deleted_at IS NULL
			ORDER BY x.id DESC
			LIMIT 1
		) lm ON TRUE
		WHERE m.user_id = $1 AND m.archived_at IS NULL
		ORDER BY c.last_message_at DESC
		LIMIT 200;
	`, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []InboxRow{}
	for rows.Next() {
		var r InboxRow
		var last structs.Message
		if err := rows.Scan(
			&r.Conversation.Id, &r.Conversation.UserLow, &r.Conversation.UserHigh, &r.Conversation.PostId,
			&r.Conversation.LastMessageAt, &r.Conversation.LastReadId, &r.Unread,
			&last.Id, &last.ConversationId, &last.SenderId, &last.Kind, &last.Body, &last.TxHash,
			&last.AmountSfluvBase, &last.PaymentVerified, &last.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.Last = &last
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountUnreadConversations feeds the notification bell.
func (a *AppDB) CountUnreadConversations(ctx context.Context, userId string) (int, error) {
	var n int
	err := a.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM conversation_members m
		WHERE m.user_id = $1
			AND m.archived_at IS NULL
			AND EXISTS (
				SELECT 1 FROM messages x
				WHERE x.conversation_id = m.conversation_id
					AND x.id > m.last_read_message_id
					AND x.sender_id <> $1
					AND x.deleted_at IS NULL
			);
	`, userId).Scan(&n)
	return n, err
}

// GetMessages pages a thread backwards from beforeId (0 = newest).
func (a *AppDB) GetMessages(ctx context.Context, conversationId int64, beforeId int64, limit int) ([]*structs.Message, bool, error) {
	rows, err := a.db.Query(ctx, `
		SELECT `+messageColumns+`
		FROM messages x
		WHERE x.conversation_id = $1
			AND x.deleted_at IS NULL
			AND ($2 = 0 OR x.id < $2)
		ORDER BY x.id DESC
		LIMIT $3;
	`, conversationId, beforeId, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	msgs := []*structs.Message{}
	for rows.Next() {
		var m structs.Message
		if err := scanMessage(rows, &m); err != nil {
			return nil, false, err
		}
		msgs = append(msgs, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}
	return msgs, hasMore, nil
}

type NewMessage struct {
	ConversationId  int64
	SenderId        string
	Kind            string
	Body            string
	TxHash          *string
	AmountSfluvBase *string
	PaymentVerified bool
}

// InsertMessage stores a message, bumps the thread's activity time, marks it
// read for the sender, and un-archives the thread for both members so a new
// message always resurfaces it.
func (a *AppDB) InsertMessage(ctx context.Context, n *NewMessage) (*structs.Message, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var m structs.Message
	err = scanMessage(tx.QueryRow(ctx, `
		WITH x AS (
			INSERT INTO messages(conversation_id, sender_id, kind, body, tx_hash, amount_sfluv_base, payment_verified)
			VALUES ($1, $2, $3, $4, $5, $6::numeric, $7)
			RETURNING *
		)
		SELECT `+messageColumns+` FROM x;
	`, n.ConversationId, n.SenderId, n.Kind, n.Body, n.TxHash, n.AmountSfluvBase, n.PaymentVerified), &m)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrPaymentAlreadyAttached
		}
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE conversations SET last_message_at = $2 WHERE id = $1;
	`, n.ConversationId, m.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE conversation_members
		SET archived_at = NULL,
			last_read_message_id = CASE WHEN user_id = $2 THEN $3 ELSE last_read_message_id END
		WHERE conversation_id = $1;
	`, n.ConversationId, n.SenderId, m.Id); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &m, nil
}

func (a *AppDB) MarkConversationRead(ctx context.Context, conversationId int64, userId string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE conversation_members m
		SET last_read_message_id = GREATEST(
			m.last_read_message_id,
			COALESCE((SELECT MAX(id) FROM messages WHERE conversation_id = $1), 0)
		)
		WHERE m.conversation_id = $1 AND m.user_id = $2;
	`, conversationId, userId)
	return err
}

func (a *AppDB) ArchiveConversation(ctx context.Context, conversationId int64, userId string) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE conversation_members SET archived_at = NOW()
		WHERE conversation_id = $1 AND user_id = $2;
	`, conversationId, userId)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConversationNotFound
	}
	return nil
}

// GetUnverifiedPayments lists payment messages in a thread still waiting on the
// indexer, so reading a thread can retry their verification.
func (a *AppDB) GetUnverifiedPayments(ctx context.Context, conversationId int64) ([]*structs.Message, error) {
	rows, err := a.db.Query(ctx, `
		SELECT `+messageColumns+`
		FROM messages x
		WHERE x.conversation_id = $1
			AND x.kind = 'payment'
			AND x.payment_verified = FALSE
			AND x.deleted_at IS NULL
			AND x.created_at > NOW() - INTERVAL '7 days';
	`, conversationId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*structs.Message{}
	for rows.Next() {
		var m structs.Message
		if err := scanMessage(rows, &m); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (a *AppDB) MarkPaymentVerified(ctx context.Context, messageId int64, amountBase string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE messages SET payment_verified = TRUE, amount_sfluv_base = $2::numeric
		WHERE id = $1 AND kind = 'payment';
	`, messageId, amountBase)
	return err
}

// GetMessageAny loads a message regardless of membership, for reports and
// moderation only.
func (a *AppDB) GetMessageAny(ctx context.Context, messageId int64) (*structs.Message, error) {
	var m structs.Message
	err := scanMessage(a.db.QueryRow(ctx, `SELECT `+messageColumns+` FROM messages x WHERE x.id = $1;`, messageId), &m)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConversationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UserCanSeeMessage reports whether a user is a member of the message's thread.
func (a *AppDB) UserCanSeeMessage(ctx context.Context, messageId int64, userId string) (bool, error) {
	var ok bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM messages x
			JOIN conversation_members m ON m.conversation_id = x.conversation_id
			WHERE x.id = $1 AND m.user_id = $2
		);
	`, messageId, userId).Scan(&ok)
	return ok, err
}

func (a *AppDB) RemoveMessage(ctx context.Context, messageId int64) error {
	_, err := a.db.Exec(ctx, `UPDATE messages SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL;`, messageId)
	return err
}

// GetPrimaryWalletAddress is the address "Send SFLUV" pays: the user's primary
// smart wallet, the same one the rest of the app treats as theirs.
func (a *AppDB) GetPrimaryWalletAddress(ctx context.Context, userId string) (string, error) {
	var addr string
	err := a.db.QueryRow(ctx, `
		SELECT TRIM(COALESCE(primary_wallet_address, '')) FROM users WHERE id = $1 AND active = TRUE;
	`, userId).Scan(&addr)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return addr, err
}

// GetConversationPostChips loads the post summary shown at the top of threads.
// Removed posts still resolve (the thread outlives the listing) but say so.
func (a *AppDB) GetConversationPostChips(ctx context.Context, postIds []int64) (map[int64]*structs.ConversationPostChip, error) {
	out := map[int64]*structs.ConversationPostChip{}
	if len(postIds) == 0 {
		return out, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT p.id, p.title, p.category, p.price_sfluv::text, p.accepts_sfluv, p.accepts_cash,
			p.status, p.author_id,
			(SELECT ph.id FROM bulletin_post_photos ph WHERE ph.post_id = p.id ORDER BY ph.position LIMIT 1)
		FROM bulletin_posts p
		WHERE p.id = ANY($1);
	`, postIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c structs.ConversationPostChip
		if err := rows.Scan(&c.Id, &c.Title, &c.Category, &c.PriceSfluv, &c.AcceptsSfluv, &c.AcceptsCash,
			&c.Status, &c.AuthorId, &c.PhotoId); err != nil {
			return nil, err
		}
		out[c.Id] = &c
	}
	return out, rows.Err()
}

// FindConversationId returns the existing thread between two users about a post
// (or their plain thread), or 0 when there is none. Used to tell a new thread
// from a reopened one before the rate limit applies.
func (a *AppDB) FindConversationId(ctx context.Context, userA, userB string, postId *int64) (int64, error) {
	low, high := orderedPair(userA, userB)
	var id int64
	err := a.db.QueryRow(ctx, `
		SELECT id FROM conversations
		WHERE user_low = $1 AND user_high = $2 AND COALESCE(post_id, 0) = COALESCE($3, 0);
	`, low, high, postId).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return id, err
}
