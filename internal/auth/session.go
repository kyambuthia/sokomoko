package auth

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func (s *Service) shouldUseSecureCookies(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if s.trustProxyHeaders && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return s.environment == "production"
}

// startSession issues a fresh session, revoking any session the request
// already carried so a pre-login session ID can never be promoted (session
// fixation).
func (s *Service) startSession(w http.ResponseWriter, r *http.Request, userID int) error {
	if existing, err := r.Cookie(sessionCookieName); err == nil && existing.Value != "" {
		if err := s.store.DeleteSession(r.Context(), existing.Value); err != nil {
			log.Printf("sign-in: revoke previous session: %v", err)
		}
	}
	sessionToken, err := generateOpaqueToken(32)
	if err != nil {
		return err
	}
	csrfToken, err := generateOpaqueToken(32)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(sessionDuration)

	err = s.store.CreateSession(r.Context(), db.Session{
		ID:        sessionToken,
		UserID:    userID,
		CSRFToken: csrfToken,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Expires:  expiresAt,
		MaxAge:   int(sessionDuration.Seconds()),
		Domain:   s.sessionCookieDomain,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.shouldUseSecureCookies(r),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Logout revokes the session and clears the cookie.
func (s *Service) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			// Revoke the session even if the browser stops waiting; clearing
			// the cookie alone would leave a copied token valid.
			if err := s.store.DeleteSession(context.WithoutCancel(r.Context()), cookie.Value); err != nil {
				log.Printf("logout: revoke session: %v", err)
			}
		}
		s.clearSessionCookie(w, r)
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (s *Service) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		Domain:   s.sessionCookieDomain,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.shouldUseSecureCookies(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const userContextKey contextKey = "user"

// WantsJSON reports whether the client asked for a JSON response, as the
// storefront's web components do.
func WantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

// AuthMiddleware requires a valid session. Browsers are redirected to /login
// with a next parameter; JSON clients get a 401.
func (s *Service) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())
		if user == nil {
			user = s.sessionUser(r)
		}
		if user == nil {
			if _, err := r.Cookie(sessionCookieName); err == nil {
				s.clearSessionCookie(w, r)
			}
			if WantsJSON(r) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"authentication required","login":"/login"}`))
				return
			}
			target := "/login"
			if r.Method == http.MethodGet && r.URL.Path != "/" {
				target += "?next=" + url.QueryEscape(r.URL.RequestURI())
			}
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalUser attaches the session user to the context when one is present
// but never blocks the request. Public pages use it to personalise rendering.
func (s *Service) OptionalUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetUserFromContext(r.Context()) == nil {
			if user := s.sessionUser(r); user != nil {
				r = r.WithContext(context.WithValue(r.Context(), userContextKey, user))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) sessionUser(r *http.Request) *db.User {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	sess, err := s.store.GetSession(r.Context(), cookie.Value)
	if err != nil {
		log.Printf("session lookup failed: %v", err)
		return nil
	}
	if sess == nil || !sess.ExpiresAt.After(time.Now()) {
		return nil
	}
	user, err := s.store.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		log.Printf("session user lookup failed for user_id=%d: %v", sess.UserID, err)
		return nil
	}
	if user == nil {
		_ = s.store.DeleteSession(r.Context(), cookie.Value)
	}
	return user
}

// GetUserFromContext retrieves the user from the request context
func GetUserFromContext(ctx context.Context) *db.User {
	user, ok := ctx.Value(userContextKey).(*db.User)
	if !ok {
		return nil
	}
	return user
}

// RequireRole is a middleware that checks if the authenticated user has the required role.
func RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())
		if user == nil || user.Role != role {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAnyRole is a middleware that checks if the authenticated user has any allowed role.
func RequireAnyRole(roles []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())
		if user == nil || !containsRole(roles, user.Role) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// APIAuthMiddleware preserves the versioned API error contract without redirects.
func (s *Service) APIAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := s.sessionUser(r)
		if user == nil {
			writeAPIJSONError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}
