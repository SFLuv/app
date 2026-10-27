package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
)

// Answering a support request in chat.
//
// A support request from the website is an email thread: somebody writes in, we
// reply by email, they reply, and every round trip is an hour. A request from
// inside the app is from an account we can reach directly, and the fastest thing
// for both ends is to open a chat.
//
// What the button does, in one call, so a staff member never has to find the
// person in a search box and hope they picked the right one:
//
//  1. Opens — or reopens — the direct thread with the sender.
//  2. Posts the ticket as a reference card, so both ends can see which request
//     they are talking about. That is a real message in the thread, which means
//     the sender's phone says somebody is looking at their problem.
//  3. Returns a draft opening line for the staff member to edit before sending.
//
// Step 3 is deliberately a draft rather than a sent message. The card says "we
// have your request"; what we actually say about it is a person's words, and
// auto-sending a template under a staff member's name would make a human reply
// indistinguishable from a robot one.

// OpenSupportConversationResponse is what the button gets back.
type OpenSupportConversationResponse struct {
	ConversationId int64  `json:"conversation_id"`
	UserId         string `json:"user_id"`
	// The opening line, prefilled in the composer and editable before sending.
	Draft string `json:"draft"`
	// The reference card's message, already in the thread.
	Message *structs.Message `json:"message,omitempty"`
}

// OpenSupportConversation is POST /admin/support-requests/{id}/conversation.
//
// Admin-only: it is the staff half of the reply flow, and it posts a message
// into somebody's inbox attributed to whoever pressed the button.
func (a *AppService) OpenSupportConversation(w http.ResponseWriter, r *http.Request) {
	staffId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}

	requestId, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || requestId <= 0 {
		bulletinError(w, http.StatusNotFound, "not_found", "That support request could not be found.")
		return
	}

	ctx := r.Context()
	request, err := a.db.GetSupportRequest(ctx, requestId)
	if err != nil {
		a.logger.Logf("error loading support request %d: %s", requestId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if request == nil {
		bulletinError(w, http.StatusNotFound, "not_found", "That support request could not be found.")
		return
	}
	if request.UserId == nil || strings.TrimSpace(*request.UserId) == "" {
		bulletinError(w, http.StatusBadRequest, "no_account",
			"That request came from the website, so there's no account to message. Reply by email instead.")
		return
	}
	senderId := strings.TrimSpace(*request.UserId)
	if senderId == staffId {
		bulletinError(w, http.StatusBadRequest, "own_request", "That's your own request.")
		return
	}

	// Both ends need chat for the thread to be any use. Checked at press time
	// rather than relying on the flag written when the alert was raised, which
	// may be an hour stale by now.
	if !a.BetaFeatureAllowed(ctx, senderId, structs.BetaFeatureChat) {
		bulletinError(w, http.StatusConflict, "no_chat",
			"They can't receive messages right now. Reply by email instead.")
		return
	}
	if !a.BetaFeatureAllowed(ctx, staffId, structs.BetaFeatureChat) {
		bulletinError(w, http.StatusConflict, "no_chat",
			"Your account doesn't have messaging turned on yet.")
		return
	}
	// A staff member needs a public profile before they can message anybody,
	// the same as everyone else — a reply from a nameless account would be worse
	// than an email.
	if !a.requireBulletinParticipant(w, r, staffId) {
		return
	}

	conv, _, err := a.db.GetOrCreateConversation(ctx, staffId, senderId, nil)
	if err != nil {
		a.logger.Logf("error opening a support conversation with %s: %s", senderId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg, err := a.db.InsertMessageWithAttachments(ctx, &db.NewMessage{
		ConversationId: conv.Id,
		SenderId:       staffId,
		Kind:           structs.MessageKindAttachment,
	}, []db.NewAttachment{{
		Kind:             structs.AttachmentKindSupport,
		SupportRequestId: &request.Id,
	}})
	if err != nil {
		a.logger.Logf("error attaching support request %d to conversation %d: %s", request.Id, conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg.IsMine = true
	a.hydrateMessages(ctx, conv, []*structs.Message{msg}, staffId)
	a.fanOutMessage(ctx, conv, msg, staffId, "About your message to SFLuv")

	writeJSON(w, http.StatusOK, OpenSupportConversationResponse{
		ConversationId: conv.Id,
		UserId:         senderId,
		Draft:          supportReplyDraft(ctx, a, staffId, request),
		Message:        msg,
	})
}

// supportReplyDraft composes the opening line a staff member sees in the
// composer.
//
// Written as a person would write it, with the ticket reference out of the way
// in the card above rather than quoted into the text — the card is already
// saying which request this is, and repeating it in the message body is how a
// reply starts reading like an auto-responder.
//
// The sender's first name is used when they gave one. Deliberately the name they
// typed into the form rather than their public profile name: it is what they
// chose to call themselves when writing to us.
func supportReplyDraft(ctx context.Context, a *AppService, staffId string, request *structs.SupportRequest) string {
	greeting := "Hi"
	if first := firstName(request.Name); first != "" {
		greeting = "Hi " + first
	}

	subject := strings.TrimSpace(request.Subject)
	about := "your message"
	if subject != "" && subject != "(no subject)" {
		about = "your message about " + strings.ToLower(trimTrailingPunctuation(subject))
	}

	staffName := a.publicName(ctx, staffId)
	if staffName == "Someone" {
		staffName = ""
	}

	draft := greeting + " — thanks for getting in touch about " + about + ". I'm looking at it now"
	if staffName != "" {
		draft += ".\n\n— " + staffName + ", SFLuv"
	} else {
		draft += "."
	}
	return draft
}

// firstName is the first word of a name, for a greeting.
func firstName(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) == 0 {
		return ""
	}
	// An email address in the name field is not a name. The website form allows
	// it, so this guards against "Hi jane@example.com".
	if strings.ContainsRune(fields[0], '@') {
		return ""
	}
	return fields[0]
}

// trimTrailingPunctuation drops a trailing full stop or similar, so a subject
// can be folded into a sentence.
func trimTrailingPunctuation(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), ".!?,;:")
}
