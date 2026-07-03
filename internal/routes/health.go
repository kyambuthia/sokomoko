package routes

import (
	"context"
	"net/http"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
)

func Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writePlainText(w, r, http.StatusOK, "ok")
	}
}

func Ready(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.Readiness.PingContext(ctx); err != nil {
			writePlainText(w, r, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		writePlainText(w, r, http.StatusOK, "ready")
	}
}

func writePlainText(w http.ResponseWriter, r *http.Request, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(body))
}
