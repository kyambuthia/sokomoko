package routes

import (
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type SearchFormValues struct {
	QueryString string `json:"queryString"`
}

func Search(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		// HANDLE GET REQUESTS -  /search ROUTE
		case "GET":
			query := req.URL.Query().Get("q")
			if query != "" {
				// Perform search
				products, err := db.SearchProducts(strings.TrimSpace(query))
				if err != nil {
					log.Printf("Error searching products: %v", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}

				// Build template data
				data := map[string]interface{}{
					"Query":     query,
					"Products":  products,
					"NoResults": len(products) == 0,
				}

				err = tmpl.ExecuteTemplate(w, "root_template", data)
				if err != nil {
					log.Fatal(err)
				}
			} else {
				data := map[string]interface{}{
					"Query":     "",
					"Products":  []db.Product{},
					"NoResults": false,
				}
				err := tmpl.ExecuteTemplate(w, "root_template", data)
				if err != nil {
					log.Fatal(err)
				}
			}

		// HANDLE POST REQUESTS - /search ROUTE
		case "POST":
			reqBody, err := io.ReadAll(req.Body)
			if err != nil {
				log.Printf("Unable to read request body: %v", err)
				http.Error(w, "Unable to read request body", http.StatusBadRequest)
				return
			}

			// read form data
			var formData SearchFormValues
			err = json.Unmarshal(reqBody, &formData)
			if err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}

			// Perform search
			products, err := db.SearchProducts(strings.TrimSpace(formData.QueryString))
			if err != nil {
				log.Printf("Error searching products: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			// Return JSON
			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(products)
			if err != nil {
				log.Printf("Error encoding products: %v", err)
			}
		}
	}
}
