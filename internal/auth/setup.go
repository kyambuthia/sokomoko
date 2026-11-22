package auth

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type AdminSetupPageData struct {
	Title     string
	Role      string
	Message   string
	Error     string
	ShowForm  bool
	CSRFToken string
}

// isLoopbackRequest reports whether the request came straight from this
// machine. A request relayed by a reverse proxy on the same host also arrives
// from loopback, so any forwarding header disqualifies it.
func isLoopbackRequest(r *http.Request) bool {
	for _, header := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip", "Forwarded", "Via"} {
		if len(r.Header.Values(header)) > 0 {
			return false
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Service) validAdminSetupToken(r *http.Request) bool {
	expected := strings.TrimSpace(s.adminSetupToken)
	if expected == "" {
		return false
	}

	provided := strings.TrimSpace(r.Header.Get("X-Admin-Setup-Token"))
	if provided == "" && r.Method == http.MethodPost {
		provided = strings.TrimSpace(r.PostFormValue("setup_token"))
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *Service) isAdminSetupAuthorized(r *http.Request) bool {
	if strings.TrimSpace(s.adminSetupToken) != "" {
		return s.validAdminSetupToken(r)
	}
	return isLoopbackRequest(r)
}

// AdminSetup is the first-run bootstrap flow that creates the root admin account.
// Staff accounts are created later by the admin from Team (/staff/signup).
func (s *Service) AdminSetup(tmpl TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hasAdmin, err := s.store.HasAdminUser(r.Context())
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if hasAdmin {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if !s.isAdminSetupAuthorized(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		data := AdminSetupPageData{
			Title:     "Initial Admin Setup",
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

		adminUsername := strings.TrimSpace(r.FormValue("admin_username"))
		adminEmail := normalizeEmail(r.FormValue("admin_email"))
		adminPassword := r.FormValue("admin_password")
		confirmPassword := r.FormValue("confirm_password")

		if adminUsername == "" || adminEmail == "" || adminPassword == "" || confirmPassword == "" {
			data.Error = "All admin fields are required"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if !isValidUsername(adminUsername) {
			data.Error = "Admin username must be 3-32 characters and use letters, numbers, dots, dashes, or underscores"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if !isValidEmail(adminEmail) {
			data.Error = "Enter a valid admin email address"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if adminPassword != confirmPassword {
			data.Error = "Passwords do not match"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}
		if !isStrongEnoughPassword(adminPassword) {
			data.Error = "Admin password must be 10 to 72 characters"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}

		adminHash, err := hashPassword(adminPassword)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		_, err = s.store.BootstrapAdmin(r.Context(),
			db.User{Username: adminUsername, Email: adminEmail, PasswordHash: adminHash},
			nil,
			nil,
		)
		if errors.Is(err, db.ErrAdminExists) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "admin bootstrap failed", "error", err)
			statusCode, message := mapAccountCreationError(err, "Failed to create admin account. Username or email may already exist.")
			data.Error = message
			renderWithStatus(w, tmpl, statusCode, data)
			return
		}

		data.ShowForm = false
		data.Message = "Root admin account has been created. You can add staff accounts later from Team."
		renderWithStatus(w, tmpl, 0, data)
	}
}
