package lifecycle

import (
	"context"
	"testing"
	"time"
)

type blockingMaintenance struct{ started chan struct{} }

func (s *blockingMaintenance) ReleaseExpiredReservations(ctx context.Context) (int, error) {
	close(s.started)
	<-ctx.Done()
	return 0, ctx.Err()
}
func (s *blockingMaintenance) CleanupSessions(context.Context) error {
	panic("cleanup after cancellation")
}
func (s *blockingMaintenance) CleanupPasswordResetTokens(context.Context) error {
	panic("cleanup after cancellation")
}
func (s *blockingMaintenance) ExpireUnpaidOrders(context.Context, time.Time) (int, error) {
	return 0, nil
}

func TestMaintain_CancelsDatabaseWorkBeforeContinuing(t *testing.T) {
	store := &blockingMaintenance{started: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); Maintain(ctx, store) }()
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("maintenance never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("maintenance ignored cancellation")
	}
}

type lockedMaintenance struct {
	held   bool
	failed bool
	ran    int
}

func (s *lockedMaintenance) RunExclusive(ctx context.Context, _ int64, fn func(context.Context)) (bool, error) {
	if s.failed {
		return false, context.DeadlineExceeded
	}
	if s.held {
		return false, nil
	}
	fn(ctx)
	return true, nil
}
func (s *lockedMaintenance) ReleaseExpiredReservations(context.Context) (int, error) {
	s.ran++
	return 0, nil
}
func (s *lockedMaintenance) CleanupSessions(context.Context) error            { return nil }
func (s *lockedMaintenance) CleanupPasswordResetTokens(context.Context) error { return nil }
func (s *lockedMaintenance) ExpireUnpaidOrders(context.Context, time.Time) (int, error) {
	return 0, nil
}

func TestExclusive_SkipsWhileAnotherReplicaHoldsTheLock(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		store   *lockedMaintenance
		wantRan int
	}{
		{"free", &lockedMaintenance{}, 1},
		{"held elsewhere", &lockedMaintenance{held: true}, 0},
		{"lock check failed", &lockedMaintenance{failed: true}, 1},
	} {
		job := func(ctx context.Context) { _, _ = tc.store.ReleaseExpiredReservations(ctx) }
		exclusive(ctx, tc.store, reservationsJobLockKey, job)()
		if tc.store.ran != tc.wantRan {
			t.Errorf("%s: job ran %d times, want %d", tc.name, tc.store.ran, tc.wantRan)
		}
	}
}
