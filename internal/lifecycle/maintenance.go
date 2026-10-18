package lifecycle

import (
	"context"
	"log/slog"
	"time"
)

// MaintenanceStore defines the operations needed by the maintenance runner.
type MaintenanceStore interface {
	ReleaseExpiredReservationsContext(context.Context) (int, error)
	CleanupSessionsContext(context.Context) error
	CleanupPasswordResetTokensContext(context.Context) error
	ExpireUnpaidOrders(ctx context.Context, cutoff time.Time) (int, error)
}

// ExclusiveRunner runs a job only when no other process is running it. A
// store that implements it lets several replicas share the maintenance work.
type ExclusiveRunner interface {
	RunExclusive(ctx context.Context, key int64, fn func(context.Context)) (bool, error)
}

// Advisory lock keys for the two maintenance passes. Every replica runs the
// same loop; the lock lets only one of them do each pass.
const (
	reservationsJobLockKey int64 = 0x736f6b6f72657376 // "sokoresv"
	housekeepingJobLockKey int64 = 0x736f6b6f686f7573 // "sokohous"
)

// exclusive wraps job so it runs under key when store supports it. If the
// lock cannot be checked the job runs anyway: every job is idempotent, and
// skipping it would let reservations and unpaid orders pile up.
func exclusive(ctx context.Context, store MaintenanceStore, key int64, job func(context.Context)) func() {
	runner, ok := store.(ExclusiveRunner)
	if !ok {
		return func() { job(ctx) }
	}
	return func() {
		if ctx.Err() != nil {
			return
		}
		if _, err := runner.RunExclusive(ctx, key, job); err != nil && ctx.Err() == nil {
			slog.Warn("maintenance lock unavailable; running without it", "error", err)
			job(ctx)
		}
	}
}

// UnpaidOrderTTL mirrors db.UnpaidOrderTTL; lifecycle does not import db.
const UnpaidOrderTTL = 30 * time.Minute

func Maintain(ctx context.Context, store MaintenanceStore) {
	reservations := time.NewTicker(time.Minute)
	defer reservations.Stop()
	housekeeping := time.NewTicker(5 * time.Minute)
	defer housekeeping.Stop()
	release := exclusive(ctx, store, reservationsJobLockKey, func(ctx context.Context) {
		if ctx.Err() != nil {
			return
		}
		released, err := store.ReleaseExpiredReservationsContext(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Error("reservation expiry failed", "error", err)
		}
		if released > 0 {
			slog.Info("expired reservations released", "count", released)
		}
		expired, err := store.ExpireUnpaidOrders(ctx, time.Now().Add(-UnpaidOrderTTL))
		if err != nil && ctx.Err() == nil {
			slog.Error("unpaid order expiry failed", "error", err)
		}
		if expired > 0 {
			slog.Info("unpaid orders cancelled", "count", expired)
		}
	})
	prune := exclusive(ctx, store, housekeepingJobLockKey, func(ctx context.Context) {
		for _, cleanup := range []func(context.Context) error{store.CleanupSessionsContext, store.CleanupPasswordResetTokensContext} {
			if ctx.Err() != nil {
				return
			}
			if err := cleanup(ctx); err != nil && ctx.Err() == nil {
				slog.Error("session or token cleanup failed", "error", err)
			}
		}
	})
	release()
	prune()
	for {
		select {
		case <-ctx.Done():
			return
		case <-reservations.C:
			release()
		case <-housekeeping.C:
			prune()
		}
	}
}
