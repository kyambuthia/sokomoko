package routes

import (
	"encoding/xml"
	"net/http"
	"net/url"

	"github.com/kyambuthia/sokomoko/internal/app"
	directorysvc "github.com/kyambuthia/sokomoko/internal/service/directory"
)

type sitemapURL struct {
	Loc string `xml:"loc"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// Sitemap serves an XML sitemap of public storefront pages, products, and services.
func Sitemap(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}

		base := a.PublicBase(r)
		if base == "" && r.Host != "" {
			scheme := "http"
			if r.TLS != nil || a.Production {
				scheme = "https"
			}
			base = scheme + "://" + r.Host
		}

		urls := []sitemapURL{
			{Loc: base + "/"},
			{Loc: base + "/services"},
			{Loc: base + "/search"},
		}
		seen := map[string]struct{}{
			base + "/":         {},
			base + "/services": {},
			base + "/search":   {},
		}
		addURL := func(path string) {
			loc := base + path
			if _, ok := seen[loc]; ok {
				return
			}
			seen[loc] = struct{}{}
			urls = append(urls, sitemapURL{Loc: loc})
		}

		if a.Catalog != nil {
			if products, err := a.Catalog.AllProducts(r.Context()); err == nil {
				for _, p := range products {
					if p.Slug != "" {
						addURL("/products/" + url.PathEscape(p.Slug))
					}
				}
			}
		}

		if a.Directory != nil {
			if listings, _, err := a.Directory.Browse(r.Context(), directorysvc.ServiceFilter{Limit: 100}, 1); err == nil {
				for _, l := range listings {
					if l.Slug != "" {
						addURL("/services/" + url.PathEscape(l.Slug))
					}
				}
			}
		}

		payload, err := xml.MarshalIndent(sitemapURLSet{
			Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
			URLs:  urls,
		}, "", "  ")
		if err != nil {
			app.RenderErrorPage(w, r, http.StatusInternalServerError, "Internal Server Error", "Unable to build sitemap.")
			return
		}

		body := append([]byte(xml.Header), payload...)
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	}
}
