package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
)

func TestRobotsTxt(t *testing.T) {
	a := app.New(app.Dependencies{PublicBaseURL: "https://asiliano.online"})

	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()
	RobotsTxt(a).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"User-agent: *",
		"Allow: /api/v1/openapi.json",
		"Disallow: /api/v1/checkout",
		"Sitemap: https://asiliano.online/sitemap.xml",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("robots.txt missing %q:\n%s", want, body)
		}
	}
}

func TestLLMsTxt(t *testing.T) {
	a := app.New(app.Dependencies{PublicBaseURL: "https://asiliano.online"})

	req := httptest.NewRequest(http.MethodGet, "/llms.txt", nil)
	rec := httptest.NewRecorder()
	LLMsTxt(a).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"https://asiliano.online/api/v1/openapi.json",
		"https://asiliano.online/sitemap.xml",
		"Authorization: Bearer smk_",
		"POST /api/v1/checkout/{token}/place-order",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("llms.txt missing %q:\n%s", want, body)
		}
	}
}

func TestDiscoveryMethodNotAllowed(t *testing.T) {
	a := app.New(app.Dependencies{})
	for _, tc := range []struct {
		path string
		h    http.HandlerFunc
	}{
		{"/robots.txt", RobotsTxt(a)},
		{"/llms.txt", LLMsTxt(a)},
	} {
		rec := httptest.NewRecorder()
		tc.h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s POST status = %d, want 405", tc.path, rec.Code)
		}
	}
}
