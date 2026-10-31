package app

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const metricsSnapshotTTL = 5 * time.Second

var (
	httpMetricMethods  = [...]string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OTHER"}
	httpMetricStatuses = [...]string{"1xx", "2xx", "3xx", "4xx", "5xx", "other"}
	httpDurationBounds = [...]float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
)

// HTTPMetrics records bounded-cardinality request totals and latency for scraping.
type HTTPMetrics struct {
	startedAt       time.Time
	requests        [len(httpMetricMethods)][len(httpMetricStatuses)]atomic.Uint64
	durationBuckets [len(httpDurationBounds) + 1]atomic.Uint64
	durationNanos   atomic.Uint64
	snapshotMu      sync.Mutex
	snapshot        string
	snapshotAt      time.Time
}

// NewHTTPMetrics creates a process-local HTTP metrics collector.
func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{startedAt: time.Now()}
}

// Middleware measures completed responses. The scrape endpoint is excluded so
// scraping does not change the values it returns.
func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		m.record(r.Method, recorder.status, time.Since(started))
	})
}

func (m *HTTPMetrics) record(method string, status int, duration time.Duration) {
	methodIndex := len(httpMetricMethods) - 1
	for index, known := range httpMetricMethods[:len(httpMetricMethods)-1] {
		if method == known {
			methodIndex = index
			break
		}
	}

	statusClass := status / 100
	statusIndex := len(httpMetricStatuses) - 1
	if statusClass >= 1 && statusClass <= 5 {
		statusIndex = statusClass - 1
	}
	m.requests[methodIndex][statusIndex].Add(1)

	bucket := len(httpDurationBounds)
	seconds := duration.Seconds()
	for index, bound := range httpDurationBounds {
		if seconds <= bound {
			bucket = index
			break
		}
	}
	m.durationBuckets[bucket].Add(1)
	m.durationNanos.Add(uint64(duration))
}

// Handler serves the Prometheus text exposition when a bearer token is set.
func (m *HTTPMetrics) Handler(token string) http.Handler {
	if m == nil {
		m = NewHTTPMetrics()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			writePlainHTTPError(w, r, http.StatusNotFound, "not found")
			return
		}
		if !validMetricsBearer(r.Header.Get("Authorization"), token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			writePlainHTTPError(w, r, http.StatusUnauthorized, "unauthorized")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writePlainHTTPError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		body := m.cachedExposition()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(body))
		}
	})
}

func validMetricsBearer(header, token string) bool {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(token)) == 1
}

func writePlainHTTPError(w http.ResponseWriter, r *http.Request, status int, message string) {
	body := []byte(message + "\n")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func (m *HTTPMetrics) cachedExposition() string {
	now := time.Now()
	m.snapshotMu.Lock()
	defer m.snapshotMu.Unlock()
	if m.snapshot != "" && now.Sub(m.snapshotAt) < metricsSnapshotTTL {
		return m.snapshot
	}
	m.snapshot = m.exposition()
	m.snapshotAt = now
	return m.snapshot
}

func (m *HTTPMetrics) exposition() string {
	var body strings.Builder
	fmt.Fprintln(&body, "# HELP sokomoko_http_requests_total Completed HTTP requests by bounded method and status class.")
	fmt.Fprintln(&body, "# TYPE sokomoko_http_requests_total counter")
	for methodIndex, method := range httpMetricMethods {
		for statusIndex, status := range httpMetricStatuses {
			fmt.Fprintf(&body, "sokomoko_http_requests_total{method=%q,status_class=%q} %d\n",
				method, status, m.requests[methodIndex][statusIndex].Load())
		}
	}

	fmt.Fprintln(&body, "# HELP sokomoko_http_request_duration_seconds HTTP response duration in seconds.")
	fmt.Fprintln(&body, "# TYPE sokomoko_http_request_duration_seconds histogram")
	var cumulative uint64
	for index, bound := range httpDurationBounds {
		cumulative += m.durationBuckets[index].Load()
		fmt.Fprintf(&body, "sokomoko_http_request_duration_seconds_bucket{le=%q} %d\n",
			strconv.FormatFloat(bound, 'f', -1, 64), cumulative)
	}
	cumulative += m.durationBuckets[len(httpDurationBounds)].Load()
	fmt.Fprintf(&body, "sokomoko_http_request_duration_seconds_bucket{le=%q} %d\n", "+Inf", cumulative)
	fmt.Fprintf(&body, "sokomoko_http_request_duration_seconds_sum %g\n",
		float64(m.durationNanos.Load())/float64(time.Second))
	fmt.Fprintf(&body, "sokomoko_http_request_duration_seconds_count %d\n", cumulative)

	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	fmt.Fprintln(&body, "# HELP go_goroutines Number of goroutines that currently exist.")
	fmt.Fprintln(&body, "# TYPE go_goroutines gauge")
	fmt.Fprintf(&body, "go_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintln(&body, "# HELP go_memstats_heap_alloc_bytes Number of bytes allocated and still in use.")
	fmt.Fprintln(&body, "# TYPE go_memstats_heap_alloc_bytes gauge")
	fmt.Fprintf(&body, "go_memstats_heap_alloc_bytes %d\n", memory.HeapAlloc)
	fmt.Fprintln(&body, "# HELP go_memstats_heap_inuse_bytes Number of bytes in in-use spans.")
	fmt.Fprintln(&body, "# TYPE go_memstats_heap_inuse_bytes gauge")
	fmt.Fprintf(&body, "go_memstats_heap_inuse_bytes %d\n", memory.HeapInuse)
	fmt.Fprintln(&body, "# HELP go_memstats_heap_sys_bytes Number of bytes obtained from the system for the heap.")
	fmt.Fprintln(&body, "# TYPE go_memstats_heap_sys_bytes gauge")
	fmt.Fprintf(&body, "go_memstats_heap_sys_bytes %d\n", memory.HeapSys)
	fmt.Fprintln(&body, "# HELP go_gc_cycles_total Total number of completed garbage collection cycles.")
	fmt.Fprintln(&body, "# TYPE go_gc_cycles_total counter")
	fmt.Fprintf(&body, "go_gc_cycles_total %d\n", memory.NumGC)
	fmt.Fprintln(&body, "# HELP sokomoko_process_uptime_seconds Time since this process created its metrics collector.")
	fmt.Fprintln(&body, "# TYPE sokomoko_process_uptime_seconds gauge")
	uptime := time.Since(m.startedAt).Seconds()
	if uptime < 0 {
		uptime = 0
	}
	fmt.Fprintf(&body, "sokomoko_process_uptime_seconds %g\n", uptime)

	return body.String()
}
