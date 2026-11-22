package routes

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kyambuthia/sokomoko/internal/app"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

type stubRoutesCatalog struct {
	app.CatalogService
	searchPageFn func(context.Context, string, string, int, int) ([]catalogsvc.Product, int, error)
}

func (s *stubRoutesCatalog) SearchPage(ctx context.Context, query, category string, page, limit int) ([]catalogsvc.Product, int, error) {
	if s.searchPageFn != nil {
		return s.searchPageFn(ctx, query, category, page, limit)
	}
	return nil, 0, nil
}

type stubSearchAuth struct {
	app.AuthService
}

func (s *stubSearchAuth) CSRFToken(*http.Request) string { return "csrf-test" }

func TestSearch_SanitizesNULAndInvalidUTF8(t *testing.T) {
	searchTmpl := template.Must(template.New("root_template").Parse("q={{.Query}} cat={{.Category}}"))

	for _, tc := range []struct {
		name     string
		rawURL   string
		wantCall bool
		wantQ    string
		wantCat  string
	}{
		{name: "nul only", rawURL: "/search?q=%00", wantCall: false},
		{name: "invalid utf8 ff", rawURL: "/search?q=%FF", wantCall: false},
		{name: "nul suffix", rawURL: "/search?q=lamp%00", wantCall: true, wantQ: "lamp"},
		{name: "invalid utf8 sequence", rawURL: "/search?q=%C3%28", wantCall: true, wantQ: "("},
		{name: "nul in category", rawURL: "/search?category=home%00", wantCall: true, wantCat: "home"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			a := &app.App{
				Catalog: &stubRoutesCatalog{
					searchPageFn: func(_ context.Context, q, cat string, _, _ int) ([]catalogsvc.Product, int, error) {
						called = true
						if !utf8.ValidString(q) || strings.ContainsRune(q, '\x00') {
							t.Fatalf("unsanitized query reached DB: %q", q)
						}
						if !utf8.ValidString(cat) || strings.ContainsRune(cat, '\x00') {
							t.Fatalf("unsanitized category reached DB: %q", cat)
						}
						if q != tc.wantQ || cat != tc.wantCat {
							t.Fatalf("got q=%q cat=%q, want q=%q cat=%q", q, cat, tc.wantQ, tc.wantCat)
						}
						return nil, 0, nil
					},
				},
				Auth:      &stubSearchAuth{},
				Templates: &ui.Templates{Search: searchTmpl},
			}
			rec := httptest.NewRecorder()
			Search(a).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.rawURL, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; want 200", rec.Code)
			}
			if called != tc.wantCall {
				t.Fatalf("DB called = %v; want %v", called, tc.wantCall)
			}
		})
	}
}
