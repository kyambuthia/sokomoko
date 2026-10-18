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

// UnpaidOrderTTL mirrors db.UnpaidOrderTTL; lifecycle does not import db.
const UnpaidOrderTTL = 30 * time.Minute

func Maintain(ctx context.Context, store MaintenanceStore) {
	reservations := time.NewTicker(time.Minute)
	defer reservations.Stop()
	housekeeping := time.NewTicker(5 * time.Minute)
	defer housekeeping.Stop()
	release := func() {
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
	}
	prune := func() {
		for _, cleanup := range []func(context.Context) error{store.CleanupSessionsContext, store.CleanupPasswordResetTokensContext} {
			if ctx.Err() != nil {
				return
			}
			if err := cleanup(ctx); err != nil && ctx.Err() == nil {
				slog.Error("session or token cleanup failed", "error", err)
			}
		}
	}
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
