package bootstrap

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func TestService_MigrateAndSeedAreIdempotent(t *testing.T) {
	path := fmt.Sprintf("./test_bootstrap_%d.db", time.Now().UnixNano())
	store, err := db.OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = os.Remove(path)
	})

	svc := New(store)

	if err := svc.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	if err := svc.Seed(); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	firstCount, err := store.CountProducts()
	if err != nil {
		t.Fatalf("count products after first seed: %v", err)
	}
	if firstCount == 0 {
		t.Fatal("expected seed to create catalog products")
	}

	if err := svc.Prepare(true); err != nil {
		t.Fatalf("prepare with seed failed: %v", err)
	}
	secondCount, err := store.CountProducts()
	if err != nil {
		t.Fatalf("count products after second seed: %v", err)
	}
	if secondCount != firstCount {
		t.Fatalf("expected idempotent seed count, got first=%d second=%d", firstCount, secondCount)
	}
}
