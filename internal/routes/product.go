package routes

import (
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/db"
)

type ProductPageData struct {
	Title   string
	Product *db.Product
}

func ProductDetail(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		slug := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/products/"))
		if slug == "" || strings.Contains(slug, "/") {
			NotFound(w, r)
			return
		}

		product, err := a.Store.GetProductBySlug(slug)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if product == nil {
			NotFound(w, r)
			return
		}

		a.Render(w, a.Templates.Product, ProductPageData{
			Title:   product.Name,
			Product: product,
		})
	}
}
