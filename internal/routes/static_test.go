package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	if !strings.Contains(rec.Body.String(), "{") {
		t.Fatal("expected manifest content in response body")
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
	if strings.Contains(rec.Body.String(), "module ") {
		t.Fatal("path traversal leaked go.mod content")
	}
}
