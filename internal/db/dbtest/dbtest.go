// Package dbtest provisions isolated PostgreSQL schemas for tests.
//
// Tests that need a database read SOKOMOKO_TEST_DATABASE_URL. Each call creates
// a fresh schema and returns a connection string whose search_path points at
// it, so packages can run in parallel against one database. When the variable
// is unset the tests are skipped, unless SOKOMOKO_REQUIRE_DB_TESTS=1 (as in CI),
// in which case they fail.
package dbtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	EnvDatabaseURL = "SOKOMOKO_TEST_DATABASE_URL"
	EnvRequire     = "SOKOMOKO_REQUIRE_DB_TESTS"
)

// ErrNoDatabase is returned when no test database is configured.
var ErrNoDatabase = errors.New(EnvDatabaseURL + " is not set")

// DSN creates a schema for tb and returns a connection string scoped to it.
// The schema is dropped when the test finishes.
func DSN(tb testing.TB) string {
	tb.Helper()
	dsn, cleanup, err := CreateSchema()
	if errors.Is(err, ErrNoDatabase) {
		if os.Getenv(EnvRequire) == "1" {
			tb.Fatalf("%v and %s=1", err, EnvRequire)
		}
		tb.Skipf("skipping database test: %v", err)
	}
	if err != nil {
		tb.Fatalf("create test schema: %v", err)
	}
	tb.Cleanup(cleanup)
	return dsn
}

// MainDSN is DSN for TestMain, where no testing.TB exists. It returns ok=false
// when the database is not configured and tests should be skipped; it exits
// the process when the database is required but missing.
func MainDSN() (dsn string, cleanup func(), ok bool) {
	dsn, cleanup, err := CreateSchema()
	if errors.Is(err, ErrNoDatabase) {
		if os.Getenv(EnvRequire) == "1" {
			fmt.Fprintf(os.Stderr, "%v and %s=1\n", err, EnvRequire)
			os.Exit(1)
		}
		return "", func() {}, false
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "create test schema: %v\n", err)
		os.Exit(1)
	}
	return dsn, cleanup, true
}

// CreateSchema creates a uniquely named schema and returns a scoped DSN and a
// cleanup func that drops it.
func CreateSchema() (string, func(), error) {
	base := strings.TrimSpace(os.Getenv(EnvDatabaseURL))
	if base == "" {
		return "", nil, ErrNoDatabase
	}

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return "", nil, err
	}
	schema := "test_" + hex.EncodeToString(suffix)

	admin, err := sql.Open("pgx", base)
	if err != nil {
		return "", nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		_ = admin.Close()
		return "", nil, err
	}

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = admin.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		_ = admin.Close()
	}

	scoped, err := withSearchPath(base, schema)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return scoped, cleanup, nil
}

func withSearchPath(dsn, schema string) (string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	return dsn + " search_path=" + schema, nil
}
