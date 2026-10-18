package db

import (
	"context"
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
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired); err != nil {
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
		_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", key)
	}()
	fn(ctx)
	return true, nil
}
