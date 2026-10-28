// Package realtime is the push side of the app: one websocket per foregrounded
// client, and a fan-out by user id.
//
// ── Why this exists alongside push notifications ─────────────────────────────
//
// Firebase is for reaching a device that is not being looked at. It is the wrong
// tool for a device that is: a push takes a second or two, goes through Apple or
// Google, and arrives as a system banner over whatever the person is doing —
// including over the very chat the message belongs to. This carries the same
// events in milliseconds to a client that can decide to render them as an
// in-app banner, a new bubble, or nothing at all because the thread is already
// open.
//
// The division is strict and worth stating: while a client holds a socket, the
// server sends it events here and suppresses its pushes. When the socket drops —
// backgrounded, killed, lost signal — pushes resume. Neither path is a fallback
// for the other; they cover disjoint states.
//
// ── What it is not ───────────────────────────────────────────────────────────
//
// Not a delivery guarantee, and not a message store. Every event here describes
// something already committed to Postgres, and every screen that consumes one
// can also reach the same state with an ordinary GET. A dropped socket costs a
// person live updates until it reconnects and refetches; it never costs them a
// message. That is what lets this stay a single-process in-memory hub with no
// broker behind it.
//
// ── Scale, honestly ─────────────────────────────────────────────────────────
//
// The registry is a map in one process, so this works as long as there is one
// server process. With two behind a load balancer, a client connected to A does
// not see an event published on B. Chat is a small beta and the backend runs as
// one process today, so that is a real limit rather than a theoretical one, and
// the way out is a Postgres LISTEN/NOTIFY relay or Redis pub/sub feeding
// Publish. Nothing in a caller's view of this package changes when that
// arrives — which is why Publish takes user ids rather than connections.
package realtime

import (
	"encoding/json"
	"sync"
	"time"
)

// Event kinds. The client switches on these, so a value here is API: renaming
// one breaks every build in the field until it updates.
const (
	// A message was added to a conversation the recipient is in.
	EventMessageNew = "message.new"
	// A message's reactions changed.
	EventMessageReaction = "message.reaction"
	// A message changed in place — edited. Carries the whole message, so a
	// client replaces what it has rather than appending.
	EventMessageUpdated = "message.updated"
	// A message was deleted. Carries only the id: sending the message would
	// mean sending what it used to say.
	EventMessageDeleted = "message.deleted"
	// Somebody is typing in a conversation. Not persisted anywhere.
	EventTyping = "typing"
	// A conversation's membership, title or icon changed.
	EventConversationUpdated = "conversation.updated"
	// Somebody read up to a point in a conversation.
	EventConversationRead = "conversation.read"
	// A notification that would otherwise have been a push.
	EventNotification = "notification"
	// The set of beta features this account holds has changed, so the client can
	// show or hide a surface without waiting for a relaunch.
	EventBetaChanged = "beta.changed"
)

// Event is one thing that happened, as the client receives it.
type Event struct {
	Kind string `json:"kind"`
	// Conversation is set on every chat event, so a client can route without
	// unpacking Data.
	ConversationId int64 `json:"conversation_id,omitempty"`
	// Data is the kind-specific body. Shapes are documented beside the
	// publishers in the handlers package.
	Data any `json:"data,omitempty"`
	// SentAt lets a client ignore an event older than what it already fetched,
	// which happens on reconnect.
	SentAt time.Time `json:"sent_at"`
	// Notice is how the event should be announced, when it should be at all.
	// Set on message.new: the same title and line the push carries, so a
	// banner shown in the app and a notification shown by the phone cannot
	// word the same message two different ways.
	Notice *Notice `json:"notice,omitempty"`
}

// Notice is a display-ready description of a new message, Telegram's shape:
// titled with the group, or with the person in a direct thread, and in a group
// the line names who spoke — "Maria: 📷 Photo".
type Notice struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// Spoken is the line for a screen reader: "Maria said: hello".
	Spoken     string `json:"spoken"`
	SenderId   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	IsGroup    bool   `json:"is_group"`
	// The group's picture, for a group's banner. Absent for a direct thread,
	// whose banner shows the sender's own picture from SenderId.
	IconAttachmentId *int64 `json:"icon_attachment_id,omitempty"`
	// Silent is set for a recipient who should not be interrupted: they muted
	// the thread, or are not in the chat beta. The message still arrives and
	// still counts as unread — the push path skips the same people, and this
	// tells the in-app banner to do likewise.
	Silent bool `json:"silent,omitempty"`
}

// ClientMessage is what a client may send up. Deliberately tiny: this socket is
// for receiving. The only thing a client can originate is a typing signal,
// because that is the one piece of state with nowhere else to live — everything
// else goes through an ordinary authenticated HTTP request, where the rate
// limits, role guards and validation already are.
type ClientMessage struct {
	Kind string `json:"kind"`
	// Auth, sent once as the first frame. See the comment on Serve.
	Token string `json:"token,omitempty"`
	// Typing.
	ConversationId int64 `json:"conversation_id,omitempty"`
	Typing         bool  `json:"typing,omitempty"`
}

const (
	ClientMessageAuth   = "auth"
	ClientMessageTyping = "typing"
	ClientMessagePing   = "ping"
)

// sink is one connected client, as the hub sees it. The concrete websocket is
// in conn.go; this interface is what keeps the registry testable without a
// network.
type sink interface {
	send(payload []byte)
	close()
}

// Hub holds every live connection, indexed by the user it belongs to.
type Hub struct {
	mu sync.RWMutex
	// One user may have several: a phone and a tablet, or the same phone
	// mid-reconnect with the old socket not yet reaped.
	byUser map[string]map[sink]struct{}
}

func NewHub() *Hub {
	return &Hub{byUser: map[string]map[sink]struct{}{}}
}

func (h *Hub) add(userId string, s sink) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byUser[userId] == nil {
		h.byUser[userId] = map[sink]struct{}{}
	}
	h.byUser[userId][s] = struct{}{}
}

func (h *Hub) remove(userId string, s sink) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns := h.byUser[userId]
	if conns == nil {
		return
	}
	delete(conns, s)
	if len(conns) == 0 {
		delete(h.byUser, userId)
	}
}

// IsOnline reports whether a user has at least one live socket.
//
// This is what suppresses a push: the message notifier asks before sending, and
// skips the device that is already looking at the app. It is advisory — a socket
// can die between this answer and the publish, in which case the person misses
// the banner and sees the message the next time the screen refreshes. Getting it
// wrong in the other direction, by pushing to somebody who is holding the app
// open, is the one that is actually annoying, so the race is resolved in that
// direction deliberately.
func (h *Hub) IsOnline(userId string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.byUser[userId]) > 0
}

// OnlineAmong narrows a set of user ids to those currently connected, in one
// lock acquisition rather than one per id.
func (h *Hub) OnlineAmong(userIds []string) map[string]struct{} {
	online := map[string]struct{}{}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, id := range userIds {
		if len(h.byUser[id]) > 0 {
			online[id] = struct{}{}
		}
	}
	return online
}

// Publish delivers an event to every connection of every named user.
//
// Never blocks and never errors. A connection whose buffer is full is closed
// rather than waited on: a client too slow to keep up is a client that needs to
// reconnect and refetch, and holding a publisher — which is usually an HTTP
// handler mid-request — behind a stalled socket would make one bad phone slow
// for everybody.
func (h *Hub) Publish(userIds []string, event Event) {
	if len(userIds) == 0 {
		return
	}
	if event.SentAt.IsZero() {
		event.SentAt = time.Now().UTC()
	}
	payload, err := json.Marshal(event)
	if err != nil {
		// An event we cannot encode is a programming error in the publisher, and
		// there is nobody to return it to from here. Dropping it is correct: the
		// state it describes is already committed, and the client will see it on
		// its next fetch.
		return
	}

	// Collected under the read lock and sent outside it, so a send cannot
	// deadlock against a connection removing itself.
	targets := []sink{}
	h.mu.RLock()
	seen := map[string]struct{}{}
	for _, id := range userIds {
		if _, done := seen[id]; done {
			continue
		}
		seen[id] = struct{}{}
		for s := range h.byUser[id] {
			targets = append(targets, s)
		}
	}
	h.mu.RUnlock()

	for _, s := range targets {
		s.send(payload)
	}
}

// PublishExcept is Publish, minus one user — the one who caused the event.
//
// Used for every chat event, because the sender has already applied it
// optimistically. Sending it back would make their own message appear twice for
// the length of a round trip, which reads as a bug even though it resolves.
func (h *Hub) PublishExcept(userIds []string, exclude string, event Event) {
	filtered := make([]string, 0, len(userIds))
	for _, id := range userIds {
		if id != exclude {
			filtered = append(filtered, id)
		}
	}
	h.Publish(filtered, event)
}

// Connections is the count of live sockets, for /healthz.
func (h *Hub) Connections() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, conns := range h.byUser {
		total += len(conns)
	}
	return total
}

// Users is the count of distinct connected accounts, for /healthz.
func (h *Hub) Users() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.byUser)
}
