package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const messageSelect = `
SELECT m.id, m.content, m.author_id, m.reply_to_id, m.is_pinned,
       m.created_at, m.updated_at, m.edited_at, m.attachments,
       a.username, a.avatar, a.is_admin,
       p.content, pa.username
  FROM messages m
  JOIN users a ON a.id = m.author_id
  LEFT JOIN messages p ON p.id = m.reply_to_id
  LEFT JOIN users pa ON pa.id = p.author_id`

type Cursor struct {
	CreatedAt time.Time
	ID        string
}

func (s *Store) scanMessages(ctx context.Context, sql string, args ...any) ([]Message, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		var raw []byte
		if err := rows.Scan(&m.ID, &m.Content, &m.AuthorID, &m.ReplyToID, &m.IsPinned,
			&m.CreatedAt, &m.UpdatedAt, &m.EditedAt, &raw,
			&m.AuthorUsername, &m.AuthorAvatar, &m.AuthorIsAdmin,
			&m.ReplyContent, &m.ReplyUsername); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &m.Attachments); err != nil {
				m.Attachments = nil
			}
		}
		m.Reactions = []Reaction{}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachReactions(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) attachReactions(ctx context.Context, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]string, len(msgs))
	index := make(map[string]int, len(msgs))
	for i := range msgs {
		ids[i] = msgs[i].ID
		index[msgs[i].ID] = i
	}

	rows, err := s.pool.Query(ctx, `
		SELECT r.message_id, r.emoji, u.username, u.avatar, ce.url
		  FROM reactions r
		  JOIN users u ON u.id = r.user_id
		  LEFT JOIN custom_emojis ce ON ce.id = r.custom_emoji_id
		 WHERE r.message_id = ANY($1::text[])
		 ORDER BY r.created_at, r.id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var msgID string
		var r Reaction
		if err := rows.Scan(&msgID, &r.Emoji, &r.Username, &r.Avatar, &r.CustomEmojiURL); err != nil {
			return err
		}
		if i, ok := index[msgID]; ok {
			msgs[i].Reactions = append(msgs[i].Reactions, r)
		}
	}
	return rows.Err()
}

// ListMessages returns up to limit+1 rows, newest first. The caller uses the
// extra row to decide has_more.
func (s *Store) ListMessages(ctx context.Context, limit int, before *Cursor) ([]Message, error) {
	if before != nil {
		return s.scanMessages(ctx, messageSelect+`
			 WHERE (m.created_at, m.id) < ($1, $2)
			 ORDER BY m.created_at DESC, m.id DESC
			 LIMIT $3`, before.CreatedAt, before.ID, limit+1)
	}
	return s.scanMessages(ctx, messageSelect+`
		 ORDER BY m.created_at DESC, m.id DESC
		 LIMIT $1`, limit+1)
}

func (s *Store) ListPinned(ctx context.Context) ([]Message, error) {
	return s.scanMessages(ctx, messageSelect+`
		 WHERE m.is_pinned = TRUE
		 ORDER BY m.created_at DESC, m.id DESC`)
}

func (s *Store) MessageByID(ctx context.Context, id string) (*Message, error) {
	msgs, err := s.scanMessages(ctx, messageSelect+` WHERE m.id = $1`, id)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, ErrNotFound
	}
	return &msgs[0], nil
}

func (s *Store) MessageCursor(ctx context.Context, id string) (*Cursor, error) {
	var c Cursor
	c.ID = id
	err := s.pool.QueryRow(ctx, `SELECT created_at FROM messages WHERE id = $1`, id).Scan(&c.CreatedAt)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

type NewMessage struct {
	Content     *string
	AuthorID    string
	ReplyToID   *string
	IsPinned    bool
	Attachments []Attachment
}

// ErrBadReply means reply_to_id pointed at a message that does not exist.
var ErrBadReply = errNamed("reply target does not exist")

type errString string

func (e errString) Error() string { return string(e) }
func errNamed(s string) error     { return errString(s) }

func (s *Store) CreateMessage(ctx context.Context, in NewMessage) (*Message, error) {
	if in.Attachments == nil {
		in.Attachments = []Attachment{}
	}
	raw, err := json.Marshal(in.Attachments)
	if err != nil {
		return nil, err
	}

	id := uuid.NewString()
	_, err = s.pool.Exec(ctx,
		`INSERT INTO messages (id, content, author_id, reply_to_id, is_pinned, attachments)
		 VALUES ($1, $2, $3, $4, $5, $6::json)`,
		id, in.Content, in.AuthorID, in.ReplyToID, in.IsPinned, string(raw))
	if err != nil {
		if isForeignKeyViolation(err) {
			return nil, ErrBadReply
		}
		return nil, err
	}
	return s.MessageByID(ctx, id)
}

func (s *Store) EditMessage(ctx context.Context, id, content string) (*Message, error) {
	var hasContent bool
	err := s.pool.QueryRow(ctx,
		`SELECT content IS NOT NULL AND content <> '' FROM messages WHERE id = $1`, id).Scan(&hasContent)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !hasContent {
		return nil, ErrAttachmentOnly
	}

	now := time.Now().UTC()
	_, err = s.pool.Exec(ctx,
		`UPDATE messages SET content = $2, edited_at = $3, updated_at = $3 WHERE id = $1`,
		id, content, now)
	if err != nil {
		return nil, err
	}
	return s.MessageByID(ctx, id)
}

var ErrAttachmentOnly = errNamed("message has no editable text content")

func (s *Store) TogglePin(ctx context.Context, id string) (bool, error) {
	var pinned bool
	err := s.pool.QueryRow(ctx,
		`UPDATE messages SET is_pinned = NOT is_pinned WHERE id = $1 RETURNING is_pinned`, id).Scan(&pinned)
	if err != nil {
		if isNoRows(err) {
			return false, ErrNotFound
		}
		return false, err
	}
	return pinned, nil
}

// DeleteMessage removes the row and returns the storage objects that are now
// orphaned so the caller can clean them up.
func (s *Store) DeleteMessage(ctx context.Context, id string) ([]string, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`DELETE FROM messages WHERE id = $1 RETURNING attachments`, id).Scan(&raw)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return objectNames(raw), nil
}

// ClearMessages empties the board and returns every orphaned storage object.
func (s *Store) ClearMessages(ctx context.Context) ([]string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `DELETE FROM messages RETURNING attachments`)
	if err != nil {
		return nil, err
	}
	var objects []string
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		objects = append(objects, objectNames(raw)...)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return objects, nil
}

func objectNames(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var atts []Attachment
	if err := json.Unmarshal(raw, &atts); err != nil {
		return nil
	}
	var out []string
	for _, a := range atts {
		if a.ObjectName != "" {
			out = append(out, a.ObjectName)
		}
	}
	return out
}

func (s *Store) Counts(ctx context.Context) (messages, reactions int64, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM messages), (SELECT count(*) FROM reactions)`).
		Scan(&messages, &reactions)
	return
}
