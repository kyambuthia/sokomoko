package app

import (
	"bytes"
	"crypto/tls"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeaders_RespectsProxyTrust(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		trust, tls, wantHSTS bool
	}{
		{"untrusted header", false, false, false},
		{"trusted proxy", true, false, true},
		{"direct TLS", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://shop.example.com/", nil)
			req.Header.Set("X-Forwarded-Proto", "https")
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			rec := httptest.NewRecorder()
			SecurityHeadersWithProxy(tc.trust)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)
			if got := rec.Header().Get("Strict-Transport-Security") != ""; got != tc.wantHSTS {
				t.Fatalf("HSTS present=%t, want %t", got, tc.wantHSTS)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("dynamic response can be cached")
			}
		})
	}
}

func TestStatusRecorder_TracksTheStatusActuallySent(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	_, _ = rec.Write([]byte("ok"))
	rec.WriteHeader(http.StatusInternalServerError)
	if rec.status != http.StatusOK {
		t.Fatalf("status=%d, want 200 after implicit header", rec.status)
	}
	if rec.Unwrap() == nil {
		t.Fatal("missing underlying response writer")
	}
}

func TestRequestLogPath_RedactsCallbackToken(t *testing.T) {
	if got := requestLogPath("/payments/callback/mpesa/sensitive-token"); got != "/payments/callback/mpesa/[redacted]" {
		t.Fatalf("callback log path = %q", got)
	}
	if got := requestLogPath("/payments/order/42"); got != "/payments/order/42" {
		t.Fatalf("ordinary log path = %q", got)
	}
}

func TestCanonicalHost_IPv6(t *testing.T) {
	if got := CanonicalHost("[::1]:8080"); got != "::1" {
		t.Fatalf("host=%q, want ::1", got)
	}
	if got := CanonicalHost("::1"); got != "::1" {
		t.Fatalf("bare host=%q, want ::1", got)
	}
}

func TestCanonicalHost(t *testing.T) {
	if got := CanonicalHost("Admin.Localhost:6969"); got != "admin.localhost" {
		t.Fatalf("expected admin.localhost, got %q", got)
	}
	for _, bad := range []string{
		"user@evil.example",
		"user:pass@evil.example:8080",
		"evil.example/path",
		"evil.example?query=1",
		"evil.example#frag",
	} {
		if got := CanonicalHost(bad); got != "" {
			t.Fatalf("CanonicalHost(%q) = %q, want empty", bad, got)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), SecurityHeadersWithProxy(false))

	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "object-src 'none'") {
		t.Fatalf("expected object-src 'none' in CSP header, got %q", got)
	}
}

func TestBodyLimit(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}), BodyLimit(4))

	req := httptest.NewRequest(http.MethodPost, "http://localhost/", strings.NewReader("12345"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}
}

func TestRecovererRendersErrorPageWithRequestID(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}), Recoverer(), RequestID())

	req := httptest.NewRequest(http.MethodGet, "http://localhost/panic", nil)
	req.Header.Set("X-Request-Id", "req-test-500")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected %d, got %d", http.StatusInternalServerError, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected text/html content type, got %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Internal Server Error") {
		t.Fatal("expected error page heading in body")
	}
	if !strings.Contains(body, "req-test-500") {
		t.Fatal("expected request id in error page body")
	}
}

func TestClassifyClientKind(t *testing.T) {
	agentReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	agentReq.Header.Set("Authorization", "Bearer smk_test123")
	if got := ClassifyClientKind(agentReq); got != ClientKindAgentToken {
		t.Fatalf("agent_token got %q", got)
	}

	crawlerReq := httptest.NewRequest(http.MethodGet, "/", nil)
	crawlerReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	if got := ClassifyClientKind(crawlerReq); got != ClientKindCrawler {
		t.Fatalf("crawler got %q", got)
	}

	curlReq := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products", nil)
	curlReq.Header.Set("User-Agent", "curl/8.5.0")
	if got := ClassifyClientKind(curlReq); got != ClientKindAPIClient {
		t.Fatalf("api_client (curl) got %q", got)
	}

	rawAPIReq := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products", nil)
	if got := ClassifyClientKind(rawAPIReq); got != ClientKindAPIClient {
		t.Fatalf("api_client (path) got %q", got)
	}

	browserFetchReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	browserFetchReq.Header.Set("User-Agent", "Mozilla/5.0")
	browserFetchReq.Header.Set("Sec-Fetch-Mode", "cors")
	if got := ClassifyClientKind(browserFetchReq); got != ClientKindBrowser {
		t.Fatalf("browser (fetch) got %q", got)
	}

	browserReq := httptest.NewRequest(http.MethodGet, "/", nil)
	browserReq.Header.Set("User-Agent", "Mozilla/5.0")
	if got := ClassifyClientKind(browserReq); got != ClientKindBrowser {
		t.Fatalf("browser got %q", got)
	}
}

func TestRequestLoggerAndHTTPMetrics_IncludeClientKind(t *testing.T) {
	var logBuf bytes.Buffer
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, nil)))
	defer slog.SetDefault(prevLogger)

	metrics := NewHTTPMetrics()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}), metrics.Middleware, RequestLogger())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", nil)
	req.Header.Set("Authorization", "Bearer smk_agent_123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !strings.Contains(logBuf.String(), `"client_kind":"agent_token"`) {
		t.Fatalf("expected client_kind in structured log, got: %s", logBuf.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsReq.Header.Set("Authorization", "Bearer secret-metrics-token")
	metricsRec := httptest.NewRecorder()
	metrics.Handler("secret-metrics-token").ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("metrics status=%d, want 200", metricsRec.Code)
	}
	wantLine := `sokomoko_http_requests_total{method="POST",status_class="2xx",client_kind="agent_token"} 1`
	if !strings.Contains(metricsRec.Body.String(), wantLine) {
		t.Fatalf("expected %q in metrics output:\n%s", wantLine, metricsRec.Body.String())
	}
}
