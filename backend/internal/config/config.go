package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Debug bool

	DatabaseURL string

	SecretKey   []byte
	TokenTTL    time.Duration
	TrustProxy  bool
	CORSOrigins []string

	AdminEmail           string
	AdminDefaultPassword string

	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioSecure    bool
	MinioPublicURL string

	AvatarsConfigPath string
	AvatarsDir        string

	MaxFileSize    int64
	MaxMessageLen  int
	MaxAttachments int

	KlipyAPIKey string

	FCMCredentialsPath string

	ListenAddr string
}

var ErrMissingSecret = errors.New("SECRET_KEY must be set (generate one with: openssl rand -hex 32)")

func Load() (*Config, error) {
	c := &Config{
		Debug:                envBool("DEBUG", false),
		DatabaseURL:          env("DATABASE_URL", ""),
		TokenTTL:             time.Duration(envInt("ACCESS_TOKEN_EXPIRE_MINUTES", 60*24*365)) * time.Minute,
		TrustProxy:           envBool("TRUST_PROXY", true),
		AdminEmail:           strings.ToLower(strings.TrimSpace(env("ADMIN_EMAIL", ""))),
		AdminDefaultPassword: env("ADMIN_DEFAULT_PASSWORD", ""),
		MinioEndpoint:        env("MINIO_ENDPOINT", "minio:9000"),
		MinioAccessKey:       env("MINIO_ACCESS_KEY", ""),
		MinioSecretKey:       env("MINIO_SECRET_KEY", ""),
		MinioBucket:          env("MINIO_BUCKET", "blog"),
		MinioSecure:          envBool("MINIO_SECURE", false),
		MinioPublicURL:       strings.TrimRight(env("MINIO_PUBLIC_URL", ""), "/"),
		AvatarsConfigPath:    env("AVATARS_CONFIG_PATH", "./avatars/avatars.json"),
		AvatarsDir:           env("AVATARS_DIR", "./avatars"),
		MaxFileSize:          int64(envInt("MAX_FILE_SIZE", 50*1024*1024)),
		MaxMessageLen:        envInt("MAX_MESSAGE_LENGTH", 10000),
		MaxAttachments:       envInt("MAX_ATTACHMENTS", 10),
		KlipyAPIKey:          env("KLIPY_API_KEY", ""),
		FCMCredentialsPath:   env("FCM_CREDENTIALS_PATH", ""),
		ListenAddr:           env("LISTEN_ADDR", ":8000"),
	}

	secret := env("SECRET_KEY", "")
	if secret == "" {
		return nil, ErrMissingSecret
	}
	if len(secret) < 16 {
		return nil, errors.New("SECRET_KEY must be at least 16 characters")
	}
	c.SecretKey = []byte(secret)

	raw := env("DATABASE_URL", "")
	if raw == "" {
		return nil, errors.New("DATABASE_URL must be set")
	}
	c.DatabaseURL = normalizeDSN(raw)

	origins := strings.TrimSpace(env("CORS_ORIGINS", ""))
	if origins != "" && origins != "*" {
		for _, o := range strings.Split(origins, ",") {
			if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
				c.CORSOrigins = append(c.CORSOrigins, o)
			}
		}
	}

	if c.MaxFileSize <= 0 {
		return nil, fmt.Errorf("MAX_FILE_SIZE must be positive")
	}
	return c, nil
}

// normalizeDSN accepts the SQLAlchemy-flavoured URLs the previous deployment
// used so an existing .env keeps working without edits.
func normalizeDSN(raw string) string {
	for _, prefix := range []string{"postgresql+asyncpg://", "postgresql+psycopg2://", "postgres+asyncpg://"} {
		if strings.HasPrefix(raw, prefix) {
			return "postgres://" + strings.TrimPrefix(raw, prefix)
		}
	}
	if strings.HasPrefix(raw, "postgresql://") {
		return "postgres://" + strings.TrimPrefix(raw, "postgresql://")
	}
	return raw
}

func (c *Config) AttachmentURL(objectName string) string {
	if c.MinioPublicURL != "" {
		return fmt.Sprintf("%s/%s/%s", c.MinioPublicURL, c.MinioBucket, objectName)
	}
	return fmt.Sprintf("/%s/%s", c.MinioBucket, objectName)
}

func (c *Config) AllowOrigin(origin string) string {
	if len(c.CORSOrigins) == 0 {
		return ""
	}
	for _, o := range c.CORSOrigins {
		if strings.EqualFold(o, origin) {
			return o
		}
	}
	return ""
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}
