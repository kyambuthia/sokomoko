package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/routes"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

var (
	baseTemplate, searchTemplate, authTemplate *template.Template
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
}

func main() {
	db.InitDB()
	defer db.DB.Close()

	mux := http.NewServeMux()

	// Root Route Handler.
	mux.HandleFunc("/", routes.Root(baseTemplate))

	mux.HandleFunc("/search", routes.Search(searchTemplate))

	mux.HandleFunc("/account", routes.Auth(authTemplate))

	// Admin Routes - Simple dashboard for now
	mux.HandleFunc("/admin/", func(w http.ResponseWriter, r *http.Request) {
		// Simple admin dashboard for now
		w.Write([]byte(`
			<!DOCTYPE html>
			<html>
			<head><title>Admin Dashboard</title></head>
			<body>
				<h1>Admin Dashboard</h1>
				<p>Welcome to Admin Panel</p>
				<ul>
					<li><a href="/admin/products">Product Management</a></li>
					<li><a href="/admin/orders">Order Management</a></li>
					<li><a href="/admin/reports">Sales Reports</a></li>
					<li><a href="/admin/deliveries">Delivery Management</a></li>
				</ul>
				<p><a href="/">Back to Store</a></p>
			</body>
			</html>
		`))
	})

	// Static File Handler.
	mux.Handle("/static/", routes.Static(ui.StaticFS))

	port := os.Getenv("PORT")
	if port == "" {
		port = "6969"
	}

	srvr := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	fmt.Printf("\n --- RUNNING --- \n server is listening on PORT %s \nCTRL-C to EXIT\n", srvr.Addr)

	err := srvr.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
