package db

import (
	"crypto/sha256"
	"testing"
	"time"
)

func TestEnforceRequestRateLimit_PostgreSQLWindowAndCleanup(t *testing.T) {
	s := newTestStore(t)
	sum := sha256.Sum256([]byte("post:198.51.100.42:/checkout"))
	key := sum[:]

	for i := range 3 {
		allowed, _, err := s.EnforceRequestRateLimit(t.Context(), key, 3, time.Minute)
		chatMust(t, err)
		if !allowed {
			t.Fatalf("request %d allowed=false; want true", i+1)
		}
	}

	allowed, retryAfter, err := s.EnforceRequestRateLimit(t.Context(), key, 3, time.Minute)
	chatMust(t, err)
	if allowed || retryAfter <= 0 {
		t.Fatalf("4th request allowed=%v retryAfter=%v; want false, >0", allowed, retryAfter)
	}

	chatMust(t, s.CleanupRequestRateLimits(t.Context()))
}

func TestAuthAbuseRateLimit_LockGenerationAndClear(t *testing.T) {
	s := newTestStore(t)
	sum := sha256.Sum256([]byte("login:user:alice@example.com"))
	key := sum[:]

	locked, _, gen, err := s.CheckAuthAbuseLock(t.Context(), key)
	chatMust(t, err)
	if locked || gen != 0 {
		t.Fatalf("initial lock=(%v,%d); want false, 0", locked, gen)
	}

	// First failure below maxFailures (2) does not lock.
	chatMust(t, s.RecordAuthAbuseFailure(t.Context(), key, gen, 2, 10*time.Second, time.Minute))
	locked, _, gen, err = s.CheckAuthAbuseLock(t.Context(), key)
	chatMust(t, err)
	if locked {
		t.Fatal("locked after 1 failure; want unlocked until maxFailures=2")
	}

	// Stale generation is ignored; current generation locks.
	chatMust(t, s.RecordAuthAbuseFailure(t.Context(), key, gen+5, 2, 10*time.Second, time.Minute))
	locked, _, gen, err = s.CheckAuthAbuseLock(t.Context(), key)
	chatMust(t, err)
	if locked {
		t.Fatal("stale generation should not advance failure count")
	}

	chatMust(t, s.RecordAuthAbuseFailure(t.Context(), key, gen, 2, 10*time.Second, time.Minute))
	locked, retryAfter, _, err := s.CheckAuthAbuseLock(t.Context(), key)
	chatMust(t, err)
	if !locked || retryAfter <= 0 {
		t.Fatalf("after maxFailures locked=%v retryAfter=%v; want true, >0", locked, retryAfter)
	}

	// Clearing resets lock state; cleanup runs cleanly.
	chatMust(t, s.ClearAuthAbuseFailures(t.Context(), key, time.Minute))
	locked, _, _, err = s.CheckAuthAbuseLock(t.Context(), key)
	chatMust(t, err)
	if locked {
		t.Fatal("still locked after ClearAuthAbuseFailures")
	}
	chatMust(t, s.CleanupAuthAbuseLimits(t.Context()))
}
