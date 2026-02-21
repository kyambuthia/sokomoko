package config

import (
	"os"
	"strings"
)

const (
	defaultPort        = "6969"
	defaultDBPath      = "./db/t.db"
	defaultEnvironment = "development"
)

type Config struct {
	Environment     string
	Port            string
	DBPath          string
	AllowedHostsRaw string
	AdminSetupToken string
}

func LoadFromEnv() Config {
	return Config{
		Environment:     readEnv("ENV", defaultEnvironment),
		Port:            readEnv("PORT", defaultPort),
		DBPath:          readEnv("DB_PATH", defaultDBPath),
		AllowedHostsRaw: strings.TrimSpace(os.Getenv("ALLOWED_HOSTS")),
		AdminSetupToken: strings.TrimSpace(os.Getenv("ADMIN_SETUP_TOKEN")),
	}
}

func (c Config) IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(c.Environment), "production")
}

func readEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
