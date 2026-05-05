package auth

import (
	"context"
	"crypto/subtle"
	"html/template"
	"log"
	"net/http"
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
			// No session cookie, so no CSRF protection needed
			next.ServeHTTP(w, r)
			return
		}

		// Retrieve session to get the CSRF token
		sess, err := s.store.GetSession(cookie.Value)
		if err != nil || sess == nil {
			// Invalid or expired session
			log.Printf("CSRF check: invalid session")
			http.Error(w, "Invalid session", http.StatusForbidden)
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
			http.Error(w, "CSRF token missing", http.StatusForbidden)
			return
		}

		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(expectedToken)) != 1 {
			log.Printf("CSRF check: token mismatch")
			http.Error(w, "CSRF token invalid", http.StatusForbidden)
			return
		}

		// Token is valid, add to context for template access
		ctx := context.WithValue(r.Context(), csrfTokenContextKey, expectedToken)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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

	sess, err := s.store.GetSession(cookie.Value)
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
