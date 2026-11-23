package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/SFLuv/app/backend/structs"
)

const (
	// messageSearchPageSize is smaller than a thread page: a result list is
	// scanned by eye, row by row, and thirty is more than a screen of them.
	messageSearchPageSize = 30
	// messageSearchMinRunes is the shortest text worth searching for. One
	// letter matches most of a thread, which is a slow way to show the person
	// nothing useful.
	messageSearchMinRunes = 2
	// messageSearchMaxRunes caps the text, which can be no longer than a
	// message and still match one.
	messageSearchMaxRunes = messageMaxRunes
)

// SearchConversationMessages is GET /conversations/{id}/messages/search.
//
// A case-insensitive substring search over one thread's text, newest match
// first, paged with before exactly as the thread is. Each result is hydrated
// the way the thread hydrates it, so the app shows a result with the same
// message component it uses in the conversation and can jump to it by id.
//
// The membership check is the thread's own (loadMemberConversation), and what
// can match is the thread's set minus deleted messages — see
// db.SearchMessages. Payments are not retried here, unlike the thread's first
// page: a search is a read of what is already there.
func (a *AppService) SearchConversationMessages(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}
	before := int64(0)
	if raw := r.URL.Query().Get("before"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			bulletinError(w, http.StatusBadRequest, "bad_cursor", "That page link is no longer valid.")
			return
		}
		before = parsed
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	// Too short is an empty result, not an error: the app searches as the
	// person types, and the first keystroke is not a mistake.
	if utf8.RuneCountInString(query) < messageSearchMinRunes {
		writeJSON(w, http.StatusOK, structs.MessagesPage{Messages: []*structs.Message{}, HasMore: false})
		return
	}
	if utf8.RuneCountInString(query) > messageSearchMaxRunes {
		writeJSON(w, http.StatusOK, structs.MessagesPage{Messages: []*structs.Message{}, HasMore: false})
		return
	}

	msgs, hasMore, err := a.db.SearchMessages(r.Context(), conv.Id, query, before, messageSearchPageSize)
	if err != nil {
		a.logger.Logf("error searching messages in conversation %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	markMine(msgs, userId)
	a.hydrateMessages(r.Context(), conv, msgs, userId)
	writeJSON(w, http.StatusOK, structs.MessagesPage{Messages: msgs, HasMore: hasMore})
}
