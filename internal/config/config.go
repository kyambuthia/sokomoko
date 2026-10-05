// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultPort        = "6969"
	defaultDatabaseURL = "postgres://sokomoko@localhost:5432/sokomoko?sslmode=disable"
	defaultEnvironment = "development"
	defaultPostRateMax = 0
	defaultPostRateWin = 60

	defaultAuthAbuseMaxFailures        = 0
	defaultAuthAbuseBackoffBaseSeconds = 1
	defaultAuthAbuseBackoffMaxSeconds  = 300

	defaultDBMaxOpenConns           = 20
	defaultDBMaxIdleConns           = 10
	defaultDBConnMaxLifetimeSeconds = 1800
	defaultDBQueryTimeoutSeconds    = 10

	defaultSMTPPort = "587"
)

type Config struct {
	Environment         string
	Port                string
	DatabaseURL         string
	DatabaseURLExplicit bool
	DBMaxOpenConns      int
	DBMaxIdleConns      int
	DBConnMaxLifetime   int
	DBQueryTimeout      int
	AllowedHostsRaw     string
	AdminSetupToken     string
	SessionCookieDomain string
	TrustProxyHeaders   bool
	LogFormat           string
	LogLevel            string

	PostRateLimitMax     int
	PostRateLimitWindow  int
	AuthAbuseMaxFailures int
	AuthAbuseBackoffBase int
	AuthAbuseBackoffMax  int

	PasswordResetBaseURL string
	SMTPHost             string
	SMTPPort             string
	SMTPUsername         string
	SMTPPassword         string
	SMTPFrom             string
}

func LoadFromEnv() Config {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	cfg := Config{
		Environment:          strings.ToLower(readEnv("ENV", defaultEnvironment)),
		Port:                 readEnv("PORT", defaultPort),
		DatabaseURL:          databaseURL,
		DatabaseURLExplicit:  databaseURL != "",
		DBMaxOpenConns:       readEnvInt("DB_MAX_OPEN_CONNS", defaultDBMaxOpenConns),
		DBMaxIdleConns:       readEnvInt("DB_MAX_IDLE_CONNS", defaultDBMaxIdleConns),
		DBConnMaxLifetime:    readEnvInt("DB_CONN_MAX_LIFETIME_SECONDS", defaultDBConnMaxLifetimeSeconds),
		DBQueryTimeout:       readEnvInt("DB_QUERY_TIMEOUT_SECONDS", defaultDBQueryTimeoutSeconds),
		AllowedHostsRaw:      strings.TrimSpace(os.Getenv("ALLOWED_HOSTS")),
		AdminSetupToken:      strings.TrimSpace(os.Getenv("ADMIN_SETUP_TOKEN")),
		SessionCookieDomain:  strings.TrimSpace(strings.ToLower(os.Getenv("SESSION_COOKIE_DOMAIN"))),
		TrustProxyHeaders:    readEnvBool("TRUST_PROXY_HEADERS", false),
		LogFormat:            strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT"))),
		LogLevel:             strings.ToLower(readEnv("LOG_LEVEL", "info")),
		PostRateLimitMax:     readEnvInt("POST_RATE_LIMIT_MAX", defaultPostRateMax),
		PostRateLimitWindow:  readEnvInt("POST_RATE_LIMIT_WINDOW_SECONDS", defaultPostRateWin),
		AuthAbuseMaxFailures: readEnvInt("AUTH_ABUSE_MAX_FAILURES", defaultAuthAbuseMaxFailures),
		AuthAbuseBackoffBase: readEnvInt("AUTH_ABUSE_BACKOFF_BASE_SECONDS", defaultAuthAbuseBackoffBaseSeconds),
		AuthAbuseBackoffMax:  readEnvInt("AUTH_ABUSE_BACKOFF_MAX_SECONDS", defaultAuthAbuseBackoffMaxSeconds),
		PasswordResetBaseURL: strings.TrimSpace(os.Getenv("PASSWORD_RESET_BASE_URL")),
		SMTPHost:             strings.TrimSpace(strings.ToLower(os.Getenv("SMTP_HOST"))),
		SMTPPort:             readEnv("SMTP_PORT", defaultSMTPPort),
		SMTPUsername:         strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword:         strings.TrimSpace(os.Getenv("SMTP_PASSWORD")),
		SMTPFrom:             strings.TrimSpace(os.Getenv("SMTP_FROM")),
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = defaultDatabaseURL
	}
	if cfg.LogFormat == "" {
		cfg.LogFormat = "text"
		if cfg.IsProduction() {
			cfg.LogFormat = "json"
		}
	}
	return cfg
}

func (c Config) IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(c.Environment), "production")
}

// Validate reports configuration that is unsafe or unusable. In production the
// database URL and trusted hosts must be set explicitly.
func (c Config) Validate() error {
	var problems []string
	if _, err := strconv.Atoi(c.Port); err != nil {
		problems = append(problems, fmt.Sprintf("PORT %q is not a number", c.Port))
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		problems = append(problems, fmt.Sprintf("LOG_FORMAT %q must be json or text", c.LogFormat))
	}
	if c.IsProduction() {
		if !c.DatabaseURLExplicit {
			problems = append(problems, "DATABASE_URL is required in production")
		}
		if c.AllowedHostsRaw == "" {
			problems = append(problems, "ALLOWED_HOSTS is required in production")
		}
		if c.PasswordResetBaseURL == "" {
			problems = append(problems, "PASSWORD_RESET_BASE_URL is required in production so reset links never trust the Host header")
		}
	}
	if (c.SMTPHost == "") != (c.SMTPFrom == "") {
		problems = append(problems, "SMTP_HOST and SMTP_FROM must be set together")
	}
	if len(problems) > 0 {
		return errors.New("invalid configuration: " + strings.Join(problems, "; "))
	}
	return nil
}

func readEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func readEnvInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func readEnvBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
