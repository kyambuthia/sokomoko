package db

import (
	"context"
	"database/sql/driver"
	"log/slog"
	"time"
)

// RunExclusive runs fn while holding a session advisory lock on its own
// connection. It does not wait: when another process holds the lock it
// returns false without running fn.
func (s *Store) RunExclusive(ctx context.Context, key int64, fn func(context.Context)) (bool, error) {
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	discard := func() {
		// database/sql drops the physical session when Raw returns ErrBadConn.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired); err != nil {
		discard()
		return false, err
	}
	if !acquired {
		return false, nil
	}
	defer func() {
		// Unlock even when ctx is already cancelled; closing the connection
		// would release it anyway, but the pool keeps connections open.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		err := conn.QueryRowContext(unlockCtx, "SELECT pg_advisory_unlock($1)", key).Scan(&unlocked)
		if err != nil || !unlocked {
			slog.Warn("maintenance advisory unlock failed; discarding connection", "key", key, "unlocked", unlocked, "error", err)
			// Do not return a possibly lock-holding session to the pool.
			discard()
		}
	}()
	fn(ctx)
	return true, nil
}
