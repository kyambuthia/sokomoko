package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

const (
	sessionCookieName  = "session_token"
	sessionDuration    = 24 * time.Hour
	resetTokenDuration = 45 * time.Minute
	resetEmailTimeout  = 5 * time.Second
)

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

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

func requestScheme(r *http.Request) string {
	if r != nil && (r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")) {
		return "https"
	}
	return "http"
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
