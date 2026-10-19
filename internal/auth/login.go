package auth

import (
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type LoginPageData struct {
	Title     string
	Username  string
	Error     string
	Next      string
	CSRFToken string
}

type AdminLoginPageData struct {
	Title         string
	Username      string
	Error         string
	Next          string
	SetupRequired bool
	CSRFToken     string
}

type loginConfig struct {
	title        string
	abuseScope   string
	allowedRoles []string
	tmpl         *template.Template
}

// Login handles customer authentication.
func (s *Service) Login(tmpl *template.Template) http.HandlerFunc {
	cfg := loginConfig{title: "Login", abuseScope: "login:user", allowedRoles: []string{db.RoleUser}, tmpl: tmpl}
	return func(w http.ResponseWriter, r *http.Request) {
		s.handleLogin(w, r, cfg, func(username string) any {
			return LoginPageData{Title: cfg.title, Username: username, CSRFToken: s.CSRFTokenForSession(r), Next: safeNextPath(r.FormValue("next"))}
		}, func(data any, msg string) any {
			d := data.(LoginPageData)
			d.Error = msg
			return d
		})
	}
}

// AdminLogin handles admin and staff authentication on the admin host. When no
// admin exists yet it redirects to the first-run /setup flow.
func (s *Service) AdminLogin(tmpl *template.Template) http.HandlerFunc {
	return s.workspaceLogin(tmpl, "/setup")
}

// WorkspaceLogin is AdminLogin for hosts without a setup flow (the partner
// host). Before an admin exists it explains that setup must be completed on
// the admin host instead of redirecting into a loop.
func (s *Service) WorkspaceLogin(tmpl *template.Template) http.HandlerFunc {
	return s.workspaceLogin(tmpl, "")
}

func (s *Service) workspaceLogin(tmpl *template.Template, setupPath string) http.HandlerFunc {
	cfg := loginConfig{title: "Admin Login", abuseScope: "login:admin", allowedRoles: []string{db.RoleAdmin, db.RoleStaff}, tmpl: tmpl}
	if setupPath == "" {
		cfg.allowedRoles = append(cfg.allowedRoles, db.RolePartner)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		hasAdmin, err := s.store.HasAdminUser(r.Context())
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !hasAdmin {
			if setupPath != "" {
				http.Redirect(w, r, setupPath, http.StatusFound)
				return
			}
			renderWithStatus(w, tmpl, http.StatusServiceUnavailable, AdminLoginPageData{
				Title:         cfg.title,
				SetupRequired: true,
				Error:         "Platform setup is not complete. Create the first admin account on the admin host, then sign in here.",
			})
			return
		}
		s.handleLogin(w, r, cfg, func(username string) any {
			return AdminLoginPageData{Title: cfg.title, Username: username, CSRFToken: s.CSRFTokenForSession(r), Next: safeNextPath(r.FormValue("next"))}
		}, func(data any, msg string) any {
			d := data.(AdminLoginPageData)
			d.Error = msg
			return d
		})
	}
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request, cfg loginConfig, page func(username string) any, withError func(data any, msg string) any) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		renderWithStatus(w, cfg.tmpl, 0, page(""))
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	data := page(username)

	if username == "" || password == "" {
		renderWithStatus(w, cfg.tmpl, http.StatusBadRequest, withError(data, "Username and password are required"))
		return
	}
	if s.checkAbuseLock(w, cfg.abuseScope, username, false, r) {
		renderWithStatus(w, cfg.tmpl, http.StatusTooManyRequests, withError(data, "Too many attempts. Please wait and try again."))
		return
	}

	user, err := s.findUserByIdentifier(r.Context(), username)
	if err != nil {
		log.Printf("login lookup failed: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if user != nil && !containsRole(cfg.allowedRoles, user.Role) {
		user = nil
	}
	if !checkPassword(user, password) {
		s.markAbuseFailure(cfg.abuseScope, username, false, r)
		renderWithStatus(w, cfg.tmpl, http.StatusUnauthorized, withError(data, "Invalid credentials"))
		return
	}

	s.clearAbuseFailures(cfg.abuseScope, username, false, r)
	if user.Role == db.RolePartner {
		partnerID, err := s.store.PartnerIDForUser(r.Context(), user.ID)
		if err != nil {
			log.Printf("partner lookup failed: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if partnerID == 0 {
			renderWithStatus(w, cfg.tmpl, http.StatusForbidden, withError(data, "This vendor account is suspended. Contact the marketplace team."))
			return
		}
	}
	if err := s.startSession(w, r, user.ID); err != nil {
		log.Printf("create session failed: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, safeNextPath(r.FormValue("next")), http.StatusFound)
}

// safeNextPath only allows same-site relative paths as post-login redirects,
// which prevents open redirects such as next=//evil.example.
func safeNextPath(raw string) string {
	next := strings.TrimSpace(raw)
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") ||
		strings.ContainsAny(next, "\\\r\n") || len(next) > 512 {
		return "/"
	}
	parsed, err := url.Parse(next)
	if err != nil || parsed.Host != "" || parsed.Scheme != "" {
		return "/"
	}
	return next
}
