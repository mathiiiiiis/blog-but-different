package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/mathiiiiiis/blog-but-different/backend/internal/auth"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/blob"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/config"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/gifs"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/httpx"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/hub"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/push"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/store"
)

type Avatar struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Server struct {
	cfg    *config.Config
	store  *store.Store
	signer *auth.Signer
	blobs  blob.Store
	hub    *hub.Hub
	gifs   *gifs.Client
	push   *push.Sender

	avatars   []Avatar
	avatarIDs map[string]struct{}

	messageLimiter  *httpx.Limiter
	reactionLimiter *httpx.Limiter
	authLimiter     *httpx.Limiter
	gifLimiter      *httpx.Limiter
	uploadLimiter   *httpx.Limiter
}

type Deps struct {
	Config *config.Config
	Store  *store.Store
	Signer *auth.Signer
	Blobs  blob.Store
	Hub    *hub.Hub
	Gifs   *gifs.Client
	Push   *push.Sender
}

func NewServer(d Deps) *Server {
	s := &Server{
		cfg:    d.Config,
		store:  d.Store,
		signer: d.Signer,
		blobs:  d.Blobs,
		hub:    d.Hub,
		gifs:   d.Gifs,
		push:   d.Push,

		messageLimiter:  httpx.NewLimiter(30),
		reactionLimiter: httpx.NewLimiter(60),
		authLimiter:     httpx.NewLimiter(10),
		gifLimiter:      httpx.NewLimiter(30),
		uploadLimiter:   httpx.NewLimiter(20),
	}
	s.loadAvatars()
	return s
}

// StartBackground runs the limiter sweepers for the lifetime of done.
func (s *Server) StartBackground(done <-chan struct{}) {
	for _, l := range []*httpx.Limiter{
		s.messageLimiter, s.reactionLimiter, s.authLimiter, s.gifLimiter, s.uploadLimiter,
	} {
		go l.Sweep(done)
	}
}

func (s *Server) loadAvatars() {
	s.avatars = []Avatar{{ID: "default", Name: "Default", URL: "/avatars/default.png"}}

	raw, err := os.ReadFile(s.cfg.AvatarsConfigPath)
	if err == nil {
		var parsed struct {
			Avatars []Avatar `json:"avatars"`
		}
		if err := json.Unmarshal(raw, &parsed); err == nil && len(parsed.Avatars) > 0 {
			s.avatars = parsed.Avatars
		}
	}

	s.avatarIDs = make(map[string]struct{}, len(s.avatars))
	for _, a := range s.avatars {
		s.avatarIDs[a.ID] = struct{}{}
	}
	s.avatarIDs["default"] = struct{}{}
}

// resolveAvatar keeps unknown identifiers out of the database instead of
// storing whatever string a client sends.
func (s *Server) resolveAvatar(id string) string {
	id = strings.TrimSpace(id)
	if _, ok := s.avatarIDs[id]; ok {
		return id
	}
	return "default"
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /api/auth/login", s.authLimiter.Limit(s.cfg.TrustProxy, s.handleLogin))
	mux.Handle("POST /api/auth/change-password", httpx.Handler(s.handleChangePassword))
	mux.Handle("POST /api/auth/guest", s.authLimiter.Limit(s.cfg.TrustProxy, s.handleCreateGuest))
	mux.Handle("GET /api/auth/me", httpx.Handler(s.handleMe))
	mux.Handle("PATCH /api/auth/avatar", httpx.Handler(s.handleUpdateAvatar))

	mux.Handle("GET /api/messages", httpx.Handler(s.handleListMessages))
	mux.Handle("POST /api/messages", s.messageLimiter.Limit(s.cfg.TrustProxy, s.handleCreateMessage))
	mux.Handle("PATCH /api/messages/{id}", httpx.Handler(s.handleEditMessage))
	mux.Handle("DELETE /api/messages/{id}", httpx.Handler(s.handleDeleteMessage))
	mux.Handle("POST /api/messages/{id}/pin", httpx.Handler(s.handleTogglePin))
	mux.Handle("POST /api/messages/{id}/reactions", s.reactionLimiter.Limit(s.cfg.TrustProxy, s.handleToggleReaction))

	mux.Handle("GET /api/emojis", httpx.Handler(s.handleListEmojis))
	mux.Handle("POST /api/emojis", s.uploadLimiter.Limit(s.cfg.TrustProxy, s.handleCreateEmoji))
	mux.Handle("DELETE /api/emojis/{id}", httpx.Handler(s.handleDeleteEmoji))

	mux.Handle("GET /api/gifs/search", s.gifLimiter.Limit(s.cfg.TrustProxy, s.handleSearchGifs))
	mux.Handle("GET /api/gifs/trending", s.gifLimiter.Limit(s.cfg.TrustProxy, s.handleTrendingGifs))

	mux.Handle("POST /api/fcm/register", httpx.Handler(s.handleRegisterFCM))

	mux.Handle("GET /api/avatars", httpx.Handler(s.handleListAvatars))
	mux.Handle("GET /api/stats", httpx.Handler(s.handleStats))
	mux.Handle("GET /api/health", httpx.Handler(s.handleHealth))

	mux.Handle("GET /avatars/", s.avatarFileServer())
	mux.HandleFunc("/ws", s.handleWebSocket)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Detail(w, http.StatusNotFound, "Not found")
	})

	return httpx.Chain(mux,
		httpx.Recover,
		httpx.Logging,
		httpx.SecurityHeaders,
		httpx.CORS(s.cfg.AllowOrigin),
	)
}

func (s *Server) avatarFileServer() http.Handler {
	fs := http.FileServer(http.Dir(s.cfg.AvatarsDir))
	return http.StripPrefix("/avatars/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			httpx.Detail(w, http.StatusNotFound, "Not found")
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		fs.ServeHTTP(w, r)
	}))
}

// currentUser resolves the bearer token. A missing or unusable token is not an
// error; it simply means the caller is anonymous.
func (s *Server) currentUser(r *http.Request) (*store.User, error) {
	token := auth.BearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return nil, nil
	}
	userID, err := s.signer.Subject(token)
	if err != nil {
		return nil, nil
	}
	user, err := s.store.UserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (s *Server) requireUser(r *http.Request) (*store.User, error) {
	user, err := s.currentUser(r)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, httpx.Errorf(http.StatusUnauthorized, "Not authenticated")
	}
	return user, nil
}

func (s *Server) requireAdmin(r *http.Request) (*store.User, error) {
	user, err := s.currentUser(r)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, httpx.Errorf(http.StatusUnauthorized, "Authentication required")
	}
	if !user.IsAdmin {
		return nil, httpx.Errorf(http.StatusForbidden, "Admin privileges required")
	}
	return user, nil
}

// deleteObjects cleans up storage without letting a slow bucket hold up a response.
func (s *Server) deleteObjects(ctx context.Context, objects []string) {
	for _, name := range objects {
		if name == "" {
			continue
		}
		if err := s.blobs.Delete(ctx, name); err != nil {
			logWarn("deleting stored object", "object", name, "error", err)
		}
	}
}
