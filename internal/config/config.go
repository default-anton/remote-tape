package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvironmentDevelopment = "development"
	EnvironmentProduction  = "production"
)

type Config struct {
	General      GeneralSettings
	Provisioning ProvisioningSettings
	Security     SecuritySettings
	Cleanup      CleanupSettings
}

type GeneralSettings struct {
	Environment        string
	HTTPAddr           string
	DatabasePath       string
	ControlPlaneURL    string
	SessionsBaseDomain string
	LogLevel           string
}

type ProvisioningSettings struct {
	DefaultDropletSize  string
	DefaultRegion       string
	ImageID             string
	HealthCheckTimeout  time.Duration
	FinalizationTimeout time.Duration
}

type SecuritySettings struct {
	AdminCookieSessionDuration time.Duration
	LoginRateLimitMaxAttempts  int
	LoginRateLimitWindow       time.Duration
	DigitalOceanAPIToken       SensitiveString
	CloudflareAPIToken         SensitiveString
}

type CleanupSettings struct {
	OrphanedDropletTTL  time.Duration
	CompletedSessionTTL time.Duration
	FailedSessionTTL    time.Duration
	LogsRetention       time.Duration
}

type SensitiveString string

func (s SensitiveString) Set() bool {
	return strings.TrimSpace(string(s)) != ""
}

func (s SensitiveString) Redacted() string {
	if !s.Set() {
		return ""
	}
	return "[set]"
}

func Load() (Config, error) {
	cfg := Config{
		General: GeneralSettings{
			Environment:        getEnv("REMOTE_TAPE_ENV", EnvironmentDevelopment),
			HTTPAddr:           getEnv("REMOTE_TAPE_HTTP_ADDR", "127.0.0.1:8080"),
			DatabasePath:       getEnv("REMOTE_TAPE_DATABASE_PATH", "./data/control-plane.db"),
			ControlPlaneURL:    getEnv("REMOTE_TAPE_CONTROL_PLANE_URL", "http://127.0.0.1:8080"),
			SessionsBaseDomain: getEnv("REMOTE_TAPE_SESSIONS_BASE_DOMAIN", "sessions.localhost"),
			LogLevel:           getEnv("REMOTE_TAPE_LOG_LEVEL", "info"),
		},
		Provisioning: ProvisioningSettings{
			DefaultDropletSize: getEnv("REMOTE_TAPE_DEFAULT_DROPLET_SIZE", "s-2vcpu-2gb"),
			DefaultRegion:      getEnv("REMOTE_TAPE_DEFAULT_REGION", "nyc3"),
			ImageID:            getEnv("REMOTE_TAPE_IMAGE_ID", "ubuntu-24-04-x64"),
		},
		Security: SecuritySettings{
			DigitalOceanAPIToken: SensitiveString(os.Getenv("REMOTE_TAPE_DIGITALOCEAN_API_TOKEN")),
			CloudflareAPIToken:   SensitiveString(os.Getenv("REMOTE_TAPE_CLOUDFLARE_API_TOKEN")),
		},
		Cleanup: CleanupSettings{},
	}

	var err error
	cfg.Provisioning.HealthCheckTimeout, err = getDuration("REMOTE_TAPE_HEALTH_CHECK_TIMEOUT", "60s")
	if err != nil {
		return Config{}, err
	}
	cfg.Provisioning.FinalizationTimeout, err = getDuration("REMOTE_TAPE_FINALIZATION_TIMEOUT", "15m")
	if err != nil {
		return Config{}, err
	}
	cfg.Security.AdminCookieSessionDuration, err = getDuration("REMOTE_TAPE_ADMIN_COOKIE_SESSION_DURATION", "7d")
	if err != nil {
		return Config{}, err
	}
	cfg.Security.LoginRateLimitWindow, err = getDuration("REMOTE_TAPE_LOGIN_RATE_LIMIT_WINDOW", "15m")
	if err != nil {
		return Config{}, err
	}
	cfg.Cleanup.OrphanedDropletTTL, err = getDuration("REMOTE_TAPE_ORPHANED_DROPLET_TTL", "2h")
	if err != nil {
		return Config{}, err
	}
	cfg.Cleanup.CompletedSessionTTL, err = getDuration("REMOTE_TAPE_COMPLETED_SESSION_TTL", "7d")
	if err != nil {
		return Config{}, err
	}
	cfg.Cleanup.FailedSessionTTL, err = getDuration("REMOTE_TAPE_FAILED_SESSION_TTL", "1d")
	if err != nil {
		return Config{}, err
	}
	cfg.Cleanup.LogsRetention, err = getDuration("REMOTE_TAPE_LOGS_RETENTION", "14d")
	if err != nil {
		return Config{}, err
	}
	cfg.Security.LoginRateLimitMaxAttempts, err = getPositiveInt("REMOTE_TAPE_LOGIN_RATE_LIMIT_MAX_ATTEMPTS", 5)
	if err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var errs []error

	if c.General.Environment != EnvironmentDevelopment && c.General.Environment != EnvironmentProduction {
		errs = append(errs, fmt.Errorf("REMOTE_TAPE_ENV must be %q or %q", EnvironmentDevelopment, EnvironmentProduction))
	}
	if strings.TrimSpace(c.General.HTTPAddr) == "" {
		errs = append(errs, errors.New("REMOTE_TAPE_HTTP_ADDR is required"))
	} else if host, port, err := net.SplitHostPort(c.General.HTTPAddr); err != nil || port == "" || strings.Contains(host, "/") {
		errs = append(errs, fmt.Errorf("REMOTE_TAPE_HTTP_ADDR must be host:port: %q", c.General.HTTPAddr))
	}
	if strings.TrimSpace(c.General.DatabasePath) == "" {
		errs = append(errs, errors.New("REMOTE_TAPE_DATABASE_PATH is required"))
	}
	if err := validateAbsoluteURL("REMOTE_TAPE_CONTROL_PLANE_URL", c.General.ControlPlaneURL); err != nil {
		errs = append(errs, err)
	}
	if err := validateDomain("REMOTE_TAPE_SESSIONS_BASE_DOMAIN", c.General.SessionsBaseDomain); err != nil {
		errs = append(errs, err)
	}
	if c.General.LogLevel != "debug" && c.General.LogLevel != "info" && c.General.LogLevel != "warn" && c.General.LogLevel != "error" {
		errs = append(errs, errors.New("REMOTE_TAPE_LOG_LEVEL must be debug, info, warn, or error"))
	}

	if strings.TrimSpace(c.Provisioning.DefaultDropletSize) == "" {
		errs = append(errs, errors.New("REMOTE_TAPE_DEFAULT_DROPLET_SIZE is required"))
	}
	if strings.TrimSpace(c.Provisioning.DefaultRegion) == "" {
		errs = append(errs, errors.New("REMOTE_TAPE_DEFAULT_REGION is required"))
	}
	if strings.TrimSpace(c.Provisioning.ImageID) == "" {
		errs = append(errs, errors.New("REMOTE_TAPE_IMAGE_ID is required"))
	}
	if c.Provisioning.HealthCheckTimeout <= 0 {
		errs = append(errs, errors.New("REMOTE_TAPE_HEALTH_CHECK_TIMEOUT must be positive"))
	}
	if c.Provisioning.FinalizationTimeout <= 0 {
		errs = append(errs, errors.New("REMOTE_TAPE_FINALIZATION_TIMEOUT must be positive"))
	}
	if c.Security.AdminCookieSessionDuration <= 0 {
		errs = append(errs, errors.New("REMOTE_TAPE_ADMIN_COOKIE_SESSION_DURATION must be positive"))
	}
	if c.Security.LoginRateLimitMaxAttempts <= 0 {
		errs = append(errs, errors.New("REMOTE_TAPE_LOGIN_RATE_LIMIT_MAX_ATTEMPTS must be positive"))
	}
	if c.Security.LoginRateLimitWindow <= 0 {
		errs = append(errs, errors.New("REMOTE_TAPE_LOGIN_RATE_LIMIT_WINDOW must be positive"))
	}
	if c.Cleanup.OrphanedDropletTTL <= 0 {
		errs = append(errs, errors.New("REMOTE_TAPE_ORPHANED_DROPLET_TTL must be positive"))
	}
	if c.Cleanup.CompletedSessionTTL <= 0 {
		errs = append(errs, errors.New("REMOTE_TAPE_COMPLETED_SESSION_TTL must be positive"))
	}
	if c.Cleanup.FailedSessionTTL <= 0 {
		errs = append(errs, errors.New("REMOTE_TAPE_FAILED_SESSION_TTL must be positive"))
	}
	if c.Cleanup.LogsRetention <= 0 {
		errs = append(errs, errors.New("REMOTE_TAPE_LOGS_RETENTION must be positive"))
	}

	if c.General.Environment == EnvironmentProduction {
		if strings.HasPrefix(c.General.ControlPlaneURL, "http://") {
			errs = append(errs, errors.New("REMOTE_TAPE_CONTROL_PLANE_URL must use https in production"))
		}
		if c.General.SessionsBaseDomain == "sessions.localhost" {
			errs = append(errs, errors.New("REMOTE_TAPE_SESSIONS_BASE_DOMAIN must be configured in production"))
		}
	}

	return errors.Join(errs...)
}

func (c Config) LogAttrs() []any {
	return []any{
		"environment", c.General.Environment,
		"http_addr", c.General.HTTPAddr,
		"database_path", c.General.DatabasePath,
		"control_plane_url", c.General.ControlPlaneURL,
		"sessions_base_domain", c.General.SessionsBaseDomain,
		"default_droplet_size", c.Provisioning.DefaultDropletSize,
		"default_region", c.Provisioning.DefaultRegion,
		"image_id", c.Provisioning.ImageID,
		"health_check_timeout", c.Provisioning.HealthCheckTimeout.String(),
		"finalization_timeout", c.Provisioning.FinalizationTimeout.String(),
		"admin_cookie_session_duration", c.Security.AdminCookieSessionDuration.String(),
		"login_rate_limit_max_attempts", c.Security.LoginRateLimitMaxAttempts,
		"login_rate_limit_window", c.Security.LoginRateLimitWindow.String(),
		"digitalocean_api_token", c.Security.DigitalOceanAPIToken.Redacted(),
		"cloudflare_api_token", c.Security.CloudflareAPIToken.Redacted(),
		"orphaned_droplet_ttl", c.Cleanup.OrphanedDropletTTL.String(),
		"completed_session_ttl", c.Cleanup.CompletedSessionTTL.String(),
		"failed_session_ttl", c.Cleanup.FailedSessionTTL.String(),
		"logs_retention", c.Cleanup.LogsRetention.String(),
	}
}

func getEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func getPositiveInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return n, nil
}

func getDuration(name, fallback string) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		value = fallback
	}
	duration, err := parseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}

func parseDuration(value string) (time.Duration, error) {
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(value)
}

func validateAbsoluteURL(name, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute URL", name)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", name)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must not include query or fragment", name)
	}
	return nil
}

func validateDomain(name, domain string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.Contains(domain, "://") || strings.ContainsAny(domain, "/?#") {
		return fmt.Errorf("%s must be a domain name, not a URL", name)
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return fmt.Errorf("%s must not start or end with a dot", name)
	}
	return nil
}
