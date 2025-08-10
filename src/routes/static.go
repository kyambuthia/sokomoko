package routes

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

func Static(staticFS embed.FS) http.Handler {
	staticSubFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	return http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS)))
}
