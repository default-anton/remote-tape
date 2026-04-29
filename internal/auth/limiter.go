package auth

import (
	"sync"
	"time"
)

type LoginRateLimiter struct {
	maxAttempts int
	window      time.Duration
	mu          sync.Mutex
	attempts    map[string][]time.Time
}

func NewLoginRateLimiter(maxAttempts int, window time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{maxAttempts: maxAttempts, window: window, attempts: map[string][]time.Time{}}
}

func (l *LoginRateLimiter) Limited(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := l.pruneLocked(ip, now)
	return len(entries) >= l.maxAttempts
}

func (l *LoginRateLimiter) RecordFailure(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := l.pruneLocked(ip, now)
	l.attempts[ip] = append(entries, now)
}

func (l *LoginRateLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

func (l *LoginRateLimiter) pruneLocked(ip string, now time.Time) []time.Time {
	cutoff := now.Add(-l.window)
	entries := l.attempts[ip]
	kept := entries[:0]
	for _, at := range entries {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(l.attempts, ip)
		return nil
	}
	l.attempts[ip] = kept
	return kept
}
