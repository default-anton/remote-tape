package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientIPUsesForwardedHeaderOnlyFromTrustedProxy(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.99, 203.0.113.10, 127.0.0.1")

	if got := ClientIP(request); got != "203.0.113.10" {
		t.Fatalf("ClientIP() = %q", got)
	}

	request.RemoteAddr = "198.51.100.20:1234"
	if got := ClientIP(request); got != "198.51.100.20" {
		t.Fatalf("ClientIP() from untrusted proxy = %q", got)
	}
}

func TestClientIPIgnoresForwardedHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("Forwarded", `for=198.51.100.99`)
	request.Header.Set("X-Forwarded-For", "203.0.113.10, 127.0.0.1")

	if got := ClientIP(request); got != "203.0.113.10" {
		t.Fatalf("ClientIP() = %q", got)
	}
}

func TestLoginRateLimiterCountsPendingAttempts(t *testing.T) {
	limiter := NewLoginRateLimiter(1, 15*time.Minute)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	if !limiter.BeginAttempt("203.0.113.10", now) {
		t.Fatal("first attempt was limited")
	}
	if limiter.BeginAttempt("203.0.113.10", now) {
		t.Fatal("second concurrent attempt was allowed")
	}
	limiter.Reset("203.0.113.10")
	if !limiter.BeginAttempt("203.0.113.10", now) {
		t.Fatal("attempt after reset was limited")
	}
}

func TestLoginRateLimiterCleansUpAndBoundsTrackedIPs(t *testing.T) {
	limiter := NewLoginRateLimiter(5, 15*time.Minute)
	limiter.maxIPs = 2
	limiter.cleanupPeriod = time.Minute
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	limiter.RecordFailure("203.0.113.1", now.Add(-20*time.Minute))
	limiter.RecordFailure("203.0.113.2", now)
	limiter.RecordFailure("203.0.113.3", now.Add(time.Minute))
	limiter.RecordFailure("203.0.113.4", now.Add(2*time.Minute))

	if len(limiter.buckets) > 2 {
		t.Fatalf("tracked IPs = %d", len(limiter.buckets))
	}
	if _, ok := limiter.buckets["203.0.113.1"]; ok {
		t.Fatal("expired bucket was retained")
	}
	if _, ok := limiter.buckets["203.0.113.2"]; ok {
		t.Fatal("oldest bucket was not evicted")
	}
	for i := 3; i <= 4; i++ {
		ip := fmt.Sprintf("203.0.113.%d", i)
		if _, ok := limiter.buckets[ip]; !ok {
			t.Fatalf("%s was not retained", ip)
		}
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
