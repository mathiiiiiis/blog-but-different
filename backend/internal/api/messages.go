package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mathiiiiiis/blog-but-different/backend/internal/httpx"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/hub"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/media"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/store"
)

func event(kind string, data any) hub.Event { return hub.Event{Type: kind, Data: data} }

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) error {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	limit = min(max(limit, 1), 100)

	before := strings.TrimSpace(r.URL.Query().Get("before"))

	var cursor *store.Cursor
	if before != "" {
		c, err := s.store.MessageCursor(r.Context(), before)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		cursor = c
	}

	rows, err := s.store.ListMessages(r.Context(), limit, cursor)
	if err != nil {
		return err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	// The store returns newest first; the UI renders oldest to newest.
	messages := make([]messageResponse, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		messages = append(messages, newMessageResponse(&rows[i]))
	}

	pinned := []messageResponse{}
	if before == "" {
		pinnedRows, err := s.store.ListPinned(r.Context())
		if err != nil {
			return err
		}
		for i := range pinnedRows {
			pinned = append(pinned, newMessageResponse(&pinnedRows[i]))
		}
	}

	httpx.JSON(w, http.StatusOK, messageList{
		Messages:       messages,
		PinnedMessages: pinned,
		Total:          len(messages),
		HasMore:        hasMore,
	})
	return nil
}

type messageForm struct {
	content        string
	replyToID      string
	gifURL         string
	gifID          string
	gifPreviewURL  string
	stickerURL     string
	stickerName    string
	attachments    []store.Attachment
	uploadedObject []string
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) error {
	user, err := s.requireAdmin(r)
	if err != nil {
		return err
	}

	form, err := s.readMessageForm(w, r)
	if err != nil {
		if form != nil {
			s.deleteObjects(context.WithoutCancel(r.Context()), form.uploadedObject)
		}
		return err
	}

	content := form.content
	isPinned := false

	if strings.HasPrefix(content, "/") {
		command, args, _ := strings.Cut(strings.TrimSpace(content), " ")
		command = strings.ToLower(command)

		switch command {
		case "/clear":
			objects, err := s.store.ClearMessages(r.Context())
			if err != nil {
				s.deleteObjects(context.WithoutCancel(r.Context()), form.uploadedObject)
				return err
			}
			objects = append(objects, form.uploadedObject...)
			go s.deleteObjects(context.WithoutCancel(r.Context()), objects)

			s.hub.Broadcast(event("chat_cleared", map[string]any{}))
			httpx.JSON(w, http.StatusOK, commandResponse{
				Success: true,
				Command: "clear",
				Message: "Chat cleared successfully",
			})
			return nil

		case "/pin":
			if strings.TrimSpace(args) == "" {
				s.deleteObjects(context.WithoutCancel(r.Context()), form.uploadedObject)
				return httpx.Errorf(http.StatusBadRequest,
					"Usage: /pin <message content> - Creates a pinned message")
			}
			content = args
			isPinned = true
		}
	}

	if len([]rune(content)) > s.cfg.MaxMessageLen {
		s.deleteObjects(context.WithoutCancel(r.Context()), form.uploadedObject)
		return httpx.Errorf(http.StatusBadRequest,
			fmt.Sprintf("Message content exceeds maximum length of %d characters", s.cfg.MaxMessageLen))
	}

	attachments := form.attachments
	if form.gifURL != "" {
		attachments = append(attachments, store.Attachment{
			Type:       "gif",
			URL:        form.gifURL,
			Name:       "GIF",
			GifID:      optional(form.gifID),
			PreviewURL: optional(form.gifPreviewURL),
		})
	}
	if form.stickerURL != "" {
		name := form.stickerName
		if name == "" {
			name = "Sticker"
		}
		attachments = append(attachments, store.Attachment{
			Type: "sticker",
			URL:  form.stickerURL,
			Name: name,
		})
	}

	if content == "" && len(attachments) == 0 {
		return httpx.Errorf(http.StatusBadRequest,
			"Message must have content, attachments, a GIF, or a sticker")
	}

	created, err := s.store.CreateMessage(r.Context(), store.NewMessage{
		Content:     optional(content),
		AuthorID:    user.ID,
		ReplyToID:   optional(form.replyToID),
		IsPinned:    isPinned,
		Attachments: attachments,
	})
	if err != nil {
		s.deleteObjects(context.WithoutCancel(r.Context()), form.uploadedObject)
		if errors.Is(err, store.ErrBadReply) {
			return httpx.Errorf(http.StatusBadRequest, "The message being replied to no longer exists")
		}
		return err
	}

	response := newMessageResponse(created)
	s.hub.Broadcast(event("new_message", response))

	body := "Sent an attachment"
	if created.Content != nil && *created.Content != "" {
		body = *created.Content
	}
	if runes := []rune(body); len(runes) > 100 {
		body = string(runes[:100]) + "…"
	}
	go s.push.Notify(context.WithoutCancel(r.Context()),
		"New post from "+user.Username, body)

	httpx.JSON(w, http.StatusOK, response)
	return nil
}

// readMessageForm streams the multipart body, enforcing per-file and total
// size limits before anything is buffered in full.
func (s *Server) readMessageForm(w http.ResponseWriter, r *http.Request) (*messageForm, error) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxFileSize+(4<<20))

	reader, err := r.MultipartReader()
	if err != nil {
		return nil, httpx.Errorf(http.StatusBadRequest, "Expected a multipart form body")
	}

	form := &messageForm{}
	values := map[string]*string{
		"content":         &form.content,
		"reply_to_id":     &form.replyToID,
		"gif_url":         &form.gifURL,
		"gif_id":          &form.gifID,
		"gif_preview_url": &form.gifPreviewURL,
		"sticker_url":     &form.stickerURL,
		"sticker_name":    &form.stickerName,
	}

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return form, httpx.Errorf(http.StatusBadRequest, "Malformed multipart body")
		}

		if part.FileName() == "" {
			target, known := values[part.FormName()]
			if !known {
				_, _ = io.Copy(io.Discard, io.LimitReader(part, 1<<20))
				part.Close()
				continue
			}
			raw, err := io.ReadAll(io.LimitReader(part, 64<<10))
			part.Close()
			if err != nil {
				return form, httpx.Errorf(http.StatusBadRequest, "Malformed multipart body")
			}
			*target = strings.TrimSpace(string(raw))
			continue
		}

		if len(form.attachments) >= s.cfg.MaxAttachments {
			part.Close()
			return form, httpx.Errorf(http.StatusBadRequest,
				fmt.Sprintf("A message may carry at most %d attachments", s.cfg.MaxAttachments))
		}

		attachment, object, err := s.storePart(r.Context(), part)
		part.Close()
		if err != nil {
			return form, err
		}
		form.attachments = append(form.attachments, attachment)
		form.uploadedObject = append(form.uploadedObject, object)
	}

	if err := validateRemoteURL(form.gifURL); err != nil {
		return form, err
	}
	if err := validateRemoteURL(form.gifPreviewURL); err != nil {
		return form, err
	}
	if err := validateRemoteURL(form.stickerURL); err != nil {
		return form, err
	}
	return form, nil
}

func (s *Server) storePart(ctx context.Context, part *multipart.Part) (store.Attachment, string, error) {
	data, err := readLimited(part, s.cfg.MaxFileSize)
	if err != nil {
		if errors.Is(err, errTooLarge) {
			return store.Attachment{}, "", httpx.Errorf(http.StatusRequestEntityTooLarge,
				fmt.Sprintf("File %q exceeds the maximum allowed size (%d MB)",
					part.FileName(), s.cfg.MaxFileSize/(1024*1024)))
		}
		return store.Attachment{}, "", httpx.Errorf(http.StatusBadRequest, "Could not read uploaded file")
	}

	kind, err := media.Validate(data, part.Header.Get("Content-Type"))
	if err != nil {
		if errors.Is(err, media.ErrMismatch) {
			return store.Attachment{}, "", httpx.Errorf(http.StatusBadRequest,
				fmt.Sprintf("File content for %q does not match its content type", part.FileName()))
		}
		return store.Attachment{}, "", httpx.Errorf(http.StatusBadRequest,
			fmt.Sprintf("File type of %q is not allowed", part.FileName()))
	}

	object, err := s.blobs.Put(ctx, kind.Folder, part.FileName(), kind.ContentType, kind.Inline, data)
	if err != nil {
		return store.Attachment{}, "", err
	}

	size := int64(len(data))
	return store.Attachment{
		Type:       kind.Category,
		URL:        s.cfg.AttachmentURL(object),
		Name:       safeDisplayName(part.FileName()),
		Size:       &size,
		ObjectName: object,
	}, object, nil
}

var errTooLarge = errors.New("payload too large")

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errTooLarge
	}
	return data, nil
}

// validateRemoteURL keeps anything but a plain web URL out of attachment records.
func validateRemoteURL(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > 2000 {
		return httpx.Errorf(http.StatusBadRequest, "Attachment URL is too long")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return httpx.Errorf(http.StatusBadRequest, "Attachment URL must be an http or https address")
	}
	return nil
}

// safeDisplayName strips control characters and path segments from the label
// shown next to a download.
func safeDisplayName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	if runes := []rune(name); len(runes) > 200 {
		name = string(runes[:200])
	}
	return name
}

func (s *Server) handleEditMessage(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireAdmin(r); err != nil {
		return err
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := httpx.DecodeJSON(r, &body, 64<<10); err != nil {
		return err
	}

	content := strings.TrimSpace(body.Content)
	if content == "" {
		return httpx.Errorf(http.StatusBadRequest, "Content must not be empty")
	}
	if len([]rune(content)) > 2000 {
		return httpx.Errorf(http.StatusBadRequest, "Content exceeds maximum length of 2000 characters")
	}

	updated, err := s.store.EditMessage(r.Context(), r.PathValue("id"), content)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			return httpx.Errorf(http.StatusNotFound, "Message not found")
		case errors.Is(err, store.ErrAttachmentOnly):
			return httpx.Errorf(http.StatusBadRequest, "Cannot edit attachment-only messages")
		}
		return err
	}

	response := newMessageResponse(updated)
	s.hub.Broadcast(event("message_edited", response))
	httpx.JSON(w, http.StatusOK, response)
	return nil
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) error {
	user, err := s.requireAdmin(r)
	if err != nil {
		return err
	}

	id := r.PathValue("id")
	objects, err := s.store.DeleteMessage(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Errorf(http.StatusNotFound, "Message not found")
		}
		return err
	}

	go s.deleteObjects(context.WithoutCancel(r.Context()), objects)

	logInfo("message deleted", "message", id, "by", user.Username)
	s.hub.Broadcast(event("message_deleted", map[string]any{"message_id": id}))
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Message deleted"})
	return nil
}

func (s *Server) handleTogglePin(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireAdmin(r); err != nil {
		return err
	}

	id := r.PathValue("id")
	pinned, err := s.store.TogglePin(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Errorf(http.StatusNotFound, "Message not found")
		}
		return err
	}

	s.hub.Broadcast(event("message_pinned_update", map[string]any{
		"message_id": id,
		"is_pinned":  pinned,
	}))

	action := "unpinned"
	if pinned {
		action = "pinned"
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"message":   "Message " + action,
		"is_pinned": pinned,
	})
	return nil
}

func optional(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
