package routes

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
	directorysvc "github.com/kyambuthia/sokomoko/internal/service/directory"
)

type sitemapCatalogStub struct {
	app.CatalogService
	products []catalogsvc.Product
}

func (s *sitemapCatalogStub) AllProducts(context.Context) ([]catalogsvc.Product, error) {
	return s.products, nil
}

type sitemapDirectoryStub struct {
	app.DirectoryService
	listings []directorysvc.ServiceListing
}

func (s *sitemapDirectoryStub) Browse(context.Context, directorysvc.ServiceFilter, int) ([]directorysvc.ServiceListing, int, error) {
	return s.listings, len(s.listings), nil
}

func TestSitemap_ReturnsValidXML(t *testing.T) {
	a := app.New(app.Dependencies{
		PublicBaseURL: "https://asiliano.online",
		Catalog: &sitemapCatalogStub{
			products: []catalogsvc.Product{{Slug: "ceramic-vase"}},
		},
		Directory: &sitemapDirectoryStub{
			listings: []directorysvc.ServiceListing{{Slug: "demo-haircut"}},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec := httptest.NewRecorder()
	Sitemap(a).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/xml") {
		t.Fatalf("Content-Type = %q, want application/xml", got)
	}

	var parsed sitemapURLSet
	if err := xml.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("xml.Unmarshal: %v\nbody:\n%s", err, rec.Body.String())
	}
	if parsed.Xmlns != "http://www.sitemaps.org/schemas/sitemap/0.9" {
		t.Fatalf("xmlns = %q", parsed.Xmlns)
	}

	locs := make(map[string]bool, len(parsed.URLs))
	for _, u := range parsed.URLs {
		locs[u.Loc] = true
	}
	for _, want := range []string{
		"https://asiliano.online/",
		"https://asiliano.online/services",
		"https://asiliano.online/search",
		"https://asiliano.online/products/ceramic-vase",
		"https://asiliano.online/services/demo-haircut",
	} {
		if !locs[want] {
			t.Fatalf("missing %q in sitemap locs: %v", want, locs)
		}
	}
}
