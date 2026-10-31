package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// contextKey is a custom type for context keys to avoid collisions.
type csrfContextKey string

const csrfTokenContextKey csrfContextKey = "csrf_token"

// CSRFMiddleware validates CSRF tokens for state-changing requests (POST, PUT, PATCH, DELETE)
// for cookie-authenticated sessions. It exempts safe methods (GET, HEAD, OPTIONS, TRACE).
func (s *Service) CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt safe methods from CSRF protection
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// Only enforce CSRF for cookie-authenticated requests
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			// Anonymous forms have no session token yet. Reject browser requests
			// that identify a different origin, while leaving non-browser clients
			// without Origin or Referer headers usable.
			if !s.sameOriginRequest(r) {
				if isAPIRequest(r) {
					writeAPIJSONError(w, http.StatusForbidden, "csrf_origin_invalid", "Cross-origin request rejected")
					return
				}
				http.Error(w, "Cross-origin request rejected", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Retrieve session to get the CSRF token
		sess, err := s.store.GetSession(r.Context(), cookie.Value)
		if err != nil {
			log.Printf("CSRF check: session lookup failed: %v", err)
			if isAPIRequest(r) {
				writeAPIJSONError(w, http.StatusServiceUnavailable, "service_unavailable", "Service unavailable")
				return
			}
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		if sess == nil {
			if isAPIRequest(r) {
				writeAPIJSONError(w, http.StatusForbidden, "invalid_session", "Invalid session")
				return
			}
			// The cookie refers to an expired or revoked session (for example
			// after a password reset). Treat the request as anonymous rather
			// than locking the browser out of /login: drop the cookie from both
			// the response and this request so downstream handlers cannot
			// authenticate with it.
			s.clearSessionCookie(w, r)
			r = withoutSessionCookie(r)
			next.ServeHTTP(w, r)
			return
		}

		// Get CSRF token from request
		providedToken := strings.TrimSpace(r.FormValue("csrf_token"))
		if providedToken == "" {
			providedToken = strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
		}

		// Validate token using constant-time comparison
		expectedToken := strings.TrimSpace(sess.CSRFToken)
		if expectedToken == "" || providedToken == "" {
			log.Printf("CSRF check: missing token (expected: %t, provided: %t)", expectedToken != "", providedToken != "")
			if isAPIRequest(r) {
				writeAPIJSONError(w, http.StatusForbidden, "csrf_token_missing", "CSRF token missing")
				return
			}
			http.Error(w, "CSRF token missing", http.StatusForbidden)
			return
		}

		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(expectedToken)) != 1 {
			log.Printf("CSRF check: token mismatch")
			if isAPIRequest(r) {
				writeAPIJSONError(w, http.StatusForbidden, "csrf_token_invalid", "CSRF token invalid")
				return
			}
			http.Error(w, "CSRF token invalid", http.StatusForbidden)
			return
		}

		// Token is valid, add to context for template access
		ctx := context.WithValue(r.Context(), csrfTokenContextKey, expectedToken)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sameOriginRequest guards anonymous state-changing requests such as login
// and password-reset forms. Browser origin metadata is checked when present;
// clients that omit it remain compatible.
func (s *Service) sameOriginRequest(r *http.Request) bool {
	if r == nil {
		return false
	}

	fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	if fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none" {
		return false
	}

	var source *url.URL
	origins := r.Header.Values("Origin")
	if len(origins) > 1 {
		return false
	}
	if len(origins) == 1 && strings.TrimSpace(origins[0]) != "" {
		parsed, err := url.Parse(strings.TrimSpace(origins[0]))
		if err != nil || parsed.User != nil || parsed.Host == "" ||
			(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return false
		}
		source = parsed
	} else {
		referers := r.Header.Values("Referer")
		if len(referers) > 1 {
			return false
		}
		if len(referers) == 0 || strings.TrimSpace(referers[0]) == "" {
			return true
		}
		parsed, err := url.Parse(strings.TrimSpace(referers[0]))
		if err != nil || !parsed.IsAbs() || parsed.User != nil || parsed.Host == "" {
			return false
		}
		source = parsed
	}

	expectedScheme := requestScheme(r, s.trustProxyHeaders)
	if !strings.EqualFold(source.Scheme, expectedScheme) {
		return false
	}

	sourceHost, sourcePort := originHostAndPort(source.Scheme, source.Host)
	requestHost, requestPort := originHostAndPort(expectedScheme, r.Host)
	return sourceHost != "" && requestHost != "" &&
		sourceHost == requestHost && sourcePort == requestPort
}

func originHostAndPort(scheme, host string) (string, string) {
	parsed := url.URL{Host: host}
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if port == "" {
		switch strings.ToLower(scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	if hostname == "" || port == "" {
		return "", ""
	}
	return hostname, port
}

func isAPIRequest(r *http.Request) bool {
	return r != nil && (r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/"))
}

func writeAPIJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErrorEnvelope{
		Error: apiErrorBody{Code: code, Message: message},
	})
}

type apiErrorEnvelope struct {
	Error apiErrorBody `json:"error"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// isSafeMethod checks if the HTTP method is safe (doesn't change state)
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// CSRFTokenFromContext retrieves the CSRF token from the request context
func CSRFTokenFromContext(ctx context.Context) string {
	token, ok := ctx.Value(csrfTokenContextKey).(string)
	if !ok {
		return ""
	}
	return token
}

// CSRFTokenForSession retrieves the CSRF token for an authenticated session
func (s *Service) CSRFTokenForSession(r *http.Request) string {
	// First check context (set by middleware)
	if token := CSRFTokenFromContext(r.Context()); token != "" {
		return token
	}

	// Fall back to retrieving from session cookie
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}

	sess, err := s.store.GetSession(r.Context(), cookie.Value)
	if err != nil || sess == nil {
		return ""
	}

	return sess.CSRFToken
}

// CSRFTokenInput returns a template.HTML safe hidden input field with the CSRF token
func (s *Service) CSRFTokenInput(r *http.Request) template.HTML {
	token := s.CSRFTokenForSession(r)
	if token == "" {
		return ""
	}
	return template.HTML(`<input type="hidden" name="csrf_token" value="` + template.HTMLEscapeString(token) + `">`)
}

// CSRFToken returns the CSRF token string for use in templates
func (s *Service) CSRFToken(r *http.Request) string {
	return s.CSRFTokenForSession(r)
}

// withoutSessionCookie returns a shallow copy of r whose Cookie header omits the
// session cookie.
func withoutSessionCookie(r *http.Request) *http.Request {
	r2 := r.Clone(r.Context())
	r2.Header.Del("Cookie")
	for _, c := range r.Cookies() {
		if c.Name != sessionCookieName {
			r2.AddCookie(c)
		}
	}
	return r2
}
