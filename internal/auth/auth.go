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
)

var runtimeEnvironment = "development"
var runtimeAdminSetupToken string
var runtimeSessionCookieDomain string

func SetEnvironment(env string) {
	clean := strings.TrimSpace(strings.ToLower(env))
	if clean == "" {
		clean = "development"
	}
	runtimeEnvironment = clean
}

func SetAdminSetupToken(token string) {
	runtimeAdminSetupToken = strings.TrimSpace(token)
}

func SetSessionCookieDomain(domain string) {
	runtimeSessionCookieDomain = strings.TrimSpace(strings.ToLower(domain))
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

func shouldUseSecureCookies(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return runtimeEnvironment == "production"
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

func startSession(w http.ResponseWriter, r *http.Request, store *db.Store, userID int) error {
	sessionToken, err := generateOpaqueToken(32)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(sessionDuration)

	err = store.CreateSession(db.Session{
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
		Domain:   runtimeSessionCookieDomain,
		Path:     "/",
		HttpOnly: true,
		Secure:   shouldUseSecureCookies(r),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func isStrongEnoughPassword(password string) bool {
	return len(strings.TrimSpace(password)) >= 10
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

func validAdminSetupToken(r *http.Request) bool {
	expected := strings.TrimSpace(runtimeAdminSetupToken)
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

func isAdminSetupAuthorized(r *http.Request) bool {
	if strings.TrimSpace(runtimeAdminSetupToken) != "" {
		return validAdminSetupToken(r)
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

func shouldExposeResetLink() bool {
	return runtimeEnvironment != "production"
}

func findUserByIdentifier(store *db.Store, identifier string) (*db.User, error) {
	trimmed := strings.TrimSpace(identifier)
	if strings.Contains(trimmed, "@") {
		return store.GetUserByEmail(normalizeEmail(trimmed))
	}
	return store.GetUserByUsername(trimmed)
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
func SignUp(store *db.Store, tmpl *template.Template) http.HandlerFunc {
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
		if !isStrongEnoughPassword(password) {
			data.Error = "Password must be at least 10 characters"
			renderWithStatus(w, tmpl, http.StatusBadRequest, data)
			return
		}

		_, err := createUserWithPassword(store, username, email, password, "user")
		if err != nil {
			log.Printf("Error creating user: %v", err)
			data.Error = "Failed to create user. Username or email might already exist."
			renderWithStatus(w, tmpl, http.StatusConflict, data)
			return
		}

		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// StaffSignUp handles staff account creation by admin users.
func StaffSignUp(store *db.Store, tmpl *template.Template) http.HandlerFunc {
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
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if !isStrongEnoughPassword(password) {
			data.Error = "Password must be at least 10 characters"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		_, err := createUserWithPassword(store, username, email, password, "staff")
		if err != nil {
			data.Error = "Failed to create staff account. Username or email may already exist."
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		data.Message = "Staff account created successfully."
		renderWithStatus(w, tmpl, 0, data)
	}
}

// Login handles normal user authentication.
func Login(store *db.Store, tmpl *template.Template) http.HandlerFunc {
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

		user, err := store.GetUserByUsername(username)
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

		if err := startSession(w, r, store, user.ID); err != nil {
			log.Printf("Error creating session in DB: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// AdminLogin handles admin/staff authentication.
func AdminLogin(store *db.Store, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := AdminLoginPageData{Title: "Admin Login"}

		hasAdmin, err := store.HasAdminUser()
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

		user, err := store.GetUserByUsername(username)
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

		if err := startSession(w, r, store, user.ID); err != nil {
			log.Printf("Error creating admin session in DB: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// AdminSetup is the first-run bootstrap flow that creates the root admin and optional staff accounts.
func AdminSetup(store *db.Store, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hasAdmin, err := store.HasAdminUser()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if hasAdmin {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if !isAdminSetupAuthorized(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		products, err := store.GetAllProducts()
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
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if adminPassword != confirmPassword {
			data.Error = "Passwords do not match"
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if !isStrongEnoughPassword(adminPassword) {
			data.Error = "Admin password must be at least 10 characters"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		staffCount := recommendedStaff
		if staffCountRaw != "" {
			parsed, parseErr := strconv.Atoi(staffCountRaw)
			if parseErr != nil || parsed < 0 || parsed > 20 {
				data.Error = "Staff count must be a number between 0 and 20"
				renderWithStatus(w, tmpl, 0, data)
				return
			}
			staffCount = parsed
		}

		if _, err := createUserWithPassword(store, adminUsername, adminEmail, adminPassword, "admin"); err != nil {
			data.Error = "Failed to create admin account. Username or email may already exist."
			renderWithStatus(w, tmpl, 0, data)
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

				if _, err := createUserWithPassword(store, username, email, tempPassword, "staff"); err != nil {
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

func PasswordResetRequest(store *db.Store, tmpl *template.Template, allowedRoles []string, title string, helper string) http.HandlerFunc {
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

		user, err := findUserByIdentifier(store, identifier)
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
			if err := store.CreatePasswordResetToken(user.ID, resetToken, time.Now().Add(resetTokenDuration)); err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if shouldExposeResetLink() {
				data.ResetLink = "/password-reset/confirm?token=" + resetToken
			}
		}

		renderWithStatus(w, tmpl, 0, data)
	}
}

func PasswordResetConfirm(store *db.Store, tmpl *template.Template, allowedRoles []string, title string, helper string, loginPath string) http.HandlerFunc {
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

			resetToken, err := store.GetValidPasswordResetToken(token)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if resetToken == nil {
				data.Error = "Reset token is invalid or expired"
				renderWithStatus(w, tmpl, 0, data)
				return
			}

			user, err := store.GetUserByID(resetToken.UserID)
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

		resetToken, err := store.GetValidPasswordResetToken(token)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if resetToken == nil {
			data.Error = "Reset token is invalid or expired"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		user, err := store.GetUserByID(resetToken.UserID)
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

		used, err := store.UsePasswordResetToken(token, passwordHash, salt)
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
func Logout(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)
		if err == nil {
			_ = store.DeleteSession(cookie.Value)
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			Domain:   runtimeSessionCookieDomain,
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
