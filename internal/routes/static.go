package routes

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
)

func Static(staticFS embed.FS) http.Handler {
	staticSubFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			app.RenderErrorPage(w, r, http.StatusInternalServerError, "Internal Server Error", "Static assets are unavailable right now.")
		})
	}
	embeddedHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS)))
	localBase := filepath.Clean(filepath.Join("internal", "ui", "static"))

	// Try serving from local disk first (for dev/dynamic images),
	// then fall back to embedded FS.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assetPath := strings.TrimPrefix(r.URL.Path, "/static/")
		if assetPath == "" {
			NotFound(w, r)
			return
		}
		for _, part := range strings.Split(assetPath, "/") {
			if part == ".." {
				NotFound(w, r)
				return
			}
		}

		cleanAssetPath := strings.TrimPrefix(path.Clean("/"+assetPath), "/")
		localPath := filepath.Clean(filepath.Join(localBase, filepath.FromSlash(cleanAssetPath)))
		if localPath != localBase && !strings.HasPrefix(localPath, localBase+string(filepath.Separator)) {
			NotFound(w, r)
			return
		}

		// Check if file exists on disk.
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, localPath)
			return
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = "/static/" + cleanAssetPath
		embeddedHandler.ServeHTTP(w, r2)
	})
}
