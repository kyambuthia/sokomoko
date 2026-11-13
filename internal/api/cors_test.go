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
	})

	t.Run("does not grant CORS to untrusted origin", func(t *testing.T) {
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
	})
}
