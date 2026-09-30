package routes

import (
	"log"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
)

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

				a.Render(w, a.Templates.Search, searchPage(query, products, a.Auth.CSRFToken(req)))
			} else {
				a.Render(w, a.Templates.Search, searchPage("", nil, a.Auth.CSRFToken(req)))
			}

		case "POST":
			w.Header().Set("Deprecation", "true")
			w.Header().Set("Link", "</api/v1/catalog/search>; rel=successor-version")
			http.Error(w, "Search JSON moved to GET /api/v1/catalog/search?q=...", http.StatusGone)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
