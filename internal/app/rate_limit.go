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
	mu      sync.Mutex
	window  time.Duration
	max     int
	entries map[string]rateLimitEntry
}

func newIPRateLimiter(max int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		window:  window,
		max:     max,
		entries: make(map[string]rateLimitEntry),
	}
}

func (l *ipRateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.windowStart) >= l.window {
		l.entries[key] = rateLimitEntry{windowStart: now, count: 1}
		l.cleanupLocked(now)
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
	if len(l.entries) < 4096 {
		return
	}
	for key, entry := range l.entries {
		if now.Sub(entry.windowStart) >= l.window {
			delete(l.entries, key)
		}
	}
}

func RateLimitByIP(max int, window time.Duration, methods ...string) Middleware {
	if max <= 0 || window <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	methodSet := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		clean := strings.ToUpper(strings.TrimSpace(method))
		if clean != "" {
			methodSet[clean] = struct{}{}
		}
	}

	limiter := newIPRateLimiter(max, window)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(methodSet) > 0 {
				if _, ok := methodSet[r.Method]; !ok {
					next.ServeHTTP(w, r)
					return
				}
			}

			ip := clientIPFromRemoteAddr(r.RemoteAddr)
			allowed, retryAfter := limiter.allow(ip, time.Now())
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
