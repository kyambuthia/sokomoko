package routes

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/db"
)

type SearchFormValues struct {
	QueryString string `json:"queryString"`
}

func Search(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		// HANDLE GET REQUESTS -  /search ROUTE
		case "GET":
			query := req.URL.Query().Get("q")
			if query != "" {
				// Perform search
				products, err := a.Store.SearchProducts(strings.TrimSpace(query))
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
				a.Render(w, a.Templates.Search, data)
			} else {
				data := map[string]interface{}{
					"Query":     "",
					"Products":  []db.Product{},
					"NoResults": false,
				}
				a.Render(w, a.Templates.Search, data)
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
			products, err := a.Store.SearchProducts(strings.TrimSpace(formData.QueryString))
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
