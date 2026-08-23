package blob

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store interface {
	Put(ctx context.Context, folder, filename, contentType string, inline bool, data []byte) (string, error)
	Delete(ctx context.Context, objectName string) error
}

type Minio struct {
	client *minio.Client
	bucket string
}

type Options struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Secure    bool
}

func New(ctx context.Context, opt Options) (*Minio, error) {
	client, err := minio.New(opt.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(opt.AccessKey, opt.SecretKey, ""),
		Secure: opt.Secure,
	})
	if err != nil {
		return nil, err
	}

	m := &Minio{client: client, bucket: opt.Bucket}
	if err := m.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Minio) ensureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("checking bucket %q: %w", m.bucket, err)
	}
	if exists {
		return nil
	}
	if err := m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("creating bucket %q: %w", m.bucket, err)
	}

	policy, err := json.Marshal(map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{map[string]any{
			"Effect":    "Allow",
			"Principal": map[string]any{"AWS": "*"},
			"Action":    []string{"s3:GetObject"},
			"Resource":  []string{fmt.Sprintf("arn:aws:s3:::%s/*", m.bucket)},
		}},
	})
	if err != nil {
		return err
	}
	if err := m.client.SetBucketPolicy(ctx, m.bucket, string(policy)); err != nil {
		return fmt.Errorf("setting read policy on %q: %w", m.bucket, err)
	}
	slog.Info("created storage bucket", "bucket", m.bucket)
	return nil
}

func (m *Minio) Put(ctx context.Context, folder, filename, contentType string, inline bool, data []byte) (string, error) {
	object := path.Join(folder, uuid.NewString()+extension(filename))

	opts := minio.PutObjectOptions{
		ContentType:  contentType,
		CacheControl: "public, max-age=31536000, immutable",
	}
	if !inline {
		opts.ContentDisposition = fmt.Sprintf("attachment; filename*=UTF-8''%s", urlEscape(safeFilename(filename)))
	}

	_, err := m.client.PutObject(ctx, m.bucket, object, bytes.NewReader(data), int64(len(data)), opts)
	if err != nil {
		return "", err
	}
	return object, nil
}

func (m *Minio) Delete(ctx context.Context, objectName string) error {
	return m.client.RemoveObject(ctx, m.bucket, objectName, minio.RemoveObjectOptions{})
}

// extension keeps a short, lowercase, alphanumeric suffix so stored keys stay
// recognisable without ever echoing user-controlled path segments.
func extension(filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	if len(ext) < 2 || len(ext) > 6 {
		return ""
	}
	for _, r := range ext[1:] {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') {
			return ""
		}
	}
	return ext
}

func safeFilename(filename string) string {
	name := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return "download"
	}
	if len(name) > 128 {
		name = name[:128]
	}
	return name
}

func urlEscape(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0F])
	}
	return b.String()
}
