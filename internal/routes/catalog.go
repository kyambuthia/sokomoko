package routes

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
	"github.com/kyambuthia/sokomoko/internal/ui"
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
		products, err := a.Catalog.Browse(r.Context(), catalogsvc.Filter{Limit: homeProductLimit})
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
		product, err := a.Catalog.ProductBySlug(r.Context(), slug)
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
			products, err = a.Catalog.SearchInCategory(r.Context(), query, category)
		case category != "":
			products, err = a.Catalog.Browse(r.Context(), catalogsvc.Filter{Category: category})
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

// CategoriesAPI serves GET /api/categories for the header department menus.
func CategoriesAPI(a *app.App) http.HandlerFunc {
	type categoryJSON struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
		URL  string `json:"url"`
		Icon string `json:"icon"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		categories, err := a.Catalog.Categories(r.Context())
		if err != nil {
			log.Printf("api categories: %v", err)
			app.JSON(w, http.StatusInternalServerError, map[string]string{"error": "categories unavailable"})
			return
		}
		out := make([]categoryJSON, 0, len(categories))
		for _, c := range categories {
			out = append(out, categoryJSON{
				Name: c.Name,
				Slug: c.Slug,
				URL:  "/search?category=" + urlQueryEscape(c.Slug),
				Icon: ui.CategoryIcon(c.Slug),
			})
		}
		w.Header().Set("Cache-Control", "public, max-age=60")
		app.JSON(w, http.StatusOK, map[string]any{"categories": out})
	}
}

func serverError(w http.ResponseWriter, r *http.Request) {
	app.RenderErrorPage(w, r, http.StatusInternalServerError, "Internal Server Error", "Something went wrong. Please try again.")
}
