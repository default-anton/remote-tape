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

	"github.com/default-anton/remote-tape/internal/auth"
	"github.com/default-anton/remote-tape/internal/config"
	"github.com/default-anton/remote-tape/internal/controlui"
	"github.com/default-anton/remote-tape/internal/database"
	"github.com/default-anton/remote-tape/internal/dns"
	"github.com/default-anton/remote-tape/internal/provisioning"
	"github.com/default-anton/remote-tape/internal/reconciler"
	"github.com/default-anton/remote-tape/internal/server"
	"github.com/default-anton/remote-tape/internal/session"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(run(ctx))
}

func run(ctx context.Context) int {
	cfg, err := config.Load()
	logger := newLogger("info")
	if err != nil {
		logger.ErrorContext(ctx, "configuration invalid", "error", err)
		return 2
	}
	logger = newLogger(cfg.General.LogLevel)
	logger.InfoContext(ctx, "configuration loaded", cfg.LogAttrs()...)
	authManager, err := newAuthManager(cfg, logger)
	if err != nil {
		logger.ErrorContext(ctx, "auth initialization failed", "error", err)
		return 1
	}
	if err := validateControlUI(cfg); err != nil {
		logger.ErrorContext(ctx, "control UI unavailable", "error", err)
		return 1
	}

	db, err := database.Open(ctx, cfg.General.DatabasePath)
	if err != nil {
		logger.ErrorContext(ctx, "database open failed", "error", err)
		return 1
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.ErrorContext(context.Background(), "database close failed", "error", err)
		}
	}()

	migrationResult, err := database.Migrate(ctx, db, logger)
	if err != nil {
		logger.ErrorContext(ctx, "database migration failed", "error", err)
		return 1
	}
	logger.InfoContext(ctx, "database ready",
		"schema_version", migrationResult.Current,
		"migrations_applied", len(migrationResult.Applied),
	)

	sessionRepo := session.NewRepository(db)
	provisioner, err := provisioning.NewDigitalOceanInstanceProvider(provisioning.DigitalOceanConfig{
		APIToken: string(cfg.Security.DigitalOceanAPIToken),
		SSHKeys:  cfg.Provisioning.DigitalOceanSSHKeys,
	})
	if err != nil {
		logger.ErrorContext(ctx, "digitalocean provisioner initialization failed", "error", err)
		return 1
	}
	dnsManager, err := newDNSManager(ctx, cfg)
	if err != nil {
		logger.ErrorContext(ctx, "cloudflare dns initialization failed", "error", err)
		return 1
	}

	listener, err := net.Listen("tcp", cfg.General.HTTPAddr)
	if err != nil {
		logger.ErrorContext(ctx, "http listen failed", "addr", cfg.General.HTTPAddr, "error", err)
		return 1
	}
	defer listener.Close()

	handler, err := server.New(db, logger, server.Options{
		ControlPlaneURL:     cfg.General.ControlPlaneURL,
		SessionsBaseDomain:  cfg.General.SessionsBaseDomain,
		DefaultRegion:       cfg.Provisioning.DefaultRegion,
		DefaultInstanceSize: cfg.Provisioning.DefaultInstanceSize,
		ImageID:             cfg.Provisioning.ImageID,
		Auth:                authManager,
	})
	if err != nil {
		logger.ErrorContext(ctx, "server initialization failed", "error", err)
		return 1
	}

	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	rec := reconciler.New(sessionRepo, provisioner, dnsManager, logger, reconciler.Options{Interval: cfg.Provisioning.ReconcileInterval, SessionsBaseDomain: cfg.General.SessionsBaseDomain})
	reconcileCtx, stopReconciler := context.WithCancel(ctx)
	reconcilerDone := make(chan struct{})
	go func() {
		rec.Run(reconcileCtx)
		close(reconcilerDone)
	}()
	defer func() {
		stopReconciler()
		<-reconcilerDone
	}()

	serveErr := make(chan error, 1)
	go func() {
		logger.InfoContext(ctx, "control plane listening", "addr", listener.Addr().String())
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.ErrorContext(shutdownCtx, "http server shutdown failed", "error", err)
			if closeErr := httpServer.Close(); closeErr != nil {
				logger.ErrorContext(context.Background(), "http server close failed", "error", closeErr)
			}
			return 1
		}
		if err := <-serveErr; err != nil {
			logger.ErrorContext(context.Background(), "http server failed", "error", err)
			return 1
		}
		logger.InfoContext(shutdownCtx, "control plane stopped")
		return 0
	case err := <-serveErr:
		if err != nil {
			logger.ErrorContext(ctx, "http server failed", "error", err)
			return 1
		}
		logger.InfoContext(ctx, "control plane stopped")
		return 0
	}
}

func newDNSManager(ctx context.Context, cfg config.Config) (dns.Manager, error) {
	return newDNSManagerWithAPI(ctx, cfg, "", nil)
}

func newDNSManagerWithAPI(ctx context.Context, cfg config.Config, apiBaseURL string, client *http.Client) (dns.Manager, error) {
	if !cfg.Security.CloudflareAPIToken.Set() {
		return dns.DisabledManager{}, nil
	}
	manager, err := dns.NewCloudflareManager(dns.CloudflareConfig{
		APIToken:   string(cfg.Security.CloudflareAPIToken),
		BaseDomain: cfg.General.SessionsBaseDomain,
		APIBaseURL: apiBaseURL,
		HTTPClient: client,
	})
	if err != nil {
		return nil, err
	}
	if err := manager.Validate(ctx); err != nil {
		return nil, err
	}
	return manager, nil
}

func newAuthManager(cfg config.Config, logger *slog.Logger) (*auth.Manager, error) {
	passwordHash := string(cfg.Security.AdminPasswordHash)
	if passwordHash == "" && cfg.General.Environment == config.EnvironmentDevelopment {
		var err error
		passwordHash, err = auth.HashPassword(string(cfg.Security.DevAdminPassword))
		if err != nil {
			return nil, err
		}
	}
	return auth.NewManager(auth.Config{
		PasswordHash:         passwordHash,
		CookieAuthKey:        []byte(cfg.Security.CookieAuthKey),
		CookieEncryptionKey:  []byte(cfg.Security.CookieEncryptionKey),
		SessionDuration:      cfg.Security.AdminCookieSessionDuration,
		RateLimitMaxAttempts: cfg.Security.LoginRateLimitMaxAttempts,
		RateLimitWindow:      cfg.Security.LoginRateLimitWindow,
		SecureCookies:        cfg.General.Environment == config.EnvironmentProduction,
		Logger:               logger,
	})
}

func validateControlUI(cfg config.Config) error {
	if cfg.General.Environment == config.EnvironmentProduction && !controlui.Built() {
		return errors.New("embedded web UI is not built; run pnpm --dir web build before building the Go binary")
	}
	return nil
}

func newLogger(level string) *slog.Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel}))
}
