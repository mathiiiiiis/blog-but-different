package store

import (
	"context"

	"github.com/google/uuid"
)

type ToggleResult struct {
	Added          bool
	CustomEmojiURL *string
}

// ToggleReaction is a single round trip per branch and tolerates two
// concurrent identical requests without erroring.
func (s *Store) ToggleReaction(ctx context.Context, messageID, userID, emoji string, customEmojiID *string) (*ToggleResult, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM messages WHERE id = $1)`, messageID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}

	var url *string
	if customEmojiID != nil && *customEmojiID != "" {
		err := s.pool.QueryRow(ctx, `SELECT url FROM custom_emojis WHERE id = $1`, *customEmojiID).Scan(&url)
		if err != nil {
			if !isNoRows(err) {
				return nil, err
			}
			customEmojiID = nil
		}
	} else {
		customEmojiID = nil
	}

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM reactions WHERE message_id = $1 AND user_id = $2 AND emoji = $3`,
		messageID, userID, emoji)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() > 0 {
		return &ToggleResult{Added: false, CustomEmojiURL: url}, nil
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO reactions (id, message_id, user_id, emoji, custom_emoji_id)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (message_id, user_id, emoji) DO NOTHING`,
		uuid.NewString(), messageID, userID, emoji, customEmojiID)
	if err != nil {
		return nil, err
	}
	return &ToggleResult{Added: true, CustomEmojiURL: url}, nil
}

func (s *Store) ListCustomEmojis(ctx context.Context) ([]CustomEmoji, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, url, object_name, created_at FROM custom_emojis ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []CustomEmoji{}
	for rows.Next() {
		var e CustomEmoji
		if err := rows.Scan(&e.ID, &e.Name, &e.URL, &e.ObjectName, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

var ErrDuplicate = errNamed("already exists")

func (s *Store) CreateCustomEmoji(ctx context.Context, name, url, objectName, createdBy string) (*CustomEmoji, error) {
	var e CustomEmoji
	err := s.pool.QueryRow(ctx,
		`INSERT INTO custom_emojis (id, name, url, object_name, created_by_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, name, url, object_name, created_at`,
		uuid.NewString(), name, url, objectName, createdBy).
		Scan(&e.ID, &e.Name, &e.URL, &e.ObjectName, &e.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return &e, nil
}

func (s *Store) DeleteCustomEmoji(ctx context.Context, id string) (*CustomEmoji, error) {
	var e CustomEmoji
	err := s.pool.QueryRow(ctx,
		`DELETE FROM custom_emojis WHERE id = $1
		 RETURNING id, name, url, object_name, created_at`, id).
		Scan(&e.ID, &e.Name, &e.URL, &e.ObjectName, &e.CreatedAt)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &e, nil
}

func (s *Store) SaveFCMToken(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO fcm_tokens (id, token) VALUES ($1, $2) ON CONFLICT (token) DO NOTHING`,
		uuid.NewString(), token)
	return err
}

func (s *Store) FCMTokens(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT token FROM fcm_tokens`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) DeleteFCMTokens(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM fcm_tokens WHERE token = ANY($1::text[])`, tokens)
	return err
}
