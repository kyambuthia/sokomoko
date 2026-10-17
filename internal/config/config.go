// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
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
	loadErrors          []string
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
	var loadErrors []string
	readInt := func(name string, fallback int) int {
		return readEnvInt(name, fallback, &loadErrors)
	}
	cfg := Config{
		Environment:          strings.ToLower(readEnv("ENV", defaultEnvironment)),
		Port:                 readEnv("PORT", defaultPort),
		DatabaseURL:          databaseURL,
		DatabaseURLExplicit:  databaseURL != "",
		DBMaxOpenConns:       readInt("DB_MAX_OPEN_CONNS", defaultDBMaxOpenConns),
		DBMaxIdleConns:       readInt("DB_MAX_IDLE_CONNS", defaultDBMaxIdleConns),
		DBConnMaxLifetime:    readInt("DB_CONN_MAX_LIFETIME_SECONDS", defaultDBConnMaxLifetimeSeconds),
		DBQueryTimeout:       readInt("DB_QUERY_TIMEOUT_SECONDS", defaultDBQueryTimeoutSeconds),
		AllowedHostsRaw:      strings.TrimSpace(os.Getenv("ALLOWED_HOSTS")),
		AdminSetupToken:      strings.TrimSpace(os.Getenv("ADMIN_SETUP_TOKEN")),
		SessionCookieDomain:  strings.TrimSpace(strings.ToLower(os.Getenv("SESSION_COOKIE_DOMAIN"))),
		TrustProxyHeaders:    readEnvBool("TRUST_PROXY_HEADERS", false, &loadErrors),
		LogFormat:            strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT"))),
		LogLevel:             strings.ToLower(readEnv("LOG_LEVEL", "info")),
		PostRateLimitMax:     readInt("POST_RATE_LIMIT_MAX", defaultPostRateMax),
		PostRateLimitWindow:  readInt("POST_RATE_LIMIT_WINDOW_SECONDS", defaultPostRateWin),
		AuthAbuseMaxFailures: readInt("AUTH_ABUSE_MAX_FAILURES", defaultAuthAbuseMaxFailures),
		AuthAbuseBackoffBase: readInt("AUTH_ABUSE_BACKOFF_BASE_SECONDS", defaultAuthAbuseBackoffBaseSeconds),
		AuthAbuseBackoffMax:  readInt("AUTH_ABUSE_BACKOFF_MAX_SECONDS", defaultAuthAbuseBackoffMaxSeconds),
		PasswordResetBaseURL: strings.TrimSpace(os.Getenv("PASSWORD_RESET_BASE_URL")),
		SMTPHost:             strings.TrimSpace(strings.ToLower(os.Getenv("SMTP_HOST"))),
		SMTPPort:             readEnv("SMTP_PORT", defaultSMTPPort),
		SMTPUsername:         strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword:         strings.TrimSpace(os.Getenv("SMTP_PASSWORD")),
		SMTPFrom:             strings.TrimSpace(os.Getenv("SMTP_FROM")),
	}
	cfg.loadErrors = loadErrors
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
	problems := append([]string{}, c.loadErrors...)
	if !validPort(c.Port) {
		problems = append(problems, "PORT must be between 1 and 65535")
	}
	if c.Environment != "" && c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		problems = append(problems, "ENV must be development, test, or production")
	}
	if err := c.ValidateDatabase(); err != nil {
		problems = append(problems, err.Error())
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		problems = append(problems, fmt.Sprintf("LOG_FORMAT %q must be json or text", c.LogFormat))
	}
	switch c.LogLevel {
	case "", "debug", "info", "warn", "error":
	default:
		problems = append(problems, "LOG_LEVEL must be debug, info, warn, or error")
	}
	hosts := map[string]bool{}
	for _, part := range strings.Split(c.AllowedHostsRaw, ",") {
		host := strings.ToLower(strings.TrimSpace(part))
		if host == "" {
			continue
		}
		if !validHost(host) {
			problems = append(problems, "ALLOWED_HOSTS must contain exact hostnames without schemes, ports, paths, or wildcards")
		}
		hosts[host] = true
	}
	if c.PostRateLimitMax > 0 && c.PostRateLimitWindow <= 0 {
		problems = append(problems, "POST_RATE_LIMIT_WINDOW_SECONDS must be positive when rate limiting is enabled")
	}
	if c.AuthAbuseMaxFailures > 0 && (c.AuthAbuseBackoffBase <= 0 || c.AuthAbuseBackoffMax < c.AuthAbuseBackoffBase) {
		problems = append(problems, "AUTH_ABUSE backoff must be positive with max >= base")
	}
	if c.IsProduction() {
		if len(hosts) == 0 {
			problems = append(problems, "ALLOWED_HOSTS is required in production")
		}
		if c.PasswordResetBaseURL == "" {
			problems = append(problems, "PASSWORD_RESET_BASE_URL is required in production so reset links never trust the Host header")
		}
		if c.SMTPHost == "" || c.SMTPFrom == "" {
			problems = append(problems, "SMTP_HOST and SMTP_FROM are required in production for password recovery")
		}
		if c.AdminSetupToken != "" && len(c.AdminSetupToken) < 32 {
			problems = append(problems, "ADMIN_SETUP_TOKEN must contain at least 32 bytes in production")
		}
	}
	if c.PasswordResetBaseURL != "" {
		u, err := url.Parse(c.PasswordResetBaseURL)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") {
			problems = append(problems, "PASSWORD_RESET_BASE_URL must be an absolute HTTP(S) origin")
		} else if c.IsProduction() && (u.Scheme != "https" || !hosts[strings.ToLower(u.Hostname())]) {
			problems = append(problems, "PASSWORD_RESET_BASE_URL must use HTTPS and a host from ALLOWED_HOSTS in production")
		}
	}
	if c.SessionCookieDomain != "" {
		domain := strings.TrimPrefix(c.SessionCookieDomain, ".")
		if !validHost(domain) || !strings.Contains(domain, ".") || net.ParseIP(domain) != nil {
			problems = append(problems, "SESSION_COOKIE_DOMAIN must be a DNS domain")
		}
		for host := range hosts {
			if host != domain && !strings.HasSuffix(host, "."+domain) {
				problems = append(problems, "SESSION_COOKIE_DOMAIN must contain every allowed host; leave it empty for host-only cookies")
				break
			}
		}
	}
	if (c.SMTPHost == "") != (c.SMTPFrom == "") {
		problems = append(problems, "SMTP_HOST and SMTP_FROM must be set together")
	}
	if c.SMTPFrom != "" {
		if _, err := mail.ParseAddress(c.SMTPFrom); err != nil || strings.ContainsAny(c.SMTPFrom, "\r\n") {
			problems = append(problems, "SMTP_FROM must be a valid email address")
		}
		if !validPort(c.SMTPPort) {
			problems = append(problems, "SMTP_PORT must be between 1 and 65535")
		}
	}
	if (c.SMTPUsername == "") != (c.SMTPPassword == "") {
		problems = append(problems, "SMTP_USERNAME and SMTP_PASSWORD must be set together")
	}
	if len(problems) > 0 {
		return errors.New("invalid configuration: " + strings.Join(problems, "; "))
	}
	return nil
}

// ValidateDatabase is also used by release commands, which do not need SMTP,
// public hosts, or web server settings.
func (c Config) ValidateDatabase() error {
	if c.IsProduction() && !c.DatabaseURLExplicit {
		return errors.New("DATABASE_URL is required in production")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.DBMaxOpenConns < 0 || c.DBMaxIdleConns < 0 || (c.DBMaxOpenConns > 0 && c.DBMaxIdleConns > c.DBMaxOpenConns) {
		return errors.New("database pool requires 0 <= DB_MAX_IDLE_CONNS <= DB_MAX_OPEN_CONNS")
	}
	for _, problem := range c.loadErrors {
		if strings.HasPrefix(problem, "DB_") {
			return errors.New(problem)
		}
	}
	return nil
}

func validPort(value string) bool {
	port, err := strconv.Atoi(value)
	return err == nil && port > 0 && port <= 65535
}

func validHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func readEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func readEnvInt(name string, fallback int, problems *[]string) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		*problems = append(*problems, name+" must be a non-negative integer")
		return fallback
	}
	return parsed
}

func readEnvBool(name string, fallback bool, problems *[]string) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		*problems = append(*problems, name+" must be a boolean")
		return fallback
	}
	return parsed
}
