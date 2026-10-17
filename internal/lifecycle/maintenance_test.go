package lifecycle

import (
	"context"
	"testing"
	"time"
)

type blockingMaintenance struct{ started chan struct{} }

func (s *blockingMaintenance) ReleaseExpiredReservationsContext(ctx context.Context) (int, error) {
	close(s.started)
	<-ctx.Done()
	return 0, ctx.Err()
}
func (s *blockingMaintenance) CleanupSessionsContext(context.Context) error {
	panic("cleanup after cancellation")
}
func (s *blockingMaintenance) CleanupPasswordResetTokensContext(context.Context) error {
	panic("cleanup after cancellation")
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
