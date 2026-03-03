package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitByIP_AllowsWithinLimit(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), RateLimitByIP(2, time.Minute, http.MethodPost))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://localhost/login", nil)
		req.RemoteAddr = "203.0.113.10:12345"
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("request %d status=%d want=%d", i+1, rec.Code, http.StatusNoContent)
		}
	}
}

func TestRateLimitByIP_BlocksWhenExceeded(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), RateLimitByIP(2, time.Minute, http.MethodPost))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://localhost/login", nil)
		req.RemoteAddr = "203.0.113.11:12345"
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		if i < 2 && rec.Code != http.StatusNoContent {
			t.Fatalf("request %d status=%d want=%d", i+1, rec.Code, http.StatusNoContent)
		}
		if i == 2 {
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("request %d status=%d want=%d", i+1, rec.Code, http.StatusTooManyRequests)
			}
			if rec.Header().Get("Retry-After") == "" {
				t.Fatal("expected Retry-After header when rate limit is exceeded")
			}
		}
	}
}

func TestRateLimitByIP_IgnoresNonConfiguredMethod(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), RateLimitByIP(1, time.Minute, http.MethodPost))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
		req.RemoteAddr = "203.0.113.12:12345"
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("request %d status=%d want=%d", i+1, rec.Code, http.StatusNoContent)
		}
	}
}

func TestRateLimitByIP_TracksPerIP(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), RateLimitByIP(1, time.Minute, http.MethodPost))

	reqA := httptest.NewRequest(http.MethodPost, "http://localhost/login", nil)
	reqA.RemoteAddr = "203.0.113.20:1111"
	recA := httptest.NewRecorder()
	h.ServeHTTP(recA, reqA)
	if recA.Code != http.StatusNoContent {
		t.Fatalf("first ip status=%d want=%d", recA.Code, http.StatusNoContent)
	}

	reqB := httptest.NewRequest(http.MethodPost, "http://localhost/login", nil)
	reqB.RemoteAddr = "203.0.113.21:2222"
	recB := httptest.NewRecorder()
	h.ServeHTTP(recB, reqB)
	if recB.Code != http.StatusNoContent {
		t.Fatalf("second ip status=%d want=%d", recB.Code, http.StatusNoContent)
	}
}
