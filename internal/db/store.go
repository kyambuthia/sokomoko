// Package db is the PostgreSQL persistence layer for Sokomoko.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

const (
	defaultQueryTimeout = 10 * time.Second
	maxTxAttempts       = 3
)

// PoolOptions tunes the database/sql connection pool.
type PoolOptions struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	QueryTimeout    time.Duration
}

func (o PoolOptions) withDefaults() PoolOptions {
	if o.MaxOpenConns <= 0 {
		o.MaxOpenConns = 20
	}
	if o.MaxIdleConns <= 0 || o.MaxIdleConns > o.MaxOpenConns {
		o.MaxIdleConns = o.MaxOpenConns / 2
		if o.MaxIdleConns < 1 {
			o.MaxIdleConns = 1
		}
	}
	if o.ConnMaxLifetime <= 0 {
		o.ConnMaxLifetime = 30 * time.Minute
	}
	if o.ConnMaxIdleTime <= 0 {
		o.ConnMaxIdleTime = 5 * time.Minute
	}
	if o.QueryTimeout <= 0 {
		o.QueryTimeout = defaultQueryTimeout
	}
	return o
}

type Store struct {
	DB           *sql.DB
	queryTimeout time.Duration
}

// querier is satisfied by both *sql.DB and *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// OpenStore connects to PostgreSQL using a libpq style URL or DSN with the
// default pool settings.
func OpenStore(databaseURL string) (*Store, error) {
	return Open(context.Background(), databaseURL, PoolOptions{})
}

// Open connects to PostgreSQL and verifies the connection.
func Open(ctx context.Context, databaseURL string, opts PoolOptions) (*Store, error) {
	dsn := strings.TrimSpace(databaseURL)
	if dsn == "" {
		return nil, errors.New("database url is required")
	}
	opts = opts.withDefaults()

	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	conn.SetMaxOpenConns(opts.MaxOpenConns)
	conn.SetMaxIdleConns(opts.MaxIdleConns)
	conn.SetConnMaxLifetime(opts.ConnMaxLifetime)
	conn.SetConnMaxIdleTime(opts.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := conn.PingContext(pingCtx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	return &Store{DB: conn, queryTimeout: opts.QueryTimeout}, nil
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) PingContext(ctx context.Context) error {
	if s == nil || s.DB == nil {
		return sql.ErrConnDone
	}
	return s.DB.PingContext(ctx)
}

// ctx returns a context bounded by the store's query timeout. Store methods do
// not take a caller context yet, so this keeps a slow query from pinning a
// connection forever.
func (s *Store) ctx() (context.Context, context.CancelFunc) {
	timeout := s.queryTimeout
	if timeout <= 0 {
		timeout = defaultQueryTimeout
	}
	return context.WithTimeout(context.Background(), timeout)
}

// withTx runs fn inside a transaction, retrying on serialization failures and
// deadlocks. fn must be safe to re-run.
func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	var err error
	for attempt := 1; attempt <= maxTxAttempts; attempt++ {
		err = s.runTx(ctx, fn)
		if err == nil || !isRetryableTxError(err) || ctx.Err() != nil {
			return err
		}
		time.Sleep(time.Duration(attempt*attempt) * 10 * time.Millisecond)
	}
	return err
}

func (s *Store) runTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return wrapDBError(err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := fn(tx); err != nil {
		return err
	}
	return wrapDBError(tx.Commit())
}

func isRetryableTxError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "40001", "40P01":
		return true
	}
	return false
}
