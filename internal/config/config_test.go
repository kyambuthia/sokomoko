package config

import "testing"

func TestLoadFromEnv_Defaults(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("PORT", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("ALLOWED_HOSTS", "")

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
}

func TestLoadFromEnv_Values(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("PORT", "8080")
	t.Setenv("DB_PATH", "/tmp/app.db")
	t.Setenv("ALLOWED_HOSTS", "localhost,admin.localhost")

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
	if !cfg.IsProduction() {
		t.Fatal("IsProduction() = false, want true")
	}
}
