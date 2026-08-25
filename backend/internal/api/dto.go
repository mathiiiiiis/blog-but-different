package api

import (
	"time"

	"github.com/mathiiiiiis/blog-but-different/backend/internal/store"
)

type tokenResponse struct {
	AccessToken        string `json:"access_token"`
	TokenType          string `json:"token_type"`
	MustChangePassword bool   `json:"must_change_password"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	IsAdmin   bool      `json:"is_admin"`
	CanPost   bool      `json:"can_post"`
	Avatar    string    `json:"avatar"`
	CreatedAt time.Time `json:"created_at"`
}

func newUserResponse(u *store.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Username:  u.Username,
		IsAdmin:   u.IsAdmin,
		CanPost:   u.CanPost(),
		Avatar:    orDefault(u.Avatar),
		CreatedAt: u.CreatedAt,
	}
}

type attachmentResponse struct {
	Type       string  `json:"type"`
	URL        string  `json:"url"`
	Name       string  `json:"name"`
	Size       *int64  `json:"size"`
	GifID      *string `json:"gif_id"`
	PreviewURL *string `json:"preview_url"`
}

type reactionSummary struct {
	Emoji          string   `json:"emoji"`
	Count          int      `json:"count"`
	Users          []string `json:"users"`
	UserAvatars    []string `json:"user_avatars"`
	CustomEmojiURL *string  `json:"custom_emoji_url"`
}

type replyInfo struct {
	ID             string  `json:"id"`
	Content        *string `json:"content"`
	AuthorUsername string  `json:"author_username"`
}

type messageResponse struct {
	ID             string               `json:"id"`
	Content        *string              `json:"content"`
	AuthorID       string               `json:"author_id"`
	AuthorUsername string               `json:"author_username"`
	AuthorAvatar   string               `json:"author_avatar"`
	IsAdmin        bool                 `json:"is_admin"`
	IsPinned       bool                 `json:"is_pinned"`
	Attachments    []attachmentResponse `json:"attachments"`
	Reactions      []reactionSummary    `json:"reactions"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      *time.Time           `json:"updated_at"`
	EditedAt       *time.Time           `json:"edited_at"`
	ReplyTo        *replyInfo           `json:"reply_to"`
}

type messageList struct {
	Messages       []messageResponse `json:"messages"`
	PinnedMessages []messageResponse `json:"pinned_messages"`
	Total          int               `json:"total"`
	HasMore        bool              `json:"has_more"`
}

type commandResponse struct {
	Success bool           `json:"success"`
	Command string         `json:"command"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

type customEmojiResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

func newCustomEmojiResponse(e *store.CustomEmoji) customEmojiResponse {
	return customEmojiResponse{ID: e.ID, Name: e.Name, URL: e.URL, CreatedAt: e.CreatedAt}
}

// newMessageResponse collapses reactions into per-emoji summaries, preserving
// first-seen ordering so the UI does not reshuffle on every refresh.
func newMessageResponse(m *store.Message) messageResponse {
	order := make([]string, 0, len(m.Reactions))
	byEmoji := make(map[string]*reactionSummary, len(m.Reactions))

	for _, r := range m.Reactions {
		summary, ok := byEmoji[r.Emoji]
		if !ok {
			summary = &reactionSummary{
				Emoji:       r.Emoji,
				Users:       []string{},
				UserAvatars: []string{},
			}
			byEmoji[r.Emoji] = summary
			order = append(order, r.Emoji)
		}
		summary.Count++
		summary.Users = append(summary.Users, r.Username)
		summary.UserAvatars = append(summary.UserAvatars, orDefault(r.Avatar))
		if summary.CustomEmojiURL == nil && r.CustomEmojiURL != nil {
			summary.CustomEmojiURL = r.CustomEmojiURL
		}
	}

	reactions := make([]reactionSummary, 0, len(order))
	for _, emoji := range order {
		reactions = append(reactions, *byEmoji[emoji])
	}

	attachments := make([]attachmentResponse, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		attachments = append(attachments, attachmentResponse{
			Type:       a.Type,
			URL:        a.URL,
			Name:       a.Name,
			Size:       a.Size,
			GifID:      a.GifID,
			PreviewURL: a.PreviewURL,
		})
	}

	var reply *replyInfo
	if m.ReplyToID != nil && m.ReplyUsername != nil {
		content := m.ReplyContent
		if content == nil || *content == "" {
			placeholder := "Attachment"
			content = &placeholder
		}
		reply = &replyInfo{ID: *m.ReplyToID, Content: content, AuthorUsername: *m.ReplyUsername}
	}

	return messageResponse{
		ID:             m.ID,
		Content:        m.Content,
		AuthorID:       m.AuthorID,
		AuthorUsername: m.AuthorUsername,
		AuthorAvatar:   orDefault(m.AuthorAvatar),
		IsAdmin:        m.AuthorIsAdmin,
		IsPinned:       m.IsPinned,
		Attachments:    attachments,
		Reactions:      reactions,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
		EditedAt:       m.EditedAt,
		ReplyTo:        reply,
	}
}

func orDefault(avatar string) string {
	if avatar == "" {
		return "default"
	}
	return avatar
}
