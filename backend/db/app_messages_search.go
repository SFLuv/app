package db

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/SFLuv/app/backend/structs"
)

// encodeMentions is the JSONB a message's mentions are stored as. Nil — and
// an empty list, which means the same thing — store NULL, so a message that
// tags nobody looks exactly like every message written before mentions
// existed and nothing downstream has to tell "[]" from NULL.
func encodeMentions(mentions []structs.MessageMention) *string {
	if len(mentions) == 0 {
		return nil
	}
	raw, err := json.Marshal(mentions)
	if err != nil {
		// A slice of three plain fields cannot fail to marshal; if it somehow
		// did, losing the highlight is better than losing the message.
		return nil
	}
	encoded := string(raw)
	return &encoded
}

// decodeMentions reads the stored column back. A value that does not parse is
// treated as no mentions rather than failing the read: the column is only
// ever written by encodeMentions, and a whole thread refusing to load over one
// row's highlight would be the wrong trade if that ever stops being true.
func decodeMentions(raw *string) []structs.MessageMention {
	if raw == nil || *raw == "" {
		return nil
	}
	var mentions []structs.MessageMention
	if err := json.Unmarshal([]byte(*raw), &mentions); err != nil {
		return nil
	}
	if len(mentions) == 0 {
		return nil
	}
	return mentions
}

// escapeLikePattern makes a person's search text safe to put inside an
// ILIKE pattern, so it matches literally.
//
// Without it, searching for "50%" or "a_b" would be read as wildcards, and a
// lone "%" would match every message in the thread. The backslash is escaped
// first so the escapes added for the other two are not themselves doubled.
// Postgres's default LIKE escape character is the backslash, which is what the
// query relies on (it also says ESCAPE '\' explicitly, so a server with
// standard_conforming_strings off reads it the same way).
func escapeLikePattern(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(text)
}

// SearchMessages finds messages in one thread whose text contains query,
// newest first, paged backwards from beforeId (0 = newest) exactly like
// GetMessages.
//
// What it can return is GetMessages's set minus every deleted message. The
// thread shows a sender's own deletion as a tombstone, but a tombstone has no
// text to match and the row's kept body is for moderation only — see scanMessage
// — so matching against it would surface words the sender took back. An admin
// removal is excluded for the reason GetMessages excludes it. Nothing else in
// GetMessages narrows what a member can see (there is no join point or
// "deleted for me" cut-off on the thread read), so nothing else narrows it here.
//
// A plain substring scan, not a full-text index: it walks one conversation by
// messages_conversation_all_idx and filters, which is cheap at the size a
// single thread reaches. A trigram index is the answer if that stops being
// true.
func (a *AppDB) SearchMessages(ctx context.Context, conversationId int64, query string, beforeId int64, limit int) ([]*structs.Message, bool, error) {
	rows, err := a.db.Query(ctx, `
		SELECT `+messageColumns+`
		FROM messages x
		WHERE x.conversation_id = $1
			AND x.deleted_at IS NULL
			AND x.body ILIKE '%' || $2 || '%' ESCAPE '\'
			AND ($3 = 0 OR x.id < $3)
		ORDER BY x.id DESC
		LIMIT $4;
	`, conversationId, escapeLikePattern(query), beforeId, limit+1)
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
