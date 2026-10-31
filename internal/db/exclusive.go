package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"log/slog"
	"time"
)

const advisoryUnlockTimeout = 5 * time.Second

// discardSQLConn prevents a session with an uncertain advisory-lock state from
// being returned to the database/sql pool.
func discardSQLConn(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
}

// releaseAdvisoryLock releases a session lock even if the caller was canceled.
// A failed or unexpected unlock discards the physical session so the lock
// cannot remain attached to a pooled connection.
func releaseAdvisoryLock(conn *sql.Conn, key int64) {
	unlockCtx, cancel := context.WithTimeout(context.Background(), advisoryUnlockTimeout)
	defer cancel()
	var unlocked bool
	err := conn.QueryRowContext(unlockCtx, "SELECT pg_advisory_unlock($1)", key).Scan(&unlocked)
	if err != nil || !unlocked {
		slog.Warn("PostgreSQL advisory unlock failed; discarding connection", "key", key, "unlocked", unlocked, "error", err)
		discardSQLConn(conn)
	}
}

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
		discardSQLConn(conn)
		return false, err
	}
	if !acquired {
		return false, nil
	}
	defer releaseAdvisoryLock(conn, key)
	fn(ctx)
	return true, nil
}
