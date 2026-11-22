package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type stubTrafficRollupStore struct {
	mu      sync.Mutex
	batches [][]db.TrafficRollup
}

func (s *stubTrafficRollupStore) UpsertTrafficRollups(_ context.Context, rollups []db.TrafficRollup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]db.TrafficRollup, len(rollups))
	copy(cp, rollups)
	s.batches = append(s.batches, cp)
	return nil
}

func TestClassifySurface(t *testing.T) {
	cases := []struct {
		name string
		host string
		path string
		want string
	}{
		{name: "static asset", host: "localhost", path: "/static/stylesheets/app.css", want: SurfaceStatic},
		{name: "favicon", host: "admin.localhost", path: "/favicon.ico", want: SurfaceStatic},
		{name: "api host", host: "api.example.com", path: "/v1/products", want: SurfaceAPI},
		{name: "api path", host: "localhost", path: "/api/v1/products", want: SurfaceAPI},
		{name: "admin host", host: "admin.localhost:8080", path: "/reports", want: SurfaceAdmin},
		{name: "partner host", host: "partner.localhost", path: "/orders", want: SurfacePartner},
		{name: "courier host", host: "courier.localhost", path: "/deliveries", want: SurfaceCourier},
		{name: "storefront default", host: "localhost", path: "/products/mug", want: SurfaceStorefront},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://"+tc.host+tc.path, nil)
			if got := ClassifySurface(req); got != tc.want {
				t.Fatalf("ClassifySurface(%s%s) = %q, want %q", tc.host, tc.path, got, tc.want)
			}
		})
	}
}

func TestTrafficCollectorFlushesOnCancel(t *testing.T) {
	store := &stubTrafficRollupStore{}
	collector := NewTrafficCollector(store)
	fixedNow := time.Date(2026, 11, 22, 10, 45, 0, 0, time.UTC)
	collector.now = func() time.Time { return fixedNow }
	collector.flushInterval = time.Hour // rely on shutdown drain flush

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		collector.Run(ctx)
		close(done)
	}()

	req1 := httptest.NewRequest(http.MethodGet, "http://api.localhost/api/v1/products", nil)
	req1.Header.Set("Authorization", "Bearer smk_test_1234")
	collector.Record(req1, http.StatusOK)

	req2 := httptest.NewRequest(http.MethodPost, "http://api.localhost/api/v1/cart", nil)
	req2.Header.Set("Authorization", "Bearer smk_test_1234")
	collector.Record(req2, http.StatusTooManyRequests)

	req3 := httptest.NewRequest(http.MethodGet, "http://localhost/static/app.js", nil)
	req3.Header.Set("User-Agent", "Mozilla/5.0")
	collector.Record(req3, http.StatusInternalServerError)

	cancel()
	<-done

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.batches) != 1 {
		t.Fatalf("batches = %d, want 1", len(store.batches))
	}
	byKey := map[db.TrafficRollupKey]db.TrafficRollupCounts{}
	for _, r := range store.batches[0] {
		byKey[r.TrafficRollupKey] = r.TrafficRollupCounts
	}
	apiCounts := byKey[db.TrafficRollupKey{
		BucketHour: fixedNow.Truncate(time.Hour),
		ClientKind: ClientKindAgentToken,
		Surface:    SurfaceAPI,
	}]
	if apiCounts.RequestCount != 2 || apiCounts.Error4xxCount != 1 || apiCounts.RateLimitedCount != 1 || apiCounts.Error5xxCount != 0 {
		t.Fatalf("apiCounts = %+v", apiCounts)
	}
	staticCounts := byKey[db.TrafficRollupKey{
		BucketHour: fixedNow.Truncate(time.Hour),
		ClientKind: ClientKindBrowser,
		Surface:    SurfaceStatic,
	}]
	if staticCounts.RequestCount != 1 || staticCounts.Error5xxCount != 1 {
		t.Fatalf("staticCounts = %+v", staticCounts)
	}
}
