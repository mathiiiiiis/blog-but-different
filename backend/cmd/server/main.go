package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mathiiiiiis/blog-but-different/backend/internal/api"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/auth"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/blob"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/config"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/gifs"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/hub"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/push"
	"github.com/mathiiiiiis/blog-but-different/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startupCtx, cancelStartup := context.WithTimeout(ctx, 60*time.Second)
	defer cancelStartup()

	db, err := waitForDatabase(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(startupCtx); err != nil {
		return err
	}
	slog.Info("schema is up to date")

	if err := bootstrapAdmin(startupCtx, db, cfg); err != nil {
		return err
	}

	blobs, err := blob.New(startupCtx, blob.Options{
		Endpoint:  cfg.MinioEndpoint,
		AccessKey: cfg.MinioAccessKey,
		SecretKey: cfg.MinioSecretKey,
		Bucket:    cfg.MinioBucket,
		Secure:    cfg.MinioSecure,
	})
	if err != nil {
		return err
	}

	sender, err := push.NewSender(ctx, cfg.FCMCredentialsPath, db)
	if err != nil {
		// Push is optional; a bad credentials file must not stop the server.
		slog.Error("push notifications disabled", "error", err)
		sender = nil
	} else if sender == nil {
		slog.Info("push notifications disabled: no credentials configured")
	}

	connections := hub.New()

	server := api.NewServer(api.Deps{
		Config: cfg,
		Store:  db,
		Signer: auth.NewSigner(cfg.SecretKey, cfg.TokenTTL),
		Blobs:  blobs,
		Hub:    connections,
		Gifs:   gifs.NewClient(cfg.KlipyAPIKey),
		Push:   sender,
	})

	background := make(chan struct{})
	defer close(background)
	server.StartBackground(background)
	go pruneGuests(ctx, db, connections)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: it would sever long-lived websockets.
		IdleTimeout: 120 * time.Second,
		ErrorLog:    slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

// waitForDatabase retries so the container does not need an entrypoint script
// polling pg_isready before it starts.
func waitForDatabase(ctx context.Context, dsn string) (*store.Store, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		db, err := store.New(ctx, dsn)
		if err == nil {
			return db, nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return nil, errors.Join(lastErr, ctx.Err())
		}
		slog.Info("waiting for database", "attempt", attempt+1, "error", err)

		select {
		case <-ctx.Done():
			return nil, errors.Join(lastErr, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func bootstrapAdmin(ctx context.Context, db *store.Store, cfg *config.Config) error {
	if cfg.AdminEmail == "" {
		slog.Warn("ADMIN_EMAIL is unset, skipping admin bootstrap")
		return nil
	}
	if _, err := db.AdminByEmail(ctx, cfg.AdminEmail); err == nil {
		slog.Info("admin account present", "email", cfg.AdminEmail)
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	if cfg.AdminDefaultPassword == "" {
		return errors.New("ADMIN_DEFAULT_PASSWORD must be set to create the initial admin account")
	}
	hash, err := auth.HashPassword(cfg.AdminDefaultPassword)
	if err != nil {
		return err
	}
	created, err := db.EnsureAdmin(ctx, cfg.AdminEmail, "admin", hash)
	if err != nil {
		return err
	}
	if created {
		slog.Info("created admin account", "email", cfg.AdminEmail)
	}
	return nil
}

// pruneGuests drops anonymous accounts that have been idle for a day, skipping
// anyone currently connected.
func pruneGuests(ctx context.Context, db *store.Store, connections *hub.Hub) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			removed, err := db.PruneGuests(sweepCtx, 24*time.Hour, connections.OnlineUserIDs())
			cancel()
			if err != nil {
				slog.Error("pruning guests", "error", err)
				continue
			}
			if removed > 0 {
				slog.Info("pruned stale guest accounts", "count", removed)
			}
		}
	}
}
