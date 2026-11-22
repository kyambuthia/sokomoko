package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

func TestStatic_ServesEmbeddedAsset(t *testing.T) {
	handler := Static(ui.StaticFS)
	req := httptest.NewRequest(http.MethodGet, "/static/manifest.json", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/manifest+json" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/manifest+json")
	}
	if !strings.Contains(rec.Body.String(), "{") {
		t.Fatal("expected manifest content in response body")
	}
	if !strings.Contains(rec.Body.String(), "/static/icons/sokomoko.svg?v=") {
		t.Fatalf("expected versioned icon src in manifest body, got %s", rec.Body.String())
	}
}

func TestStatic_VersionedAssetsAreImmutable(t *testing.T) {
	handler := app.SecurityHeadersWithProxy(false)(Static(ui.StaticFS))

	t.Run("versioned asset gets immutable cache-control", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static/icons/sokomoko.svg?v=123", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Fatalf("Cache-Control = %q, want %q", got, "public, max-age=31536000, immutable")
		}
	})

	t.Run("missing versioned asset gets 404 with no-store", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static/does-not-exist.js?v=123", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
		}
	})
}

func TestFavicon_RedirectsToVersionedSVG(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()

	Favicon().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); !strings.HasPrefix(got, "/static/icons/sokomoko.svg?v=") {
		t.Fatalf("Location = %q, want prefix /static/icons/sokomoko.svg?v=", got)
	}
}

func TestStatic_BlocksPathTraversal(t *testing.T) {
	handler := Static(ui.StaticFS)
	req := httptest.NewRequest(http.MethodGet, "/static/../../../go.mod", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
	}
	if strings.Contains(rec.Body.String(), "module ") {
		t.Fatal("path traversal leaked go.mod content")
	}
}
