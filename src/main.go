package main

import (
	"io/fs"
	"fmt"
	"time"
	"log"
	"embed"
	"net/http"
	"html/template"
	"encoding/json"
)

//go:embed templates/*
var tmplData embed.FS

//go:embed static/*
var staticFS embed.FS

type SearchFormValues struct {
	query string `json:query`
}

func main() {
	mux := http.NewServeMux()

	// Static file handler - CSS, JS, IMGS.	
	staticSubFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	staticRouteHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS)))

	// Root Route Handler.
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		tmpl, err := template.ParseFS(
			tmplData, 
			"templates/layout.html",
			"templates/index.html",
			"templates/components/header.html",
			"templates/components/nav.html",
			"templates/components/footer.html",
		)

		if (err != nil) {
			log.Fatal(err)
		}

		err = tmpl.ExecuteTemplate(w, "root_template", "habari dunia") ; if (err != nil) {
			log.Fatal(err)
		}
	})

	// Root Route Handler.
	mux.HandleFunc("/search", func(w http.ResponseWriter, req *http.Request) {
		if (req.Method == "GET") {
			tmpl, err := template.ParseFS(
				tmplData, 
				"templates/layout.html",
				"templates/pages/search.html",
				"templates/components/header.html",
				"templates/components/nav.html",
				"templates/components/footer.html",
			)

			if (err != nil) {
				log.Fatal(err)
			}

			err = tmpl.ExecuteTemplate(w, "root_template", "habari dunia") ; if (err != nil) {
				log.Fatal(err)
			}
		} else if (req.Method == "POST") {
			var formData SearchFormValues
			err := json.NewDecoder(req.Body).Decode(&formData) ; if (err != nil) {
				http.Error(w, "Bad Request: " + err.Error(), http.StatusBadRequest)
				return
			}

			fmt.Printf("received JSON: %+v\n", formData)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "received: %s", formData.query)
		}
	})

	// Static File Handler.
	mux.Handle("/static/", staticRouteHandler)
	
	srvr := &http.Server {
		Addr: ":6969",
		Handler: mux,
		ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	fmt.Printf("server is running on PORT %s \nCTRL-C to EXIT\n", srvr.Addr)
	err = srvr.ListenAndServe() ; if (err != nil) {
		log.Fatal(err)
	}
}
