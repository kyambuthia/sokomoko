package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func (s *Service) sessionCookieNameFor(r *http.Request) string {
	if !s.shouldUseSecureCookies(r) {
		return sessionCookieName
	}
	if s.sessionCookieDomain == "" {
		return hostOnlySessionCookieName
	}
	return sharedDomainSessionCookieName
}

// sessionCookieNamesFor includes the legacy name only for cleanup. It is never
// accepted as an authenticated session when a prefixed secure cookie is active.
func (s *Service) sessionCookieNamesFor(r *http.Request) []string {
	active := s.sessionCookieNameFor(r)
	if active == sessionCookieName {
		return []string{active}
	}
	return []string{active, sessionCookieName}
}

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
func (s *Service) startSession(w http.ResponseWriter, r *http.Request, userID int, expectedPasswordHash string) error {
	s.revokeRequestSessions(r.Context(), r, "sign-in: revoke previous session")
	session, err := s.newSession(userID)
	if err != nil {
		return err
	}
	if err := s.store.CreateSessionForPasswordHash(r.Context(), session, expectedPasswordHash); err != nil {
		return err
	}
	s.setSessionCookie(w, r, session)
	return nil
}

func (s *Service) newSession(userID int) (db.Session, error) {
	sessionToken, err := generateOpaqueToken(32)
	if err != nil {
		return db.Session{}, err
	}
	csrfToken, err := generateOpaqueToken(32)
	if err != nil {
		return db.Session{}, err
	}
	return db.Session{
		ID:        sessionToken,
		UserID:    userID,
		CSRFToken: csrfToken,
		ExpiresAt: time.Now().Add(sessionDuration),
	}, nil
}

func (s *Service) setSessionCookie(w http.ResponseWriter, r *http.Request, session db.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieNameFor(r),
		Value:    session.ID,
		Expires:  session.ExpiresAt,
		MaxAge:   int(sessionDuration.Seconds()),
		Domain:   s.sessionCookieDomain,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.shouldUseSecureCookies(r),
		SameSite: http.SameSiteLaxMode,
	})
	s.clearLegacySessionCookie(w, r)
}

// Logout revokes the session and clears the cookie.
func (s *Service) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.revokeRequestSessions(context.WithoutCancel(r.Context()), r, "logout: revoke session")
		s.clearSessionCookie(w, r)
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (s *Service) revokeRequestSessions(ctx context.Context, r *http.Request, operation string) {
	cookieNames := make(map[string]struct{})
	for _, name := range s.sessionCookieNamesFor(r) {
		cookieNames[name] = struct{}{}
	}
	seen := make(map[string]struct{})
	for _, cookie := range r.Cookies() {
		if _, ok := cookieNames[cookie.Name]; !ok || cookie.Value == "" {
			continue
		}
		if _, ok := seen[cookie.Value]; ok {
			continue
		}
		seen[cookie.Value] = struct{}{}
		if err := s.store.DeleteSession(ctx, cookie.Value); err != nil {
			slog.WarnContext(ctx, "delete session failed", "operation", operation, "error", err)
		}
	}
}

func (s *Service) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	for _, name := range s.sessionCookieNamesFor(r) {
		s.clearCookie(w, r, name)
	}
}

func (s *Service) clearLegacySessionCookie(w http.ResponseWriter, r *http.Request) {
	if s.sessionCookieNameFor(r) != sessionCookieName {
		s.clearCookie(w, r, sessionCookieName)
	}
}

func (s *Service) clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
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
			for _, name := range s.sessionCookieNamesFor(r) {
				if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
					s.clearSessionCookie(w, r)
					break
				}
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
	cookie, err := r.Cookie(s.sessionCookieNameFor(r))
	if err != nil || cookie.Value == "" {
		return nil
	}
	if store, ok := s.store.(interface {
		GetActiveUserForSession(context.Context, string) (*db.User, time.Time, error)
	}); ok {
		user, expiresAt, err := store.GetActiveUserForSession(r.Context(), cookie.Value)
		if err != nil {
			slog.ErrorContext(r.Context(), "session lookup failed", "error", err)
			return nil
		}
		if user == nil || !expiresAt.After(time.Now()) {
			return nil
		}
		return user
	}

	sess, err := s.store.GetSession(r.Context(), cookie.Value)
	if err != nil {
		slog.ErrorContext(r.Context(), "session lookup failed", "error", err)
		return nil
	}
	if sess == nil || !sess.ExpiresAt.After(time.Now()) {
		return nil
	}
	user, err := s.store.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		slog.ErrorContext(r.Context(), "session user lookup failed", "user_id", sess.UserID, "error", err)
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
