package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/bootstrap"
	"github.com/kyambuthia/sokomoko/internal/config"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/notify"
	"github.com/kyambuthia/sokomoko/internal/routes"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

const (
	commandServe   = "serve"
	commandMigrate = "migrate"
	commandSeed    = "seed"
)

var ErrUsage = errors.New("invalid command usage")

type commandConfig struct {
	Name         string
	SeedOnServe  bool
	ShowHelpOnly bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stdout io.Writer) error {
	cmd, err := parseCommand(args)
	if err != nil {
		printUsage(stdout)
		return err
	}
	if cmd.ShowHelpOnly {
		printUsage(stdout)
		return nil
	}

	cfg := config.LoadFromEnv()
	switch cmd.Name {
	case commandMigrate:
		return runMigrate(cfg)
	case commandSeed:
		return runSeed(cfg)
	default:
		return runServe(cfg, cmd.SeedOnServe)
	}
}

func parseCommand(args []string) (commandConfig, error) {
	if len(args) == 0 {
		return commandConfig{Name: commandServe}, nil
	}

	switch args[0] {
	case "-h", "--help", "help":
		return commandConfig{ShowHelpOnly: true}, nil
	case commandServe:
		cmd := commandConfig{Name: commandServe}
		for _, arg := range args[1:] {
			if arg == "--seed" {
				cmd.SeedOnServe = true
				continue
			}
			return commandConfig{}, fmt.Errorf("%w: unsupported serve option %q", ErrUsage, arg)
		}
		return cmd, nil
	case commandMigrate, commandSeed:
		if len(args) > 1 {
			return commandConfig{}, fmt.Errorf("%w: %s does not accept extra arguments", ErrUsage, args[0])
		}
		return commandConfig{Name: args[0]}, nil
	default:
		return commandConfig{}, fmt.Errorf("%w: unknown command %q", ErrUsage, args[0])
	}
}

func runServe(cfg config.Config, seedOnServe bool) error {
	var resetEmailSender auth.PasswordResetEmailSender

	if cfg.SMTPHost != "" || cfg.SMTPFrom != "" {
		emailSender, senderErr := notify.NewSMTPSender(notify.SMTPConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
			AppName:  "Sokomoko",
		})
		if senderErr != nil {
			log.Printf("[startup] password_reset_email=disabled err=%v", senderErr)
		} else {
			resetEmailSender = emailSender.SendPasswordResetEmail
			log.Printf("[startup] password_reset_email=enabled host=%s port=%s from=%s", cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)
		}
	}

	templates, err := ui.ParseTemplates()
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	store, err := db.OpenStore(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.ApplySchema(); err != nil {
		return err
	}
	if seedOnServe {
		if err := bootstrap.Initialize(store); err != nil {
			return err
		}
	}

	authService := auth.NewService(store, auth.Config{
		Environment:              cfg.Environment,
		AdminSetupToken:          cfg.AdminSetupToken,
		SessionCookieDomain:      cfg.SessionCookieDomain,
		PasswordResetBaseURL:     cfg.PasswordResetBaseURL,
		PasswordResetEmailSender: resetEmailSender,
	})

	a := app.New(app.Dependencies{
		Readiness: store,
		Catalog:   catalogsvc.New(store),
		Account:   accountsvc.New(store),
		Commerce:  commerceSvc.New(store),
		Admin:     adminsvc.New(store),
		Partner:   partnersvc.New(store),
		Auth:      authService,
		Templates: templates,
		StaticFS:  ui.StaticFS,
	})

	mainMux := http.NewServeMux()
	adminMux := http.NewServeMux()
	partnerMux := http.NewServeMux()

	routes.RegisterPublic(a, mainMux)
	routes.RegisterAdmin(a, adminMux)
	routes.RegisterPartner(a, partnerMux)

	allowedHosts := parseAllowedHosts(cfg.AllowedHostsRaw)
	if len(allowedHosts) == 0 {
		allowedHosts = map[string]struct{}{
			"localhost":         {},
			"127.0.0.1":         {},
			"admin.localhost":   {},
			"partner.localhost": {},
		}
	}
	log.Printf("Trusted hosts: %s", strings.Join(sortedHostList(allowedHosts), ","))

	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := app.CanonicalHost(r.Host)
		if _, ok := allowedHosts[host]; !ok {
			http.Error(w, "Invalid host", http.StatusBadRequest)
			return
		}

		if strings.HasPrefix(host, "admin.") {
			adminMux.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(host, "partner.") {
			partnerMux.ServeHTTP(w, r)
			return
		}
		mainMux.ServeHTTP(w, r)
	})

	handler := app.Chain(
		router,
		buildMiddlewares(cfg)...,
	)

	srvr := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		IdleTimeout:       60 * time.Second,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	printStartupSummary(cfg, srvr, allowedHosts, seedOnServe)

	idleConnsClosed := make(chan struct{})
	cleanupDone := make(chan struct{})
	go runBackgroundCleanup(store, cleanupDone)
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		sig := <-sigint

		log.Printf("[shutdown] signal=%s received, shutting down server", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srvr.Shutdown(ctx); err != nil {
			log.Printf("[shutdown] graceful shutdown failed: %v", err)
		}

		close(cleanupDone)
		close(idleConnsClosed)
	}()

	if err := srvr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	<-idleConnsClosed
	log.Println("[shutdown] server stopped gracefully")
	return nil
}

func runMigrate(cfg config.Config) error {
	store, err := db.OpenStore(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.ApplySchema(); err != nil {
		return err
	}

	log.Printf("[startup] schema applied db=%s", cfg.DBPath)
	return nil
}

func runSeed(cfg config.Config) error {
	store, err := db.OpenStore(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.ApplySchema(); err != nil {
		return err
	}
	if err := bootstrap.Initialize(store); err != nil {
		return err
	}

	log.Printf("[startup] seed completed db=%s", cfg.DBPath)
	return nil
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  sokomoko serve [--seed]")
	_, _ = fmt.Fprintln(w, "  sokomoko migrate")
	_, _ = fmt.Fprintln(w, "  sokomoko seed")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  serve      Apply schema and start the HTTP server.")
	_, _ = fmt.Fprintln(w, "  migrate    Apply schema changes and exit.")
	_, _ = fmt.Fprintln(w, "  seed       Apply schema, run bootstrap seed workflows, and exit.")
}

func parseAllowedHosts(raw string) map[string]struct{} {
	hosts := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		host := app.CanonicalHost(strings.TrimSpace(part))
		if host != "" {
			hosts[host] = struct{}{}
		}
	}
	return hosts
}

func buildMiddlewares(cfg config.Config) []app.Middleware {
	middlewares := []app.Middleware{
		app.Recoverer(),
		app.RequestID(),
		app.SecurityHeaders(),
		app.BodyLimit(1 << 20),
		app.CSRFSameOrigin("session_token"),
	}

	if cfg.PostRateLimitMax > 0 {
		window := time.Duration(cfg.PostRateLimitWindow) * time.Second
		middlewares = append(middlewares, app.RateLimitByIP(cfg.PostRateLimitMax, window, http.MethodPost))
	}

	middlewares = append(middlewares, app.RequestLogger())
	return middlewares
}

func sortedHostList(hosts map[string]struct{}) []string {
	list := make([]string, 0, len(hosts))
	for host := range hosts {
		list = append(list, host)
	}
	sort.Strings(list)
	return list
}

func runBackgroundCleanup(store *db.Store, done <-chan struct{}) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	run := func() {
		if err := store.CleanupSessions(); err != nil {
			log.Printf("session cleanup failed: %v", err)
		}
		if err := store.CleanupPasswordResetTokens(); err != nil {
			log.Printf("password reset token cleanup failed: %v", err)
		}
	}

	run()
	for {
		select {
		case <-done:
			log.Println("[cleanup] background cleanup stopped")
			return
		case <-ticker.C:
			run()
		}
	}
}

func printStartupSummary(cfg config.Config, server *http.Server, allowedHosts map[string]struct{}, seedOnServe bool) {
	hosts := sortedHostList(allowedHosts)

	log.Printf("[startup] sokomoko booting")
	log.Printf("[startup] env=%s port=%s db=%s", cfg.Environment, cfg.Port, cfg.DBPath)
	log.Printf("[startup] seed_on_startup=%t", seedOnServe)
	if cfg.SessionCookieDomain != "" {
		log.Printf("[startup] session_cookie_domain=%s", cfg.SessionCookieDomain)
	}
	if cfg.PostRateLimitMax > 0 {
		log.Printf("[startup] post_rate_limit=%d requests/%ds", cfg.PostRateLimitMax, cfg.PostRateLimitWindow)
	}
	log.Printf("[startup] bind=%s", server.Addr)
	log.Printf("[startup] trusted_hosts=%s", strings.Join(hosts, ","))
	log.Printf("[startup] storefront=%s", localURL("localhost", cfg.Port))
	log.Printf("[startup] admin=%s", localURL("admin.localhost", cfg.Port))
	log.Printf("[startup] partner=%s", localURL("partner.localhost", cfg.Port))
	log.Printf("[startup] health=%s", localURL("localhost", cfg.Port)+"/healthz")
	log.Printf("[startup] ready=%s", localURL("localhost", cfg.Port)+"/readyz")
	log.Printf("[startup] press Ctrl+C to stop")
}

func localURL(host, port string) string {
	if port == "80" {
		return "http://" + host
	}
	return "http://" + host + ":" + port
}
