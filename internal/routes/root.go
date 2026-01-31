package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/db"
)

func Root(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		products, _ := a.Store.GetAllProducts() // Proceed with empty products or handle error

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
