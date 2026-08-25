package hub

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/google/uuid"
)

// Event is the envelope every client message uses.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Presence struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	IsAdmin  bool   `json:"is_admin"`
}

// Conn is one websocket. Writes go through a buffered channel so a slow or
// dead client can never block a broadcast.
type Conn struct {
	id     string
	userID string

	mu       sync.RWMutex
	presence Presence

	out      chan []byte
	done     chan struct{}
	closeOne sync.Once
}

func (c *Conn) ID() string     { return c.id }
func (c *Conn) UserID() string { return c.userID }

func (c *Conn) Presence() Presence {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.presence
}

func (c *Conn) setAvatar(avatar string) {
	c.mu.Lock()
	c.presence.Avatar = avatar
	c.mu.Unlock()
}

func (c *Conn) Outbound() <-chan []byte { return c.out }
func (c *Conn) Done() <-chan struct{}   { return c.done }

func (c *Conn) close() {
	c.closeOne.Do(func() { close(c.done) })
}

const sendBuffer = 64

type Hub struct {
	mu    sync.RWMutex
	conns map[string]*Conn
	users map[string]map[string]*Conn
}

func New() *Hub {
	return &Hub{
		conns: make(map[string]*Conn),
		users: make(map[string]map[string]*Conn),
	}
}

// Add registers a connection. firstForUser reports whether this is the user's
// only live connection, which is what drives join and leave events.
func (h *Hub) Add(p Presence) (conn *Conn, firstForUser bool) {
	conn = &Conn{
		id:       uuid.NewString(),
		userID:   p.UserID,
		presence: p,
		out:      make(chan []byte, sendBuffer),
		done:     make(chan struct{}),
	}

	h.mu.Lock()
	h.conns[conn.id] = conn
	byUser, ok := h.users[p.UserID]
	if !ok {
		byUser = make(map[string]*Conn)
		h.users[p.UserID] = byUser
	}
	byUser[conn.id] = conn
	firstForUser = len(byUser) == 1
	h.mu.Unlock()

	return conn, firstForUser
}

// Remove deregisters a connection by its own id. Calling it twice is safe.
func (h *Hub) Remove(conn *Conn) (lastForUser bool) {
	if conn == nil {
		return false
	}

	h.mu.Lock()
	if _, live := h.conns[conn.id]; !live {
		h.mu.Unlock()
		conn.close()
		return false
	}
	delete(h.conns, conn.id)
	if byUser, ok := h.users[conn.userID]; ok {
		delete(byUser, conn.id)
		if len(byUser) == 0 {
			delete(h.users, conn.userID)
			lastForUser = true
		}
	}
	h.mu.Unlock()

	conn.close()
	return lastForUser
}

func (h *Hub) Broadcast(evt Event) { h.broadcast(evt, "", "") }

func (h *Hub) BroadcastExceptConn(evt Event, connID string) { h.broadcast(evt, connID, "") }

func (h *Hub) BroadcastExceptUser(evt Event, userID string) { h.broadcast(evt, "", userID) }

func (h *Hub) broadcast(evt Event, skipConn, skipUser string) {
	payload, err := json.Marshal(evt)
	if err != nil {
		slog.Error("encoding broadcast", "type", evt.Type, "error", err)
		return
	}

	h.mu.RLock()
	targets := make([]*Conn, 0, len(h.conns))
	for id, c := range h.conns {
		if id == skipConn || (skipUser != "" && c.userID == skipUser) {
			continue
		}
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	for _, c := range targets {
		c.enqueue(payload)
	}
}

// SendTo delivers to every connection belonging to one user.
func (h *Hub) SendTo(userID string, evt Event) {
	payload, err := json.Marshal(evt)
	if err != nil {
		slog.Error("encoding message", "type", evt.Type, "error", err)
		return
	}

	h.mu.RLock()
	targets := make([]*Conn, 0, len(h.users[userID]))
	for _, c := range h.users[userID] {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	for _, c := range targets {
		c.enqueue(payload)
	}
}

// Send delivers a single event to one connection.
func (c *Conn) Send(evt Event) {
	payload, err := json.Marshal(evt)
	if err != nil {
		slog.Error("encoding message", "type", evt.Type, "error", err)
		return
	}
	c.enqueue(payload)
}

// enqueue drops the connection rather than blocking when its buffer is full;
// the reader goroutine observes Done and tears the socket down.
func (c *Conn) enqueue(payload []byte) {
	select {
	case <-c.done:
		return
	default:
	}

	select {
	case c.out <- payload:
	default:
		slog.Warn("websocket send buffer full, closing connection", "conn", c.id)
		c.close()
	}
}

func (h *Hub) OnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users)
}

func (h *Hub) OnlineUsers() []Presence {
	h.mu.RLock()
	conns := make([]*Conn, 0, len(h.users))
	for _, byUser := range h.users {
		for _, c := range byUser {
			conns = append(conns, c)
			break
		}
	}
	h.mu.RUnlock()

	out := make([]Presence, 0, len(conns))
	for _, c := range conns {
		out = append(out, c.Presence())
	}
	return out
}

func (h *Hub) OnlineUserIDs() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	out := make([]string, 0, len(h.users))
	for id := range h.users {
		out = append(out, id)
	}
	return out
}

// SetAvatar updates the cached presence for every connection of a user so the
// online list and later join events stay accurate.
func (h *Hub) SetAvatar(userID, avatar string) {
	h.mu.RLock()
	conns := make([]*Conn, 0, len(h.users[userID]))
	for _, c := range h.users[userID] {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		c.setAvatar(avatar)
	}
}
