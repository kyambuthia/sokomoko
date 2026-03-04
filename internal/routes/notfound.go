package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
)

func NotFound(w http.ResponseWriter, r *http.Request) {
	app.RenderErrorPage(w, r, http.StatusNotFound, "Page Not Found", "The page you requested could not be found.")
}
