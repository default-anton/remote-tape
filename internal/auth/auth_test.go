package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPUsesForwardedHeaderOnlyFromTrustedProxy(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.10, 127.0.0.1")

	if got := ClientIP(request); got != "203.0.113.10" {
		t.Fatalf("ClientIP() = %q", got)
	}

	request.RemoteAddr = "198.51.100.20:1234"
	if got := ClientIP(request); got != "198.51.100.20" {
		t.Fatalf("ClientIP() from untrusted proxy = %q", got)
	}
}

func TestNewManagerRejectsPlaintextPasswordHash(t *testing.T) {
	_, err := NewManager(Config{
		PasswordHash:        "password",
		CookieAuthKey:       []byte("0123456789abcdef0123456789abcdef"),
		CookieEncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		SessionDuration:     1,
	})
	if err == nil {
		t.Fatal("NewManager() error = nil")
	}
}
