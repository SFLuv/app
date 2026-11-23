package handlers

import (
	"context"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/SFLuv/app/backend/structs"
)

// messageMaxMentions is how many people one message can tag.
//
// Far above anything a person types by hand; the point is that a crafted
// request cannot make one message carry thousands of entities, each of which
// every reader's app has to lay out.
const messageMaxMentions = 50

// utf16Len is how long s is to a JavaScript string — the unit mention offsets
// are counted in. An emoji outside the Basic Multilingual Plane is one rune
// but two code units, which is exactly the case byte or rune counting gets
// wrong.
func utf16Len(s string) int {
	//
	// Ranging over a string never yields a surrogate half or an out-of-range
	// rune — invalid UTF-8 decodes as U+FFFD, one unit — so RuneLen is never
	// -1 here. The body arrives from JSON, so it is valid UTF-8 anyway.
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// normalizeMentions keeps the entries of a client's mention list that hold up,
// and drops the rest.
//
// Forgiving on purpose. A mention is decoration on a message the person has
// already written, and refusing the whole send because the app got an offset
// wrong — or because somebody left the group between being tagged and the
// message going out — would lose the message to save the highlight. So the
// message always goes, and only the entities that are actually true come with
// it.
//
// rawBody is the text as the client sent it, BEFORE the handler trims it:
// the client counted offsets against what it sent, and the server stores the
// trimmed body, so offsets are shifted by whatever leading whitespace the trim
// removed. Without that a message that started with a newline would have every
// highlight one character late.
//
// An entry is kept only if:
//   - its user is a current member (members holds exactly those);
//   - it lies inside the stored body, counted in UTF-16 code units;
//   - it does not overlap an entry kept before it (which also drops exact
//     duplicates);
//   - fewer than messageMaxMentions have been kept already.
//
// The result is sorted by offset, so a reader can walk the body once. Nil when
// nothing survives.
func normalizeMentions(requested []structs.MessageMention, rawBody string, members map[string]struct{}) []structs.MessageMention {
	if len(requested) == 0 {
		return nil
	}
	body := strings.TrimSpace(rawBody)
	if body == "" {
		return nil
	}
	shift := utf16Len(rawBody[:strings.Index(rawBody, body)])
	bodyLen := utf16Len(body)

	kept := []structs.MessageMention{}
	for _, m := range requested {
		if len(kept) >= messageMaxMentions {
			break
		}
		userId := strings.TrimSpace(m.UserId)
		if userId == "" {
			continue
		}
		if _, isMember := members[userId]; !isMember {
			continue
		}
		offset := m.Offset - shift
		if offset < 0 || m.Length <= 0 || offset+m.Length > bodyLen {
			continue
		}
		overlaps := false
		for _, k := range kept {
			if offset < k.Offset+k.Length && k.Offset < offset+m.Length {
				overlaps = true
				break
			}
		}
		if overlaps {
			continue
		}
		kept = append(kept, structs.MessageMention{UserId: userId, Offset: offset, Length: m.Length})
	}
	if len(kept) == 0 {
		return nil
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Offset < kept[j].Offset })
	return kept
}

// conversationMentions validates a request's mentions against the thread's
// current members. Skips the member lookup entirely when nothing was tagged,
// which is almost every message.
//
// A failed lookup drops the mentions rather than failing the send, for the
// reason normalizeMentions is forgiving at all.
func (a *AppService) conversationMentions(ctx context.Context, conv *structs.ConversationRow, requested []structs.MessageMention, rawBody string) []structs.MessageMention {
	if len(requested) == 0 {
		return nil
	}
	ids, err := a.db.GetConversationMemberIds(ctx, conv.Id)
	if err != nil {
		a.logger.Logf("error listing members of conversation %d for mentions: %s", conv.Id, err)
		return nil
	}
	members := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		members[id] = struct{}{}
	}
	return normalizeMentions(requested, rawBody, members)
}

// mentionedUserIds is the set of people a message tags, for the notification
// paths: a tagged member is told even when they have the thread muted.
func mentionedUserIds(msg *structs.Message) map[string]struct{} {
	if msg == nil || len(msg.Mentions) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(msg.Mentions))
	for _, m := range msg.Mentions {
		ids[m.UserId] = struct{}{}
	}
	return ids
}
