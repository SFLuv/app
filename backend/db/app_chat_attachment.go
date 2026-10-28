package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// ErrAttachmentNotFound covers "no such attachment" and "not yours to read".
var ErrAttachmentNotFound = errors.New("attachment not found")

// NewAttachment is one card to store under a message.
//
// A single struct for all seven kinds, matching the single table. The caller
// sets the fields its kind uses; the CHECK constraint in the 1.74 migration is
// what actually enforces that, so a bug here fails loudly at the insert rather
// than storing a card with nothing in it.
type NewAttachment struct {
	Kind     string
	Position int

	ContentType string
	FileName    string
	FileData    []byte
	SizeBytes   *int
	ImageWidth  *int
	ImageHeight *int

	LinkURL         string
	LinkTitle       string
	LinkDescription string
	LinkImageURL    string
	LinkSiteName    string

	TxHash           *string
	LocationId       *int
	EventId          *string
	SupportRequestId *int64
}

// InsertMessageWithAttachments writes a message and its cards in one
// transaction, so a thread never shows a message whose attachment is still
// being written — or worse, one whose attachment insert failed.
func (a *AppDB) InsertMessageWithAttachments(ctx context.Context, n *NewMessage, attachments []NewAttachment) (*structs.Message, error) {
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
				SELECT r.id FROM messages r
				WHERE r.id = $8 AND r.conversation_id = $1 AND r.deleted_at IS NULL
			))
			RETURNING *
		)
		SELECT `+messageColumns+` FROM x;
	`, n.ConversationId, n.SenderId, n.Kind, n.Body, n.TxHash, n.AmountSfluvBase,
		n.PaymentVerified, n.ReplyToId), &m)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrPaymentAlreadyAttached
		}
		return nil, err
	}

	for i := range attachments {
		at := &attachments[i]
		if err := insertAttachment(ctx, tx, &m.Id, at); err != nil {
			return nil, err
		}
	}

	// GREATEST for the reason given in InsertMessage: overlapping sends commit
	// out of timestamp order, and a plain set can move the thread backwards.
	if _, err := tx.Exec(ctx, `
		UPDATE conversations SET last_message_at = GREATEST(last_message_at, $2) WHERE id = $1;
	`, n.ConversationId, m.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE conversation_members
		SET archived_at = NULL,
			-- GREATEST for the reason last_message_at uses it: two overlapping
			-- sends commit in either order, and the later commit must not move
			-- the sender's read pointer back to the earlier message.
			last_read_message_id = CASE WHEN user_id = $2 THEN GREATEST(last_read_message_id, $3) ELSE last_read_message_id END
		WHERE conversation_id = $1 AND left_at IS NULL;
	`, n.ConversationId, n.SenderId, m.Id); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	stored, err := a.GetMessageAttachments(ctx, []int64{m.Id})
	if err == nil {
		m.Attachments = stored[m.Id]
	}
	return &m, nil
}

type attachmentExecer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func insertAttachment(ctx context.Context, q attachmentExecer, messageId *int64, at *NewAttachment) error {
	var id int64
	return q.QueryRow(ctx, `
		INSERT INTO message_attachments(
			message_id, position, kind,
			content_type, file_name, file_data, size_bytes, image_width, image_height,
			link_url, link_title, link_description, link_image_url, link_site_name,
			tx_hash, location_id, event_id, support_request_id
		) VALUES (
			$1, $2, $3,
			$4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14,
			$15, $16, $17, $18
		) RETURNING id;
	`,
		messageId, at.Position, at.Kind,
		at.ContentType, at.FileName, at.FileData, at.SizeBytes, at.ImageWidth, at.ImageHeight,
		at.LinkURL, at.LinkTitle, at.LinkDescription, at.LinkImageURL, at.LinkSiteName,
		at.TxHash, at.LocationId, at.EventId, at.SupportRequestId,
	).Scan(&id)
}

// InsertStandaloneAttachment stores bytes with no message, for a group icon.
func (a *AppDB) InsertStandaloneAttachment(ctx context.Context, at *NewAttachment) (int64, error) {
	var id int64
	err := a.db.QueryRow(ctx, `
		INSERT INTO message_attachments(
			message_id, position, kind, content_type, file_name, file_data, size_bytes, image_width, image_height
		) VALUES (NULL, 0, $1, $2, $3, $4, $5, $6, $7)
		RETURNING id;
	`, at.Kind, at.ContentType, at.FileName, at.FileData, at.SizeBytes, at.ImageWidth, at.ImageHeight).Scan(&id)
	return id, err
}

// attachmentColumns is every stored field, in the order loadAttachment reads.
const attachmentColumns = `
	a.id, a.kind, a.position,
	a.content_type, a.file_name, a.size_bytes, a.image_width, a.image_height,
	a.link_url, a.link_title, a.link_description, a.link_image_url, a.link_site_name,
	a.tx_hash, a.location_id, a.event_id, a.support_request_id`

// GetMessageAttachments loads the cards for a page of messages in one query.
//
// Returns metadata only — never file_data. Bytes are fetched one at a time by
// the serving route, because a page of twenty messages with a photo each would
// otherwise be eighty megabytes of JSON.
func (a *AppDB) GetMessageAttachments(ctx context.Context, messageIds []int64) (map[int64][]structs.MessageAttachment, error) {
	out := map[int64][]structs.MessageAttachment{}
	if len(messageIds) == 0 {
		return out, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT a.message_id, `+attachmentColumns+`
		FROM message_attachments a
		WHERE a.message_id = ANY($1)
		ORDER BY a.message_id, a.position, a.id;
	`, messageIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var messageId int64
		var at structs.MessageAttachment
		var txHash, eventId *string
		if err := rows.Scan(
			&messageId, &at.Id, &at.Kind, &at.Position,
			&at.ContentType, &at.FileName, &at.SizeBytes, &at.ImageWidth, &at.ImageHeight,
			&at.LinkURL, &at.LinkTitle, &at.LinkDescription, &at.LinkImageURL, &at.LinkSiteName,
			&txHash, &at.LocationId, &eventId, &at.SupportRequestId,
		); err != nil {
			return nil, err
		}
		if txHash != nil {
			at.TxHash = *txHash
		}
		if eventId != nil {
			at.EventId = *eventId
		}
		out[messageId] = append(out[messageId], at)
	}
	return out, rows.Err()
}

// AttachmentBlob is the bytes of one attachment, for the serving route.
type AttachmentBlob struct {
	ContentType string
	FileName    string
	Data        []byte
}

// GetAttachmentBlobForMember reads an attachment's bytes, but only for somebody
// in the conversation that holds it.
//
// Membership is part of the query rather than a separate check: an attachment id
// is a bare integer, so guessable, and a two-step check is a place for the two
// steps to disagree. A group icon has no message and therefore no conversation
// to check — it is readable by any member of the group that points at it.
func (a *AppDB) GetAttachmentBlobForMember(ctx context.Context, attachmentId int64, userId string) (*AttachmentBlob, error) {
	var blob AttachmentBlob
	err := a.db.QueryRow(ctx, `
		SELECT a.content_type, a.file_name, a.file_data
		FROM message_attachments a
		LEFT JOIN messages x ON x.id = a.message_id
		WHERE a.id = $1
			AND a.file_data IS NOT NULL
			AND (
				EXISTS (
					SELECT 1 FROM conversation_members m
					WHERE m.conversation_id = x.conversation_id
						AND m.user_id = $2
						-- left_at matters here: somebody removed from a group
						-- keeps their membership ROW, because their messages
						-- have to keep a sender. Without this clause that row
						-- would go on authorising reads of the group's files
						-- after they were removed from it.
						AND m.left_at IS NULL
				)
				OR EXISTS (
					SELECT 1 FROM conversations c
					JOIN conversation_members m ON m.conversation_id = c.id AND m.user_id = $2 AND m.left_at IS NULL
					WHERE c.icon_attachment_id = a.id
				)
			);
	`, attachmentId, userId).Scan(&blob.ContentType, &blob.FileName, &blob.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAttachmentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &blob, nil
}

// ConversationFile is one row of the info screen's file list.
type ConversationFile struct {
	Attachment structs.MessageAttachment
	MessageId  int64
	SenderId   string
	CreatedAt  int64
}

// SearchConversationFiles lists the photos and files shared in a thread,
// newest first, optionally narrowed by filename.
func (a *AppDB) SearchConversationFiles(ctx context.Context, conversationId int64, query string, limit int) ([]ConversationFile, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	var pattern any
	if query != "" {
		pattern = "%" + strings.ToLower(query) + "%"
	}

	rows, err := a.db.Query(ctx, `
		SELECT `+attachmentColumns+`, x.id, x.sender_id, x.created_at
		FROM message_attachments a
		JOIN messages x ON x.id = a.message_id
		WHERE x.conversation_id = $1
			AND x.deleted_at IS NULL
			AND a.kind IN ('image', 'file')
			AND ($2::text IS NULL OR LOWER(a.file_name) LIKE $2)
		ORDER BY x.id DESC
		LIMIT $3;
	`, conversationId, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ConversationFile{}
	for rows.Next() {
		var f ConversationFile
		at := &f.Attachment
		var txHash, eventId *string
		var created time.Time
		if err := rows.Scan(
			&at.Id, &at.Kind, &at.Position,
			&at.ContentType, &at.FileName, &at.SizeBytes, &at.ImageWidth, &at.ImageHeight,
			&at.LinkURL, &at.LinkTitle, &at.LinkDescription, &at.LinkImageURL, &at.LinkSiteName,
			&txHash, &at.LocationId, &eventId, &at.SupportRequestId,
			&f.MessageId, &f.SenderId, &created,
		); err != nil {
			return nil, err
		}
		f.CreatedAt = created.Unix()
		out = append(out, f)
	}
	return out, rows.Err()
}

// ── Reactions ────────────────────────────────────────────────────────────────

// ToggleReaction adds an emoji to a message or takes it away, and reports which
// it did.
//
// A single statement for each direction rather than a read then a write: two
// taps in flight at once would otherwise both see "not reacted" and the second
// insert would fail on the primary key. Here the delete reports whether there
// was anything to delete, and the insert is a no-op on conflict, so either
// order of two taps ends in a consistent state.
func (a *AppDB) ToggleReaction(ctx context.Context, messageId int64, userId string, emoji string) (added bool, err error) {
	tag, err := a.db.Exec(ctx, `
		DELETE FROM message_reactions
		WHERE message_id = $1 AND user_id = $2 AND emoji = $3;
	`, messageId, userId, emoji)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return false, nil
	}

	if _, err := a.db.Exec(ctx, `
		INSERT INTO message_reactions(message_id, user_id, emoji)
		VALUES ($1, $2, $3)
		ON CONFLICT (message_id, user_id, emoji) DO NOTHING;
	`, messageId, userId, emoji); err != nil {
		return false, err
	}
	return true, nil
}

// GetMessageReactions loads reactions for a page of messages, aggregated.
//
// The reader's own id decides Mine, which is what makes the chip a toggle in the
// client without a second request.
func (a *AppDB) GetMessageReactions(ctx context.Context, messageIds []int64, readerId string) (map[int64][]structs.MessageReaction, error) {
	out := map[int64][]structs.MessageReaction{}
	if len(messageIds) == 0 {
		return out, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			r.message_id,
			r.emoji,
			COUNT(*),
			BOOL_OR(r.user_id = $2),
			-- Up to five names for the long-press list. Public names only: this
			-- is the same information the bubble's author line already carries.
			(ARRAY_AGG(COALESCE(NULLIF(TRIM(p.public_name), ''), 'Someone') ORDER BY r.created_at))[1:5]
		FROM message_reactions r
		LEFT JOIN user_profiles p ON p.user_id = r.user_id
		WHERE r.message_id = ANY($1)
		GROUP BY r.message_id, r.emoji
		ORDER BY r.message_id, COUNT(*) DESC, r.emoji;
	`, messageIds, readerId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var messageId int64
		var reaction structs.MessageReaction
		if err := rows.Scan(&messageId, &reaction.Emoji, &reaction.Count, &reaction.Mine, &reaction.Names); err != nil {
			return nil, err
		}
		out[messageId] = append(out[messageId], reaction)
	}
	return out, rows.Err()
}

// GetMessageQuotes loads the quoted slice of each replied-to message.
//
// Takes the reply targets' ids, not the replies'. Anything missing from the
// result has been deleted, and the reply renders without its quote.
func (a *AppDB) GetMessageQuotes(ctx context.Context, messageIds []int64) (map[int64]*structs.MessageQuote, error) {
	out := map[int64]*structs.MessageQuote{}
	if len(messageIds) == 0 {
		return out, nil
	}

	// A sender-deleted message still returns a quote, marked, so the reply that
	// answers it keeps its quote block and says the subject is gone. An
	// admin-removed one returns nothing, for the same reason it is hidden from
	// the thread.
	rows, err := a.db.Query(ctx, `
		SELECT
			x.id, x.sender_id,
			COALESCE(NULLIF(TRIM(p.public_name), ''), ''),
			x.kind,
			-- Blanked for a deleted message: the client shows its own wording and
			-- must never be handed the original text to fall back on.
			CASE WHEN x.deleted_at IS NOT NULL THEN '' ELSE LEFT(x.body, 160) END,
			COALESCE((
				SELECT a.kind FROM message_attachments a
				WHERE a.message_id = x.id
				ORDER BY a.position, a.id
				LIMIT 1
			), ''),
			(x.deleted_at IS NOT NULL)
		FROM messages x
		LEFT JOIN user_profiles p ON p.user_id = x.sender_id
		WHERE x.id = ANY($1)
			AND (x.deleted_at IS NULL OR x.deleted_by IS NOT NULL);
	`, messageIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var q structs.MessageQuote
		if err := rows.Scan(&q.Id, &q.SenderId, &q.SenderName, &q.Kind, &q.Body,
			&q.AttachmentKind, &q.Deleted); err != nil {
			return nil, err
		}
		quote := q
		out[q.Id] = &quote
	}
	return out, rows.Err()
}

// GetSenderNames maps user ids to public names, for group bubbles.
func (a *AppDB) GetSenderNames(ctx context.Context, userIds []string) (map[string]string, error) {
	out := map[string]string{}
	if len(userIds) == 0 {
		return out, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT user_id, COALESCE(NULLIF(TRIM(public_name), ''), '')
		FROM user_profiles WHERE user_id = ANY($1);
	`, userIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}
