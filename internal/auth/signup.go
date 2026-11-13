package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type StaffSignupPageData struct {
	Title     string
	Role      string
	Message   string
	Error     string
	ShowForm  bool
	CSRFToken string
}

type SignupPageData struct {
	Title     string
	Username  string
	Email     string
	Error     string
	CSRFToken string
}

type CourierSignupPageData struct {
	Title     string
	Username  string
	Message   string
	Error     string
	CSRFToken string
}

// SignUp handles normal user registration.
func (s *Service) SignUp(tmpl TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := SignupPageData{
			Title:     "Create Account",
			CSRFToken: s.CSRFTokenForSession(r),
		}

		if r.Method == http.MethodGet {
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodGet, http.MethodPost)
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		email := normalizeEmail(r.FormValue("email"))
		password := r.FormValue("password")
		data.Username = username
		data.Email = email

		if username == "" || email == "" || password == "" {
			data.Error = "All fields are required"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if !isValidUsername(username) {
			data.Error = "Username must be 3-32 characters and use letters, numbers, dots, dashes, or underscores"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if !isValidEmail(email) {
			data.Error = "Enter a valid email address"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if !isStrongEnoughPassword(password) {
			data.Error = "Password must be 10 to 72 characters"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}

		_, err := createUserWithPassword(r.Context(), s.store, username, email, password, "user")
		if err != nil {
			slog.ErrorContext(r.Context(), "create user failed", "error", err)
			statusCode, message := mapAccountCreationError(err, "Failed to create user. Username or email might already exist.")
			data.Error = message
			renderWithStatus(w, tmpl, statusCode, data)
			return
		}

		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// StaffSignUp handles staff account creation by admin users.
func (s *Service) StaffSignUp(tmpl TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := StaffSignupPageData{
			Title:     "Staff Account Setup",
			Role:      "admin",
			ShowForm:  true,
			CSRFToken: s.CSRFTokenForSession(r),
		}

		if r.Method == http.MethodGet {
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodGet, http.MethodPost)
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		email := normalizeEmail(r.FormValue("email"))
		password := r.FormValue("password")

		if username == "" || email == "" || password == "" {
			data.Error = "All fields are required"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if !isValidUsername(username) {
			data.Error = "Username must be 3-32 characters and use letters, numbers, dots, dashes, or underscores"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if !isValidEmail(email) {
			data.Error = "Enter a valid email address"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if !isStrongEnoughPassword(password) {
			data.Error = "Password must be 10 to 72 characters"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}

		_, err := createUserWithPassword(r.Context(), s.store, username, email, password, "staff")
		if err != nil {
			statusCode, message := mapAccountCreationError(err, "Failed to create staff account. Username or email may already exist.")
			data.Error = message
			renderWithStatus(w, tmpl, statusCode, data)
			return
		}

		data.Message = "Staff account created successfully."
		renderWithStatus(w, tmpl, 0, data)
	}
}

// CourierSignUp creates a rider login for a signed-in admin. Couriers have no
// email workflow, so the required database email is a non-deliverable alias.
func (s *Service) CourierSignUp(tmpl TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := CourierSignupPageData{Title: "Add courier", CSRFToken: s.CSRFTokenForSession(r)}
		if r.Method == http.MethodGet {
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodGet, http.MethodPost)
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		data.Username = username
		if !isValidUsername(username) {
			data.Error = "Username must be 3-32 characters and use letters, numbers, dots, dashes, or underscores"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if !isStrongEnoughPassword(password) {
			data.Error = "Password must be 10 to 72 characters"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}

		sum := sha256.Sum256([]byte(strings.ToLower(username)))
		email := "courier+" + hex.EncodeToString(sum[:]) + "@courier.invalid"
		if _, err := createUserWithPassword(r.Context(), s.store, username, email, password, db.RoleCourier); err != nil {
			slog.ErrorContext(r.Context(), "create courier account failed", "error", err)
			statusCode, message := mapAccountCreationError(err, "Failed to create courier account. Username may already exist.")
			data.Error = message
			renderWithStatus(w, tmpl, statusCode, data)
			return
		}

		data.Message = "Courier account created. Share the username and initial password with the rider outside this site."
		renderWithStatus(w, tmpl, 0, data)
	}
}
