package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultLoad(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.General.Environment != EnvironmentDevelopment {
		t.Fatalf("Environment = %q", cfg.General.Environment)
	}
	if cfg.General.HTTPAddr != "127.0.0.1:8080" {
		t.Fatalf("HTTPAddr = %q", cfg.General.HTTPAddr)
	}
	if cfg.Security.AdminCookieSessionDuration != 7*24*time.Hour {
		t.Fatalf("AdminCookieSessionDuration = %v", cfg.Security.AdminCookieSessionDuration)
	}
}

func TestProductionValidation(t *testing.T) {
	cfg := validConfig()
	cfg.General.Environment = EnvironmentProduction
	cfg.General.ControlPlaneURL = "http://control.example.com"
	cfg.General.SessionsBaseDomain = "sessions.localhost"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Fatalf("Validate() error missing https requirement: %v", err)
	}
	if !strings.Contains(err.Error(), "REMOTE_TAPE_SESSIONS_BASE_DOMAIN") {
		t.Fatalf("Validate() error missing sessions domain requirement: %v", err)
	}
}

func TestDomainValidationRejectsInvalidDNSNames(t *testing.T) {
	for _, domain := range []string{
		strings.Repeat("a", 64) + ".example.com",
		"-sessions.example.com",
		"sessions-.example.com",
		"sessions_remote.example.com",
		strings.Repeat("a", 250) + ".com",
	} {
		cfg := validConfig()
		cfg.General.SessionsBaseDomain = domain
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "REMOTE_TAPE_SESSIONS_BASE_DOMAIN") {
			t.Fatalf("Validate() for %q error = %v", domain, err)
		}
	}
}

func TestDurationParsingSupportsDays(t *testing.T) {
	duration, err := parseDuration("14d")
	if err != nil {
		t.Fatalf("parseDuration() error = %v", err)
	}
	if duration != 14*24*time.Hour {
		t.Fatalf("parseDuration() = %v", duration)
	}
}

func validConfig() Config {
	return Config{
		General: GeneralSettings{
			Environment:        EnvironmentDevelopment,
			HTTPAddr:           "127.0.0.1:8080",
			DatabasePath:       "./data/control-plane.db",
			ControlPlaneURL:    "http://127.0.0.1:8080",
			SessionsBaseDomain: "sessions.localhost",
			LogLevel:           "info",
		},
		Provisioning: ProvisioningSettings{
			DefaultDropletSize:  "s-2vcpu-2gb",
			DefaultRegion:       "nyc3",
			ImageID:             "ubuntu-24-04-x64",
			HealthCheckTimeout:  time.Minute,
			FinalizationTimeout: 15 * time.Minute,
		},
		Security: SecuritySettings{
			AdminCookieSessionDuration: 7 * 24 * time.Hour,
			LoginRateLimitMaxAttempts:  5,
			LoginRateLimitWindow:       15 * time.Minute,
		},
		Cleanup: CleanupSettings{
			OrphanedDropletTTL:  2 * time.Hour,
			CompletedSessionTTL: 7 * 24 * time.Hour,
			FailedSessionTTL:    24 * time.Hour,
			LogsRetention:       14 * 24 * time.Hour,
		},
	}
}

func clearEnv(t *testing.T) {
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
}
