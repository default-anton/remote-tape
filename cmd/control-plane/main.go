package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/default-anton/remote-tape/internal/config"
	"github.com/default-anton/remote-tape/internal/database"
	"github.com/default-anton/remote-tape/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	logger := newLogger("info")
	if err != nil {
		logger.ErrorContext(ctx, "configuration invalid", "error", err)
		os.Exit(2)
	}
	logger = newLogger(cfg.General.LogLevel)
	logger.InfoContext(ctx, "configuration loaded", cfg.LogAttrs()...)

	db, err := database.Open(ctx, cfg.General.DatabasePath)
	if err != nil {
		logger.ErrorContext(ctx, "database open failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.ErrorContext(context.Background(), "database close failed", "error", err)
		}
	}()

	migrationResult, err := database.Migrate(ctx, db, logger)
	if err != nil {
		logger.ErrorContext(ctx, "database migration failed", "error", err)
		os.Exit(1)
	}
	logger.InfoContext(ctx, "database ready",
		"schema_version", migrationResult.Current,
		"migrations_applied", len(migrationResult.Applied),
	)

	httpServer := &http.Server{
		Addr:              cfg.General.HTTPAddr,
		Handler:           server.New(db, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.InfoContext(ctx, "control plane listening", "addr", cfg.General.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.ErrorContext(context.Background(), "http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.ErrorContext(shutdownCtx, "http server shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.InfoContext(shutdownCtx, "control plane stopped")
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
