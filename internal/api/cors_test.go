package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddleware(t *testing.T) {
	allowed := map[string]struct{}{
		"shop.example.com": {},
		"localhost":        {},
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := CORSMiddleware(allowed, next)

	t.Run("allows trusted origin and handles OPTIONS preflight", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/catalog/products", nil)
		req.Header.Set("Origin", "https://shop.example.com")
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d; want 204", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://shop.example.com" {
			t.Fatalf("Access-Control-Allow-Origin = %q", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Fatalf("Access-Control-Allow-Credentials = %q", got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Fatalf("Vary = %q; want Origin", got)
		}
	})

	t.Run("does not grant CORS to untrusted origin and sets Vary", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("unexpected Access-Control-Allow-Origin = %q", got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Fatalf("Vary = %q; want Origin", got)
		}
	})

	t.Run("omits Vary when Origin header is absent", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products", nil)
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200", rec.Code)
		}
		if got := rec.Header().Get("Vary"); got != "" {
			t.Fatalf("unexpected Vary = %q", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("unexpected Access-Control-Allow-Origin = %q", got)
		}
	})

	t.Run("rejects malformed origins with userinfo, path, query, or fragment", func(t *testing.T) {
		rejectedOrigins := []struct {
			name   string
			origin string
		}{
			{name: "userinfo", origin: "https://user@shop.example.com"},
			{name: "userinfo with password", origin: "https://user:pass@shop.example.com"},
			{name: "trailing slash path", origin: "https://shop.example.com/"},
			{name: "non-empty path", origin: "https://shop.example.com/api"},
			{name: "query string", origin: "https://shop.example.com?foo=bar"},
			{name: "empty query marker", origin: "https://shop.example.com?"},
			{name: "fragment", origin: "https://shop.example.com#section"},
			{name: "empty fragment marker", origin: "https://shop.example.com#"},
			{name: "non-http scheme", origin: "ftp://shop.example.com"},
		}

		for _, tc := range rejectedOrigins {
			t.Run(tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodOptions, "/api/v1/catalog/products", nil)
				req.Header.Set("Origin", tc.origin)
				handler.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d; want 200 (preflight rejected)", rec.Code)
				}
				if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
					t.Fatalf("unexpected Access-Control-Allow-Origin = %q for %q", got, tc.origin)
				}
				if got := rec.Header().Get("Vary"); got != "Origin" {
					t.Fatalf("Vary = %q; want Origin", got)
				}
			})
		}
	})

	t.Run("allows http origin when allowHTTP is true", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products", nil)
		req.Header.Set("Origin", "http://localhost")
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost" {
			t.Fatalf("Access-Control-Allow-Origin = %q; want http://localhost", got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Fatalf("Vary = %q; want Origin", got)
		}
	})
}

func TestCORSMiddlewareWithScheme_DisallowHTTP(t *testing.T) {
	allowed := map[string]struct{}{
		"shop.example.com": {},
		"localhost":        {},
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := CORSMiddlewareWithScheme(allowed, false, next)

	t.Run("rejects http origin when allowHTTP is false", func(t *testing.T) {
		for _, origin := range []string{"http://shop.example.com", "http://localhost"} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodOptions, "/api/v1/catalog/products", nil)
			req.Header.Set("Origin", origin)
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("origin %q: status = %d; want 200", origin, rec.Code)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("origin %q: unexpected Access-Control-Allow-Origin = %q", origin, got)
			}
			if got := rec.Header().Get("Vary"); got != "Origin" {
				t.Fatalf("origin %q: Vary = %q; want Origin", origin, got)
			}
		}
	})

	t.Run("allows https origin when allowHTTP is false", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/catalog/products", nil)
		req.Header.Set("Origin", "https://shop.example.com")
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d; want 204", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://shop.example.com" {
			t.Fatalf("Access-Control-Allow-Origin = %q; want https://shop.example.com", got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Fatalf("Vary = %q; want Origin", got)
		}
	})
}
