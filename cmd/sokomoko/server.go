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
	"github.com/kyambuthia/sokomoko/internal/config"
	"github.com/kyambuthia/sokomoko/internal/db"
)

type httpServer struct {
	*http.Server
}

func runHTTPServer(server *httpServer, store *db.Store) error {
	idleConnsClosed := make(chan struct{})
	cleanupDone := make(chan struct{})
	go runBackgroundCleanup(store, cleanupDone)
	go shutdownOnSignal(server.Server, cleanupDone, idleConnsClosed)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	<-idleConnsClosed
	log.Println("[shutdown] server stopped gracefully")
	return nil
}

func buildHostRouter(allowedHosts map[string]struct{}, mainMux, adminMux, partnerMux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
}

func shutdownOnSignal(server *http.Server, cleanupDone chan struct{}, idleConnsClosed chan struct{}) {
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
	sig := <-sigint

	log.Printf("[shutdown] signal=%s received, shutting down server", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[shutdown] graceful shutdown failed: %v", err)
	}

	close(cleanupDone)
	close(idleConnsClosed)
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

func buildMiddlewares(cfg config.Config, application *app.App) []app.Middleware {
	middlewares := []app.Middleware{
		app.Recoverer(),
		app.RequestID(),
		app.SecurityHeaders(),
		app.BodyLimit(1 << 20),
		application.Auth.CSRFMiddleware,
	}

	if cfg.PostRateLimitMax > 0 {
		window := time.Duration(cfg.PostRateLimitWindow) * time.Second
		middlewares = append(middlewares, app.RateLimitByIP(cfg.PostRateLimitMax, window, http.MethodPost))
	}

	middlewares = append(middlewares, app.RequestLogger())
	return middlewares
}

func sortedHostList(hosts map[string]struct{}) string {
	list := make([]string, 0, len(hosts))
	for host := range hosts {
		list = append(list, host)
	}
	sort.Strings(list)
	return strings.Join(list, ",")
}

func runBackgroundCleanup(store *db.Store, done <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
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

func printStartupSummary(cfg config.Config, server *httpServer, allowedHosts map[string]struct{}, seedOnServe bool) {
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
	log.Printf("[startup] trusted_hosts=%s", sortedHostList(allowedHosts))
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
