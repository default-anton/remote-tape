package auth

import (
	"sync"
	"time"
)

const (
	defaultLoginLimiterMaxIPs        = 4096
	defaultLoginLimiterCleanupPeriod = time.Minute
)

type LoginRateLimiter struct {
	maxAttempts   int
	window        time.Duration
	maxIPs        int
	cleanupPeriod time.Duration
	lastCleanup   time.Time
	mu            sync.Mutex
	buckets       map[string]*loginBucket
}

type loginBucket struct {
	failures []time.Time
	pending  int
	lastSeen time.Time
}

func NewLoginRateLimiter(maxAttempts int, window time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{
		maxAttempts:   maxAttempts,
		window:        window,
		maxIPs:        defaultLoginLimiterMaxIPs,
		cleanupPeriod: defaultLoginLimiterCleanupPeriod,
		buckets:       map[string]*loginBucket{},
	}
}

func (l *LoginRateLimiter) BeginAttempt(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(now)
	bucket := l.bucketLocked(ip, now)
	l.pruneLocked(bucket, now)
	if len(bucket.failures)+bucket.pending >= l.maxAttempts {
		bucket.lastSeen = now
		return false
	}
	bucket.pending++
	bucket.lastSeen = now
	return true
}

func (l *LoginRateLimiter) RecordFailure(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(now)
	bucket := l.bucketLocked(ip, now)
	l.pruneLocked(bucket, now)
	if bucket.pending > 0 {
		bucket.pending--
	}
	bucket.failures = append(bucket.failures, now)
	bucket.lastSeen = now
}

func (l *LoginRateLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, ip)
}

func (l *LoginRateLimiter) Limited(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(now)
	bucket := l.buckets[ip]
	if bucket == nil {
		return false
	}
	l.pruneLocked(bucket, now)
	if len(bucket.failures) == 0 && bucket.pending == 0 {
		delete(l.buckets, ip)
		return false
	}
	return len(bucket.failures)+bucket.pending >= l.maxAttempts
}

func (l *LoginRateLimiter) bucketLocked(ip string, now time.Time) *loginBucket {
	if bucket := l.buckets[ip]; bucket != nil {
		return bucket
	}
	if len(l.buckets) >= l.maxIPs {
		l.evictOldestLocked()
	}
	bucket := &loginBucket{lastSeen: now}
	l.buckets[ip] = bucket
	return bucket
}

func (l *LoginRateLimiter) cleanupLocked(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < l.cleanupPeriod {
		return
	}
	for ip, bucket := range l.buckets {
		l.pruneLocked(bucket, now)
		if len(bucket.failures) == 0 && bucket.pending == 0 {
			delete(l.buckets, ip)
		}
	}
	l.lastCleanup = now
}

func (l *LoginRateLimiter) evictOldestLocked() {
	oldestIP := ""
	var oldest time.Time
	for ip, bucket := range l.buckets {
		if len(bucket.failures) == 0 && bucket.pending == 0 {
			delete(l.buckets, ip)
			return
		}
		if oldestIP == "" || bucket.lastSeen.Before(oldest) {
			oldestIP = ip
			oldest = bucket.lastSeen
		}
	}
	if oldestIP != "" {
		delete(l.buckets, oldestIP)
	}
}

func (l *LoginRateLimiter) pruneLocked(bucket *loginBucket, now time.Time) {
	cutoff := now.Add(-l.window)
	kept := bucket.failures[:0]
	for _, at := range bucket.failures {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	bucket.failures = kept
}
