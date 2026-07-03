package config

import "testing"

func TestLoadFromEnv_Defaults(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("PORT", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("ALLOWED_HOSTS", "")
	t.Setenv("ADMIN_SETUP_TOKEN", "")
	t.Setenv("SESSION_COOKIE_DOMAIN", "")
	t.Setenv("POST_RATE_LIMIT_MAX", "")
	t.Setenv("POST_RATE_LIMIT_WINDOW_SECONDS", "")
	t.Setenv("AUTH_ABUSE_MAX_FAILURES", "")
	t.Setenv("AUTH_ABUSE_BACKOFF_BASE_SECONDS", "")
	t.Setenv("AUTH_ABUSE_BACKOFF_MAX_SECONDS", "")
	t.Setenv("PASSWORD_RESET_BASE_URL", "")
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("SMTP_PASSWORD", "")
	t.Setenv("SMTP_FROM", "")

	cfg := LoadFromEnv()
	if cfg.Environment != defaultEnvironment {
		t.Fatalf("Environment = %q, want %q", cfg.Environment, defaultEnvironment)
	}
	if cfg.Port != defaultPort {
		t.Fatalf("Port = %q, want %q", cfg.Port, defaultPort)
	}
	if cfg.DBPath != defaultDBPath {
		t.Fatalf("DBPath = %q, want %q", cfg.DBPath, defaultDBPath)
	}
	if cfg.AllowedHostsRaw != "" {
		t.Fatalf("AllowedHostsRaw = %q, want empty", cfg.AllowedHostsRaw)
	}
	if cfg.AdminSetupToken != "" {
		t.Fatalf("AdminSetupToken = %q, want empty", cfg.AdminSetupToken)
	}
	if cfg.SessionCookieDomain != "" {
		t.Fatalf("SessionCookieDomain = %q, want empty", cfg.SessionCookieDomain)
	}
	if cfg.PostRateLimitMax != defaultPostRateMax {
		t.Fatalf("PostRateLimitMax = %d, want %d", cfg.PostRateLimitMax, defaultPostRateMax)
	}
	if cfg.PostRateLimitWindow != defaultPostRateWin {
		t.Fatalf("PostRateLimitWindow = %d, want %d", cfg.PostRateLimitWindow, defaultPostRateWin)
	}
	if cfg.AuthAbuseMaxFailures != defaultAuthAbuseMaxFailures {
		t.Fatalf("AuthAbuseMaxFailures = %d, want %d", cfg.AuthAbuseMaxFailures, defaultAuthAbuseMaxFailures)
	}
	if cfg.AuthAbuseBackoffBase != defaultAuthAbuseBackoffBaseSeconds {
		t.Fatalf("AuthAbuseBackoffBase = %d, want %d", cfg.AuthAbuseBackoffBase, defaultAuthAbuseBackoffBaseSeconds)
	}
	if cfg.AuthAbuseBackoffMax != defaultAuthAbuseBackoffMaxSeconds {
		t.Fatalf("AuthAbuseBackoffMax = %d, want %d", cfg.AuthAbuseBackoffMax, defaultAuthAbuseBackoffMaxSeconds)
	}
	if cfg.PasswordResetBaseURL != "" {
		t.Fatalf("PasswordResetBaseURL = %q, want empty", cfg.PasswordResetBaseURL)
	}
	if cfg.SMTPHost != "" {
		t.Fatalf("SMTPHost = %q, want empty", cfg.SMTPHost)
	}
	if cfg.SMTPPort != defaultSMTPPort {
		t.Fatalf("SMTPPort = %q, want %q", cfg.SMTPPort, defaultSMTPPort)
	}
	if cfg.SMTPUsername != "" {
		t.Fatalf("SMTPUsername = %q, want empty", cfg.SMTPUsername)
	}
	if cfg.SMTPPassword != "" {
		t.Fatalf("SMTPPassword = %q, want empty", cfg.SMTPPassword)
	}
	if cfg.SMTPFrom != "" {
		t.Fatalf("SMTPFrom = %q, want empty", cfg.SMTPFrom)
	}
}

func TestLoadFromEnv_Values(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("PORT", "8080")
	t.Setenv("DB_PATH", "/tmp/app.db")
	t.Setenv("ALLOWED_HOSTS", "localhost,admin.localhost")
	t.Setenv("ADMIN_SETUP_TOKEN", "s3cr3t")
	t.Setenv("SESSION_COOKIE_DOMAIN", ".EXAMPLE.COM")
	t.Setenv("POST_RATE_LIMIT_MAX", "15")
	t.Setenv("POST_RATE_LIMIT_WINDOW_SECONDS", "90")
	t.Setenv("AUTH_ABUSE_MAX_FAILURES", "4")
	t.Setenv("AUTH_ABUSE_BACKOFF_BASE_SECONDS", "3")
	t.Setenv("AUTH_ABUSE_BACKOFF_MAX_SECONDS", "120")
	t.Setenv("PASSWORD_RESET_BASE_URL", "https://shop.example.com")
	t.Setenv("SMTP_HOST", "SMTP.EXAMPLE.COM")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_USERNAME", "mailer")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("SMTP_FROM", "alerts@example.com")

	cfg := LoadFromEnv()
	if cfg.Environment != "production" {
		t.Fatalf("Environment = %q, want production", cfg.Environment)
	}
	if cfg.Port != "8080" {
		t.Fatalf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.DBPath != "/tmp/app.db" {
		t.Fatalf("DBPath = %q, want /tmp/app.db", cfg.DBPath)
	}
	if cfg.AllowedHostsRaw != "localhost,admin.localhost" {
		t.Fatalf("AllowedHostsRaw = %q, unexpected", cfg.AllowedHostsRaw)
	}
	if cfg.AdminSetupToken != "s3cr3t" {
		t.Fatalf("AdminSetupToken = %q, want s3cr3t", cfg.AdminSetupToken)
	}
	if cfg.SessionCookieDomain != ".example.com" {
		t.Fatalf("SessionCookieDomain = %q, want .example.com", cfg.SessionCookieDomain)
	}
	if cfg.PostRateLimitMax != 15 {
		t.Fatalf("PostRateLimitMax = %d, want 15", cfg.PostRateLimitMax)
	}
	if cfg.PostRateLimitWindow != 90 {
		t.Fatalf("PostRateLimitWindow = %d, want 90", cfg.PostRateLimitWindow)
	}
	if cfg.AuthAbuseMaxFailures != 4 {
		t.Fatalf("AuthAbuseMaxFailures = %d, want 4", cfg.AuthAbuseMaxFailures)
	}
	if cfg.AuthAbuseBackoffBase != 3 {
		t.Fatalf("AuthAbuseBackoffBase = %d, want 3", cfg.AuthAbuseBackoffBase)
	}
	if cfg.AuthAbuseBackoffMax != 120 {
		t.Fatalf("AuthAbuseBackoffMax = %d, want 120", cfg.AuthAbuseBackoffMax)
	}
	if cfg.PasswordResetBaseURL != "https://shop.example.com" {
		t.Fatalf("PasswordResetBaseURL = %q, unexpected", cfg.PasswordResetBaseURL)
	}
	if cfg.SMTPHost != "smtp.example.com" {
		t.Fatalf("SMTPHost = %q, want smtp.example.com", cfg.SMTPHost)
	}
	if cfg.SMTPPort != "2525" {
		t.Fatalf("SMTPPort = %q, want 2525", cfg.SMTPPort)
	}
	if cfg.SMTPUsername != "mailer" {
		t.Fatalf("SMTPUsername = %q, want mailer", cfg.SMTPUsername)
	}
	if cfg.SMTPPassword != "secret" {
		t.Fatalf("SMTPPassword = %q, want secret", cfg.SMTPPassword)
	}
	if cfg.SMTPFrom != "alerts@example.com" {
		t.Fatalf("SMTPFrom = %q, want alerts@example.com", cfg.SMTPFrom)
	}
	if !cfg.IsProduction() {
		t.Fatal("IsProduction() = false, want true")
	}
}
