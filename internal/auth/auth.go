package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName  = "session_token"
	sessionDuration    = 24 * time.Hour
	resetTokenDuration = 45 * time.Minute
	resetEmailTimeout  = 5 * time.Second
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,31}$`)

type PasswordResetEmailSender func(ctx context.Context, recipientEmail string, resetLink string) error

type Config struct {
	Environment              string
	AdminSetupToken          string
	SessionCookieDomain      string
	PasswordResetBaseURL     string
	PasswordResetEmailSender PasswordResetEmailSender
}

type Service struct {
	store                    *db.Store
	environment              string
	adminSetupToken          string
	sessionCookieDomain      string
	passwordResetBaseURL     string
	passwordResetEmailSender PasswordResetEmailSender
}

func NewService(store *db.Store, cfg Config) *Service {
	environment := strings.TrimSpace(strings.ToLower(cfg.Environment))
	if environment == "" {
		environment = "development"
	}

	return &Service{
		store:                    store,
		environment:              environment,
		adminSetupToken:          strings.TrimSpace(cfg.AdminSetupToken),
		sessionCookieDomain:      strings.TrimSpace(strings.ToLower(cfg.SessionCookieDomain)),
		passwordResetBaseURL:     strings.TrimSpace(cfg.PasswordResetBaseURL),
		passwordResetEmailSender: cfg.PasswordResetEmailSender,
	}
}

type StaffCredential struct {
	Username     string
	Email        string
	TempPassword string
}

type AdminSetupPageData struct {
	Title            string
	Message          string
	Error            string
	RecommendedStaff int
	StaffRationale   string
	StaffCredentials []StaffCredential
	ShowForm         bool
}

type StaffSignupPageData struct {
	Title    string
	Message  string
	Error    string
	ShowForm bool
}

type SignupPageData struct {
	Title    string
	Username string
	Email    string
	Error    string
}

type LoginPageData struct {
	Title    string
	Username string
	Error    string
}

type AdminLoginPageData struct {
	Title    string
	Username string
	Error    string
}

type PasswordResetRequestData struct {
	Title      string
	Heading    string
	Helper     string
	Identifier string
	Message    string
	Error      string
	ResetLink  string
}

type PasswordResetConfirmData struct {
	Title    string
	Heading  string
	Helper   string
	Token    string
	Message  string
	Error    string
	ShowForm bool
}

func (s *Service) shouldUseSecureCookies(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return s.environment == "production"
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func generateOpaqueToken(lengthBytes int) (string, error) {
	buf := make([]byte, lengthBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashPassword(password string) (string, string, error) {
	salt, err := dbGenerateSalt()
	if err != nil {
		return "", "", err
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password+salt), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return string(hashedPassword), salt, nil
}

func dbGenerateSalt() (string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(saltBytes), nil
}

func createUserWithPassword(store *db.Store, username, email, password, role string) (int64, error) {
	passwordHash, salt, err := hashPassword(password)
	if err != nil {
		return 0, err
	}
	user := db.User{
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		Salt:         salt,
		Role:         role,
		Slug:         username,
	}
	return store.CreateUser(user)
}

func (s *Service) startSession(w http.ResponseWriter, r *http.Request, userID int) error {
	sessionToken, err := generateOpaqueToken(32)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(sessionDuration)

	err = s.store.CreateSession(db.Session{
		ID:        sessionToken,
		UserID:    userID,
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

func isStrongEnoughPassword(password string) bool {
	return len(strings.TrimSpace(password)) >= 10
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

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

func isLoopbackRequest(r *http.Request) bool {
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
	if len(provided) != len(expected) {
		return false
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

func (s *Service) shouldExposeResetLink() bool {
	return s.environment != "production"
}

func requestScheme(r *http.Request) string {
	if r != nil && (r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")) {
		return "https"
	}
	return "http"
}

func (s *Service) buildPasswordResetLink(r *http.Request, token string) string {
	relative := "/password-reset/confirm?token=" + url.QueryEscape(token)

	baseURL := strings.TrimSpace(s.passwordResetBaseURL)
	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/password-reset/confirm"
			query := parsed.Query()
			query.Set("token", token)
			parsed.RawQuery = query.Encode()
			return parsed.String()
		}
	}

	host := ""
	if r != nil {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return relative
	}

	return requestScheme(r) + "://" + host + relative
}

func (s *Service) dispatchPasswordResetEmail(recipientEmail string, resetLink string, userID int) {
	sender := s.passwordResetEmailSender
	if sender == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), resetEmailTimeout)
		defer cancel()

		if err := sender(ctx, recipientEmail, resetLink); err != nil {
			log.Printf("password reset email send failed for user_id=%d: %v", userID, err)
		}
	}()
}

func isUniqueConstraintErr(err error) bool {
	return db.IsUniqueConstraintError(err)
}

func isTransientDBErr(err error) bool {
	return db.IsTransientError(err)
}

func mapAccountCreationError(err error, conflictMessage string) (int, string) {
	if isUniqueConstraintErr(err) {
		return http.StatusConflict, conflictMessage
	}
	if isTransientDBErr(err) {
		return http.StatusServiceUnavailable, "Service is temporarily busy. Please retry in a moment."
	}
	return http.StatusInternalServerError, "Unable to create account right now. Please retry."
}

func (s *Service) findUserByIdentifier(identifier string) (*db.User, error) {
	trimmed := strings.TrimSpace(identifier)
	if strings.Contains(trimmed, "@") {
		return s.store.GetUserByEmail(normalizeEmail(trimmed))
	}
	return s.store.GetUserByUsername(trimmed)
}

func renderWithStatus(w http.ResponseWriter, tmpl *template.Template, statusCode int, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "root_template", data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if statusCode > 0 {
		w.WriteHeader(statusCode)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("Response write error: %v", err)
	}
}

// SignUp handles normal user registration.
func (s *Service) SignUp(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := SignupPageData{Title: "Create Account"}

		if r.Method == http.MethodGet {
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
			data.Error = "Password must be at least 10 characters"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}

		_, err := createUserWithPassword(s.store, username, email, password, "user")
		if err != nil {
			log.Printf("Error creating user: %v", err)
			statusCode, message := mapAccountCreationError(err, "Failed to create user. Username or email might already exist.")
			data.Error = message
			renderWithStatus(w, tmpl, statusCode, data)
			return
		}

		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// StaffSignUp handles staff account creation by admin users.
func (s *Service) StaffSignUp(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := StaffSignupPageData{
			Title:    "Staff Account Setup",
			ShowForm: true,
		}

		if r.Method == http.MethodGet {
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
			data.Error = "Password must be at least 10 characters"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}

		_, err := createUserWithPassword(s.store, username, email, password, "staff")
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

// Login handles normal user authentication.
func (s *Service) Login(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := LoginPageData{Title: "Login"}

		if r.Method == http.MethodGet {
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		data.Username = username

		if username == "" || password == "" {
			data.Error = "Username and password are required"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}

		user, err := s.store.GetUserByUsername(username)
		if err != nil || user == nil || user.Role != "user" {
			log.Printf("Login failed for user %s: %v", username, err)
			data.Error = "Invalid credentials"
			renderWithStatus(w, tmpl, http.StatusUnauthorized, data)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password+user.Salt)); err != nil {
			log.Printf("Password mismatch for user %s: %v", username, err)
			data.Error = "Invalid credentials"
			renderWithStatus(w, tmpl, http.StatusUnauthorized, data)
			return
		}

		if err := s.startSession(w, r, user.ID); err != nil {
			log.Printf("Error creating session in DB: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// AdminLogin handles admin/staff authentication.
func (s *Service) AdminLogin(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := AdminLoginPageData{Title: "Admin Login"}

		hasAdmin, err := s.store.HasAdminUser()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !hasAdmin {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}

		if r.Method == http.MethodGet {
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		data.Username = username
		if username == "" || password == "" {
			data.Error = "Username and password are required"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}

		user, err := s.store.GetUserByUsername(username)
		if err != nil || user == nil || !containsRole([]string{"admin", "staff"}, user.Role) {
			log.Printf("Admin/staff login failed for user %s: %v", username, err)
			data.Error = "Invalid credentials"
			renderWithStatus(w, tmpl, http.StatusUnauthorized, data)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password+user.Salt)); err != nil {
			data.Error = "Invalid credentials"
			renderWithStatus(w, tmpl, http.StatusUnauthorized, data)
			return
		}

		if err := s.startSession(w, r, user.ID); err != nil {
			log.Printf("Error creating admin session in DB: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// AdminSetup is the first-run bootstrap flow that creates the root admin and optional staff accounts.
func (s *Service) AdminSetup(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hasAdmin, err := s.store.HasAdminUser()
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

		products, err := s.store.GetAllProducts()
		if err != nil {
			products = []db.Product{}
		}
		recommendedStaff, rationale := recommendedStaffCount(len(products))

		data := AdminSetupPageData{
			Title:            "Initial Admin Setup",
			RecommendedStaff: recommendedStaff,
			StaffRationale:   rationale,
			ShowForm:         true,
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
			data.Error = "Admin password must be at least 10 characters"
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

		if _, err := createUserWithPassword(s.store, adminUsername, adminEmail, adminPassword, "admin"); err != nil {
			statusCode, message := mapAccountCreationError(err, "Failed to create admin account. Username or email may already exist.")
			data.Error = message
			renderWithStatus(w, tmpl, statusCode, data)
			return
		}

		staffCredentials := make([]StaffCredential, 0, staffCount)
		for i := 1; i <= staffCount; i++ {
			created := false
			for attempt := 0; attempt < 10; attempt++ {
				suffix := ""
				if attempt > 0 {
					suffix = fmt.Sprintf("_%d", attempt)
				}
				username := fmt.Sprintf("staff%02d%s", i, suffix)
				email := fmt.Sprintf("%s@sokomoko.local", username)
				tempPassword, tokenErr := generateOpaqueToken(9)
				if tokenErr != nil {
					data.Error = "Failed to generate staff credentials"
					renderWithStatus(w, tmpl, 0, data)
					return
				}
				tempPassword += "Aa1!"

				if _, err := createUserWithPassword(s.store, username, email, tempPassword, "staff"); err != nil {
					if !isUniqueConstraintErr(err) {
						data.Error = "Failed to provision staff accounts."
						renderWithStatus(w, tmpl, 0, data)
						return
					}
					continue
				}
				staffCredentials = append(staffCredentials, StaffCredential{
					Username:     username,
					Email:        email,
					TempPassword: tempPassword,
				})
				created = true
				break
			}
			if !created {
				data.Error = "Failed to provision all staff accounts."
				renderWithStatus(w, tmpl, 0, data)
				return
			}
		}

		data.ShowForm = false
		data.Message = "Root admin account has been created. Save the temporary staff credentials now."
		data.StaffCredentials = staffCredentials
		renderWithStatus(w, tmpl, 0, data)
	}
}

func (s *Service) PasswordResetRequest(tmpl *template.Template, allowedRoles []string, title string, helper string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := PasswordResetRequestData{
			Title:   title,
			Heading: title,
			Helper:  helper,
		}

		if r.Method == http.MethodGet {
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		identifier := strings.TrimSpace(r.FormValue("identifier"))
		data.Identifier = identifier
		if identifier == "" {
			data.Error = "Enter username or email"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		user, err := s.findUserByIdentifier(identifier)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data.Message = "If the account exists and is allowed in this area, a reset link is available below."
		if user != nil && containsRole(allowedRoles, user.Role) {
			resetToken, tokenErr := generateOpaqueToken(32)
			if tokenErr != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if err := s.store.CreatePasswordResetToken(user.ID, resetToken, time.Now().Add(resetTokenDuration)); err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			resetLink := s.buildPasswordResetLink(r, resetToken)
			s.dispatchPasswordResetEmail(user.Email, resetLink, user.ID)
			if s.shouldExposeResetLink() {
				data.ResetLink = resetLink
			}
		}

		renderWithStatus(w, tmpl, 0, data)
	}
}

func (s *Service) PasswordResetConfirm(tmpl *template.Template, allowedRoles []string, title string, helper string, loginPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := PasswordResetConfirmData{
			Title:    title,
			Heading:  title,
			Helper:   helper,
			ShowForm: true,
		}

		if r.Method == http.MethodGet {
			token := strings.TrimSpace(r.URL.Query().Get("token"))
			data.Token = token
			if token == "" {
				data.Error = "Missing reset token"
				renderWithStatus(w, tmpl, 0, data)
				return
			}

			resetToken, err := s.store.GetValidPasswordResetToken(token)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if resetToken == nil {
				data.Error = "Reset token is invalid or expired"
				renderWithStatus(w, tmpl, 0, data)
				return
			}

			user, err := s.store.GetUserByID(resetToken.UserID)
			if err != nil || user == nil || !containsRole(allowedRoles, user.Role) {
				data.Error = "Reset token is invalid for this account type"
				renderWithStatus(w, tmpl, 0, data)
				return
			}

			renderWithStatus(w, tmpl, 0, data)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token := strings.TrimSpace(r.FormValue("token"))
		password := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")
		data.Token = token

		if token == "" || password == "" || confirmPassword == "" {
			data.Error = "All fields are required"
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if password != confirmPassword {
			data.Error = "Passwords do not match"
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if !isStrongEnoughPassword(password) {
			data.Error = "Password must be at least 10 characters"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		resetToken, err := s.store.GetValidPasswordResetToken(token)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if resetToken == nil {
			data.Error = "Reset token is invalid or expired"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		user, err := s.store.GetUserByID(resetToken.UserID)
		if err != nil || user == nil || !containsRole(allowedRoles, user.Role) {
			data.Error = "Reset token is invalid for this account type"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		passwordHash, salt, err := hashPassword(password)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		used, err := s.store.UsePasswordResetToken(token, passwordHash, salt)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !used {
			data.Error = "Reset token is invalid or expired"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		data.ShowForm = false
		data.Message = "Password reset successful. You can now sign in at " + loginPath
		renderWithStatus(w, tmpl, 0, data)
	}
}

// Logout handles user logout
func (s *Service) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)
		if err == nil {
			_ = s.store.DeleteSession(cookie.Value)
		}

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
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const userContextKey contextKey = "user"

// AuthMiddleware provides authentication middleware for protected routes
func (s *Service) AuthMiddleware(next http.Handler) http.Handler {
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

		sess, err := s.store.GetSession(cookie.Value)
		if err != nil || sess == nil || sess.ExpiresAt.Before(time.Now()) {
			if sess != nil {
				_ = s.store.DeleteSession(cookie.Value)
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		user, err := s.store.GetUserByID(sess.UserID)
		if err != nil || user == nil {
			log.Printf("Error retrieving user from DB for session %d: %v", sess.UserID, err)
			_ = s.store.DeleteSession(cookie.Value)
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

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
