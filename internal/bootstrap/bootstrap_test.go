package bootstrap

import (
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/db/dbtest"
)

func TestService_MigrateAndSeedAreIdempotent(t *testing.T) {
	store, err := db.OpenStore(dbtest.DSN(t))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := New(store)

	if err := svc.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	if err := svc.Seed(); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	firstCount, err := store.CountProducts(t.Context())
	if err != nil {
		t.Fatalf("count products after first seed: %v", err)
	}
	if firstCount == 0 {
		t.Fatal("expected seed to create catalog products")
	}

	listings, total, err := store.BrowseServices(t.Context(), db.ServiceFilter{Limit: 10})
	if err != nil || total != 3 || len(listings) != 3 {
		t.Fatalf("sample services: count=%d err=%v", total, err)
	}
	// A seller's edited sample must not be republished or overwritten by seeding.
	if _, err := store.DB.ExecContext(t.Context(), "UPDATE service_listings SET title='Seller edited sample', state='archived', version=2 WHERE slug='demo-neighbourhood-haircut'"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Prepare(true); err != nil {
		t.Fatalf("prepare with seed failed: %v", err)
	}
	secondCount, err := store.CountProducts(t.Context())
	if err != nil {
		t.Fatalf("count products after second seed: %v", err)
	}
	var title, state string
	var version, count int
	if err := store.DB.QueryRowContext(t.Context(), "SELECT title,state,version FROM service_listings WHERE slug='demo-neighbourhood-haircut'").Scan(&title, &state, &version); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRowContext(t.Context(), "SELECT count(*) FROM service_listings").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if title != "Seller edited sample" || state != "archived" || version != 2 || count != 3 {
		t.Fatal("seeding changed an existing service or duplicated entries")
	}
	if secondCount != firstCount {
		t.Fatalf("expected idempotent seed count, got first=%d second=%d", firstCount, secondCount)
	}
}
