package routes

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

const splatViewerCSP = "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval'; connect-src 'self'; worker-src 'self' blob:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'self'"

type staticResponseWriter struct {
	http.ResponseWriter
	immutable   bool
	wroteHeader bool
}

func (w *staticResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if code >= 400 {
			w.Header().Set("Cache-Control", "no-store")
		} else if w.immutable && code == http.StatusOK {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *staticResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *staticResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func Favicon() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		http.Redirect(w, r, ui.AssetURL("icons/sokomoko.svg"), http.StatusFound)
	}
}

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
		immutable := strings.TrimSpace(r.URL.Query().Get("v")) != ""
		if cleanAssetPath == "splat-viewer.html" {
			// The self-hosted viewer ships inline bootstrap scripts and is loaded
			// only inside the same-origin product media frame.
			w.Header().Set("Content-Security-Policy", splatViewerCSP)
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			immutable = true
		} else if strings.HasSuffix(cleanAssetPath, ".sog") {
			immutable = true
		}

		sw := &staticResponseWriter{ResponseWriter: w, immutable: immutable}
		localPath := filepath.Clean(filepath.Join(localBase, filepath.FromSlash(cleanAssetPath)))
		if localPath != localBase && !strings.HasPrefix(localPath, localBase+string(filepath.Separator)) {
			NotFound(w, r)
			return
		}

		if cleanAssetPath == "manifest.json" {
			serveManifest(sw, r, localPath, staticSubFS)
			return
		}

		// Check if file exists on disk.
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			http.ServeFile(sw, r, localPath)
			return
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = "/static/" + cleanAssetPath
		embeddedHandler.ServeHTTP(sw, r2)
	})
}

func serveManifest(w http.ResponseWriter, r *http.Request, localPath string, staticSubFS fs.FS) {
	var raw []byte
	var modTime time.Time
	if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
		if data, err := os.ReadFile(localPath); err == nil {
			raw = data
			modTime = info.ModTime()
		}
	}
	if raw == nil {
		data, err := fs.ReadFile(staticSubFS, "manifest.json")
		if err != nil {
			NotFound(w, r)
			return
		}
		raw = data
	}
	versioned := strings.ReplaceAll(string(raw), `"/static/icons/sokomoko.svg"`, `"`+ui.AssetURL("icons/sokomoko.svg")+`"`)
	w.Header().Set("Content-Type", "application/manifest+json")
	http.ServeContent(w, r, "manifest.json", modTime, bytes.NewReader([]byte(versioned)))
}
