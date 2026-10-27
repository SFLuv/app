package realtime

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// How long a client has to send its auth frame before the socket is closed.
	// Short: an unauthenticated socket is an open file descriptor an anonymous
	// caller is holding, so the window is only as long as one round trip needs.
	authDeadline = 10 * time.Second

	// Keepalive. The server pings; a client that does not pong within the wait
	// is gone, whatever TCP still believes. Mobile networks drop sockets without
	// closing them constantly, and without this the registry would fill with
	// connections to phones that are off.
	pingInterval = 25 * time.Second
	pongWait     = 60 * time.Second
	writeWait    = 10 * time.Second

	// Per-connection outbound buffer. Sized for a burst — a chat catching up on
	// a few messages and their reactions — not for a backlog. Overflowing it
	// means the client is not reading, and the connection is dropped so it
	// reconnects and refetches.
	sendBuffer = 32

	// A client frame is a typing flag or an auth token. Anything larger is not
	// something this protocol has.
	maxClientFrame = 4096
)

// TokenValidator turns an access token into a user id. Injected rather than
// imported so this package does not depend on the auth middleware, and so a
// test can connect without minting a real Privy token.
type TokenValidator func(token string) (string, error)

// TypingHandler is called when a client reports typing. It returns the user ids
// to tell, which is the handler layer's business: it has to check the person is
// actually in the conversation before broadcasting that they are typing in it.
type TypingHandler func(userId string, conversationId int64, typing bool)

// Logf is the logging shape this package needs, satisfied by logger.LogCloser.
type Logf func(format string, args ...any)

// Server wires the hub to HTTP.
type Server struct {
	hub       *Hub
	validate  TokenValidator
	onTyping  TypingHandler
	logf      Logf
	upgrader  websocket.Upgrader
	allowAnyO bool
}

// NewServer builds the websocket endpoint.
//
// allowAnyOrigin exists because the clients that use this are native apps, which
// send no Origin header at all — gorilla's default check rejects a request whose
// Origin does not match Host, and a missing Origin passes that check, so the
// default is already correct for the mobile app. It is set true only for local
// development, where a browser page on another port is a useful way to watch the
// stream. The frontend does not use this socket.
func NewServer(hub *Hub, validate TokenValidator, onTyping TypingHandler, logf Logf, allowAnyOrigin bool) *Server {
	s := &Server{
		hub:       hub,
		validate:  validate,
		onTyping:  onTyping,
		logf:      logf,
		allowAnyO: allowAnyOrigin,
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			if s.allowAnyO {
				return true
			}
			origin := r.Header.Get("Origin")
			// No Origin: a native client. This is the normal case.
			if origin == "" {
				return true
			}
			return origin == "http://"+r.Host || origin == "https://"+r.Host
		},
	}
	return s
}

// conn is one websocket, with a writer goroutine so that no publisher ever
// blocks on a socket and so that all writes happen on one goroutine — which
// gorilla requires.
type conn struct {
	ws     *websocket.Conn
	out    chan []byte
	closed chan struct{}
	once   sync.Once
}

func (c *conn) send(payload []byte) {
	select {
	case c.out <- payload:
	default:
		// Buffer full: the client is not keeping up. Closing is the right answer
		// — see sendBuffer.
		c.close()
	}
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.closed)
	})
}

// Serve handles GET /ws.
//
// ── Why the token arrives in a frame rather than a header or the URL ─────────
//
// A header would be the obvious choice, and is what the rest of the API uses.
// React Native's WebSocket does accept headers, but inconsistently across
// platforms and versions, and a token that fails to attach produces a socket
// that connects and then silently receives nothing — the worst failure shape
// available.
//
// A query parameter works everywhere and is what most tutorials do. It also puts
// a bearer credential in the request line, which is the one part of an HTTPS
// request that reliably ends up in an access log, a proxy's log, and an error
// report. Privy tokens are short-lived, which makes that a smaller problem than
// it would otherwise be, not a non-problem.
//
// So: the socket opens unauthenticated, the client's first frame carries the
// token, and the server will not deliver or accept anything until it has checked
// it. The cost is one round trip before the socket is useful and the state
// machine below; the benefit is that the credential is in a frame body on an
// encrypted connection, the same place every other request puts it, and that
// failure is loud.
func (s *Server) Serve(w http.ResponseWriter, r *http.Request) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has already written a response.
		return
	}

	ws.SetReadLimit(maxClientFrame)

	c := &conn{
		ws:     ws,
		out:    make(chan []byte, sendBuffer),
		closed: make(chan struct{}),
	}

	// ── Auth ────────────────────────────────────────────────────────────────
	if err := ws.SetReadDeadline(time.Now().Add(authDeadline)); err != nil {
		_ = ws.Close()
		return
	}
	userId, ok := s.readAuth(ws)
	if !ok {
		s.writeClose(ws, websocket.ClosePolicyViolation, "auth required")
		_ = ws.Close()
		return
	}

	// Authenticated. The read deadline becomes the keepalive's, and every pong
	// pushes it out.
	if err := ws.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		_ = ws.Close()
		return
	}
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	s.hub.add(userId, c)
	defer func() {
		s.hub.remove(userId, c)
		c.close()
		_ = ws.Close()
	}()

	// Tell the client it is live, so it knows the difference between "connected"
	// and "connected and authenticated" without inferring it from traffic.
	ready, _ := json.Marshal(Event{Kind: "ready", SentAt: time.Now().UTC()})
	c.send(ready)

	go s.writeLoop(c)
	s.readLoop(c, userId)
}

// readAuth waits for the first frame and validates it.
func (s *Server) readAuth(ws *websocket.Conn) (string, bool) {
	_, raw, err := ws.ReadMessage()
	if err != nil {
		return "", false
	}
	var msg ClientMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return "", false
	}
	if msg.Kind != ClientMessageAuth || msg.Token == "" {
		return "", false
	}
	userId, err := s.validate(msg.Token)
	if err != nil || userId == "" {
		return "", false
	}
	return userId, true
}

func (s *Server) writeLoop(c *conn) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closed:
			return
		case payload := <-c.out:
			if err := c.ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.close()
				return
			}
			if err := c.ws.WriteMessage(websocket.TextMessage, payload); err != nil {
				c.close()
				return
			}
		case <-ticker.C:
			if err := c.ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.close()
				return
			}
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.close()
				return
			}
		}
	}
}

func (s *Server) readLoop(c *conn, userId string) {
	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			c.close()
			return
		}
		var msg ClientMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			// A frame we cannot parse is ignored rather than fatal: a newer
			// client sending a kind this build does not have must not lose its
			// socket over it.
			continue
		}
		switch msg.Kind {
		case ClientMessagePing:
			// Some clients prefer an application-level ping, because React
			// Native does not expose the protocol-level one.
			pong, _ := json.Marshal(Event{Kind: "pong", SentAt: time.Now().UTC()})
			c.send(pong)
		case ClientMessageTyping:
			if msg.ConversationId > 0 && s.onTyping != nil {
				// Membership is checked by the handler, not here: this package
				// has no database. A client claiming to type in a conversation
				// it is not in is told nobody.
				s.onTyping(userId, msg.ConversationId, msg.Typing)
			}
		case ClientMessageAuth:
			// Re-auth on an already-authenticated socket. Accepted and ignored:
			// a client refreshing its token has no reason to drop the socket,
			// and the identity cannot change — this connection is registered
			// under the id from the first frame, and rebinding it would hand
			// somebody else's events to whoever held the socket.
		}
	}
}

func (s *Server) writeClose(ws *websocket.Conn, code int, reason string) {
	_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
	_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
}
