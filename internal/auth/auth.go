package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"golang.org/x/crypto/bcrypt"
)

const (
	SubjectAdmin = "admin"
)

const (
	sessionCookieName = "remote_tape_session"
	csrfCookieName    = "remote_tape_csrf"
)

var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrInvalidSession  = errors.New("invalid session")
	ErrInvalidCSRF     = errors.New("invalid csrf token")
)

type Manager struct {
	passwordHash  []byte
	cookie        *securecookie.SecureCookie
	duration      time.Duration
	limiter       *LoginRateLimiter
	logger        *slog.Logger
	secureCookies bool
	now           func() time.Time
}

type Config struct {
	PasswordHash         string
	CookieAuthKey        []byte
	CookieEncryptionKey  []byte
	SessionDuration      time.Duration
	RateLimitMaxAttempts int
	RateLimitWindow      time.Duration
	SecureCookies        bool
	Logger               *slog.Logger
}

type Session struct {
	Subject string    `json:"sub"`
	Issued  time.Time `json:"iat"`
	Expires time.Time `json:"exp"`
}

func NewManager(cfg Config) (*Manager, error) {
	if strings.TrimSpace(cfg.PasswordHash) == "" {
		return nil, errors.New("password hash is required")
	}
	if len(cfg.CookieAuthKey) == 0 {
		return nil, errors.New("cookie auth key is required")
	}
	if len(cfg.CookieEncryptionKey) == 0 {
		return nil, errors.New("cookie encryption key is required")
	}
	if cfg.SessionDuration <= 0 {
		return nil, errors.New("session duration must be positive")
	}
	if cfg.RateLimitMaxAttempts <= 0 {
		cfg.RateLimitMaxAttempts = 5
	}
	if cfg.RateLimitWindow <= 0 {
		cfg.RateLimitWindow = 15 * time.Minute
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cookie := securecookie.New(cfg.CookieAuthKey, cfg.CookieEncryptionKey)
	cookie.SetSerializer(securecookie.JSONEncoder{})
	if _, err := cookie.Encode(sessionCookieName, map[string]string{"sub": SubjectAdmin}); err != nil {
		return nil, err
	}
	return &Manager{
		passwordHash:  []byte(strings.TrimSpace(cfg.PasswordHash)),
		cookie:        cookie,
		duration:      cfg.SessionDuration,
		limiter:       NewLoginRateLimiter(cfg.RateLimitMaxAttempts, cfg.RateLimitWindow),
		logger:        logger,
		secureCookies: cfg.SecureCookies,
		now:           func() time.Time { return time.Now().UTC() },
	}, nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (m *Manager) Login(w http.ResponseWriter, r *http.Request, password string) error {
	ip := ClientIP(r)
	if m.limiter.Limited(ip, m.now()) {
		m.logger.WarnContext(r.Context(), "login failed", "event", "auth.login_failed", "ip", ip, "reason", "rate_limited")
		return ErrInvalidPassword
	}
	if err := bcrypt.CompareHashAndPassword(m.passwordHash, []byte(password)); err != nil {
		m.limiter.RecordFailure(ip, m.now())
		m.logger.WarnContext(r.Context(), "login failed", "event", "auth.login_failed", "ip", ip, "reason", "invalid_password")
		return ErrInvalidPassword
	}
	m.limiter.Reset(ip)
	m.SetSessionCookie(w, SubjectAdmin)
	return nil
}

func (m *Manager) SetSessionCookie(w http.ResponseWriter, subject string) {
	now := m.now()
	value := map[string]string{
		"sub": subject,
		"iat": now.Format(time.RFC3339Nano),
		"exp": now.Add(m.duration).Format(time.RFC3339Nano),
	}
	encoded, err := m.cookie.Encode(sessionCookieName, value)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: encoded, Path: "/", HttpOnly: true, Secure: m.secureCookies, SameSite: http.SameSiteLaxMode, Expires: now.Add(m.duration)})
}

func (m *Manager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, Secure: m.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func (m *Manager) Session(r *http.Request) (Session, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return Session{}, ErrInvalidSession
	}
	value := map[string]string{}
	if err := m.cookie.Decode(sessionCookieName, cookie.Value, &value); err != nil {
		return Session{}, ErrInvalidSession
	}
	issued, err := time.Parse(time.RFC3339Nano, value["iat"])
	if err != nil {
		return Session{}, ErrInvalidSession
	}
	expires, err := time.Parse(time.RFC3339Nano, value["exp"])
	if err != nil {
		return Session{}, ErrInvalidSession
	}
	if value["sub"] != SubjectAdmin || !m.now().Before(expires) {
		return Session{}, ErrInvalidSession
	}
	return Session{Subject: value["sub"], Issued: issued, Expires: expires}, nil
}

func (m *Manager) CSRFToken(w http.ResponseWriter, r *http.Request) (string, error) {
	cookie, err := r.Cookie(csrfCookieName)
	if err == nil {
		value := map[string]string{}
		if err := m.cookie.Decode(csrfCookieName, cookie.Value, &value); err == nil && value["token"] != "" {
			return value["token"], nil
		}
	}
	return m.IssueCSRF(w)
}

func (m *Manager) IssueCSRF(w http.ResponseWriter) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	encoded, err := m.cookie.Encode(csrfCookieName, map[string]string{"token": token})
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: encoded, Path: "/", HttpOnly: true, Secure: m.secureCookies, SameSite: http.SameSiteLaxMode})
	return token, nil
}

func (m *Manager) CheckCSRF(r *http.Request) error {
	header := r.Header.Get("X-CSRF-Token")
	if header == "" {
		header = r.FormValue("csrf_token")
	}
	if header == "" {
		return ErrInvalidCSRF
	}
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return ErrInvalidCSRF
	}
	value := map[string]string{}
	if err := m.cookie.Decode(csrfCookieName, cookie.Value, &value); err != nil {
		return ErrInvalidCSRF
	}
	if subtle.ConstantTimeCompare([]byte(header), []byte(value["token"])) != 1 {
		return ErrInvalidCSRF
	}
	return nil
}

func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func randomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
