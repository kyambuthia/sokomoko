package auth

import (
	"container/list"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxAuthAbuseEntries       = 50_000
	maxAbuseKeyComponentBytes = 256
	sharedAuthAbuseTimeout    = 150 * time.Millisecond
)

// SharedAuthAbuseStore persists keyed authentication lockouts across replicas.
type SharedAuthAbuseStore interface {
	CheckAuthAbuseLock(context.Context, []byte) (bool, time.Duration, int64, error)
	RecordAuthAbuseFailure(context.Context, []byte, int64, int, time.Duration, time.Duration) error
	ClearAuthAbuseFailures(context.Context, []byte, time.Duration) error
}

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
			if lockFor > l.backoffMax/2 {
				lockFor = l.backoffMax
				break
			}
			lockFor *= 2
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

func (l *authAbuseLimiter) observeLock(key string, retryAfter time.Duration, now time.Time) {
	if retryAfter <= 0 {
		return
	}
	key = boundedAbuseKeyComponent(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(now)

	entry := l.entries[key]
	if entry.failures < l.maxFailures {
		entry.failures = l.maxFailures
	}
	lockedUntil := now.Add(retryAfter)
	if entry.lockedUntil.After(lockedUntil) {
		lockedUntil = entry.lockedUntil
	}
	entry.lockedUntil = lockedUntil
	entry.lastSeenAt = now
	l.putLocked(key, entry, now)
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

// evictOldestUnlockedLocked protects active lockouts and leaves new keys
// untracked when every slot is locked, avoiding a global authentication outage.
func (l *authAbuseLimiter) evictOldestUnlockedLocked(now time.Time) bool {
	if l.saturatedUntil.After(now) {
		return false
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
			if !entry.lockedUntil.After(now) {
				l.removeLocked(key)
				return true
			}
			if earliestExpiry.IsZero() || entry.lockedUntil.Before(earliestExpiry) {
				earliestExpiry = entry.lockedUntil
			}
			element = previous
		}
	}

	for key, entry := range l.entries {
		if entry.lru != nil {
			continue
		}
		if !entry.lockedUntil.After(now) {
			l.removeLocked(key)
			return true
		}
		if earliestExpiry.IsZero() || entry.lockedUntil.Before(earliestExpiry) {
			earliestExpiry = entry.lockedUntil
		}
	}
	if !earliestExpiry.IsZero() {
		l.saturatedUntil = earliestExpiry
	}
	return false
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
	if address, err := netip.ParseAddr(strings.TrimSpace(ip)); err == nil {
		ip = address.Unmap().String()
	}
	var key strings.Builder
	for _, component := range []string{scope, ip, identifier} {
		component = boundedAbuseKeyComponent(component)
		key.WriteString(strconv.Itoa(len(component)))
		key.WriteByte(':')
		key.WriteString(component)
	}
	return key.String()
}

func authAbuseKeyHash(secret []byte, key string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("sokomoko:auth-abuse:v1\x00"))
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(key)))
	_, _ = mac.Write(length[:])
	_, _ = mac.Write([]byte(key))
	return mac.Sum(nil)
}

func requestContext(r *http.Request) context.Context {
	if r == nil {
		return context.Background()
	}
	return r.Context()
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

func (s *Service) checkAbuseLock(w http.ResponseWriter, scope, identifier string, preserveCase bool, r *http.Request) (bool, int64) {
	if s.abuseLimiter == nil {
		return false, -1
	}
	ip := s.clientIP(r)
	key := abuseScopeKey(scope, ip, normalizeIdentifierForAbuse(identifier, preserveCase))
	now := time.Now()
	var locked bool
	var retryAfter time.Duration
	if s.sharedAbuseStore == nil {
		locked, retryAfter = s.abuseLimiter.IsLocked(key, now)
	} else {
		ctx, cancel := context.WithTimeout(requestContext(r), sharedAuthAbuseTimeout)
		locked, retryAfter, generation, err := s.sharedAbuseStore.CheckAuthAbuseLock(ctx, authAbuseKeyHash(s.abuseKey, key))
		cancel()
		if err != nil {
			slog.WarnContext(requestContext(r), "shared authentication abuse check failed; using process-local fallback", "error", err)
			locked, retryAfter = s.abuseLimiter.IsLocked(key, now)
			generation = -1
		} else {
			// Keep this instance's bounded fallback warm while shared storage works.
			s.abuseLimiter.IsLocked(key, now)
			if locked {
				s.abuseLimiter.observeLock(key, retryAfter, now)
			}
		}
		if locked {
			s.throttleStatus(w, retryAfter)
			return true, generation
		}
		return false, generation
	}
	if locked {
		s.throttleStatus(w, retryAfter)
	}
	return locked, -1
}

// checkAbuseLockByIP applies an endpoint limit shared across all identifiers
// from one client. It complements identifier-specific limits where aliases or
// attacker-chosen values would otherwise create fresh keys.
func (s *Service) checkAbuseLockByIP(w http.ResponseWriter, scope string, r *http.Request) (bool, int64) {
	return s.checkAbuseLock(w, scope+":ip", "", true, r)
}

// recordAbuseAttemptByIP records a failed request against the client-wide key.
func (s *Service) recordAbuseAttemptByIP(scope string, r *http.Request, generation int64) {
	s.recordAbuseAttempt(scope+":ip", "", true, r, generation)
}

func (s *Service) markAbuseFailure(scope, identifier string, preserveCase bool, r *http.Request, generation int64) {
	s.recordAbuseAttempt(scope, identifier, preserveCase, r, generation)
}

func (s *Service) recordAbuseAttempt(scope, identifier string, preserveCase bool, r *http.Request, generation int64) {
	if s.abuseLimiter == nil {
		return
	}
	ip := s.clientIP(r)
	key := abuseScopeKey(scope, ip, normalizeIdentifierForAbuse(identifier, preserveCase))
	now := time.Now()
	s.abuseLimiter.RecordFailure(key, now)
	if s.sharedAbuseStore == nil {
		return
	}
	if generation < 0 {
		// Without a shared generation, a delayed failure could undo a successful login reset.
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), sharedAuthAbuseTimeout)
	defer cancel()
	if err := s.sharedAbuseStore.RecordAuthAbuseFailure(ctx, authAbuseKeyHash(s.abuseKey, key), generation, s.abuseLimiter.maxFailures, s.abuseLimiter.backoffBase, s.abuseLimiter.backoffMax); err != nil {
		slog.WarnContext(requestContext(r), "shared authentication abuse update failed; retaining process-local fallback", "error", err)
	}
}

func (s *Service) clearAbuseFailures(scope, identifier string, preserveCase bool, r *http.Request) {
	if s.abuseLimiter == nil {
		return
	}
	ip := s.clientIP(r)
	key := abuseScopeKey(scope, ip, normalizeIdentifierForAbuse(identifier, preserveCase))
	s.abuseLimiter.RecordSuccess(key)
	if s.sharedAbuseStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), sharedAuthAbuseTimeout)
	defer cancel()
	if err := s.sharedAbuseStore.ClearAuthAbuseFailures(ctx, authAbuseKeyHash(s.abuseKey, key), s.abuseLimiter.backoffMax); err != nil {
		slog.WarnContext(requestContext(r), "shared authentication abuse clear failed; retaining database state", "error", err)
	}
}
