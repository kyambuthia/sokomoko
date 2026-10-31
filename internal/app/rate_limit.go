package app

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxRateLimitEntries  = 50_000
	maxRateLimitKeyBytes = 256
)

type rateLimitEntry struct {
	windowStart time.Time
	count       int
	lru         *list.Element
}

type ipRateLimiter struct {
	mu              sync.Mutex
	window          time.Duration
	max             int
	entries         map[string]rateLimitEntry
	lru             *list.List
	lastCleanup     time.Time
	cleanupInterval time.Duration
}

func newIPRateLimiter(max int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		window:          window,
		max:             max,
		entries:         make(map[string]rateLimitEntry),
		lru:             list.New(),
		cleanupInterval: window,
	}
}

func (l *ipRateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanupLocked(now)

	key = boundedRateLimitKey(key)
	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.windowStart) >= l.window {
		if ok {
			l.removeLocked(key, entry)
		}
		l.putLocked(key, rateLimitEntry{windowStart: now, count: 1})
		return true, 0
	}
	l.touchLocked(key, entry)

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

func (l *ipRateLimiter) putLocked(key string, entry rateLimitEntry) {
	if l.lru == nil {
		l.lru = list.New()
	}
	if _, exists := l.entries[key]; !exists && len(l.entries) >= maxRateLimitEntries {
		l.evictOldestLocked()
	}
	entry.lru = l.lru.PushFront(key)
	l.entries[key] = entry
}

func (l *ipRateLimiter) touchLocked(key string, entry rateLimitEntry) {
	if l.lru == nil {
		l.lru = list.New()
	}
	if entry.lru == nil {
		entry.lru = l.lru.PushFront(key)
	} else {
		l.lru.MoveToFront(entry.lru)
	}
	l.entries[key] = entry
}

func (l *ipRateLimiter) removeLocked(key string, entry rateLimitEntry) {
	if entry.lru != nil && l.lru != nil {
		l.lru.Remove(entry.lru)
	}
	delete(l.entries, key)
}

func (l *ipRateLimiter) evictOldestLocked() {
	if l.lru != nil {
		if oldest := l.lru.Back(); oldest != nil {
			key := oldest.Value.(string)
			l.lru.Remove(oldest)
			delete(l.entries, key)
			return
		}
	}
	// Entries inserted directly by package tests or older callers may not yet
	// have an LRU node, so still enforce the hard map bound in that case.
	for key := range l.entries {
		delete(l.entries, key)
		return
	}
}

func (l *ipRateLimiter) cleanupLocked(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < l.cleanupInterval {
		return
	}
	for key, entry := range l.entries {
		if now.Sub(entry.windowStart) >= l.window {
			l.removeLocked(key, entry)
		}
	}
	l.lastCleanup = now
}

func boundedRateLimitKey(key string) string {
	if len(key) <= maxRateLimitKeyBytes {
		return key
	}
	digest := sha256.Sum256([]byte(key))
	return "\x00rate-sha256:" + hex.EncodeToString(digest[:])
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
