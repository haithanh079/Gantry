package middleware

import (
	"net/http"
	"sync"
	"time"
)

// tokenBucket implements a simple token-bucket rate limiter for one client.
type tokenBucket struct {
	tokens   float64
	capacity float64
	rate     float64 // tokens per second
	lastSeen time.Time
}

func (b *tokenBucket) allow(now time.Time) bool {
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens = min(b.capacity, b.tokens+elapsed*b.rate)
	b.lastSeen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// rateLimiter holds per-IP buckets and handles periodic cleanup.
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	capacity float64
	rate     float64
}

func newRateLimiter(rps, burst float64) *rateLimiter {
	rl := &rateLimiter{
		buckets:  make(map[string]*tokenBucket),
		capacity: burst,
		rate:     rps,
	}
	go rl.cleanup()
	return rl
}

func (rl *rateLimiter) allow(ip string) bool {
	now := time.Now()
	rl.mu.Lock()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &tokenBucket{tokens: rl.capacity, capacity: rl.capacity, rate: rl.rate, lastSeen: now}
		rl.buckets[ip] = b
	}
	ok = b.allow(now)
	rl.mu.Unlock()
	return ok
}

// cleanup removes stale buckets every minute to prevent unbounded growth.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-5 * time.Minute)
		rl.mu.Lock()
		for ip, b := range rl.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// defaultLimiter allows 60 requests/second per IP with a burst of 120.
var defaultLimiter = newRateLimiter(60, 120)

// loginLimiter allows ~10 login attempts per minute per IP with a burst of 5.
var loginLimiter = newRateLimiter(10.0/60.0, 5)

// RateLimit returns middleware that enforces per-IP rate limiting.
// Requests that exceed the limit receive HTTP 429 Too Many Requests.
func clientIP(r *http.Request) string {
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}

// RateLimit returns middleware that enforces per-IP rate limiting.
// Requests that exceed the limit receive HTTP 429 Too Many Requests.
func RateLimit(next http.Handler) http.Handler {
	return rateLimitWith(defaultLimiter, next)
}

// LoginRateLimit applies a stricter per-IP limit for credential login attempts.
func LoginRateLimit(next http.Handler) http.Handler {
	return rateLimitWith(loginLimiter, next)
}

func rateLimitWith(rl *rateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
