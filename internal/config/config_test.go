package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultLoad(t *testing.T) {
	clearEnv(t)
	t.Setenv("REMOTE_TAPE_DEV_ADMIN_PASSWORD", "dev-password")
	t.Setenv("REMOTE_TAPE_COOKIE_AUTH_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("REMOTE_TAPE_COOKIE_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("REMOTE_TAPE_DIGITALOCEAN_API_TOKEN", "dop_v1_test")
	t.Setenv("REMOTE_TAPE_DIGITALOCEAN_SSH_KEYS", "12345")

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
	if cfg.Provisioning.ReconcileInterval != 5*time.Second {
		t.Fatalf("ReconcileInterval = %v", cfg.Provisioning.ReconcileInterval)
	}
}

func TestReconcileIntervalMustBePositive(t *testing.T) {
	cfg := validConfig()
	cfg.Provisioning.ReconcileInterval = 0

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "REMOTE_TAPE_RECONCILE_INTERVAL") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDevelopmentConfigWithoutCloudflareTokenIsValid(t *testing.T) {
	cfg := validConfig()
	cfg.Security.CloudflareAPIToken = ""

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestProductionConfigWithoutCloudflareTokenIsInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.General.Environment = EnvironmentProduction
	cfg.General.ControlPlaneURL = "https://control.example.com"
	cfg.General.SessionsBaseDomain = "sessions.example.com"
	cfg.Security.CloudflareAPIToken = ""

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "REMOTE_TAPE_CLOUDFLARE_API_TOKEN") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestProductionConfigWithoutPasswordHashIsInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.General.Environment = EnvironmentProduction
	cfg.Security.AdminPasswordHash = ""
	cfg.Security.DevAdminPassword = ""

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "REMOTE_TAPE_ADMIN_PASSWORD_HASH") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestPlaintextPasswordHashIsInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Security.AdminPasswordHash = "password"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "bcrypt") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDevOnlyCheapestDefaultInstanceSizeRequiresDevelopment(t *testing.T) {
	cfg := validConfig()
	cfg.Provisioning.DefaultInstanceSize = devOnlyCheapestInstanceSize
	if err := cfg.Validate(); err != nil {
		t.Fatalf("development Validate() error = %v", err)
	}

	cfg.General.Environment = EnvironmentProduction
	cfg.General.ControlPlaneURL = "https://control.example.com"
	cfg.General.SessionsBaseDomain = "sessions.example.com"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "REMOTE_TAPE_DEFAULT_INSTANCE_SIZE=s-1vcpu-512mb-10gb is allowed only when REMOTE_TAPE_ENV=development") {
		t.Fatalf("production Validate() error = %v", err)
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
			DefaultInstanceSize: "s-2vcpu-4gb",
			DefaultRegion:       "nyc3",
			ImageID:             "ubuntu-24-04-x64",
			DigitalOceanSSHKeys: []string{"12345"},
			ReconcileInterval:   5 * time.Second,
			HealthCheckTimeout:  time.Minute,
			FinalizationTimeout: 15 * time.Minute,
		},
		Security: SecuritySettings{
			AdminPasswordHash:          "$2a$10$Y9JdDULWeDYyi7vG2dSAf.KExPZ5RIZX.38y93Stah1DzqleV5E7.",
			DigitalOceanAPIToken:       "dop_v1_test",
			CloudflareAPIToken:         "cf_test",
			CookieAuthKey:              "0123456789abcdef0123456789abcdef",
			CookieEncryptionKey:        "0123456789abcdef0123456789abcdef",
			AdminCookieSessionDuration: 7 * 24 * time.Hour,
			LoginRateLimitMaxAttempts:  5,
			LoginRateLimitWindow:       15 * time.Minute,
		},
		Cleanup: CleanupSettings{
			OrphanedInstanceTTL: 2 * time.Hour,
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
}
