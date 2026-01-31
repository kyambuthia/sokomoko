package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
)

func Auth(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		a.Render(w, a.Templates.Account, "authentication route")
	}
}