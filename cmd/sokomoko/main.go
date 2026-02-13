package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/routes"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

func main() {
	templates, err := ui.ParseTemplates()
	if err != nil {
		log.Fatalf("Error parsing templates: %v", err)
	}

	store, err := db.OpenStoreFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	a := app.New(store, templates, ui.StaticFS)

	mainMux := http.NewServeMux()
	adminMux := http.NewServeMux()
	partnerMux := http.NewServeMux()

	routes.RegisterPublic(a, mainMux)
	routes.RegisterAdmin(a, adminMux)
	routes.RegisterPartner(a, partnerMux)

	allowedHosts := parseAllowedHosts(os.Getenv("ALLOWED_HOSTS"))
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "6969"
	}

	srvr := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		IdleTimeout:       60 * time.Second,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	fmt.Printf("\n --- RUNNING --- \n server is listening on PORT %s \nCTRL-C to EXIT\n", srvr.Addr)

	idleConnsClosed := make(chan struct{})
	cleanupDone := make(chan struct{})
	go runBackgroundCleanup(store, cleanupDone)
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		log.Println("Server is shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srvr.Shutdown(ctx); err != nil {
			log.Printf("Error during server shutdown: %v", err)
		}

		close(cleanupDone)
		close(idleConnsClosed)
	}()

	if err := srvr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}

	<-idleConnsClosed
	log.Println("Server stopped gracefully")
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
			return
		case <-ticker.C:
			run()
		}
	}
}
