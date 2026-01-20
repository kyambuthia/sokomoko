package routes

import (
	"html/template"
	"log"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func Root(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		products, err := db.GetAllProducts()
		if err != nil {
			log.Printf("Error fetching products: %v", err)
			// Proceed with empty products or handle error
		}

		data := struct {
			HasProducts bool
			Products    []db.Product
		}{
			HasProducts: len(products) > 0,
			Products:    products,
		}

		err = tmpl.ExecuteTemplate(w, "root_template", data)
		if err != nil {
			log.Printf("Template execution error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}
