package routes

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
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
				products, err := a.Catalog.Search(query)
				if err != nil {
					log.Printf("Error searching products: %v", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}

				a.Render(w, a.Templates.Search, searchPage(query, products))
			} else {
				a.Render(w, a.Templates.Search, searchPage("", nil))
			}

		// HANDLE POST REQUESTS - /search ROUTE
		case "POST":
			reqBody, err := io.ReadAll(req.Body)
			if err != nil {
				log.Printf("Unable to read request body: %v", err)
				http.Error(w, "Unable to read request body", http.StatusBadRequest)
				return
			}

			var formData SearchFormValues
			err = json.Unmarshal(reqBody, &formData)
			if err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}

			products, err := a.Catalog.Search(formData.QueryString)
			if err != nil {
				log.Printf("Error searching products: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(products)
			if err != nil {
				log.Printf("Error encoding products: %v", err)
			}
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
