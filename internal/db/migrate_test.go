package db

import (
	"context"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db/dbtest"
)

func TestCheckSchema_RequiresCompleteMatchingHistory(t *testing.T) {
	store, err := OpenStore(dbtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.CheckSchema(ctx); err == nil {
		t.Fatal("accepted an unmigrated database")
	}
	var exists bool
	if err := store.DB.QueryRow("SELECT to_regclass('schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("schema check mutated the database")
	}
	if err := store.ApplySchema(); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"UPDATE schema_migrations SET name = 'renamed.sql' WHERE version = 1",
		"DELETE FROM schema_migrations WHERE version = 1",
		"INSERT INTO schema_migrations (version, name) VALUES (9999, 'future.sql')",
	} {
		t.Run(statement, func(t *testing.T) {
			tx, err := store.DB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(statement); err != nil {
				_ = tx.Rollback()
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := store.CheckSchema(ctx); err == nil {
				t.Fatal("accepted incompatible migration history")
			}
		})
	}
}
