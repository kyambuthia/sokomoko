package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

const (
	SurfaceStorefront = "storefront"
	SurfaceAPI        = "api"
	SurfaceAdmin      = "admin"
	SurfacePartner    = "partner"
	SurfaceCourier    = "courier"
	SurfaceStatic     = "static"
)

const (
	defaultTrafficBufferSize   = 4096
	defaultTrafficFlushSize    = 256
	defaultTrafficFlushPeriod  = 10 * time.Second
	defaultTrafficFlushTimeout = 5 * time.Second
)

// TrafficRollupStore persists aggregated traffic rollup buckets.
type TrafficRollupStore interface {
	UpsertTrafficRollups(ctx context.Context, rollups []db.TrafficRollup) error
}

type trafficEvent struct {
	bucketHour time.Time
	clientKind string
	surface    string
	status     int
}

// TrafficCollector buffers HTTP traffic classification events and flushes
// hourly rollups to PostgreSQL in the background without blocking requests.
type TrafficCollector struct {
	store         TrafficRollupStore
	ch            chan trafficEvent
	flushInterval time.Duration
	now           func() time.Time
}

// NewTrafficCollector creates a non-blocking traffic rollup collector.
func NewTrafficCollector(store TrafficRollupStore) *TrafficCollector {
	return &TrafficCollector{
		store:         store,
		ch:            make(chan trafficEvent, defaultTrafficBufferSize),
		flushInterval: defaultTrafficFlushPeriod,
		now:           time.Now,
	}
}

// ClassifySurface maps a request to a bounded surface label
// (storefront, api, admin, partner, courier, static).
func ClassifySurface(r *http.Request) string {
	if r == nil {
		return SurfaceStorefront
	}
	path := r.URL.Path
	if strings.HasPrefix(path, "/static/") || path == "/favicon.ico" {
		return SurfaceStatic
	}
	host := CanonicalHost(r.Host)
	switch {
	case strings.HasPrefix(host, "api.") || path == "/api" || strings.HasPrefix(path, "/api/"):
		return SurfaceAPI
	case strings.HasPrefix(host, "admin."):
		return SurfaceAdmin
	case strings.HasPrefix(host, "partner."):
		return SurfacePartner
	case strings.HasPrefix(host, "courier."):
		return SurfaceCourier
	default:
		return SurfaceStorefront
	}
}

// Record enqueues one completed request without blocking the caller.
func (c *TrafficCollector) Record(r *http.Request, status int) {
	if c == nil || r == nil || r.URL.Path == "/metrics" {
		return
	}
	ev := trafficEvent{
		bucketHour: c.now().UTC().Truncate(time.Hour),
		clientKind: ClassifyClientKind(r),
		surface:    ClassifySurface(r),
		status:     status,
	}
	select {
	case c.ch <- ev:
	default:
		// Drop when buffer is full so request latency is never impacted.
	}
}

// Run drains buffered events and periodically flushes them until ctx is cancelled.
func (c *TrafficCollector) Run(ctx context.Context) {
	if c == nil || c.store == nil {
		return
	}
	interval := c.flushInterval
	if interval <= 0 {
		interval = defaultTrafficFlushPeriod
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	pending := make(map[db.TrafficRollupKey]*db.TrafficRollupCounts)
	for {
		select {
		case ev := <-c.ch:
			accumulateTrafficEvent(pending, ev)
			if len(pending) >= defaultTrafficFlushSize {
				c.flushPending(ctx, pending)
			}
		case <-ticker.C:
			c.flushPending(ctx, pending)
		case <-ctx.Done():
			for {
				select {
				case ev := <-c.ch:
					accumulateTrafficEvent(pending, ev)
				default:
					flushCtx, cancel := context.WithTimeout(context.Background(), defaultTrafficFlushTimeout)
					c.flushPending(flushCtx, pending)
					cancel()
					return
				}
			}
		}
	}
}

func accumulateTrafficEvent(pending map[db.TrafficRollupKey]*db.TrafficRollupCounts, ev trafficEvent) {
	key := db.TrafficRollupKey{
		BucketHour: ev.bucketHour,
		ClientKind: ev.clientKind,
		Surface:    ev.surface,
	}
	counts, ok := pending[key]
	if !ok {
		counts = &db.TrafficRollupCounts{}
		pending[key] = counts
	}
	counts.RequestCount++
	if ev.status >= 400 && ev.status < 500 {
		counts.Error4xxCount++
	}
	if ev.status >= 500 {
		counts.Error5xxCount++
	}
	if ev.status == http.StatusTooManyRequests {
		counts.RateLimitedCount++
	}
}

func (c *TrafficCollector) flushPending(ctx context.Context, pending map[db.TrafficRollupKey]*db.TrafficRollupCounts) {
	if len(pending) == 0 || c.store == nil {
		return
	}
	batch := make([]db.TrafficRollup, 0, len(pending))
	for k, v := range pending {
		batch = append(batch, db.TrafficRollup{
			TrafficRollupKey:    k,
			TrafficRollupCounts: *v,
		})
	}
	if err := c.store.UpsertTrafficRollups(ctx, batch); err != nil {
		slog.Warn("flush traffic rollups", "error", err, "buckets", len(batch))
		return
	}
	clear(pending)
}
