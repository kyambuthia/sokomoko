package routes

import (
	"net/http"
)

func NotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	// You can also render a custom 404 page here
	w.Write([]byte("404 - Page Not Found"))
}
