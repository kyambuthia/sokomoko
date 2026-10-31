package auth

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxAuthAbuseEntries       = 50_000
	maxAbuseKeyComponentBytes = 256
)

type abuseEntry struct {
	failures    int
	lockedUntil time.Time
	lastSeenAt  time.Time
	lru         *list.Element
}

type authAbuseLimiter struct {
	mu             sync.Mutex
	maxFailures    int
	backoffBase    time.Duration
	backoffMax     time.Duration
	entries        map[string]abuseEntry
	lru            *list.List
	cleanupEvery   time.Duration
	lastCleanupAt  time.Time
	saturatedUntil time.Time
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
		lru:          list.New(),
		cleanupEvery: cleanupEvery,
	}
}

func (l *authAbuseLimiter) IsLocked(key string, now time.Time) (bool, time.Duration) {
	key = boundedAbuseKeyComponent(key)
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanupLocked(now)

	entry, ok := l.entries[key]
	if !ok {
		retryAfter, saturated := l.saturationRetryLocked(now)
		if saturated {
			return true, retryAfter
		}
		return false, 0
	}
	if entry.lockedUntil.IsZero() || !entry.lockedUntil.After(now) {
		return false, 0
	}

	l.touchLocked(key, entry)
	retryAfter := entry.lockedUntil.Sub(now)
	if retryAfter < 0 {
		retryAfter = 0
	}
	return true, retryAfter
}

func (l *authAbuseLimiter) RecordFailure(key string, now time.Time) {
	key = boundedAbuseKeyComponent(key)
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
	l.putLocked(key, entry, now)
}

func (l *authAbuseLimiter) RecordSuccess(key string) {
	key = boundedAbuseKeyComponent(key)
	l.mu.Lock()
	defer l.mu.Unlock()

	l.removeLocked(key)
}

func (l *authAbuseLimiter) putLocked(key string, entry abuseEntry, now time.Time) {
	if l.lru == nil {
		l.lru = list.New()
	}
	if existing, ok := l.entries[key]; ok {
		if existing.lru == nil {
			entry.lru = l.lru.PushFront(key)
		} else {
			entry.lru = existing.lru
			l.lru.MoveToFront(entry.lru)
		}
	} else {
		if len(l.entries) >= maxAuthAbuseEntries && !l.evictOldestUnlockedLocked(now) {
			l.saturationRetryLocked(now)
			return
		}
		entry.lru = l.lru.PushFront(key)
	}
	l.entries[key] = entry
}

func (l *authAbuseLimiter) touchLocked(key string, entry abuseEntry) {
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

func (l *authAbuseLimiter) removeLocked(key string) {
	entry, ok := l.entries[key]
	if !ok {
		return
	}
	if entry.lru != nil && l.lru != nil {
		l.lru.Remove(entry.lru)
	}
	delete(l.entries, key)
	l.saturatedUntil = time.Time{}
}

// evictOldestUnlockedLocked only reclaims entries whose lockout has expired.
func (l *authAbuseLimiter) evictOldestUnlockedLocked(now time.Time) bool {
	if l.saturatedUntil.After(now) {
		return false
	}
	l.saturatedUntil = time.Time{}
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
			if !entry.lockedUntil.After(now) {
				l.removeLocked(key)
				return true
			}
			element = previous
		}
	}

	for key, entry := range l.entries {
		if entry.lru == nil && !entry.lockedUntil.After(now) {
			l.removeLocked(key)
			return true
		}
	}
	return false
}

// saturationRetryLocked throttles unseen identifiers only while every slot
// holds an active lockout, keeping an attacker from flushing those penalties.
func (l *authAbuseLimiter) saturationRetryLocked(now time.Time) (time.Duration, bool) {
	if len(l.entries) < maxAuthAbuseEntries {
		return 0, false
	}
	if l.saturatedUntil.After(now) {
		return l.saturatedUntil.Sub(now), true
	}
	l.saturatedUntil = time.Time{}

	var earliestExpiry time.Time
	if l.lru != nil {
		for element := l.lru.Back(); element != nil; element = element.Prev() {
			key, ok := element.Value.(string)
			if !ok {
				continue
			}
			entry, exists := l.entries[key]
			if !exists {
				continue
			}
			if !entry.lockedUntil.After(now) {
				return 0, false
			}
			if earliestExpiry.IsZero() || entry.lockedUntil.Before(earliestExpiry) {
				earliestExpiry = entry.lockedUntil
			}
		}
	}

	for _, entry := range l.entries {
		if entry.lru != nil {
			continue
		}
		if !entry.lockedUntil.After(now) {
			return 0, false
		}
		if earliestExpiry.IsZero() || entry.lockedUntil.Before(earliestExpiry) {
			earliestExpiry = entry.lockedUntil
		}
	}

	if earliestExpiry.IsZero() {
		return 0, false
	}
	l.saturatedUntil = earliestExpiry
	return earliestExpiry.Sub(now), true
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
			l.removeLocked(key)
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
	return boundedAbuseKeyComponent(scope) + "|" + boundedAbuseKeyComponent(ip) + "|" + boundedAbuseKeyComponent(identifier)
}

func boundedAbuseKeyComponent(value string) string {
	if len(value) <= maxAbuseKeyComponentBytes {
		return value
	}
	digest := sha256.Sum256([]byte(value))
	return "\x00abuse-sha256:" + hex.EncodeToString(digest[:])
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
