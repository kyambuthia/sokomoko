package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/mail"
	"net/textproto"
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

type store interface {
	CreatePasswordResetToken(ctx context.Context, userID int, token string, expiresAt time.Time) error
	CreateSession(ctx context.Context, sess db.Session) error
	CreateUser(ctx context.Context, user db.User) (int64, error)
	DeleteSession(ctx context.Context, id string) error
	GetAllProducts(ctx context.Context) ([]db.Product, error)
	GetSession(ctx context.Context, id string) (*db.Session, error)
	GetUserByEmail(ctx context.Context, email string) (*db.User, error)
	GetUserByID(ctx context.Context, id int) (*db.User, error)
	GetUserByUsername(ctx context.Context, username string) (*db.User, error)
	GetValidPasswordResetToken(ctx context.Context, token string) (*db.PasswordResetToken, error)
	HasAdminUser(ctx context.Context) (bool, error)
	BootstrapAdmin(ctx context.Context, admin db.User, staffPasswordHashes []string, name db.StaffNamer) ([]db.User, error)
	UsePasswordResetToken(ctx context.Context, token, passwordHash string) (bool, error)
	PartnerIDForUser(ctx context.Context, userID int) (int, error)
	UpdateUserPassword(ctx context.Context, userID int, passwordHash string) error
}

type Config struct {
	Environment              string
	AdminSetupToken          string
	SessionCookieDomain      string
	AuthAbuseMaxFailures     int
	AuthAbuseBackoffBase     time.Duration
	AuthAbuseBackoffMax      time.Duration
	PasswordResetBaseURL     string
	PasswordResetEmailSender PasswordResetEmailSender
	// TrustProxyHeaders makes X-Forwarded-For and X-Forwarded-Proto authoritative.
	// Enable it only behind a reverse proxy that overwrites those headers.
	TrustProxyHeaders bool
}

type Service struct {
	store                    store
	environment              string
	adminSetupToken          string
	resetEmailBackoff        []time.Duration // nil means defaultResetEmailBackoff
	sessionCookieDomain      string
	abuseLimiter             *authAbuseLimiter
	passwordResetBaseURL     string
	passwordResetEmailSender PasswordResetEmailSender
	trustProxyHeaders        bool
}

func NewService(store store, cfg Config) *Service {
	environment := strings.TrimSpace(strings.ToLower(cfg.Environment))
	if environment == "" {
		environment = "development"
	}

	return &Service{
		store:                    store,
		environment:              environment,
		adminSetupToken:          strings.TrimSpace(cfg.AdminSetupToken),
		sessionCookieDomain:      strings.TrimSpace(strings.ToLower(cfg.SessionCookieDomain)),
		abuseLimiter:             newAuthAbuseLimiter(cfg.AuthAbuseMaxFailures, cfg.AuthAbuseBackoffBase, cfg.AuthAbuseBackoffMax),
		passwordResetBaseURL:     strings.TrimSpace(cfg.PasswordResetBaseURL),
		passwordResetEmailSender: cfg.PasswordResetEmailSender,
		trustProxyHeaders:        cfg.TrustProxyHeaders,
	}
}

func normalizeIdentifierForAbuse(identifier string, preserveCase bool) string {
	normalized := strings.TrimSpace(identifier)
	if preserveCase {
		return normalized
	}
	return strings.ToLower(normalized)
}

// ClientIP returns the client address for rate limiting. X-Forwarded-For is
// only honoured when trustProxy is set, otherwise any client could spoof it to
// dodge throttling. Only the right-most entry is used: it is the one the
// trusted proxy wrote, while entries to its left come from the client when the
// proxy appends to the header.
func ClientIP(r *http.Request, trustProxy bool) string {
	if r == nil {
		return "unknown"
	}
	if trustProxy {
		values := r.Header.Values("X-Forwarded-For")
		if len(values) > 0 {
			last := values[len(values)-1]
			if i := strings.LastIndex(last, ","); i >= 0 {
				last = last[i+1:]
			}
			if candidate := strings.TrimSpace(last); candidate != "" {
				return candidate
			}
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	if host == "" {
		return "unknown"
	}
	return host
}

func (s *Service) clientIP(r *http.Request) string {
	return ClientIP(r, s.trustProxyHeaders)
}

func abuseScopeKey(scope, ip, identifier string) string {
	return scope + "|" + ip + "|" + identifier
}

func (s *Service) throttleStatus(w http.ResponseWriter, retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	seconds := int(retryAfter.Seconds())
	if retryAfter%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
}

func (s *Service) checkAbuseLock(w http.ResponseWriter, scope, identifier string, preserveCase bool, r *http.Request) bool {
	if s.abuseLimiter == nil {
		return false
	}
	ip := s.clientIP(r)
	key := abuseScopeKey(scope, ip, normalizeIdentifierForAbuse(identifier, preserveCase))
	locked, retryAfter := s.abuseLimiter.IsLocked(key, time.Now())
	if !locked {
		return false
	}
	s.throttleStatus(w, retryAfter)
	return true
}

func (s *Service) markAbuseFailure(scope, identifier string, preserveCase bool, r *http.Request) {
	if s.abuseLimiter == nil {
		return
	}
	ip := s.clientIP(r)
	key := abuseScopeKey(scope, ip, normalizeIdentifierForAbuse(identifier, preserveCase))
	s.abuseLimiter.RecordFailure(key, time.Now())
}

func (s *Service) clearAbuseFailures(scope, identifier string, preserveCase bool, r *http.Request) {
	if s.abuseLimiter == nil {
		return
	}
	ip := s.clientIP(r)
	key := abuseScopeKey(scope, ip, normalizeIdentifierForAbuse(identifier, preserveCase))
	s.abuseLimiter.RecordSuccess(key)
}

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

type PasswordResetRequestData struct {
	Title      string
	Heading    string
	Helper     string
	Identifier string
	Message    string
	Error      string
	ResetLink  string
	CSRFToken  string
}

type PasswordResetConfirmData struct {
	Title     string
	Heading   string
	Helper    string
	Token     string
	Message   string
	Error     string
	ShowForm  bool
	CSRFToken string
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

// startSession issues a fresh session, revoking any session the request
// already carried so a pre-login session ID can never be promoted (session
// fixation).
func (s *Service) startSession(w http.ResponseWriter, r *http.Request, userID int) error {
	if existing, err := r.Cookie(sessionCookieName); err == nil && existing.Value != "" {
		if err := s.store.DeleteSession(r.Context(), existing.Value); err != nil {
			log.Printf("sign-in: revoke previous session: %v", err)
		}
	}
	sessionToken, err := generateOpaqueToken(32)
	if err != nil {
		return err
	}
	csrfToken, err := generateOpaqueToken(32)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(sessionDuration)

	err = s.store.CreateSession(r.Context(), db.Session{
		ID:        sessionToken,
		UserID:    userID,
		CSRFToken: csrfToken,
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

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
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
		var err error
		backoff := s.resetEmailBackoff
		if backoff == nil {
			backoff = defaultResetEmailBackoff
		}
		for attempt := 0; attempt < len(backoff)+1; attempt++ {
			if attempt > 0 {
				time.Sleep(backoff[attempt-1])
			}
			ctx, cancel := context.WithTimeout(context.Background(), resetEmailTimeout)
			err = sender(ctx, recipientEmail, resetLink)
			cancel()
			if err == nil || permanentMailError(err) {
				break
			}
		}
		if err != nil {
			log.Printf("password reset email send failed for user_id=%d: %v", userID, err)
		}
	}()
}

// defaultResetEmailBackoff is the wait before each retry of a reset email.
var defaultResetEmailBackoff = []time.Duration{time.Second, 4 * time.Second}

// permanentMailError reports an SMTP 5xx reply, which a retry cannot fix.
func permanentMailError(err error) bool {
	var reply *textproto.Error
	return errors.As(err, &reply) && reply.Code >= 500
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

func (s *Service) findUserByIdentifier(ctx context.Context, identifier string) (*db.User, error) {
	trimmed := strings.TrimSpace(identifier)
	if strings.Contains(trimmed, "@") {
		return s.store.GetUserByEmail(ctx, normalizeEmail(trimmed))
	}
	return s.store.GetUserByUsername(ctx, trimmed)
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
		data := SignupPageData{
			Title:     "Create Account",
			CSRFToken: s.CSRFTokenForSession(r),
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

func (s *Service) PasswordResetRequest(tmpl *template.Template, allowedRoles []string, title string, helper string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := PasswordResetRequestData{
			Title:     title,
			Heading:   title,
			Helper:    helper,
			CSRFToken: s.CSRFTokenForSession(r),
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

		if s.checkAbuseLock(w, "password-reset:request", identifier, false, r) {
			data.Error = "Too many attempts. Please wait and try again."
			renderWithStatus(w, tmpl, http.StatusTooManyRequests, data)
			return
		}

		user, err := s.findUserByIdentifier(r.Context(), identifier)
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
			if err := s.store.CreatePasswordResetToken(r.Context(), user.ID, resetToken, time.Now().Add(resetTokenDuration)); err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			resetLink := s.buildPasswordResetLink(r, resetToken)
			s.dispatchPasswordResetEmail(user.Email, resetLink, user.ID)
			if s.shouldExposeResetLink() {
				data.ResetLink = resetLink
			}
			s.clearAbuseFailures("password-reset:request", identifier, false, r)
		} else {
			s.markAbuseFailure("password-reset:request", identifier, false, r)
		}

		renderWithStatus(w, tmpl, 0, data)
	}
}

func (s *Service) PasswordResetConfirm(tmpl *template.Template, allowedRoles []string, title string, helper string, loginPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := PasswordResetConfirmData{
			Title:     title,
			Heading:   title,
			Helper:    helper,
			ShowForm:  true,
			CSRFToken: s.CSRFTokenForSession(r),
		}

		if r.Method == http.MethodGet {
			token := strings.TrimSpace(r.URL.Query().Get("token"))
			data.Token = token
			abuseIdentifier := token
			if abuseIdentifier == "" {
				abuseIdentifier = "missing-token"
			}
			if s.checkAbuseLock(w, "password-reset:confirm", abuseIdentifier, true, r) {
				data.Error = "Too many attempts. Please wait and try again."
				renderWithStatus(w, tmpl, http.StatusTooManyRequests, data)
				return
			}
			if token == "" {
				s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
				data.Error = "Missing reset token"
				renderWithStatus(w, tmpl, 0, data)
				return
			}

			resetToken, err := s.store.GetValidPasswordResetToken(r.Context(), token)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if resetToken == nil {
				s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
				data.Error = "Reset token is invalid or expired"
				renderWithStatus(w, tmpl, 0, data)
				return
			}

			user, err := s.store.GetUserByID(r.Context(), resetToken.UserID)
			if err != nil || user == nil || !containsRole(allowedRoles, user.Role) {
				s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
				data.Error = "Reset token is invalid for this account type"
				renderWithStatus(w, tmpl, 0, data)
				return
			}
			s.clearAbuseFailures("password-reset:confirm", abuseIdentifier, true, r)

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
		abuseIdentifier := token
		if abuseIdentifier == "" {
			abuseIdentifier = "missing-token"
		}

		if s.checkAbuseLock(w, "password-reset:confirm", abuseIdentifier, true, r) {
			data.Error = "Too many attempts. Please wait and try again."
			renderWithStatus(w, tmpl, http.StatusTooManyRequests, data)
			return
		}

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
			data.Error = "Password must be 10 to 72 characters"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		resetToken, err := s.store.GetValidPasswordResetToken(r.Context(), token)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if resetToken == nil {
			s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
			data.Error = "Reset token is invalid or expired"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		user, err := s.store.GetUserByID(r.Context(), resetToken.UserID)
		if err != nil || user == nil || !containsRole(allowedRoles, user.Role) {
			s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
			data.Error = "Reset token is invalid for this account type"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		passwordHash, err := hashPassword(password)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		used, err := s.store.UsePasswordResetToken(r.Context(), token, passwordHash)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !used {
			s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
			data.Error = "Reset token is invalid or expired"
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		s.clearAbuseFailures("password-reset:confirm", abuseIdentifier, true, r)

		data.ShowForm = false
		data.Message = "Password reset successful. You can now sign in at " + loginPath
		renderWithStatus(w, tmpl, 0, data)
	}
}

// Logout revokes the session and clears the cookie.
func (s *Service) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			// Revoke the session even if the browser stops waiting; clearing
			// the cookie alone would leave a copied token valid.
			if err := s.store.DeleteSession(context.WithoutCancel(r.Context()), cookie.Value); err != nil {
				log.Printf("logout: revoke session: %v", err)
			}
		}
		s.clearSessionCookie(w, r)
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (s *Service) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
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
			if _, err := r.Cookie(sessionCookieName); err == nil {
				s.clearSessionCookie(w, r)
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
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	sess, err := s.store.GetSession(r.Context(), cookie.Value)
	if err != nil {
		log.Printf("session lookup failed: %v", err)
		return nil
	}
	if sess == nil || !sess.ExpiresAt.After(time.Now()) {
		return nil
	}
	user, err := s.store.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		log.Printf("session user lookup failed for user_id=%d: %v", sess.UserID, err)
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
