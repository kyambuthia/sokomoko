package auth

import (
	"sync"
	"time"
)

type abuseEntry struct {
	failures     int
	lockedUntil  time.Time
	lastSeenAt   time.Time
}

type authAbuseLimiter struct {
	mu             sync.Mutex
	maxFailures    int
	backoffBase    time.Duration
	backoffMax     time.Duration
	entries        map[string]abuseEntry
	cleanupEvery   time.Duration
	lastCleanupAt  time.Time
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
