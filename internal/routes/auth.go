package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
)

func Auth(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.Render(w, a.Templates.Account, "authentication route")
	}
}
