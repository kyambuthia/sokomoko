package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/kyambuthia/sokomoko/internal/api"
	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/bootstrap"
	"github.com/kyambuthia/sokomoko/internal/config"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/lifecycle"
	"github.com/kyambuthia/sokomoko/internal/notify"
	"github.com/kyambuthia/sokomoko/internal/routes"
	"github.com/kyambuthia/sokomoko/internal/secret"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

var runHTTPServerFunc = runHTTPServer

// openStore connects to PostgreSQL with the configured pool settings.
func openStore(cfg config.Config) (*db.Store, error) {
	return db.Open(context.Background(), cfg.DatabaseURL, db.PoolOptions{
		MaxOpenConns:    cfg.DBMaxOpenConns,
		MaxIdleConns:    cfg.DBMaxIdleConns,
		ConnMaxLifetime: time.Duration(cfg.DBConnMaxLifetime) * time.Second,
		QueryTimeout:    time.Duration(cfg.DBQueryTimeout) * time.Second,
	})
}

func runServe(cfg config.Config, seedOnServe bool) error {
	if err := cfg.Validate(); err != nil {
		return wrapCommandError(commandServe, "validate config", err)
	}
	if cfg.IsProduction() && seedOnServe {
		return wrapCommandError(commandServe, "prepare store", fmt.Errorf("demo seeding is disabled in production"))
	}
	resetEmailSender, err := buildPasswordResetEmailSender(cfg)
	if err != nil {
		return wrapCommandError(commandServe, "configure password reset email", err)
	}

	templates, err := ui.ParseTemplates()
	if err != nil {
		return wrapCommandError(commandServe, "parse templates", fmt.Errorf("parse templates: %w", err))
	}

	store, err := openStore(cfg)
	if err != nil {
		return wrapCommandError(commandServe, "open store", err)
	}
	defer store.Close()

	if err := prepareStore(cfg, store, seedOnServe); err != nil {
		return wrapCommandError(commandServe, "prepare store", err)
	}

	secrets, err := secretsBox(cfg)
	if err != nil {
		return wrapCommandError(commandServe, "configure secrets", err)
	}
	if commerce, err := store.GetCommerceSettings(); err == nil {
		ui.SetCurrency(commerce.Currency)
	} else {
		return wrapCommandError(commandServe, "load commerce settings", err)
	}

	application := app.Compose(app.ComposeOptions{
		Store:         store,
		Templates:     templates,
		StaticFS:      ui.StaticFS,
		Secrets:       secrets,
		PublicBaseURL: cfg.PublicBaseURL,
		Production:    cfg.IsProduction(),
		Auth: auth.Config{
			Environment:              cfg.Environment,
			AdminSetupToken:          cfg.AdminSetupToken,
			SessionCookieDomain:      cfg.SessionCookieDomain,
			AuthAbuseMaxFailures:     cfg.AuthAbuseMaxFailures,
			AuthAbuseBackoffBase:     time.Duration(cfg.AuthAbuseBackoffBase) * time.Second,
			AuthAbuseBackoffMax:      time.Duration(cfg.AuthAbuseBackoffMax) * time.Second,
			PasswordResetBaseURL:     cfg.PasswordResetBaseURL,
			PasswordResetEmailSender: resetEmailSender,
			TrustProxyHeaders:        cfg.TrustProxyHeaders,
		},
	})
	server, allowedHosts := buildServer(cfg, application)
	printStartupSummary(cfg, server, allowedHosts, seedOnServe)

	return wrapCommandError(commandServe, "serve http", runHTTPServerFunc(server, store))
}

// secretsBox returns the box that seals payment credentials. Outside
// production a fixed development key is used when SECRETS_KEY is unset, so
// local setup works; production without a key cannot store credentials.
func secretsBox(cfg config.Config) (*secret.Box, error) {
	box, err := secret.New(cfg.SecretsKey)
	if err != nil || box != nil {
		return box, err
	}
	if cfg.IsProduction() {
		log.Printf("[startup] SECRETS_KEY unset: payment provider credentials cannot be saved")
		return nil, nil
	}
	log.Printf("[startup] SECRETS_KEY unset: using the development secrets key")
	return secret.Development(), nil
}

func prepareStore(cfg config.Config, store *db.Store, seed bool) error {
	if cfg.IsProduction() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return store.CheckSchema(ctx)
	}
	return bootstrap.New(store).Prepare(seed)
}

func runMigrate(cfg config.Config) error {
	if err := cfg.ValidateDatabase(); err != nil {
		return wrapCommandError(commandMigrate, "validate database config", err)
	}
	store, err := openStore(cfg)
	if err != nil {
		return wrapCommandError(commandMigrate, "open store", err)
	}
	defer store.Close()

	if err := bootstrap.New(store).Migrate(); err != nil {
		return wrapCommandError(commandMigrate, "apply schema", err)
	}

	log.Printf("[startup] schema applied version=%d", db.LatestSchemaVersion())
	return nil
}

func runSeed(cfg config.Config) error {
	if cfg.IsProduction() {
		return wrapCommandError(commandSeed, "prepare store", fmt.Errorf("demo seeding is disabled in production; provision the administrator through /setup"))
	}
	if err := cfg.ValidateDatabase(); err != nil {
		return wrapCommandError(commandSeed, "validate database config", err)
	}
	store, err := openStore(cfg)
	if err != nil {
		return wrapCommandError(commandSeed, "open store", err)
	}
	defer store.Close()

	if err := bootstrap.New(store).Prepare(true); err != nil {
		return wrapCommandError(commandSeed, "prepare store", err)
	}

	log.Printf("[startup] seed completed")
	return nil
}

func buildPasswordResetEmailSender(cfg config.Config) (auth.PasswordResetEmailSender, error) {
	var resetEmailSender auth.PasswordResetEmailSender
	if cfg.SMTPHost == "" && cfg.SMTPFrom == "" {
		return nil, nil
	}

	emailSender, err := notify.NewSMTPSender(notify.SMTPConfig{
		Host:       cfg.SMTPHost,
		Port:       cfg.SMTPPort,
		Username:   cfg.SMTPUsername,
		Password:   cfg.SMTPPassword,
		From:       cfg.SMTPFrom,
		AppName:    "Sokomoko",
		RequireTLS: cfg.IsProduction(),
	})
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP configuration: %w", err)
	}

	resetEmailSender = emailSender.SendPasswordResetEmail
	log.Printf("[startup] password_reset_email=enabled host=%s port=%s from=%s", cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)
	return resetEmailSender, nil
}

func buildServer(cfg config.Config, application *app.App) (*httpServer, map[string]struct{}) {
	readiness := &lifecycle.Readiness{Check: application.Readiness.PingContext}
	application.Readiness = readiness
	mainMux := http.NewServeMux()
	adminMux := http.NewServeMux()
	partnerMux := http.NewServeMux()
	apiMux := http.NewServeMux()

	routes.RegisterPublic(application, mainMux)
	routes.RegisterAdmin(application, adminMux)
	routes.RegisterPartner(application, partnerMux)
	api.Register(application, apiMux)

	allowedHosts := parseAllowedHosts(cfg.AllowedHostsRaw)
	if len(allowedHosts) == 0 {
		allowedHosts = map[string]struct{}{
			"localhost":         {},
			"127.0.0.1":         {},
			"admin.localhost":   {},
			"partner.localhost": {},
		}
	}
	log.Printf("Trusted hosts: %s", sortedHostList(allowedHosts))

	handler := app.Chain(
		buildHostRouter(allowedHosts, mainMux, adminMux, partnerMux, apiMux),
		buildMiddlewares(cfg, application)...,
	)

	return &httpServer{
		Drain: readiness.Drain,
		Server: &http.Server{
			Addr:              ":" + cfg.Port,
			Handler:           handler,
			IdleTimeout:       60 * time.Second,
			ReadTimeout:       15 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      15 * time.Second,
		},
	}, allowedHosts
}
