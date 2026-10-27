package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/realtime"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils/middleware"
)

// The realtime side of chat: who gets told what, and when a push is suppressed
// because a socket already carried it.
//
// Every publisher here is best-effort and nil-safe. A server running without a
// hub, or a hub with nobody connected, behaves exactly as the backend did before
// this existed — clients poll and pushes arrive. That is the property that lets
// these calls be sprinkled through handlers without a guard at each one.

// ServeRealtime is GET /ws.
//
// Deliberately NOT behind withChatBeta, even though chat is what mostly uses it.
// Two reasons:
//
//  1. One of the events it carries is "your beta access changed". A socket gated
//     on holding the beta could never deliver the event that says you now hold
//     it, so a person granted access would still have to relaunch — which is the
//     exact thing this is here to fix.
//  2. It carries notifications that have nothing to do with chat.
//
// The socket is still authenticated: no frame is delivered until the client has
// presented a valid token, and every event is addressed to a user id. Holding a
// socket without the chat beta gets you your own non-chat events and nothing
// else, because nothing will ever publish a chat event to you.
func (a *AppService) ServeRealtime(w http.ResponseWriter, r *http.Request) {
	if a.realtimeServer == nil {
		// No hub wired. 503 rather than 404, so a client can tell "this build
		// has no socket" from "wrong URL" and stop retrying either way.
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	a.realtimeServer.Serve(w, r)
}

// BuildRealtimeServer constructs the websocket endpoint for this service.
//
// Called once at startup. The token validator and the typing handler are passed
// in here rather than imported by the realtime package, so that package has no
// dependency on auth or on the database.
func (a *AppService) BuildRealtimeServer(hub *realtime.Hub, allowAnyOrigin bool) {
	a.realtime = hub
	a.realtimeServer = realtime.NewServer(
		hub,
		middleware.ValidatePrivyToken,
		a.handleTypingSignal,
		a.logger.Logf,
		allowAnyOrigin,
	)
}

// handleTypingSignal relays "X is typing" to the rest of a conversation.
//
// Membership is checked here, not in the realtime package, which has no
// database: a client can send any conversation id it likes, and a person who is
// not in a thread must not be able to make a typing bubble appear in it. The
// check is also what resolves the id to a member list to fan out to.
//
// Nothing is persisted. A typing signal that arrives for a socket that drops
// mid-word leaves a bubble the client times out on its own — which is why the
// client expires them rather than waiting for a "stopped typing" that may never
// come.
func (a *AppService) handleTypingSignal(userId string, conversationId int64, typing bool) {
	if a.realtime == nil {
		return
	}
	// Short timeout: this is a cosmetic signal on a hot path, and a slow query
	// here would hold a websocket read loop.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conv, err := a.db.GetConversationForMember(ctx, conversationId, userId)
	if err != nil {
		return
	}

	members, err := a.db.GetConversationMemberIds(ctx, conv.Id)
	if err != nil {
		return
	}

	name := ""
	if names, err := a.db.GetSenderNames(ctx, []string{userId}); err == nil {
		name = names[userId]
	}

	a.realtime.PublishExcept(members, userId, realtime.Event{
		Kind:           realtime.EventTyping,
		ConversationId: conv.Id,
		Data: structs.TypingEvent{
			ConversationId: conv.Id,
			UserId:         userId,
			UserName:       name,
			Typing:         typing,
		},
	})
}

// ── Publishers ───────────────────────────────────────────────────────────────

// publishMessage tells a conversation's members about a new message, and reports
// which of them were reached.
//
// The returned set is what suppresses pushes: a member who received this over a
// socket has already been told, and sending them a system banner for the same
// message — over the chat they are looking at — is the thing this whole
// mechanism exists to stop.
//
// The sender is excluded from the fan-out but included in the "already told"
// set, since they are the one who sent it.
func (a *AppService) publishMessage(ctx context.Context, conv *structs.ConversationRow, message *structs.Message, senderId string) map[string]struct{} {
	reached := map[string]struct{}{senderId: {}}
	if a.realtime == nil {
		return reached
	}

	members, err := a.db.GetConversationMemberIds(ctx, conv.Id)
	if err != nil {
		return reached
	}

	online := a.realtime.OnlineAmong(members)
	for id := range online {
		reached[id] = struct{}{}
	}

	// A copy per recipient, because IsMine is relative to who is reading it and
	// the stored message does not know. Cheap: this is one small struct.
	for _, id := range members {
		if id == senderId {
			continue
		}
		if _, isOnline := online[id]; !isOnline {
			continue
		}
		forThem := *message
		forThem.IsMine = false
		a.realtime.Publish([]string{id}, realtime.Event{
			Kind:           realtime.EventMessageNew,
			ConversationId: conv.Id,
			Data:           &forThem,
		})
	}
	return reached
}

// publishConversationUpdated tells members that a thread's membership, title or
// picture changed, so an open info screen reflects it without a refresh.
func (a *AppService) publishConversationUpdated(ctx context.Context, conversationId int64, actorId string) {
	if a.realtime == nil {
		return
	}
	members, err := a.db.GetConversationMemberIds(ctx, conversationId)
	if err != nil {
		return
	}
	// The actor included, unlike a message: they may have changed something from
	// one screen that another of their own screens is showing.
	a.realtime.Publish(members, realtime.Event{
		Kind:           realtime.EventConversationUpdated,
		ConversationId: conversationId,
		Data:           map[string]string{"actor_id": actorId},
	})
}

// publishReaction tells a thread that a message's reactions changed.
func (a *AppService) publishReaction(ctx context.Context, conversationId int64, messageId int64, actorId string, reactions []structs.MessageReaction) {
	if a.realtime == nil {
		return
	}
	members, err := a.db.GetConversationMemberIds(ctx, conversationId)
	if err != nil {
		return
	}
	// Mine is relative to the reader, and these are the actor's. Cleared before
	// sending rather than recomputed per recipient: a client that receives a
	// reaction change for a message it has on screen re-reads its own state for
	// the toggle, and a wrong Mine flag would make somebody else's reaction
	// look like theirs.
	shared := make([]structs.MessageReaction, len(reactions))
	for i, reaction := range reactions {
		shared[i] = reaction
		shared[i].Mine = false
	}

	a.realtime.PublishExcept(members, actorId, realtime.Event{
		Kind:           realtime.EventMessageReaction,
		ConversationId: conversationId,
		Data: map[string]any{
			"message_id": messageId,
			"reactions":  shared,
		},
	})
}

// publishRead tells the other members somebody has caught up, which is what
// drives a read receipt.
func (a *AppService) publishRead(ctx context.Context, conversationId int64, userId string, lastReadId int64) {
	if a.realtime == nil {
		return
	}
	members, err := a.db.GetConversationMemberIds(ctx, conversationId)
	if err != nil {
		return
	}
	a.realtime.PublishExcept(members, userId, realtime.Event{
		Kind:           realtime.EventConversationRead,
		ConversationId: conversationId,
		Data: map[string]any{
			"user_id":              userId,
			"last_read_message_id": lastReadId,
		},
	})
}

// PublishNotification delivers a notification over the socket to whoever is
// connected, and reports who was reached so the push can skip them.
//
// This is the general case of publishMessage: the same suppression rule applied
// to every other kind of notification the server sends. The data bag is the same
// one a push carries, so the client's existing routing — targetFromData — works
// unchanged on both.
func (a *AppService) PublishNotification(userIds []string, title, body string, data map[string]string) map[string]struct{} {
	reached := map[string]struct{}{}
	if a.realtime == nil || len(userIds) == 0 {
		return reached
	}
	online := a.realtime.OnlineAmong(userIds)
	if len(online) == 0 {
		return reached
	}
	targets := make([]string, 0, len(online))
	for id := range online {
		targets = append(targets, id)
		reached[id] = struct{}{}
	}
	a.realtime.Publish(targets, realtime.Event{
		Kind: realtime.EventNotification,
		Data: structs.RealtimeNotification{Title: title, Body: body, Data: data},
	})
	return reached
}

// PublishBetaChanged tells a client its beta access changed, so a surface
// appears or disappears without a relaunch.
//
// Called from the admin grant and revoke paths. This is the event the socket
// cannot be gated on holding the beta to receive — see the note on ServeRealtime.
func (a *AppService) PublishBetaChanged(userId string, features []string) {
	if a.realtime == nil || strings.TrimSpace(userId) == "" {
		return
	}
	a.realtime.Publish([]string{userId}, realtime.Event{
		Kind: realtime.EventBetaChanged,
		Data: map[string]any{"features": features},
	})
}
