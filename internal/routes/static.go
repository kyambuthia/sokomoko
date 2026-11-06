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

const splatViewerCSP = "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval'; connect-src 'self'; worker-src 'self' blob:; base-uri 'self'; form-action 'self'; frame-ancestors 'self'"

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
		if cleanAssetPath == "splat-viewer.html" {
			// The self-hosted viewer ships inline bootstrap scripts and is loaded
			// only inside the same-origin product media frame.
			w.Header().Set("Content-Security-Policy", splatViewerCSP)
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if strings.HasSuffix(cleanAssetPath, ".sog") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
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
