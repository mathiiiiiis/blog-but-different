package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mathiiiiiis/blog-but-different/backend/internal/auth"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/httpx"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/store"
)

func logWarn(msg string, args ...any) { slog.Warn(msg, args...) }

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &body, 8<<10); err != nil {
		return err
	}

	user, err := s.store.AdminByEmail(r.Context(), strings.TrimSpace(body.Email))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}

	// Always run one comparison, against a dummy digest when the account is
	// missing, so a wrong address costs the same as a wrong password.
	hash := dummyHash
	if user != nil && user.PasswordHash != nil && *user.PasswordHash != "" {
		hash = *user.PasswordHash
	}
	if ok := auth.VerifyPassword(body.Password, hash); !ok || user == nil {
		return httpx.Errorf(http.StatusUnauthorized, "Invalid credentials")
	}

	token, err := s.signer.Issue(user.ID)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, tokenResponse{
		AccessToken:        token,
		TokenType:          "bearer",
		MustChangePassword: user.MustChangePassword,
	})
	return nil
}

// dummyHash is a valid bcrypt digest of a value nobody can supply, used only to
// keep the failure path's timing comparable to the success path.
const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) error {
	user, err := s.requireAdmin(r)
	if err != nil {
		return err
	}

	var body struct {
		NewPassword string `json:"new_password"`
	}
	if err := httpx.DecodeJSON(r, &body, 8<<10); err != nil {
		return err
	}
	if len(body.NewPassword) < 8 {
		return httpx.Errorf(http.StatusBadRequest, "Password must be at least 8 characters")
	}
	if len(body.NewPassword) > 72 {
		return httpx.Errorf(http.StatusBadRequest, "Password must be at most 72 characters")
	}

	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		return err
	}
	if err := s.store.SetPassword(r.Context(), user.ID, hash); err != nil {
		return err
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Password changed successfully"})
	return nil
}

func (s *Server) handleCreateGuest(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Avatar string `json:"avatar"`
	}
	if r.ContentLength > 0 {
		if err := httpx.DecodeJSON(r, &body, 4<<10); err != nil {
			return err
		}
	}

	guest, err := s.store.CreateGuest(r.Context(), s.resolveAvatar(body.Avatar))
	if err != nil {
		return err
	}
	token, err := s.signer.Issue(guest.ID)
	if err != nil {
		return err
	}

	httpx.JSON(w, http.StatusOK, tokenResponse{AccessToken: token, TokenType: "bearer"})
	return nil
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) error {
	user, err := s.requireUser(r)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, newUserResponse(user))
	return nil
}

func (s *Server) handleUpdateAvatar(w http.ResponseWriter, r *http.Request) error {
	user, err := s.requireUser(r)
	if err != nil {
		return err
	}

	requested := r.URL.Query().Get("avatar")
	if requested == "" {
		return httpx.Errorf(http.StatusBadRequest, "Missing avatar")
	}
	avatar := s.resolveAvatar(requested)
	if avatar != requested {
		return httpx.Errorf(http.StatusBadRequest, "Unknown avatar")
	}

	if err := s.store.SetAvatar(r.Context(), user.ID, avatar); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Errorf(http.StatusNotFound, "User not found")
		}
		return err
	}

	s.hub.SetAvatar(user.ID, avatar)
	s.hub.Broadcast(event("user_avatar_changed", map[string]any{
		"user_id": user.ID,
		"avatar":  avatar,
	}))

	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Avatar updated", "avatar": avatar})
	return nil
}

func (s *Server) handleRegisterFCM(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Token string `json:"token"`
	}
	if err := httpx.DecodeJSON(r, &body, 4<<10); err != nil {
		return err
	}

	token := strings.TrimSpace(body.Token)
	if token == "" {
		return httpx.Errorf(http.StatusBadRequest, "Missing token")
	}
	if len(token) > 512 {
		return httpx.Errorf(http.StatusBadRequest, "Token is too long")
	}
	if err := s.store.SaveFCMToken(r.Context(), token); err != nil {
		return err
	}

	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) handleListAvatars(w http.ResponseWriter, r *http.Request) error {
	httpx.JSON(w, http.StatusOK, map[string]any{"avatars": s.avatars})
	return nil
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) error {
	messages, reactions, err := s.store.Counts(r.Context())
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"messages":  messages,
		"reactions": reactions,
		"online":    s.hub.OnlineCount(),
	})
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) error {
	if err := s.store.Ping(r.Context()); err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":    "degraded",
			"timestamp": time.Now().UTC(),
		})
		return nil
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
	})
	return nil
}
