package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
)

func Root(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if req.URL.Path != "/" {
			NotFound(w, req)
			return
		}

		products, err := a.Catalog.AllProducts()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		a.Render(w, a.Templates.Index, indexPage(products))
	}
}
