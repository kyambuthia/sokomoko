package main

import (
	"context"
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
	"github.com/kyambuthia/sokomoko/internal/routes"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

func main() {
	cfg := config.LoadFromEnv()
	auth.SetEnvironment(cfg.Environment)
	auth.SetAdminSetupToken(cfg.AdminSetupToken)

	templates, err := ui.ParseTemplates()
	if err != nil {
		log.Fatalf("Error parsing templates: %v", err)
	}

	store, err := db.OpenStore(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	bootstrap.Initialize(store)

	a := app.New(store, templates, ui.StaticFS)

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
		app.Recoverer(),
		app.RequestID(),
		app.SecurityHeaders(),
		app.BodyLimit(1<<20),
		app.CSRFSameOrigin("session_token"),
		app.RequestLogger(),
	)

	srvr := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		IdleTimeout:       60 * time.Second,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	printStartupSummary(cfg, srvr, allowedHosts)

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
		log.Fatal(err)
	}

	<-idleConnsClosed
	log.Println("[shutdown] server stopped gracefully")
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

func printStartupSummary(cfg config.Config, server *http.Server, allowedHosts map[string]struct{}) {
	hosts := sortedHostList(allowedHosts)

	log.Printf("[startup] sokomoko booting")
	log.Printf("[startup] env=%s port=%s db=%s", cfg.Environment, cfg.Port, cfg.DBPath)
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
