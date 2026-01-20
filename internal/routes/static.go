package routes

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func Static(staticFS embed.FS) http.Handler {
	// Try serving from local disk first (for dev/dynamic images)
	// then fall back to embedded FS
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/static/")
		localPath := filepath.Join("internal", "ui", "static", path)

		// Check if file exists on disk
		if _, err := os.Stat(localPath); err == nil {
			http.ServeFile(w, r, localPath)
			return
		}

		// Fallback to embedded FS
		staticSubFS, err := fs.Sub(staticFS, "static")
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS))).ServeHTTP(w, r)
	})
}
