package config

import (
	"os"
	"strconv"
	"strings"
)

const (
	defaultPort        = "6969"
	defaultDBPath      = "./db/t.db"
	defaultEnvironment = "development"
	defaultPostRateMax = 0
	defaultPostRateWin = 60
)

type Config struct {
	Environment         string
	Port                string
	DBPath              string
	AllowedHostsRaw     string
	AdminSetupToken     string
	SessionCookieDomain string
	PostRateLimitMax    int
	PostRateLimitWindow int
}

func LoadFromEnv() Config {
	return Config{
		Environment:         readEnv("ENV", defaultEnvironment),
		Port:                readEnv("PORT", defaultPort),
		DBPath:              readEnv("DB_PATH", defaultDBPath),
		AllowedHostsRaw:     strings.TrimSpace(os.Getenv("ALLOWED_HOSTS")),
		AdminSetupToken:     strings.TrimSpace(os.Getenv("ADMIN_SETUP_TOKEN")),
		SessionCookieDomain: strings.TrimSpace(strings.ToLower(os.Getenv("SESSION_COOKIE_DOMAIN"))),
		PostRateLimitMax:    readEnvInt("POST_RATE_LIMIT_MAX", defaultPostRateMax),
		PostRateLimitWindow: readEnvInt("POST_RATE_LIMIT_WINDOW_SECONDS", defaultPostRateWin),
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
