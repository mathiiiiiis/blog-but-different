package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const userCols = `id, email, username, password_hash, is_admin, must_change_password, avatar, created_at, last_seen`

const guestAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func GenerateGuestUsername() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "guest_" + uuid.NewString()[:8]
	}
	var sb strings.Builder
	sb.WriteString("guest_")
	for _, c := range b {
		sb.WriteByte(guestAlphabet[int(c)%len(guestAlphabet)])
	}
	return sb.String()
}

func scanUser(row interface {
	Scan(dest ...any) error
}) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.IsAdmin,
		&u.MustChangePassword, &u.Avatar, &u.CreatedAt, &u.LastSeen)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) UserByID(ctx context.Context, id string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

func (s *Store) AdminByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userCols+` FROM users WHERE lower(email) = lower($1) AND is_admin = TRUE`, email))
}

// CreateGuest retries on the astronomically unlikely username collision rather
// than failing the request.
func (s *Store) CreateGuest(ctx context.Context, avatar string) (*User, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		u, err := scanUser(s.pool.QueryRow(ctx,
			`INSERT INTO users (id, username, is_admin, must_change_password, avatar)
			 VALUES ($1, $2, FALSE, FALSE, $3)
			 RETURNING `+userCols,
			uuid.NewString(), GenerateGuestUsername(), avatar))
		if err == nil {
			return u, nil
		}
		if !isUniqueViolation(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("could not allocate a guest username: %w", lastErr)
}

func (s *Store) EnsureAdmin(ctx context.Context, email, username, passwordHash string) (created bool, err error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO users (id, email, username, password_hash, is_admin, must_change_password, avatar)
		 VALUES ($1, $2, $3, $4, TRUE, TRUE, 'default')
		 ON CONFLICT DO NOTHING`,
		uuid.NewString(), email, username, passwordHash)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) SetPassword(ctx context.Context, userID, hash string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, must_change_password = FALSE WHERE id = $1`,
		userID, hash)
	return err
}

func (s *Store) SetAvatar(ctx context.Context, userID, avatar string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET avatar = $2 WHERE id = $1`, userID, avatar)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TouchLastSeen(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET last_seen = now() WHERE id = $1`, userID)
	return err
}

// PruneGuests removes anonymous accounts older than the cutoff, skipping any
// that are currently connected.
func (s *Store) PruneGuests(ctx context.Context, olderThan time.Duration, keep []string) (int64, error) {
	// A nil slice binds as SQL NULL, and `NOT (id = ANY(NULL))` evaluates to
	// NULL rather than true, which would silently match nothing.
	if keep == nil {
		keep = []string{}
	}
	cutoff := time.Now().UTC().Add(-olderThan)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM users
		  WHERE is_admin = FALSE
		    AND email IS NULL
		    AND created_at < $1
		    AND NOT (id = ANY($2::text[]))`,
		cutoff, keep)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
