package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/bootstrap"
	"github.com/kyambuthia/sokomoko/internal/config"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/notify"
	"github.com/kyambuthia/sokomoko/internal/routes"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

var runHTTPServerFunc = runHTTPServer

func runServe(cfg config.Config, seedOnServe bool) error {
	resetEmailSender, err := buildPasswordResetEmailSender(cfg)
	if err != nil {
		return wrapCommandError(commandServe, "configure password reset email", err)
	}

	templates, err := ui.ParseTemplates()
	if err != nil {
		return wrapCommandError(commandServe, "parse templates", fmt.Errorf("parse templates: %w", err))
	}

	store, err := db.OpenStore(cfg.DBPath)
	if err != nil {
		return wrapCommandError(commandServe, "open store", err)
	}
	defer store.Close()

	bootstrapService := bootstrap.New(store)
	if err := bootstrapService.Prepare(seedOnServe); err != nil {
		return wrapCommandError(commandServe, "prepare store", err)
	}

	application := app.Compose(app.ComposeOptions{
		Store:     store,
		Templates: templates,
		StaticFS:  ui.StaticFS,
		Auth: auth.Config{
			Environment:              cfg.Environment,
			AdminSetupToken:          cfg.AdminSetupToken,
			SessionCookieDomain:      cfg.SessionCookieDomain,
			AuthAbuseMaxFailures:     cfg.AuthAbuseMaxFailures,
			AuthAbuseBackoffBase:     time.Duration(cfg.AuthAbuseBackoffBase) * time.Second,
			AuthAbuseBackoffMax:      time.Duration(cfg.AuthAbuseBackoffMax) * time.Second,
			PasswordResetBaseURL:     cfg.PasswordResetBaseURL,
			PasswordResetEmailSender: resetEmailSender,
		},
	})
	server, allowedHosts := buildServer(cfg, application)
	printStartupSummary(cfg, server, allowedHosts, seedOnServe)

	return wrapCommandError(commandServe, "serve http", runHTTPServerFunc(server, store))
}

func runMigrate(cfg config.Config) error {
	store, err := db.OpenStore(cfg.DBPath)
	if err != nil {
		return wrapCommandError(commandMigrate, "open store", err)
	}
	defer store.Close()

	if err := bootstrap.New(store).Migrate(); err != nil {
		return wrapCommandError(commandMigrate, "apply schema", err)
	}

	log.Printf("[startup] schema applied db=%s", cfg.DBPath)
	return nil
}

func runSeed(cfg config.Config) error {
	store, err := db.OpenStore(cfg.DBPath)
	if err != nil {
		return wrapCommandError(commandSeed, "open store", err)
	}
	defer store.Close()

	if err := bootstrap.New(store).Prepare(true); err != nil {
		return wrapCommandError(commandSeed, "prepare store", err)
	}

	log.Printf("[startup] seed completed db=%s", cfg.DBPath)
	return nil
}

func buildPasswordResetEmailSender(cfg config.Config) (auth.PasswordResetEmailSender, error) {
	var resetEmailSender auth.PasswordResetEmailSender
	if cfg.SMTPHost == "" && cfg.SMTPFrom == "" {
		return nil, nil
	}

	emailSender, err := notify.NewSMTPSender(notify.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
		AppName:  "Sokomoko",
	})
	if err != nil {
		log.Printf("[startup] password_reset_email=disabled err=%v", err)
		return nil, nil
	}

	resetEmailSender = emailSender.SendPasswordResetEmail
	log.Printf("[startup] password_reset_email=enabled host=%s port=%s from=%s", cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)
	return resetEmailSender, nil
}

func buildServer(cfg config.Config, application *app.App) (*httpServer, map[string]struct{}) {
	mainMux := http.NewServeMux()
	adminMux := http.NewServeMux()
	partnerMux := http.NewServeMux()

	routes.RegisterPublic(application, mainMux)
	routes.RegisterAdmin(application, adminMux)
	routes.RegisterPartner(application, partnerMux)

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
		buildHostRouter(allowedHosts, mainMux, adminMux, partnerMux),
		buildMiddlewares(cfg, application)...,
	)

	return &httpServer{
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
