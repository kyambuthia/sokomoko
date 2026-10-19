package auth

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	"golang.org/x/crypto/bcrypt"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,31}$`)

// maxPasswordBytes is bcrypt's input limit; longer inputs are rejected rather
// than silently truncated.
const maxPasswordBytes = 72

func hashPassword(password string) (string, error) {
	if len(password) > maxPasswordBytes {
		return "", bcrypt.ErrPasswordTooLong
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// dummyPasswordHash is compared against when a login names an unknown user so
// that response timing does not reveal which usernames exist.
var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("sokomoko-timing-equalizer"), bcrypt.DefaultCost)

func checkPassword(user *db.User, password string) bool {
	if user == nil {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
}

func createUserWithPassword(ctx context.Context, store store, username, email, password, role string) (int64, error) {
	passwordHash, err := hashPassword(password)
	if err != nil {
		return 0, err
	}
	user := db.User{
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         role,
	}
	return store.CreateUser(ctx, user)
}

// HashPassword hashes a password with bcrypt. Passwords over 72 bytes are
// rejected rather than truncated.
func HashPassword(password string) (string, error) {
	return hashPassword(password)
}

// ValidateNewAccount applies the username, email, and password rules shared
// by every account creation form. It returns a user-facing problem, or ""
// when the input is acceptable.
func ValidateNewAccount(username, email, password string) string {
	switch {
	case strings.TrimSpace(username) == "" || strings.TrimSpace(email) == "" || password == "":
		return "All fields are required"
	case !isValidUsername(username):
		return "Username must be 3-32 characters and use letters, numbers, dots, dashes, or underscores"
	case !isValidEmail(email):
		return "Enter a valid email address"
	case !isStrongEnoughPassword(password):
		return "Password must be 10 to 72 characters"
	}
	return ""
}

func isStrongEnoughPassword(password string) bool {
	return len(strings.TrimSpace(password)) >= 10 && len(password) <= maxPasswordBytes
}

func isValidUsername(username string) bool {
	return usernamePattern.MatchString(strings.TrimSpace(username))
}

func isValidEmail(email string) bool {
	clean := strings.TrimSpace(strings.ToLower(email))
	if clean == "" || len(clean) > 254 {
		return false
	}
	parsed, err := mail.ParseAddress(clean)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(parsed.Address), clean)
}

// PasswordChangePageData backs the signed-in password change form.
type PasswordChangePageData struct {
	Title     string
	Username  string
	Action    string
	BackHref  string
	Message   string
	Error     string
	CSRFToken string
}

// PasswordChange lets a signed-in user replace their password after proving
// they know the current one. Every session for the account is revoked and the
// browser gets a fresh one. Mount it behind AuthMiddleware.
func (s *Service) PasswordChange(tmpl *template.Template, action, backHref string) http.HandlerFunc {
	const scope = "password-change"
	return func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		data := PasswordChangePageData{
			Title: "Change password", Username: user.Username, Action: action, BackHref: backHref,
			CSRFToken: s.CSRFTokenForSession(r),
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if r.URL.Query().Get("changed") == "1" {
				data.Message = "Password changed. Other devices have been signed out."
			}
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		identifier := strconv.Itoa(user.ID)
		if s.checkAbuseLock(w, scope, identifier, true, r) {
			data.Error = "Too many attempts. Please wait and try again."
			renderWithStatus(w, tmpl, http.StatusTooManyRequests, data)
			return
		}
		current := r.FormValue("current_password")
		password := r.FormValue("password")
		confirm := r.FormValue("confirm_password")
		switch {
		case current == "" || password == "" || confirm == "":
			data.Error = "All fields are required"
		case !checkPassword(user, current):
			s.markAbuseFailure(scope, identifier, true, r)
			data.Error = "Current password is incorrect"
		case password != confirm:
			data.Error = "New passwords do not match"
		case !isStrongEnoughPassword(password):
			data.Error = "Password must be 10 to 72 characters"
		case password == current:
			data.Error = "Choose a password you have not been using"
		}
		if data.Error != "" {
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		s.clearAbuseFailures(scope, identifier, true, r)
		hash, err := hashPassword(password)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if err := s.store.UpdateUserPassword(r.Context(), user.ID, hash); err != nil {
			log.Printf("password change failed for user_id=%d: %v", user.ID, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if err := s.startSession(w, r, user.ID); err != nil {
			log.Printf("create session after password change: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, action+"?changed=1", http.StatusSeeOther)
	}
}
