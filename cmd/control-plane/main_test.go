package main

import (
	"context"
	"net"
	"path/filepath"
	"testing"
)

func TestRunReturnsFailureWhenHTTPPortUnavailable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	setRunTestEnv(t, listener.Addr().String(), filepath.Join(t.TempDir(), "control-plane.db"))

	if code := run(context.Background()); code != 1 {
		t.Fatalf("run() exit code = %d, want 1", code)
	}
}

func setRunTestEnv(t *testing.T, httpAddr string, databasePath string) {
	t.Helper()
	for _, name := range []string{
		"REMOTE_TAPE_ENV",
		"REMOTE_TAPE_HTTP_ADDR",
		"REMOTE_TAPE_DATABASE_PATH",
		"REMOTE_TAPE_CONTROL_PLANE_URL",
		"REMOTE_TAPE_SESSIONS_BASE_DOMAIN",
		"REMOTE_TAPE_LOG_LEVEL",
		"REMOTE_TAPE_DEFAULT_DROPLET_SIZE",
		"REMOTE_TAPE_DEFAULT_REGION",
		"REMOTE_TAPE_IMAGE_ID",
		"REMOTE_TAPE_HEALTH_CHECK_TIMEOUT",
		"REMOTE_TAPE_FINALIZATION_TIMEOUT",
		"REMOTE_TAPE_ADMIN_PASSWORD_HASH",
		"REMOTE_TAPE_DEV_ADMIN_PASSWORD",
		"REMOTE_TAPE_COOKIE_AUTH_KEY",
		"REMOTE_TAPE_COOKIE_ENCRYPTION_KEY",
		"REMOTE_TAPE_ADMIN_COOKIE_SESSION_DURATION",
		"REMOTE_TAPE_LOGIN_RATE_LIMIT_WINDOW",
		"REMOTE_TAPE_LOGIN_RATE_LIMIT_MAX_ATTEMPTS",
		"REMOTE_TAPE_ORPHANED_DROPLET_TTL",
		"REMOTE_TAPE_COMPLETED_SESSION_TTL",
		"REMOTE_TAPE_FAILED_SESSION_TTL",
		"REMOTE_TAPE_LOGS_RETENTION",
		"REMOTE_TAPE_DIGITALOCEAN_API_TOKEN",
		"REMOTE_TAPE_CLOUDFLARE_API_TOKEN",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("REMOTE_TAPE_HTTP_ADDR", httpAddr)
	t.Setenv("REMOTE_TAPE_DATABASE_PATH", databasePath)
	t.Setenv("REMOTE_TAPE_LOG_LEVEL", "error")
	t.Setenv("REMOTE_TAPE_DEV_ADMIN_PASSWORD", "dev-password")
	t.Setenv("REMOTE_TAPE_COOKIE_AUTH_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("REMOTE_TAPE_COOKIE_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
}
