package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"log/slog"
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

type requestIDKey struct{}

// RequestID propagates a well-formed inbound X-Request-Id or generates one. IDs
// from clients are only accepted when short and alphanumeric so they cannot be
// used to forge log lines.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
			if !validRequestID(requestID) {
				requestID = generateRequestID()
			}
			w.Header().Set("X-Request-Id", requestID)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID)))
		})
	}
}

// RequestIDFromContext returns the request ID set by RequestID.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !isAlnum && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

// RequestLogger emits one structured access log line per request.
func RequestLogger() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			slog.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("host", r.Host),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", RequestIDFromContext(r.Context())),
			)
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
