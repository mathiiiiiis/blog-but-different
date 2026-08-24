package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/mathiiiiiis/blog-but-different/backend/internal/gifs"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/httpx"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/media"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/store"
)

func logInfo(msg string, args ...any) { slog.Info(msg, args...) }

func (s *Server) handleToggleReaction(w http.ResponseWriter, r *http.Request) error {
	user, err := s.requireUser(r)
	if err != nil {
		return err
	}

	var body struct {
		Emoji         string  `json:"emoji"`
		CustomEmojiID *string `json:"custom_emoji_id"`
	}
	if err := httpx.DecodeJSON(r, &body, 4<<10); err != nil {
		return err
	}

	emoji := strings.TrimSpace(body.Emoji)
	if emoji == "" || len(emoji) > 50 || strings.ContainsAny(emoji, "\x00\n\r") {
		return httpx.Errorf(http.StatusBadRequest, "Invalid emoji")
	}

	messageID := r.PathValue("id")
	result, err := s.store.ToggleReaction(r.Context(), messageID, user.ID, emoji, body.CustomEmojiID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Errorf(http.StatusNotFound, "Message not found")
		}
		return err
	}

	payload := map[string]any{
		"message_id":       messageID,
		"emoji":            emoji,
		"user_id":          user.ID,
		"username":         user.Username,
		"avatar":           orDefault(user.Avatar),
		"custom_emoji_url": result.CustomEmojiURL,
	}

	if result.Added {
		s.hub.Broadcast(event("reaction_added", payload))
		httpx.JSON(w, http.StatusOK, map[string]string{"message": "Reaction added", "action": "added"})
		return nil
	}

	s.hub.Broadcast(event("reaction_removed", payload))
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Reaction removed", "action": "removed"})
	return nil
}

func (s *Server) handleListEmojis(w http.ResponseWriter, r *http.Request) error {
	emojis, err := s.store.ListCustomEmojis(r.Context())
	if err != nil {
		return err
	}

	out := make([]customEmojiResponse, 0, len(emojis))
	for i := range emojis {
		out = append(out, newCustomEmojiResponse(&emojis[i]))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"emojis": out})
	return nil
}

var emojiName = regexp.MustCompile(`^[a-zA-Z0-9_]{2,50}$`)

func (s *Server) handleCreateEmoji(w http.ResponseWriter, r *http.Request) error {
	user, err := s.requireAdmin(r)
	if err != nil {
		return err
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxFileSize+(1<<20))

	reader, err := r.MultipartReader()
	if err != nil {
		return httpx.Errorf(http.StatusBadRequest, "Expected a multipart form body")
	}

	var (
		name    string
		payload []byte
		claimed string
		found   bool
	)

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return httpx.Errorf(http.StatusBadRequest, "Malformed multipart body")
		}

		switch {
		case part.FormName() == "name" && part.FileName() == "":
			raw, err := readLimited(part, 1<<10)
			part.Close()
			if err != nil {
				return httpx.Errorf(http.StatusBadRequest, "Malformed multipart body")
			}
			name = strings.TrimSpace(string(raw))

		case part.FormName() == "file" && !found:
			claimed = part.Header.Get("Content-Type")
			payload, err = readLimited(part, s.cfg.MaxFileSize)
			part.Close()
			if err != nil {
				if errors.Is(err, errTooLarge) {
					return httpx.Errorf(http.StatusRequestEntityTooLarge, "Emoji image is too large")
				}
				return httpx.Errorf(http.StatusBadRequest, "Could not read uploaded file")
			}
			found = true

		default:
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 1<<20))
			part.Close()
		}
	}

	if !emojiName.MatchString(name) {
		return httpx.Errorf(http.StatusBadRequest,
			"Emoji name must be 2-50 alphanumeric characters or underscores")
	}
	if !found || len(payload) == 0 {
		return httpx.Errorf(http.StatusBadRequest, "An image file is required")
	}

	kind, err := media.Validate(payload, claimed)
	if err != nil || kind.Category != "image" {
		return httpx.Errorf(http.StatusBadRequest, "File must be an image")
	}

	object, err := s.blobs.Put(r.Context(), "emojis", name+extensionFor(kind.ContentType), kind.ContentType, true, payload)
	if err != nil {
		return err
	}

	emoji, err := s.store.CreateCustomEmoji(r.Context(), name, s.cfg.AttachmentURL(object), object, user.ID)
	if err != nil {
		s.deleteObjects(context.WithoutCancel(r.Context()), []string{object})
		if errors.Is(err, store.ErrDuplicate) {
			return httpx.Errorf(http.StatusBadRequest, "Emoji name already exists")
		}
		return err
	}

	response := newCustomEmojiResponse(emoji)
	logInfo("custom emoji created", "name", emoji.Name, "by", user.Username)
	s.hub.Broadcast(event("custom_emoji_added", map[string]any{
		"id": emoji.ID, "name": emoji.Name, "url": emoji.URL,
	}))

	httpx.JSON(w, http.StatusOK, response)
	return nil
}

func extensionFor(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/avif":
		return ".avif"
	default:
		return ""
	}
}

func (s *Server) handleDeleteEmoji(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireAdmin(r); err != nil {
		return err
	}

	emoji, err := s.store.DeleteCustomEmoji(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Errorf(http.StatusNotFound, "Emoji not found")
		}
		return err
	}

	if emoji.ObjectName != nil {
		go s.deleteObjects(context.WithoutCancel(r.Context()), []string{*emoji.ObjectName})
	}

	logInfo("custom emoji deleted", "name", emoji.Name)
	s.hub.Broadcast(event("custom_emoji_removed", map[string]any{
		"id": emoji.ID, "name": emoji.Name,
	}))

	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Emoji deleted"})
	return nil
}

func (s *Server) handleSearchGifs(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireUser(r); err != nil {
		return err
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		return httpx.Errorf(http.StatusBadRequest, "Search query is required")
	}
	if len(query) > 200 {
		return httpx.Errorf(http.StatusBadRequest, "Search query is too long")
	}

	result, err := s.gifs.Search(r.Context(), query, gifLimit(r), r.URL.Query().Get("pos"))
	return s.writeGifs(w, result, err)
}

func (s *Server) handleTrendingGifs(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireUser(r); err != nil {
		return err
	}
	result, err := s.gifs.Trending(r.Context(), gifLimit(r), r.URL.Query().Get("pos"))
	return s.writeGifs(w, result, err)
}

func (s *Server) writeGifs(w http.ResponseWriter, result *gifs.Response, err error) error {
	switch {
	case errors.Is(err, gifs.ErrNotConfigured):
		return httpx.Errorf(http.StatusServiceUnavailable,
			"GIF search not configured. Please set KLIPY_API_KEY.")
	case errors.Is(err, gifs.ErrUpstream):
		slog.Warn("gif provider error", "error", err)
		return httpx.Errorf(http.StatusServiceUnavailable, "GIF search temporarily unavailable")
	case err != nil:
		return err
	}
	httpx.JSON(w, http.StatusOK, result)
	return nil
}

func gifLimit(r *http.Request) int {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	return min(max(limit, 1), 50)
}
