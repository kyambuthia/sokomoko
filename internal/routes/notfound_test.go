package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotFound_RendersStructuredPageWithRequestID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	rec.Header().Set("X-Request-Id", "req-404-test")

	NotFound(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("content type = %q, want text/html", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Page Not Found") {
		t.Fatal("expected page not found heading")
	}
	if !strings.Contains(body, "req-404-test") {
		t.Fatal("expected request id in not found page body")
	}
}
