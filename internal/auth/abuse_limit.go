package auth

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type abuseEntry struct {
	failures    int
	lockedUntil time.Time
	lastSeenAt  time.Time
}

type authAbuseLimiter struct {
	mu            sync.Mutex
	maxFailures   int
	backoffBase   time.Duration
	backoffMax    time.Duration
	entries       map[string]abuseEntry
	cleanupEvery  time.Duration
	lastCleanupAt time.Time
}

func newAuthAbuseLimiter(maxFailures int, backoffBase, backoffMax time.Duration) *authAbuseLimiter {
	if maxFailures <= 0 || backoffBase <= 0 || backoffMax <= 0 {
		return nil
	}
	if backoffMax < backoffBase {
		backoffMax = backoffBase
	}

	cleanupEvery := backoffMax
	if cleanupEvery < 5*time.Minute {
		cleanupEvery = 5 * time.Minute
	}

	return &authAbuseLimiter{
		maxFailures:  maxFailures,
		backoffBase:  backoffBase,
		backoffMax:   backoffMax,
		entries:      make(map[string]abuseEntry),
		cleanupEvery: cleanupEvery,
	}
}

func (l *authAbuseLimiter) IsLocked(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanupLocked(now)

	entry, ok := l.entries[key]
	if !ok || entry.lockedUntil.IsZero() || !entry.lockedUntil.After(now) {
		return false, 0
	}

	retryAfter := entry.lockedUntil.Sub(now)
	if retryAfter < 0 {
		retryAfter = 0
	}
	return true, retryAfter
}

func (l *authAbuseLimiter) RecordFailure(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanupLocked(now)

	entry := l.entries[key]
	entry.failures++
	entry.lastSeenAt = now
	if entry.failures >= l.maxFailures {
		excess := entry.failures - l.maxFailures
		lockFor := l.backoffBase
		for i := 0; i < excess; i++ {
			if lockFor >= l.backoffMax {
				lockFor = l.backoffMax
				break
			}
			lockFor *= 2
			if lockFor > l.backoffMax {
				lockFor = l.backoffMax
				break
			}
		}
		entry.lockedUntil = now.Add(lockFor)
	}
	l.entries[key] = entry
}

func (l *authAbuseLimiter) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.entries, key)
}

func (l *authAbuseLimiter) cleanupLocked(now time.Time) {
	if !l.lastCleanupAt.IsZero() && now.Sub(l.lastCleanupAt) < l.cleanupEvery {
		return
	}

	for key, entry := range l.entries {
		if entry.lockedUntil.After(now) {
			continue
		}
		if now.Sub(entry.lastSeenAt) >= l.backoffMax {
			delete(l.entries, key)
		}
	}

	l.lastCleanupAt = now
}

func normalizeIdentifierForAbuse(identifier string, preserveCase bool) string {
	normalized := strings.TrimSpace(identifier)
	if preserveCase {
		return normalized
	}
	return strings.ToLower(normalized)
}

// ClientIP returns the client address for rate limiting. X-Forwarded-For is
// only honoured when trustProxy is set, otherwise any client could spoof it to
// dodge throttling. Only the right-most entry is used: it is the one the
// trusted proxy wrote, while entries to its left come from the client when the
// proxy appends to the header.
func ClientIP(r *http.Request, trustProxy bool) string {
	if r == nil {
		return "unknown"
	}
	if trustProxy {
		values := r.Header.Values("X-Forwarded-For")
		if len(values) > 0 {
			last := values[len(values)-1]
			if i := strings.LastIndex(last, ","); i >= 0 {
				last = last[i+1:]
			}
			if candidate := strings.TrimSpace(last); candidate != "" {
				return candidate
			}
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	if host == "" {
		return "unknown"
	}
	return host
}

func (s *Service) clientIP(r *http.Request) string {
	return ClientIP(r, s.trustProxyHeaders)
}

func abuseScopeKey(scope, ip, identifier string) string {
	return scope + "|" + ip + "|" + identifier
}

func (s *Service) throttleStatus(w http.ResponseWriter, retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	seconds := int(retryAfter.Seconds())
	if retryAfter%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
}

func (s *Service) checkAbuseLock(w http.ResponseWriter, scope, identifier string, preserveCase bool, r *http.Request) bool {
	if s.abuseLimiter == nil {
		return false
	}
	ip := s.clientIP(r)
	key := abuseScopeKey(scope, ip, normalizeIdentifierForAbuse(identifier, preserveCase))
	locked, retryAfter := s.abuseLimiter.IsLocked(key, time.Now())
	if !locked {
		return false
	}
	s.throttleStatus(w, retryAfter)
	return true
}

func (s *Service) markAbuseFailure(scope, identifier string, preserveCase bool, r *http.Request) {
	if s.abuseLimiter == nil {
		return
	}
	ip := s.clientIP(r)
	key := abuseScopeKey(scope, ip, normalizeIdentifierForAbuse(identifier, preserveCase))
	s.abuseLimiter.RecordFailure(key, time.Now())
}

func (s *Service) clearAbuseFailures(scope, identifier string, preserveCase bool, r *http.Request) {
	if s.abuseLimiter == nil {
		return
	}
	ip := s.clientIP(r)
	key := abuseScopeKey(scope, ip, normalizeIdentifierForAbuse(identifier, preserveCase))
	s.abuseLimiter.RecordSuccess(key)
}
