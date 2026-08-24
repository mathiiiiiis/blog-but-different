package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const scope = "https://www.googleapis.com/auth/firebase.messaging"

// TokenStore is the subset of the database the sender needs.
type TokenStore interface {
	FCMTokens(ctx context.Context) ([]string, error)
	DeleteFCMTokens(ctx context.Context, tokens []string) error
}

// Sender delivers notifications through FCM HTTP v1. A nil Sender is a no-op,
// so callers never need to branch on whether push is configured.
type Sender struct {
	projectID string
	source    oauth2.TokenSource
	store     TokenStore
	http      *http.Client
	baseURL   string
}

// NewSender returns nil when no credentials path is configured.
func NewSender(ctx context.Context, credentialsPath string, store TokenStore) (*Sender, error) {
	if credentialsPath == "" {
		return nil, nil
	}

	raw, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("reading FCM credentials: %w", err)
	}

	var meta struct {
		ProjectID string `json:"project_id"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("parsing FCM credentials: %w", err)
	}
	if meta.ProjectID == "" {
		return nil, fmt.Errorf("FCM credentials are missing project_id")
	}

	creds, err := google.CredentialsFromJSON(ctx, raw, scope)
	if err != nil {
		return nil, fmt.Errorf("loading FCM credentials: %w", err)
	}

	return &Sender{
		projectID: meta.ProjectID,
		source:    creds.TokenSource,
		store:     store,
		http:      &http.Client{Timeout: 15 * time.Second},
		baseURL:   "https://fcm.googleapis.com",
	}, nil
}

// Notify fans out to every registered device and prunes tokens FCM reports as
// dead. It is safe to call on a nil receiver.
func (s *Sender) Notify(ctx context.Context, title, body string) {
	if s == nil {
		return
	}

	tokens, err := s.store.FCMTokens(ctx)
	if err != nil {
		slog.Error("loading push tokens", "error", err)
		return
	}
	if len(tokens) == 0 {
		return
	}

	accessToken, err := s.source.Token()
	if err != nil {
		slog.Error("minting FCM access token", "error", err)
		return
	}

	var (
		mu    sync.Mutex
		dead  []string
		wg    sync.WaitGroup
		limit = make(chan struct{}, 8)
	)

	for _, token := range tokens {
		wg.Add(1)
		limit <- struct{}{}
		go func(token string) {
			defer wg.Done()
			defer func() { <-limit }()

			expired, err := s.send(ctx, accessToken.AccessToken, token, title, body)
			if err != nil {
				slog.Warn("push delivery failed", "error", err)
			}
			if expired {
				mu.Lock()
				dead = append(dead, token)
				mu.Unlock()
			}
		}(token)
	}
	wg.Wait()

	if len(dead) > 0 {
		if err := s.store.DeleteFCMTokens(ctx, dead); err != nil {
			slog.Error("pruning dead push tokens", "error", err)
		} else {
			slog.Info("pruned dead push tokens", "count", len(dead))
		}
	}
}

// send reports whether the token should be removed from the database.
func (s *Sender) send(ctx context.Context, accessToken, deviceToken, title, body string) (expired bool, err error) {
	payload, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"token": deviceToken,
			"notification": map[string]any{
				"title": title,
				"body":  body,
			},
		},
	})
	if err != nil {
		return false, err
	}

	url := fmt.Sprintf("%s/v1/projects/%s/messages:send", s.baseURL, s.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return false, nil
	}

	// 404 UNREGISTERED and 400 INVALID_ARGUMENT on the token field both mean
	// the registration is gone for good; anything else may be transient.
	if resp.StatusCode == http.StatusNotFound ||
		strings.Contains(string(respBody), "UNREGISTERED") ||
		strings.Contains(string(respBody), "registration-token-not-registered") {
		return true, nil
	}
	return false, fmt.Errorf("fcm returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
