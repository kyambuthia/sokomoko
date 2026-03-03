package config

import "testing"

func TestLoadFromEnv_Defaults(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("PORT", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("ALLOWED_HOSTS", "")
	t.Setenv("ADMIN_SETUP_TOKEN", "")
	t.Setenv("SESSION_COOKIE_DOMAIN", "")

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
}

func TestLoadFromEnv_Values(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("PORT", "8080")
	t.Setenv("DB_PATH", "/tmp/app.db")
	t.Setenv("ALLOWED_HOSTS", "localhost,admin.localhost")
	t.Setenv("ADMIN_SETUP_TOKEN", "s3cr3t")
	t.Setenv("SESSION_COOKIE_DOMAIN", ".EXAMPLE.COM")

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
	if !cfg.IsProduction() {
		t.Fatal("IsProduction() = false, want true")
	}
}
