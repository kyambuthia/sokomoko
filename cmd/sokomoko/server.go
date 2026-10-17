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
	"github.com/kyambuthia/sokomoko/internal/config"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/lifecycle"
)

type httpServer struct {
	*http.Server
	Drain func()
}

func runHTTPServer(server *httpServer, store *db.Store) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := lifecycle.Run(ctx, server.Server, server.Drain, func(ctx context.Context) {
		lifecycle.Maintain(ctx, store)
	})
	if err == nil {
		log.Println("[shutdown] server stopped gracefully")
	}
	return err
}

func buildHostRouter(allowedHosts map[string]struct{}, mainMux, adminMux, partnerMux, apiMux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := app.CanonicalHost(r.Host)
		if _, ok := allowedHosts[host]; !ok {
			http.Error(w, "Invalid host", http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/") {
			apiMux.ServeHTTP(w, r)
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
		app.RequestID(),
		app.RequestLogger(),
		app.Recoverer(),
		app.SecurityHeadersWithProxy(cfg.TrustProxyHeaders),
		app.BodyLimit(1 << 20),
		application.Auth.CSRFMiddleware,
	}

	if cfg.PostRateLimitMax > 0 {
		window := time.Duration(cfg.PostRateLimitWindow) * time.Second
		clientIP := func(r *http.Request) string { return auth.ClientIP(r, cfg.TrustProxyHeaders) }
		middlewares = append(middlewares, app.RateLimitByKey(cfg.PostRateLimitMax, window, clientIP, http.MethodPost))
	}

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

func printStartupSummary(cfg config.Config, server *httpServer, allowedHosts map[string]struct{}, seedOnServe bool) {
	log.Printf("[startup] sokomoko %s booting", version)
	log.Printf("[startup] env=%s port=%s schema_version=%d", cfg.Environment, cfg.Port, db.LatestSchemaVersion())
	log.Printf("[startup] seed_on_startup=%t", seedOnServe)
	if cfg.SessionCookieDomain != "" {
		log.Printf("[startup] session_cookie_domain=%s", cfg.SessionCookieDomain)
	}
	if cfg.PostRateLimitMax > 0 {
		log.Printf("[startup] post_rate_limit=%d requests/%ds", cfg.PostRateLimitMax, cfg.PostRateLimitWindow)
	}
	if cfg.AuthAbuseMaxFailures > 0 {
		log.Printf("[startup] auth_abuse_limit=enabled max_failures=%d backoff_base=%ds backoff_max=%ds", cfg.AuthAbuseMaxFailures, cfg.AuthAbuseBackoffBase, cfg.AuthAbuseBackoffMax)
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
