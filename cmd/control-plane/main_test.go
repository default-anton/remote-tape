package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/default-anton/remote-tape/internal/config"
	"github.com/default-anton/remote-tape/internal/dns"
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

func TestRunReturnsFailureForInvalidProvisioningDefaults(t *testing.T) {
	setRunTestEnv(t, "127.0.0.1:0", filepath.Join(t.TempDir(), "control-plane.db"))
	t.Setenv("REMOTE_TAPE_DEFAULT_REGION", "nyc3")
	t.Setenv("REMOTE_TAPE_DEFAULT_INSTANCE_SIZE", "c-2")

	if code := run(context.Background()); code != 1 {
		t.Fatalf("run() exit code = %d, want 1", code)
	}
}

func TestNewDNSManagerUsesDisabledManagerWithoutToken(t *testing.T) {
	cfg := config.Config{
		General: config.GeneralSettings{SessionsBaseDomain: "sessions.example.com"},
	}
	manager, err := newDNSManagerWithAPI(context.Background(), cfg, "http://127.0.0.1", nil)
	if err != nil {
		t.Fatalf("newDNSManagerWithAPI() error = %v", err)
	}
	if _, ok := manager.(dns.DisabledManager); !ok {
		t.Fatalf("manager type = %T, want dns.DisabledManager", manager)
	}
}

func TestNewDNSManagerValidatesCloudflareZoneWithToken(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer cf_test" {
			t.Fatalf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		var result []map[string]string
		if r.URL.Path == "/client/v4/zones" && r.URL.Query().Get("name") == "example.com" {
			result = []map[string]string{{"id": "zone_123", "name": "example.com"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": result})
	}))
	defer server.Close()

	cfg := config.Config{
		General:  config.GeneralSettings{SessionsBaseDomain: "sessions.example.com"},
		Security: config.SecuritySettings{CloudflareAPIToken: "cf_test"},
	}
	manager, err := newDNSManagerWithAPI(context.Background(), cfg, server.URL, server.Client())
	if err != nil {
		t.Fatalf("newDNSManagerWithAPI() error = %v", err)
	}
	if _, ok := manager.(*dns.CloudflareManager); !ok {
		t.Fatalf("manager type = %T, want *dns.CloudflareManager", manager)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want suffix discovery requests", requests)
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
		"REMOTE_TAPE_DEFAULT_INSTANCE_SIZE",
		"REMOTE_TAPE_DEFAULT_REGION",
		"REMOTE_TAPE_IMAGE_ID",
		"REMOTE_TAPE_RECONCILE_INTERVAL",
		"REMOTE_TAPE_HEALTH_CHECK_TIMEOUT",
		"REMOTE_TAPE_FINALIZATION_TIMEOUT",
		"REMOTE_TAPE_ADMIN_PASSWORD_HASH",
		"REMOTE_TAPE_DEV_ADMIN_PASSWORD",
		"REMOTE_TAPE_COOKIE_AUTH_KEY",
		"REMOTE_TAPE_COOKIE_ENCRYPTION_KEY",
		"REMOTE_TAPE_ADMIN_COOKIE_SESSION_DURATION",
		"REMOTE_TAPE_LOGIN_RATE_LIMIT_WINDOW",
		"REMOTE_TAPE_LOGIN_RATE_LIMIT_MAX_ATTEMPTS",
		"REMOTE_TAPE_ORPHANED_INSTANCE_TTL",
		"REMOTE_TAPE_COMPLETED_SESSION_TTL",
		"REMOTE_TAPE_FAILED_SESSION_TTL",
		"REMOTE_TAPE_LOGS_RETENTION",
		"REMOTE_TAPE_DIGITALOCEAN_API_TOKEN",
		"REMOTE_TAPE_DIGITALOCEAN_SSH_KEYS",
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
	t.Setenv("REMOTE_TAPE_DIGITALOCEAN_API_TOKEN", "dop_v1_test")
	t.Setenv("REMOTE_TAPE_DIGITALOCEAN_SSH_KEYS", "12345")
}
