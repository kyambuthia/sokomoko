package main

import (
	"fmt"
	"html/template"
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

var (
	baseTemplate, searchTemplate, authTemplate, adminTemplate, adminLoginTemplate *template.Template
)

func init() {
	var err error

	baseTemplateFiles := []string{
		"templates/base/base.tmpl.html",
		"templates/base/banner.tmpl.html",
		"templates/base/header.tmpl.html",
		"templates/base/nav.tmpl.html",
		"templates/base/main.tmpl.html",
		"templates/base/footer.tmpl.html",
	}

	// Parse the base template once
	base, err := template.ParseFS(ui.TmplData, baseTemplateFiles...)
	if err != nil {
		log.Fatalf("Error parsing base templates: %v", err)
	}

	// Clone the base template and parse additional files for other routes
	baseTemplate, err = template.Must(base.Clone()).ParseFS(ui.TmplData, "templates/pages/index.html")
	if err != nil {
		log.Fatalf("Error parsing index template: %v", err)
	}

	searchTemplate, err = template.Must(base.Clone()).ParseFS(ui.TmplData, "templates/pages/search.html")
	if err != nil {
		log.Fatalf("Error parsing search template: %v", err)
	}

	authTemplate, err = template.Must(base.Clone()).ParseFS(ui.TmplData, "templates/pages/account.html")
	if err != nil {
		log.Fatalf("Error parsing account template: %v", err)
	}

	adminTemplate, err = template.Must(base.Clone()).ParseFS(ui.TmplData, "templates/pages/admin.html")
	if err != nil {
		log.Fatalf("Error parsing admin template: %v", err)
	}

	adminLoginTemplate, err = template.Must(base.Clone()).ParseFS(ui.TmplData, "templates/pages/admin_login.html")
	if err != nil {
		log.Fatalf("Error parsing admin login template: %v", err)
	}
}

func main() {
	db.InitDB()
	defer db.DB.Close()

	mainMux := http.NewServeMux()
	adminMux := http.NewServeMux()

	// Main Site Routes
	mainMux.HandleFunc("/", routes.Root(baseTemplate))
	mainMux.HandleFunc("/search", routes.Search(searchTemplate))
	mainMux.HandleFunc("/account", routes.Auth(authTemplate))
	mainMux.Handle("/static/", routes.Static(ui.StaticFS))

	// Admin Site Routes
	adminMux.HandleFunc("/login", auth.AdminLogin(adminLoginTemplate))

	// Protected Admin Routes
	adminHandlers := http.NewServeMux()
	adminHandlers.HandleFunc("/", routes.AdminDashboard(adminTemplate))
	adminHandlers.HandleFunc("/products", routes.AdminProducts(adminTemplate))
	adminHandlers.HandleFunc("/orders", routes.AdminOrders(adminTemplate))
	adminHandlers.HandleFunc("/reports", routes.AdminReports(adminTemplate))
	adminHandlers.HandleFunc("/deliveries", routes.AdminDeliveries(adminTemplate))

	// Wrap with Auth Middleware
	protectedAdmin := auth.AuthMiddleware(auth.RequireRole("admin", adminHandlers))
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

	err := srvr.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
