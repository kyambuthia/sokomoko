package app

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateLimitEntry struct {
	windowStart time.Time
	count       int
}

type ipRateLimiter struct {
	mu              sync.Mutex
	window          time.Duration
	max             int
	entries         map[string]rateLimitEntry
	lastCleanup     time.Time
	cleanupInterval time.Duration
}

func newIPRateLimiter(max int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		window:          window,
		max:             max,
		entries:         make(map[string]rateLimitEntry),
		cleanupInterval: window,
	}
}

func (l *ipRateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanupLocked(now)

	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.windowStart) >= l.window {
		l.entries[key] = rateLimitEntry{windowStart: now, count: 1}
		return true, 0
	}

	if entry.count >= l.max {
		retryAfter := l.window - now.Sub(entry.windowStart)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}

	entry.count++
	l.entries[key] = entry
	return true, 0
}

func (l *ipRateLimiter) cleanupLocked(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < l.cleanupInterval {
		return
	}
	for key, entry := range l.entries {
		if now.Sub(entry.windowStart) >= l.window {
			delete(l.entries, key)
		}
	}
	l.lastCleanup = now
}

// RateLimitByIP limits requests per remote address. When methods are given,
// only requests with those methods are counted and limited.
func RateLimitByIP(max int, window time.Duration, methods ...string) Middleware {
	return RateLimitByKey(max, window, func(r *http.Request) string {
		return clientIPFromRemoteAddr(r.RemoteAddr)
	}, methods...)
}

// RateLimitByKey limits requests per key, for example a proxy-aware client IP.
func RateLimitByKey(max int, window time.Duration, keyFunc func(*http.Request) string, methods ...string) Middleware {
	if max <= 0 || window <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	limited := map[string]bool{}
	for _, m := range methods {
		limited[strings.ToUpper(strings.TrimSpace(m))] = true
	}

	limiter := newIPRateLimiter(max, window)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(limited) > 0 && !limited[r.Method] {
				next.ServeHTTP(w, r)
				return
			}
			allowed, retryAfter := limiter.allow(keyFunc(r), time.Now())
			if !allowed {
				retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
				if retryAfterSeconds < 1 {
					retryAfterSeconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIPFromRemoteAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	if host == "" {
		return "unknown"
	}
	return host
}
