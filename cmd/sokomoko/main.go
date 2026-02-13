package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	// Subdomain Router
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if strings.Contains(host, ":") {
			host = strings.Split(host, ":")[0]
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "6969"
	}

	srvr := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	fmt.Printf("\n --- RUNNING --- \n server is listening on PORT %s \nCTRL-C to EXIT\n", srvr.Addr)

	idleConnsClosed := make(chan struct{})
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

		close(idleConnsClosed)
	}()

	if err := srvr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}

	<-idleConnsClosed
	log.Println("Server stopped gracefully")
}
