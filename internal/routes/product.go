package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
)

type ProductPageData struct {
	Title   string
	Product *catalogsvc.Product
}

func ProductDetail(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		slug := r.URL.Path[len("/products/"):]
		product, err := a.Catalog.ProductBySlug(slug)
		if err != nil {
			if err == catalogsvc.ErrInvalidProductSlug {
				NotFound(w, r)
				return
			}
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if product == nil {
			NotFound(w, r)
			return
		}

		a.Render(w, a.Templates.Product, productPage(product))
	}
}
