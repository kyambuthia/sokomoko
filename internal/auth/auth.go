package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"golang.org/x/crypto/bcrypt"
)

// Session represents a user session
type Session struct {
	UserID    int
	ExpiresAt time.Time
}

// sessions is a simple in-memory store for sessions. In a real application,
// this would be a persistent store (e.g., database, Redis).
var sessions = make(map[string]Session)

// Auth handles the /auth/ route and renders the login page
func Auth(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "login.html", nil)
	}
}

// SignUp handles user registration
func SignUp(tmpl *template.Template) http.HandlerFunc {
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
		email := r.FormValue("email")
		password := r.FormValue("password")

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
			Role:         "user", // Default role
		}

		_, err = db.CreateUser(user)
		if err != nil {
			log.Printf("Error creating user: %v", err)
			http.Error(w, "Failed to create user. Username or email might already exist.", http.StatusConflict)
			return
		}

		w.WriteHeader(http.StatusCreated)
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// Login handles user authentication
func Login(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			tmpl.ExecuteTemplate(w, "login.html", nil)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			http.Error(w, "Username and password are required", http.StatusBadRequest)
			return
		}

		user, err := db.GetUserByUsername(username)
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
		expiresAt := time.Now().Add(24 * time.Hour)

		sessions[sessionToken] = Session{
			UserID:    user.ID,
			ExpiresAt: expiresAt,
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    sessionToken,
			Expires:  expiresAt,
			HttpOnly: true,
			Secure:   true, // Set to true in production with HTTPS
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// Logout handles user logout
func Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err == nil {
			// Delete session from store
			delete(sessions, cookie.Value)
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    "",
			Expires:  time.Now().Add(-time.Hour), // Set expiration to a past time to delete the cookie
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const userContextKey contextKey = "user"

// AuthMiddleware provides authentication middleware for protected routes
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		sess, ok := sessions[cookie.Value]
		if !ok || sess.ExpiresAt.Before(time.Now()) {
			// Session invalid or expired
			delete(sessions, cookie.Value) // Clean up expired session
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		user, err := db.GetUserByID(sess.UserID)
		if err != nil || user == nil {
			log.Printf("Error retrieving user from DB for session %d: %v", sess.UserID, err)
			delete(sessions, cookie.Value) // Invalidate session if user not found
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// Renew session expiration
		sess.ExpiresAt = time.Now().Add(24 * time.Hour)
		sessions[cookie.Value] = sess

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
