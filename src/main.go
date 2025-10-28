package main

import (
	"fmt"
	"time"
	"log"
	"embed"
	"net/http"
	"html/template"
	"os"

	"github.com/kyambuthia/sokomoko/src/db"
	"github.com/kyambuthia/sokomoko/src/routes"
)

//go:embed templates/*
var tmplData embed.FS

//go:embed static/*
var staticFS embed.FS

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
	base, err := template.ParseFS(tmplData, baseTemplateFiles...)
	if err != nil {
		log.Fatal(err)
	}

	// Clone the base template and parse additional files for other routes
	baseTemplate, err = template.Must(base.Clone()).ParseFS(tmplData, "templates/pages/index.html")
	if err != nil {
		log.Fatal(err)
	}

	searchTemplate, err = template.Must(base.Clone()).ParseFS(tmplData, "templates/pages/search.html")
	if err != nil {
		log.Fatal(err)
	}

	authTemplate, err = template.Must(base.Clone()).ParseFS(tmplData, "templates/pages/account.html")
	if err != nil {
		log.Fatal(err)
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

	// Static File Handler.
	mux.Handle("/static/", routes.Static(staticFS))

	port := os.Getenv("PORT")
	if port == "" {
		port = "6969"
	}
	
srvr := &http.Server {
		Addr: ":" + port,
		Handler: mux,
		ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	fmt.Printf("\n --- RUNNING --- \n server is listening on PORT %s \nCTRL-C to EXIT\n", srvr.Addr)

	err := srvr.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
