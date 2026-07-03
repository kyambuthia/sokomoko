package auth

import (
	"testing"
	"time"
)

func TestNewAuthAbuseLimiter_DisabledWhenNotConfigured(t *testing.T) {
	if limiter := newAuthAbuseLimiter(0, time.Second, 10*time.Second); limiter != nil {
		t.Fatal("expected nil limiter when max failures is disabled")
	}
	if limiter := newAuthAbuseLimiter(3, 0, 10*time.Second); limiter != nil {
		t.Fatal("expected nil limiter when base backoff is invalid")
	}
	if limiter := newAuthAbuseLimiter(3, time.Second, 0); limiter != nil {
		t.Fatal("expected nil limiter when max backoff is invalid")
	}
}

func TestAuthAbuseLimiter_ExponentialBackoffAndReset(t *testing.T) {
	limiter := newAuthAbuseLimiter(2, time.Second, 4*time.Second)
	if limiter == nil {
		t.Fatal("expected limiter")
	}

	key := "scope|127.0.0.1|identifier"
	now := time.Unix(100, 0)

	limiter.RecordFailure(key, now)
	locked, _ := limiter.IsLocked(key, now)
	if locked {
		t.Fatal("expected first failure not to lock")
	}

	limiter.RecordFailure(key, now)
	locked, retry := limiter.IsLocked(key, now)
	if !locked {
		t.Fatal("expected lock at threshold")
	}
	if retry < time.Second || retry > time.Second+50*time.Millisecond {
		t.Fatalf("retry=%v want close to 1s", retry)
	}

	next := now.Add(time.Second + 10*time.Millisecond)
	limiter.RecordFailure(key, next)
	locked, retry = limiter.IsLocked(key, next)
	if !locked {
		t.Fatal("expected lock after subsequent failure")
	}
	if retry < 2*time.Second || retry > 2*time.Second+50*time.Millisecond {
		t.Fatalf("retry=%v want close to 2s", retry)
	}

	third := next.Add(2*time.Second + 10*time.Millisecond)
	limiter.RecordFailure(key, third)
	locked, retry = limiter.IsLocked(key, third)
	if !locked {
		t.Fatal("expected lock after another failure")
	}
	if retry < 4*time.Second || retry > 4*time.Second+50*time.Millisecond {
		t.Fatalf("retry=%v want close to 4s", retry)
	}

	limiter.RecordSuccess(key)
	locked, _ = limiter.IsLocked(key, third)
	if locked {
		t.Fatal("expected success to clear lock")
	}
}
