package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type StaffCredential struct {
	Username     string
	Email        string
	TempPassword string
}

type AdminSetupPageData struct {
	Title            string
	Role             string
	Message          string
	Error            string
	RecommendedStaff int
	StaffRationale   string
	StaffCredentials []StaffCredential
	ShowForm         bool
	CSRFToken        string
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
	if provided == "" {
		provided = strings.TrimSpace(r.FormValue("setup_token"))
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *Service) isAdminSetupAuthorized(r *http.Request) bool {
	if strings.TrimSpace(s.adminSetupToken) != "" {
		return s.validAdminSetupToken(r)
	}
	return isLoopbackRequest(r)
}

func recommendedStaffCount(productCount int) (int, string) {
	base := 2
	extra := 0
	if productCount > 100 {
		extra = (productCount - 100 + 74) / 75
	}
	total := base + extra
	if total > 12 {
		total = 12
	}
	reason := "Baseline recommendation is 2 staff (operations + customer support), plus 1 extra for each additional ~75 products after the first 100."
	return total, reason
}

// AdminSetup is the first-run bootstrap flow that creates the root admin and optional staff accounts.
func (s *Service) AdminSetup(tmpl *template.Template) http.HandlerFunc {
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

		products, err := s.store.GetAllProducts(r.Context())
		if err != nil {
			products = []db.Product{}
		}
		recommendedStaff, rationale := recommendedStaffCount(len(products))

		data := AdminSetupPageData{
			Title:            "Initial Admin Setup",
			Role:             "admin",
			RecommendedStaff: recommendedStaff,
			StaffRationale:   rationale,
			ShowForm:         true,
			CSRFToken:        s.CSRFTokenForSession(r),
		}

		if r.Method == http.MethodGet {
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		adminUsername := strings.TrimSpace(r.FormValue("admin_username"))
		adminEmail := normalizeEmail(r.FormValue("admin_email"))
		adminPassword := r.FormValue("admin_password")
		confirmPassword := r.FormValue("confirm_password")
		staffCountRaw := strings.TrimSpace(r.FormValue("staff_count"))

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

		staffCount := recommendedStaff
		if staffCountRaw != "" {
			parsed, parseErr := strconv.Atoi(staffCountRaw)
			if parseErr != nil || parsed < 0 || parsed > 20 {
				data.Error = "Staff count must be a number between 0 and 20"
				renderWithStatus(w, tmpl, http.StatusBadRequest, data)
				return
			}
			staffCount = parsed
		}

		adminHash, err := hashPassword(adminPassword)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		tempPasswords := make([]string, staffCount)
		staffHashes := make([]string, staffCount)
		for i := range tempPasswords {
			token, tokenErr := generateOpaqueToken(12)
			if tokenErr != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			tempPasswords[i] = token
			if staffHashes[i], err = hashPassword(token); err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		staffUsers, err := s.store.BootstrapAdmin(r.Context(),
			db.User{Username: adminUsername, Email: adminEmail, PasswordHash: adminHash},
			staffHashes,
			func(i, attempt int) (string, string) {
				username := fmt.Sprintf("staff%02d", i)
				if attempt > 0 {
					username = fmt.Sprintf("staff%02d_%d", i, attempt)
				}
				return username, username + "@sokomoko.local"
			},
		)
		if errors.Is(err, db.ErrAdminExists) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if err != nil {
			log.Printf("admin bootstrap failed: %v", err)
			statusCode, message := mapAccountCreationError(err, "Failed to create admin account. Username or email may already exist.")
			data.Error = message
			renderWithStatus(w, tmpl, statusCode, data)
			return
		}

		staffCredentials := make([]StaffCredential, 0, len(staffUsers))
		for i, user := range staffUsers {
			staffCredentials = append(staffCredentials, StaffCredential{
				Username:     user.Username,
				Email:        user.Email,
				TempPassword: tempPasswords[i],
			})
		}

		data.ShowForm = false
		data.Message = "Root admin account has been created. Save the temporary staff credentials now."
		data.StaffCredentials = staffCredentials
		renderWithStatus(w, tmpl, 0, data)
	}
}
