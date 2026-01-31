package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/auth"
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

	mainMux := http.NewServeMux()
	adminMux := http.NewServeMux()

	// Main Site Routes
	mainMux.HandleFunc("/", routes.Root(store, templates.Index))
	mainMux.HandleFunc("/search", routes.Search(store, templates.Search))
	mainMux.HandleFunc("/account", routes.Auth(templates.Account))
	mainMux.Handle("/static/", routes.Static(ui.StaticFS))

	// Admin Site Routes
	adminMux.HandleFunc("/login", auth.AdminLogin(store, templates.AdminLogin))

	// Protected Admin Routes
	adminHandlers := http.NewServeMux()
	adminHandlers.HandleFunc("/", routes.AdminDashboard(templates.Admin))
	adminHandlers.HandleFunc("/products", routes.AdminProducts(templates.Admin))
	adminHandlers.HandleFunc("/orders", routes.AdminOrders(templates.Admin))
	adminHandlers.HandleFunc("/reports", routes.AdminReports(templates.Admin))
	adminHandlers.HandleFunc("/deliveries", routes.AdminDeliveries(templates.Admin))

	// Wrap with Auth Middleware
	protectedAdmin := auth.AuthMiddleware(store, auth.RequireRole("admin", adminHandlers))
	adminMux.Handle("/", protectedAdmin)
	adminMux.Handle("/static/", routes.Static(ui.StaticFS))

	// Subdomain Router
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if strings.HasPrefix(host, "admin.") {
			adminMux.ServeHTTP(w, r)
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

	err = srvr.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
