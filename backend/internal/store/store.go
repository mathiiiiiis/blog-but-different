package store

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Migrate applies the schema. Every statement is idempotent, so it is safe to
// run against a fresh database or one created by the previous stack.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaSQL)
	return err
}

type User struct {
	ID                 string
	Email              *string
	Username           string
	PasswordHash       *string
	IsAdmin            bool
	MustChangePassword bool
	Avatar             string
	CreatedAt          time.Time
	LastSeen           time.Time
}

func (u *User) CanPost() bool { return u.IsAdmin }

type Attachment struct {
	Type       string  `json:"type"`
	URL        string  `json:"url"`
	Name       string  `json:"name"`
	Size       *int64  `json:"size,omitempty"`
	ObjectName string  `json:"object_name,omitempty"`
	GifID      *string `json:"gif_id,omitempty"`
	PreviewURL *string `json:"preview_url,omitempty"`
}

type Message struct {
	ID          string
	Content     *string
	AuthorID    string
	ReplyToID   *string
	IsPinned    bool
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	EditedAt    *time.Time
	Attachments []Attachment

	AuthorUsername string
	AuthorAvatar   string
	AuthorIsAdmin  bool

	ReplyContent  *string
	ReplyUsername *string

	Reactions []Reaction
}

type Reaction struct {
	Emoji          string
	Username       string
	Avatar         string
	CustomEmojiURL *string
}

type CustomEmoji struct {
	ID         string
	Name       string
	URL        string
	ObjectName *string
	CreatedAt  time.Time
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
