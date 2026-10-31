package app

import (
	"container/list"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kyambuthia/sokomoko/internal/netutil"
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
	saturatedUntil  time.Time
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
		if l.makeRoomLocked(now) {
			return true, 0
		}
		l.putLocked(key, rateLimitEntry{windowStart: now, count: 1})
		return true, 0
	}
	entry = l.touchLocked(key, entry)

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
	entry.lru = l.lru.PushFront(key)
	l.entries[key] = entry
}

func (l *ipRateLimiter) touchLocked(key string, entry rateLimitEntry) rateLimitEntry {
	if l.lru == nil {
		l.lru = list.New()
	}
	if entry.lru == nil {
		entry.lru = l.lru.PushFront(key)
	} else {
		l.lru.MoveToFront(entry.lru)
	}
	l.entries[key] = entry
	return entry
}

func (l *ipRateLimiter) removeLocked(key string, entry rateLimitEntry) {
	if entry.lru != nil && l.lru != nil {
		l.lru.Remove(entry.lru)
	}
	delete(l.entries, key)
	l.saturatedUntil = time.Time{}
}

// makeRoomLocked preserves active rate limits. When every slot is penalized,
// new keys are left untracked until a window expires instead of evicting a client.
func (l *ipRateLimiter) makeRoomLocked(now time.Time) bool {
	if len(l.entries) < maxRateLimitEntries {
		return false
	}
	if l.saturatedUntil.After(now) {
		return true
	}
	l.saturatedUntil = time.Time{}

	var earliestExpiry time.Time
	if l.lru != nil {
		for element := l.lru.Back(); element != nil; {
			previous := element.Prev()
			key, ok := element.Value.(string)
			if !ok {
				l.lru.Remove(element)
				element = previous
				continue
			}
			entry, exists := l.entries[key]
			if !exists {
				l.lru.Remove(element)
				element = previous
				continue
			}
			expiry := entry.windowStart.Add(l.window)
			if !expiry.After(now) || entry.count < l.max {
				l.removeLocked(key, entry)
				return false
			}
			if earliestExpiry.IsZero() || expiry.Before(earliestExpiry) {
				earliestExpiry = expiry
			}
			element = previous
		}
	}

	// Map entries without list nodes can occur in package-level fixtures.
	for key, entry := range l.entries {
		if entry.lru != nil {
			continue
		}
		expiry := entry.windowStart.Add(l.window)
		if !expiry.After(now) || entry.count < l.max {
			l.removeLocked(key, entry)
			return false
		}
		if earliestExpiry.IsZero() || expiry.Before(earliestExpiry) {
			earliestExpiry = expiry
		}
	}

	if earliestExpiry.IsZero() {
		earliestExpiry = now.Add(l.window)
	}
	l.saturatedUntil = earliestExpiry
	return true
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
			allowed, retryAfter := limiter.allow(netutil.NormalizeRateLimitKey(keyFunc(r)), time.Now())
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

// SharedRequestRateLimitStore persists a privacy-preserving request key across
// application replicas. Implementations must make each consume atomic.
type SharedRequestRateLimitStore interface {
	EnforceRequestRateLimit(context.Context, []byte, int, time.Duration) (bool, time.Duration, error)
}

// SharedRateLimitByKey enforces a POST limit across replicas. Database errors
// fall back to the bounded process-local limiter so a transient database issue
// does not block every form submission.
func SharedRateLimitByKey(store SharedRequestRateLimitStore, secret []byte, max int, window time.Duration, keyFunc func(*http.Request) string, methods ...string) Middleware {
	if max <= 0 || window <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	if store == nil || len(secret) == 0 {
		return RateLimitByKey(max, window, keyFunc, methods...)
	}

	limited := map[string]bool{}
	for _, method := range methods {
		limited[strings.ToUpper(strings.TrimSpace(method))] = true
	}
	fallback := newIPRateLimiter(max, window)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(limited) > 0 && !limited[r.Method] {
				next.ServeHTTP(w, r)
				return
			}

			key := netutil.NormalizeRateLimitKey(keyFunc(r))
			ctx, cancel := context.WithTimeout(r.Context(), 150*time.Millisecond)
			allowed, retryAfter, err := store.EnforceRequestRateLimit(ctx, requestRateLimitKey(secret, key), max, window)
			cancel()
			if err != nil {
				slog.WarnContext(r.Context(), "shared request rate limit unavailable; using process-local fallback", "error", err)
				allowed, retryAfter = fallback.allow(key, time.Now())
			}
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

func requestRateLimitKey(secret []byte, raw string) []byte {
	key := netutil.NormalizeRateLimitKey(raw)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("sokomoko:post-rate-limit:v1\x00"))
	_, _ = mac.Write([]byte(key))
	return mac.Sum(nil)
}
