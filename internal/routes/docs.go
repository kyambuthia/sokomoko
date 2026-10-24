package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
)

// RegisterDocs wires the static public feature guide on the docs host.
func RegisterDocs(a *app.App, mux *http.ServeMux) {
	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.Handle("/static/", Static(a.StaticFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		content, err := a.StaticFS.ReadFile("static/docs/index.html")
		if err != nil {
			http.Error(w, "Documentation is unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodGet {
			_, _ = w.Write(content)
		}
	})
}
