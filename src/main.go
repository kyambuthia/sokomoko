package main

import (
	"fmt"
	"time"
	"log"
	"embed"
	"net/http"
	"html/template"

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

	baseTemplateFiles := []string {
		"templates/base/base.tmpl.html",
		"templates/base/banner.tmpl.html",
		"templates/base/header.tmpl.html",
		"templates/base/nav.tmpl.html",
		"templates/base/main.tmpl.html",
		"templates/base/footer.tmpl.html",
	}

	rootTemplateFiles := append(baseTemplateFiles, "templates/pages/index.html")

	searchRouteTemplateFiles := append(baseTemplateFiles, "templates/pages/search.html")

	authRouteTemplateFiles := append(baseTemplateFiles, "templates/pages/account.html")
	
	baseTemplate, err = template.ParseFS(tmplData, rootTemplateFiles...); if err != nil {
		log.Fatal(err)
	}

	searchTemplate, err = template.ParseFS(tmplData, searchRouteTemplateFiles...); if err != nil {
		log.Fatal(err)
	}

	authTemplate, err = template.ParseFS(tmplData, authRouteTemplateFiles...); if err != nil {
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
	
	srvr := &http.Server {
		Addr: ":6969",
		Handler: mux,
		ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	fmt.Printf("\n --- RUNNING --- \n server is listening on PORT %s \nCTRL-C to EXIT\n", srvr.Addr)

	err := srvr.ListenAndServe(); if (err != nil) {
		log.Fatal(err)
	}
}
