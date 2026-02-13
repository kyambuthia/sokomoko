package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/db"
)

func Root(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		products, err := a.Store.GetAllProducts()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := struct {
			HasProducts bool
			Products    []db.Product
		}{
			HasProducts: len(products) > 0,
			Products:    products,
		}
		a.Render(w, a.Templates.Index, data)
	}
}
