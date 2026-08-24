package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/hub"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/store"
)

const (
	authTimeout    = 5 * time.Second
	pingInterval   = 30 * time.Second
	writeTimeout   = 10 * time.Second
	readLimitBytes = 16 << 10
	typingCooldown = time.Second
)

type clientFrame struct {
	Type   string `json:"type"`
	Token  string `json:"token"`
	Avatar string `json:"avatar"`
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns:  s.originPatterns(),
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		slog.Debug("websocket handshake rejected", "error", err)
		return
	}
	conn.SetReadLimit(readLimitBytes)

	// Detach from the request context so the socket outlives the handshake.
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()

	user, issuedToken, ok := s.authenticateSocket(ctx, conn)
	if !ok {
		return
	}

	presence := hub.Presence{
		UserID:   user.ID,
		Username: user.Username,
		Avatar:   orDefault(user.Avatar),
		IsAdmin:  user.IsAdmin,
	}
	client, first := s.hub.Add(presence)

	// Remove keys on the connection itself, never the user id: a user may hold
	// several sockets and removing by user id leaks every one of them.
	defer func() {
		if last := s.hub.Remove(client); last {
			s.hub.Broadcast(event("user_leave", map[string]any{
				"user_id":      presence.UserID,
				"username":     presence.Username,
				"avatar":       presence.Avatar,
				"is_admin":     presence.IsAdmin,
				"online_count": s.hub.OnlineCount(),
			}))
		}
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	if first {
		s.hub.BroadcastExceptConn(event("user_join", map[string]any{
			"user_id":      presence.UserID,
			"username":     presence.Username,
			"avatar":       presence.Avatar,
			"is_admin":     presence.IsAdmin,
			"online_count": s.hub.OnlineCount(),
		}), client.ID())
	}

	var tokenField any
	if issuedToken != "" {
		tokenField = issuedToken
	}
	client.Send(event("connected", map[string]any{
		"user_id":      user.ID,
		"username":     user.Username,
		"avatar":       presence.Avatar,
		"is_admin":     user.IsAdmin,
		"can_post":     user.CanPost(),
		"token":        tokenField,
		"online_users": s.hub.OnlineUsers(),
		"online_count": s.hub.OnlineCount(),
	}))

	if err := s.store.TouchLastSeen(ctx, user.ID); err != nil {
		slog.Warn("updating last_seen", "error", err)
	}

	go s.pumpOutbound(ctx, cancel, conn, client)
	s.pumpInbound(ctx, conn, client, user)
}

// originPatterns mirrors the configured CORS origins. With none configured the
// library's default same-origin check applies, which is what the nginx
// deployment wants.
func (s *Server) originPatterns() []string {
	patterns := make([]string, 0, len(s.cfg.CORSOrigins))
	for _, origin := range s.cfg.CORSOrigins {
		if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" {
			patterns = append(patterns, parsed.Host)
			continue
		}
		patterns = append(patterns, origin)
	}
	return patterns
}

// authenticateSocket reads the opening auth frame. A supplied but unusable
// token is rejected rather than silently downgraded to a fresh guest, which
// would otherwise mint an account on every reconnect.
func (s *Server) authenticateSocket(ctx context.Context, conn *websocket.Conn) (*store.User, string, bool) {
	readCtx, cancel := context.WithTimeout(ctx, authTimeout)
	defer cancel()

	var frame clientFrame
	_, data, err := conn.Read(readCtx)
	if err == nil {
		if err := json.Unmarshal(data, &frame); err != nil {
			conn.Close(websocket.StatusUnsupportedData, "malformed auth frame")
			return nil, "", false
		}
	} else if !errors.Is(err, context.DeadlineExceeded) {
		return nil, "", false
	}

	token := strings.TrimSpace(frame.Token)
	if token != "" {
		userID, err := s.signer.Subject(token)
		if err == nil {
			user, err := s.store.UserByID(ctx, userID)
			if err == nil {
				return user, "", true
			}
			if !errors.Is(err, store.ErrNotFound) {
				slog.Error("loading websocket user", "error", err)
				conn.Close(websocket.StatusInternalError, "")
				return nil, "", false
			}
		}
		conn.Close(4001, "invalid token")
		return nil, "", false
	}

	guest, err := s.store.CreateGuest(ctx, "default")
	if err != nil {
		slog.Error("creating websocket guest", "error", err)
		conn.Close(websocket.StatusInternalError, "")
		return nil, "", false
	}
	issued, err := s.signer.Issue(guest.ID)
	if err != nil {
		slog.Error("issuing guest token", "error", err)
		conn.Close(websocket.StatusInternalError, "")
		return nil, "", false
	}
	return guest, issued, true
}

// pumpOutbound owns every write to the socket, including keepalive pings, so
// concurrent broadcasts never interleave on the wire.
func (s *Server) pumpOutbound(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, client *hub.Conn) {
	defer cancel()

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-client.Done():
			conn.Close(websocket.StatusPolicyViolation, "client too slow")
			return
		case payload := <-client.Outbound():
			writeCtx, done := context.WithTimeout(ctx, writeTimeout)
			err := conn.Write(writeCtx, websocket.MessageText, payload)
			done()
			if err != nil {
				return
			}
		case <-ticker.C:
			pingCtx, done := context.WithTimeout(ctx, writeTimeout)
			err := conn.Ping(pingCtx)
			done()
			if err != nil {
				return
			}
		}
	}
}

func (s *Server) pumpInbound(ctx context.Context, conn *websocket.Conn, client *hub.Conn, user *store.User) {
	var lastTyping time.Time

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		var frame clientFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			continue
		}

		switch frame.Type {
		case "typing":
			// Throttled so a client cannot turn one socket into a broadcast amplifier.
			if time.Since(lastTyping) < typingCooldown {
				continue
			}
			lastTyping = time.Now()
			s.hub.BroadcastExceptConn(event("typing", map[string]any{
				"user_id":  user.ID,
				"username": user.Username,
			}), client.ID())

		case "update_avatar":
			avatar := s.resolveAvatar(frame.Avatar)
			if err := s.store.SetAvatar(ctx, user.ID, avatar); err != nil {
				slog.Warn("updating avatar over websocket", "error", err)
				continue
			}
			s.hub.SetAvatar(user.ID, avatar)
			s.hub.Broadcast(event("user_avatar_changed", map[string]any{
				"user_id": user.ID,
				"avatar":  avatar,
			}))
		}
	}
}
