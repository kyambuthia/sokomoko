package routes

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
)

const homeProductLimit = 48

type IndexPageData struct {
	Title       string
	HasProducts bool
	Products    []catalogsvc.Product
	Viewer      Viewer
	CSRFToken   string
}

type ProductPageData struct {
	Title       string
	Product     *catalogsvc.Product
	Breadcrumbs []Breadcrumb
	Viewer      Viewer
	CSRFToken   string
}

type Breadcrumb struct {
	Label string `json:"label"`
	Href  string `json:"href,omitempty"`
}

type SearchPageData struct {
	Title     string
	Query     string
	Category  string
	Products  []catalogsvc.Product
	NoResults bool
	Viewer    Viewer
	CSRFToken string
}

// Root serves the storefront home page.
func Root(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		products, err := a.Catalog.Browse(catalogsvc.Filter{Limit: homeProductLimit})
		if err != nil {
			log.Printf("home: list products: %v", err)
			serverError(w, r)
			return
		}
		a.Render(w, a.Templates.Index, IndexPageData{
			Title:       "Home",
			HasProducts: len(products) > 0,
			Products:    products,
			Viewer:      viewerFromRequest(r),
			CSRFToken:   a.Auth.CSRFToken(r),
		})
	}
}

// ProductDetail serves /products/{slug}.
func ProductDetail(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		slug := strings.TrimPrefix(r.URL.Path, "/products/")
		product, err := a.Catalog.ProductBySlug(slug)
		if errors.Is(err, catalogsvc.ErrInvalidProductSlug) {
			NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("product %q: %v", slug, err)
			serverError(w, r)
			return
		}
		if product == nil {
			NotFound(w, r)
			return
		}
		crumbs := []Breadcrumb{{Label: "Home", Href: "/"}}
		if product.Category != "" {
			crumbs = append(crumbs, Breadcrumb{Label: product.Category, Href: "/search?category=" + urlQueryEscape(product.Category)})
		}
		crumbs = append(crumbs, Breadcrumb{Label: product.Name})

		a.Render(w, a.Templates.Product, ProductPageData{
			Title:       product.Name,
			Product:     product,
			Breadcrumbs: crumbs,
			Viewer:      viewerFromRequest(r),
			CSRFToken:   a.Auth.CSRFToken(r),
		})
	}
}

// Search renders the search page. A category without a query lists the whole
// category.
func Search(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		if strings.EqualFold(category, catalogsvc.AllCategories) {
			category = ""
		}

		var (
			products []catalogsvc.Product
			err      error
		)
		switch {
		case query != "":
			products, err = a.Catalog.SearchInCategory(query, category)
		case category != "":
			products, err = a.Catalog.Browse(catalogsvc.Filter{Category: category})
		}
		if err != nil {
			log.Printf("search %q: %v", query, err)
			serverError(w, r)
			return
		}

		a.Render(w, a.Templates.Search, SearchPageData{
			Title:     "Search",
			Query:     query,
			Category:  category,
			Products:  products,
			NoResults: len(products) == 0 && (query != "" || category != ""),
			Viewer:    viewerFromRequest(r),
			CSRFToken: a.Auth.CSRFToken(r),
		})
	}
}

type searchResultJSON struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	URL      string `json:"url"`
	Price    string `json:"price"`
	Currency string `json:"currency"`
	InStock  bool   `json:"inStock"`
	ImageURL string `json:"imageUrl"`
	Category string `json:"category,omitempty"`
}

// SearchAPI serves GET /api/search?q=&category= for the header quick-find.
// It is a safe GET so it needs no CSRF token and can be cached briefly.
func SearchAPI(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		results := []searchResultJSON{}
		if len([]rune(query)) >= 2 {
			products, err := a.Catalog.SearchInCategory(query, r.URL.Query().Get("category"))
			if err != nil {
				log.Printf("api search %q: %v", query, err)
				app.JSON(w, http.StatusInternalServerError, map[string]string{"error": "search unavailable"})
				return
			}
			for i, p := range products {
				if i == 8 {
					break
				}
				results = append(results, searchResultJSON{
					ID: p.ID, Name: p.Name, Slug: p.Slug, URL: "/products/" + p.Slug,
					Price: p.Price.String(), Currency: "USD", InStock: p.InStock(),
					ImageURL: p.ImageURL, Category: p.Category,
				})
			}
		}
		app.JSON(w, http.StatusOK, map[string]any{"query": query, "results": results})
	}
}

// CategoriesAPI serves GET /api/categories for the header department menus.
func CategoriesAPI(a *app.App) http.HandlerFunc {
	type categoryJSON struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
		URL  string `json:"url"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		categories, err := a.Catalog.Categories()
		if err != nil {
			log.Printf("api categories: %v", err)
			app.JSON(w, http.StatusInternalServerError, map[string]string{"error": "categories unavailable"})
			return
		}
		out := make([]categoryJSON, 0, len(categories))
		for _, c := range categories {
			out = append(out, categoryJSON{Name: c.Name, Slug: c.Slug, URL: "/search?category=" + urlQueryEscape(c.Slug)})
		}
		w.Header().Set("Cache-Control", "public, max-age=60")
		app.JSON(w, http.StatusOK, map[string]any{"categories": out})
	}
}

func serverError(w http.ResponseWriter, r *http.Request) {
	app.RenderErrorPage(w, r, http.StatusInternalServerError, "Internal Server Error", "Something went wrong. Please try again.")
}
