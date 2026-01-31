package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
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

	routes.RegisterPublic(a, mainMux)
	routes.RegisterAdmin(a, adminMux)

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
