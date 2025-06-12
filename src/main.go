package main

import (
	"fmt"
	"time"
	"log"
	"embed"
	"net/http"
	"html/template"
)

//go:embed templates/*
var tmplData embed.FS

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		tmpl, err := template.ParseFS(tmplData, "templates/indx.html") ; if (err != nil) {
			log.Fatal(err)
		}

		tmpl.ExecuteTemplate(w, "root_template", "habari dunia")
	})
	
	srvr := &http.Server {
		Addr: ":6969",
		Handler: mux,
		ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	err := srvr.ListenAndServe()
	fmt.Printf("server is running on https://localhost/$s", srvr.Addr)

	if (err != nil) {
		log.Fatal(err)
	}
}
