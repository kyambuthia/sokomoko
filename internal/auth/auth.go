package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "session_token"
	sessionDuration   = 24 * time.Hour
)

func shouldUseSecureCookies(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return strings.EqualFold(os.Getenv("ENV"), "production")
}

// Auth handles the /auth/ route and renders the login page
func Auth(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "login.html", nil)
	}
}

// SignUp handles user registration
func SignUp(store *db.Store, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			tmpl.ExecuteTemplate(w, "signup.html", nil)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		username := r.FormValue("username")
		email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
		password := r.FormValue("password")
		username = strings.TrimSpace(username)

		if username == "" || email == "" || password == "" {
			http.Error(w, "All fields are required", http.StatusBadRequest)
			return
		}

		// Generate a salt
		saltBytes := make([]byte, 16)
		_, err := rand.Read(saltBytes)
		if err != nil {
			log.Printf("Error generating salt: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		salt := base64.URLEncoding.EncodeToString(saltBytes)

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password+salt), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Error hashing password: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		user := db.User{
			Username:     username,
			Email:        email,
			PasswordHash: string(hashedPassword),
			Salt:         salt,
			Role:         "user",   // Default role
			Slug:         username, // Simple slug
		}

		_, err = store.CreateUser(user)
		if err != nil {
			log.Printf("Error creating user: %v", err)
			http.Error(w, "Failed to create user. Username or email might already exist.", http.StatusConflict)
			return
		}

		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// Login handles user authentication
func Login(store *db.Store, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			tmpl.ExecuteTemplate(w, "root_template", nil)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		if username == "" || password == "" {
			http.Error(w, "Username and password are required", http.StatusBadRequest)
			return
		}

		user, err := store.GetUserByUsername(username)
		if err != nil || user == nil {
			log.Printf("Login failed for user %s: %v", username, err)
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		// Compare the hashed password with the provided password + stored salt
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password+user.Salt)); err != nil {
			log.Printf("Password mismatch for user %s: %v", username, err)
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		// Generate a secure session token
		b := make([]byte, 32)
		_, err = rand.Read(b)
		if err != nil {
			log.Printf("Error generating session token: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		sessionToken := base64.URLEncoding.EncodeToString(b)
		expiresAt := time.Now().Add(sessionDuration)

		// Create session in DB
		err = store.CreateSession(db.Session{
			ID:        sessionToken,
			UserID:    user.ID,
			ExpiresAt: expiresAt,
		})
		if err != nil {
			log.Printf("Error creating session in DB: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    sessionToken,
			Expires:  expiresAt,
			MaxAge:   int(sessionDuration.Seconds()),
			Path:     "/",
			HttpOnly: true,
			Secure:   shouldUseSecureCookies(r),
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// AdminLogin handles admin authentication and ensures only admins can login
func AdminLogin(store *db.Store, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			tmpl.ExecuteTemplate(w, "root_template", nil)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		if username == "" || password == "" {
			http.Error(w, "Username and password are required", http.StatusBadRequest)
			return
		}

		user, err := store.GetUserByUsername(username)
		if err != nil || user == nil || user.Role != "admin" {
			log.Printf("Admin login failed for user %s: %v", username, err)
			http.Error(w, "Invalid admin credentials", http.StatusUnauthorized)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password+user.Salt)); err != nil {
			http.Error(w, "Invalid admin credentials", http.StatusUnauthorized)
			return
		}

		// Success
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Printf("Error generating admin session token: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		sessionToken := base64.URLEncoding.EncodeToString(b)
		expiresAt := time.Now().Add(sessionDuration)

		// Create session in DB
		err = store.CreateSession(db.Session{
			ID:        sessionToken,
			UserID:    user.ID,
			ExpiresAt: expiresAt,
		})
		if err != nil {
			log.Printf("Error creating admin session in DB: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    sessionToken,
			Expires:  expiresAt,
			MaxAge:   int(sessionDuration.Seconds()),
			Path:     "/",
			HttpOnly: true,
			Secure:   shouldUseSecureCookies(r),
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// Logout handles user logout
func Logout(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)
		if err == nil {
			// Delete session from DB
			_ = store.DeleteSession(cookie.Value)
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			Path:     "/",
			HttpOnly: true,
			Secure:   shouldUseSecureCookies(r),
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const userContextKey contextKey = "user"

// AuthMiddleware provides authentication middleware for protected routes
func AuthMiddleware(store *db.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			if err == http.ErrNoCookie {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		sess, err := store.GetSession(cookie.Value)
		if err != nil || sess == nil || sess.ExpiresAt.Before(time.Now()) {
			// Session invalid or expired
			if sess != nil {
				_ = store.DeleteSession(cookie.Value)
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		user, err := store.GetUserByID(sess.UserID)
		if err != nil || user == nil {
			log.Printf("Error retrieving user from DB for session %d: %v", sess.UserID, err)
			_ = store.DeleteSession(cookie.Value)
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
