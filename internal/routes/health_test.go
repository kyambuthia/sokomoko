package routes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
)

type stubReadiness struct {
	ping func(context.Context) error
}

func (s *stubReadiness) PingContext(ctx context.Context) error {
	if s.ping != nil {
		return s.ping(ctx)
	}
	return nil
}

func TestHealth_AllowsGetAndHead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method   string
		wantBody string
	}{
		{method: http.MethodGet, wantBody: "ok"},
		{method: http.MethodHead, wantBody: ""},
	}

	handler := Health()
	for _, tc := range tests {
		tc := tc
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tc.method, "/healthz", nil)
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
				t.Fatalf("content type = %q, want text/plain; charset=utf-8", got)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Fatalf("body = %q, want %q", got, tc.wantBody)
			}
		})
	}
}

func TestReady_AllowsGetAndHead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method   string
		wantBody string
	}{
		{method: http.MethodGet, wantBody: "ready"},
		{method: http.MethodHead, wantBody: ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()

			readiness := &stubReadiness{}
			handler := Ready(&app.App{Readiness: readiness})
			req := httptest.NewRequest(tc.method, "/readyz", nil)
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
				t.Fatalf("content type = %q, want text/plain; charset=utf-8", got)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Fatalf("body = %q, want %q", got, tc.wantBody)
			}
		})
	}
}

func TestReady_ReturnsUnavailableWhenReadinessFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method   string
		wantBody string
	}{
		{method: http.MethodGet, wantBody: "database unavailable"},
		{method: http.MethodHead, wantBody: ""},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()

			handler := Ready(&app.App{
				Readiness: &stubReadiness{
					ping: func(context.Context) error {
						return errors.New("db down")
					},
				},
			})
			req := httptest.NewRequest(tc.method, "/readyz", nil)
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
			}
			if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
				t.Fatalf("content type = %q, want text/plain; charset=utf-8", got)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Fatalf("body = %q, want %q", got, tc.wantBody)
			}
		})
	}
}
