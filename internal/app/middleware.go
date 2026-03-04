package app

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Middleware func(http.Handler) http.Handler

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
			if requestID == "" {
				requestID = generateRequestID()
			}
			w.Header().Set("X-Request-Id", requestID)
			next.ServeHTTP(w, r)
		})
	}
}

func RequestLogger() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.Printf("%s %s host=%s status=%d duration=%s", r.Method, r.URL.Path, r.Host, rec.status, time.Since(start).Round(time.Millisecond))
		})
	}
}

func Recoverer() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("panic recovered: %v", rec)
					RenderErrorPage(w, r, http.StatusInternalServerError, "Internal Server Error", "Something went wrong while processing your request.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func SecurityHeaders() Middleware {
	csp := "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Content-Security-Policy", csp)
			if isHTTPS(r) {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func BodyLimit(maxBytes int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func CSRFSameOrigin(sessionCookieName string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			if isCSRFBypassPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if _, err := r.Cookie(sessionCookieName); err != nil {
				next.ServeHTTP(w, r)
				return
			}

			origin := strings.TrimSpace(r.Header.Get("Origin"))
			referer := strings.TrimSpace(r.Header.Get("Referer"))

			if origin == "" && referer == "" {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			if origin != "" && !sameHost(origin, r) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			if origin == "" && referer != "" && !sameHost(referer, r) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isCSRFBypassPath(path string) bool {
	switch strings.TrimSpace(path) {
	case "/login", "/signup", "/password-reset/request", "/password-reset/confirm":
		return true
	default:
		return false
	}
}

func sameHost(raw string, r *http.Request) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Host == "" {
		return false
	}

	requestScheme := "http"
	if isHTTPS(r) {
		requestScheme = "https"
	}

	candidateScheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if candidateScheme == "" {
		candidateScheme = requestScheme
	}
	if candidateScheme != requestScheme {
		return false
	}

	requestHost, requestPort := hostAndPort(r.Host, requestScheme)
	if requestHost == "" {
		return false
	}

	candidateHost := strings.ToLower(strings.TrimSpace(u.Hostname()))
	candidatePort := normalizePort(u.Port(), candidateScheme)

	return candidateHost == requestHost && candidatePort == requestPort
}

func hostAndPort(rawHost, scheme string) (string, string) {
	parsed, err := url.Parse("http://" + strings.TrimSpace(rawHost))
	if err != nil {
		return "", normalizePort("", scheme)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	return host, normalizePort(parsed.Port(), scheme)
}

func normalizePort(port, scheme string) string {
	cleanScheme := strings.ToLower(strings.TrimSpace(scheme))
	if cleanScheme != "https" {
		cleanScheme = "http"
	}
	if strings.TrimSpace(port) != "" {
		return strings.TrimSpace(port)
	}
	if cleanScheme == "https" {
		return "443"
	}
	return "80"
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func generateRequestID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}

func canonicalHost(host string) string {
	if idx := strings.Index(host, ":"); idx >= 0 {
		return strings.ToLower(strings.TrimSpace(host[:idx]))
	}
	return strings.ToLower(strings.TrimSpace(host))
}

func CanonicalHost(host string) string {
	return canonicalHost(host)
}
