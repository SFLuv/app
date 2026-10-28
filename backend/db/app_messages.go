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
	// The unique index the conflict target names is now partial
	// (WHERE kind = 'direct'), so the inferred target has to match it — hence
	// the WHERE clause on the ON CONFLICT. Without it Postgres cannot infer the
	// index and the insert fails outright rather than conflicting.
	err = tx.QueryRow(ctx, `
		INSERT INTO conversations(kind, user_low, user_high, post_id, started_by)
		VALUES ('direct', $1, $2, $3, $4)
		ON CONFLICT (user_low, user_high, COALESCE(post_id, 0)) WHERE kind = 'direct' DO NOTHING
		RETURNING id;
	`, low, high, postId, me).Scan(&id)
	if err == nil {
		created = true
	} else if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `
			SELECT id FROM conversations
			WHERE kind = 'direct' AND user_low = $1 AND user_high = $2
				AND COALESCE(post_id, 0) = COALESCE($3, 0);
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
		UPDATE conversation_members SET archived_at = NULL, left_at = NULL
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

// conversationColumns is the thread's own row, in ConversationRow's field order.
//
// user_low and user_high are coalesced because they are nullable in a group.
// ConversationRow keeps them as plain strings, so a group reads as "" rather
// than forcing every caller through a nil check for a case it has already
// excluded by testing Kind.
const conversationColumns = `
	c.id, c.kind, c.title, c.icon_attachment_id, COALESCE(c.owner_id, ''),
	COALESCE(c.user_low, ''), COALESCE(c.user_high, ''), c.post_id, c.last_message_at`

func scanConversation(row pgx.Row, c *structs.ConversationRow) error {
	return row.Scan(&c.Id, &c.Kind, &c.Title, &c.IconAttachmentId, &c.OwnerId,
		&c.UserLow, &c.UserHigh, &c.PostId, &c.LastMessageAt, &c.LastReadId, &c.MyRole)
}

// GetConversationForMember loads a thread the caller belongs to.
//
// left_at IS NULL is part of the membership test: somebody who left a group is
// still a row in conversation_members — their messages have to keep a sender —
// but they are not a member any more and must not be able to read it.
func (a *AppDB) GetConversationForMember(ctx context.Context, conversationId int64, userId string) (*structs.ConversationRow, error) {
	var c structs.ConversationRow
	err := scanConversation(a.db.QueryRow(ctx, `
		SELECT `+conversationColumns+`, m.last_read_message_id, m.role
		FROM conversations c
		JOIN conversation_members m ON m.conversation_id = c.id AND m.user_id = $2 AND m.left_at IS NULL
		WHERE c.id = $1 AND c.deleted_at IS NULL;
	`, conversationId, userId), &c)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConversationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetConversationMemberIds lists who is currently in a thread, which is the
// fan-out set for both pushes and realtime events.
func (a *AppDB) GetConversationMemberIds(ctx context.Context, conversationId int64) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT user_id FROM conversation_members
		WHERE conversation_id = $1 AND left_at IS NULL;
	`, conversationId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
	x.amount_sfluv_base::text, x.payment_verified, x.created_at, x.reply_to_id,
	x.edited_at, (x.deleted_at IS NOT NULL AND x.deleted_by IS NOT NULL)`

// scanMessage reads a message and, if it was deleted by its sender, blanks
// everything that was in it.
//
// The blanking is HERE rather than in each handler on purpose. The row keeps
// its text so a report filed just before the delete still has something to show
// an admin (see the 1.75 migration), which means every member-facing read is
// one forgotten line away from serving deleted content. Doing it in the single
// function they all go through is the only version of that which stays true as
// handlers are added.
func scanMessage(row pgx.Row, m *structs.Message) error {
	if err := row.Scan(&m.Id, &m.ConversationId, &m.SenderId, &m.Kind, &m.Body, &m.TxHash,
		&m.AmountSfluvBase, &m.PaymentVerified, &m.CreatedAt, &m.ReplyToId,
		&m.EditedAt, &m.Deleted); err != nil {
		return err
	}
	if m.Deleted {
		m.Body = ""
		m.TxHash = nil
		m.AmountSfluvBase = nil
		m.PaymentVerified = false
		// The attachments of a deleted message are never loaded — the hydration
		// step skips it — so there is nothing to clear there.
	}
	return nil
}

type InboxRow struct {
	Conversation structs.ConversationRow
	Unread       int
	Last         *structs.Message
	MemberCount  int
	// The newest reaction in the thread, whether or not it is newer than the
	// last message. The handler decides whether it is the thing worth showing;
	// comparing here would put a display decision in the query.
	LatestReaction *structs.ConversationReaction
	// Who reacted, so the handler can mark the reader's own.
	LatestReactionUserId string
	// Whether this member has the thread muted right now.
	Muted bool
	// How far the OTHER members have read, for the sender's "Read" state.
	OtherLastReadId int64
}

// ListConversations is the inbox: threads that have at least one message,
// newest activity first, with the latest message and the unread count.
func (a *AppDB) ListConversations(ctx context.Context, userId string) ([]InboxRow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			`+conversationColumns+`, m.last_read_message_id, m.role,
			(
				SELECT COUNT(*) FROM messages u
				WHERE u.conversation_id = c.id
					AND u.id > m.last_read_message_id
					AND u.sender_id <> $1
					AND u.deleted_at IS NULL
			),
			(
				SELECT COUNT(*) FROM conversation_members cm
				WHERE cm.conversation_id = c.id AND cm.left_at IS NULL
			),
			lm.id, lm.conversation_id, lm.sender_id, lm.kind, lm.body, lm.tx_hash,
			lm.amount_sfluv_base::text, lm.payment_verified, lm.created_at, lm.reply_to_id,
			lr.emoji, lr.reactor_id, lr.reacted_at, lr.excerpt,
			COALESCE(m.muted_until > NOW(), FALSE),
			COALESCE((
				SELECT MIN(o.last_read_message_id) FROM conversation_members o
				WHERE o.conversation_id = c.id AND o.user_id <> $1 AND o.left_at IS NULL
			), 0)
		FROM conversation_members m
		JOIN conversations c ON c.id = m.conversation_id
		JOIN LATERAL (
			SELECT * FROM messages x
			WHERE x.conversation_id = c.id AND x.deleted_at IS NULL
			ORDER BY x.id DESC
			LIMIT 1
		) lm ON TRUE
		-- The newest reaction in the thread, so the inbox can say "X reacted to
		-- …" when that is genuinely the latest activity. A reaction is not a
		-- message, so it cannot come from the lateral above.
		--
		-- LEFT JOIN: a thread with no reactions is the common case and must not
		-- be dropped from the inbox. Bounded by LIMIT 1 and driven by
		-- message_reactions_message_idx through the join on message_id.
		LEFT JOIN LATERAL (
			SELECT r.emoji, r.user_id AS reactor_id, r.created_at AS reacted_at,
				LEFT(COALESCE(NULLIF(rx.body, ''), ''), 80) AS excerpt
			FROM message_reactions r
			JOIN messages rx ON rx.id = r.message_id
			WHERE rx.conversation_id = c.id AND rx.deleted_at IS NULL
			ORDER BY r.created_at DESC
			LIMIT 1
		) lr ON TRUE
		WHERE m.user_id = $1
			AND m.archived_at IS NULL
			AND m.left_at IS NULL
			-- Deleted for everybody. A soft delete, so the rows are still here
			-- for moderation; they are simply not anybody's conversation now.
			AND c.deleted_at IS NULL
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
		// Nullable because the reaction lateral is a LEFT JOIN.
		var emoji, reactorId, excerpt *string
		var reactedAt *time.Time
		c := &r.Conversation
		if err := rows.Scan(
			&c.Id, &c.Kind, &c.Title, &c.IconAttachmentId, &c.OwnerId,
			&c.UserLow, &c.UserHigh, &c.PostId, &c.LastMessageAt,
			&c.LastReadId, &c.MyRole, &r.Unread, &r.MemberCount,
			&last.Id, &last.ConversationId, &last.SenderId, &last.Kind, &last.Body, &last.TxHash,
			&last.AmountSfluvBase, &last.PaymentVerified, &last.CreatedAt, &last.ReplyToId,
			&emoji, &reactorId, &reactedAt, &excerpt, &r.Muted, &r.OtherLastReadId,
		); err != nil {
			return nil, err
		}
		r.Last = &last
		if emoji != nil && reactedAt != nil {
			r.LatestReactionUserId = derefString(reactorId)
			r.LatestReaction = &structs.ConversationReaction{
				Emoji:          *emoji,
				MessageExcerpt: derefString(excerpt),
				CreatedAt:      *reactedAt,
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountUnreadConversations feeds the notification bell.
func (a *AppDB) CountUnreadConversations(ctx context.Context, userId string) (int, error) {
	var n int
	err := a.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM conversation_members m
		JOIN conversations c ON c.id = m.conversation_id AND c.deleted_at IS NULL
		WHERE m.user_id = $1
			AND m.archived_at IS NULL
			AND m.left_at IS NULL
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
	// Sender deletions come back as tombstones; admin removals do not come back
	// at all.
	//
	// The distinction is the point of deleted_by. Somebody deleting their own
	// message should leave "Message was deleted" so the thread still makes
	// sense — especially where a reply answers it. An admin removing reported
	// content should leave nothing, because a gap that announces itself tells
	// everyone in the thread that something was actioned, which is the
	// reported person's business and nobody else's.
	rows, err := a.db.Query(ctx, `
		SELECT `+messageColumns+`
		FROM messages x
		WHERE x.conversation_id = $1
			AND (x.deleted_at IS NULL OR x.deleted_by IS NOT NULL)
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
	ReplyToId       *int64
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
			INSERT INTO messages(conversation_id, sender_id, kind, body, tx_hash, amount_sfluv_base, payment_verified, reply_to_id)
			VALUES ($1, $2, $3, $4, $5, $6::numeric, $7, (
				-- A reply may only quote a message in the same thread. Resolved
				-- here rather than validated in the handler so there is no
				-- window between the check and the insert, and so a quote of a
				-- message in somebody else's conversation becomes a plain
				-- message rather than an error — the reply is still what the
				-- person meant to send.
				SELECT r.id FROM messages r
				WHERE r.id = $8 AND r.conversation_id = $1 AND r.deleted_at IS NULL
			))
			RETURNING *
		)
		SELECT `+messageColumns+` FROM x;
	`, n.ConversationId, n.SenderId, n.Kind, n.Body, n.TxHash, n.AmountSfluvBase, n.PaymentVerified, n.ReplyToId), &m)
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
	// A new message resurfaces a thread somebody archived, but must not drag
	// back in somebody who left a group: left_at IS NULL is what keeps "I left
	// this group" from being undone by the next person to post in it.
	if _, err := tx.Exec(ctx, `
		UPDATE conversation_members
		SET archived_at = NULL,
			last_read_message_id = CASE WHEN user_id = $2 THEN $3 ELSE last_read_message_id END
		WHERE conversation_id = $1 AND left_at IS NULL;
	`, n.ConversationId, n.SenderId, m.Id); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &m, nil
}

// EditWindow is how long a sender has to change what they said.
//
// Five minutes: long enough to fix a typo or a wrong number, short enough that
// nobody can rewrite a conversation somebody has already acted on. An edit is
// also marked, so the other person can see that the words changed.
const EditWindow = 5 * time.Minute

// ErrEditWindowClosed means the message is too old to edit.
var ErrEditWindowClosed = errors.New("too late to edit that message")

// ErrNotMyMessage means the caller did not send it.
var ErrNotMyMessage = errors.New("not your message")

// EditMessage changes a message's text, within the window and only for its
// sender.
//
// Every condition is in the WHERE clause rather than checked first and written
// second: the window is a deadline, and a check-then-write leaves a gap in
// which it can pass. The distinct error cases are told apart afterwards by
// re-reading, which only happens on the failure path.
func (a *AppDB) EditMessage(ctx context.Context, messageId int64, userId string, body string) (*structs.Message, error) {
	var m structs.Message
	err := scanMessage(a.db.QueryRow(ctx, `
		WITH x AS (
			UPDATE messages
			SET body = $3, edited_at = NOW()
			WHERE id = $1
				AND sender_id = $2
				AND deleted_at IS NULL
				-- Text and attachment captions only. There is nothing editable
				-- about a payment (the amount is on a chain) or a system line
				-- (nobody wrote it).
				AND kind IN ('text', 'attachment')
				AND created_at > NOW() - $4::interval
			RETURNING *
		)
		SELECT `+messageColumns+` FROM x;
	`, messageId, userId, body, EditWindow.String()), &m)
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing matched. Work out which rule it broke, so the person is told
		// something they can act on.
		var senderId string
		var createdAt time.Time
		var deleted bool
		var kind string
		if lookupErr := a.db.QueryRow(ctx, `
			SELECT sender_id, created_at, deleted_at IS NOT NULL, kind FROM messages WHERE id = $1;
		`, messageId).Scan(&senderId, &createdAt, &deleted, &kind); lookupErr != nil {
			return nil, ErrConversationNotFound
		}
		if senderId != userId {
			return nil, ErrNotMyMessage
		}
		if deleted || (kind != "text" && kind != "attachment") {
			return nil, ErrNotMyMessage
		}
		return nil, ErrEditWindowClosed
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DeleteOwnMessage removes a message for everybody, leaving a tombstone.
//
// deleted_by records that this was the sender's own doing, which is what makes
// the read path return it as "Message was deleted" rather than hiding it the way
// an admin removal is hidden.
func (a *AppDB) DeleteOwnMessage(ctx context.Context, messageId int64, userId string) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE messages
		SET deleted_at = NOW(), deleted_by = $2
		WHERE id = $1 AND sender_id = $2 AND deleted_at IS NULL;
	`, messageId, userId)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotMyMessage
	}
	return nil
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

// UserCanSeeMessage reports whether a user is currently in the message's thread.
//
// left_at IS NULL is load-bearing, for the reason given on the attachment read:
// a membership row outlives the membership, so without it somebody removed from
// a group would still pass this check for every message in it.
func (a *AppDB) UserCanSeeMessage(ctx context.Context, messageId int64, userId string) (bool, error) {
	var ok bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM messages x
			JOIN conversation_members m ON m.conversation_id = x.conversation_id
			WHERE x.id = $1 AND m.user_id = $2 AND m.left_at IS NULL
		);
	`, messageId, userId).Scan(&ok)
	return ok, err
}

// MessageIsInConversation reports whether a message belongs to a particular
// thread.
//
// Needed alongside UserCanSeeMessage because the two answer different
// questions, and only asking the first one leaves a gap: a person in
// conversation A, acting on a message id that belongs to conversation B, passes
// "are you in this message's thread" whenever they are also in B. The reaction
// endpoint takes a conversation from its path and a message id from the same
// path, and nothing otherwise forces the two to agree.
func (a *AppDB) MessageIsInConversation(ctx context.Context, messageId int64, conversationId int64) (bool, error) {
	var ok bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM messages WHERE id = $1 AND conversation_id = $2 AND deleted_at IS NULL
		);
	`, messageId, conversationId).Scan(&ok)
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
		WHERE kind = 'direct' AND user_low = $1 AND user_high = $2
			AND COALESCE(post_id, 0) = COALESCE($3, 0);
	`, low, high, postId).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return id, err
}
