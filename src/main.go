package main

import (
	"os"
	"io"
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
	QueryString string `json:"queryString"`
}

// item
type Item struct {
	TimeCreated time.Time `json:"time_created"`
	TimeUpdated time.Time `time:"time_updated"`
	Name string `json:"name"`
	Category string `json:"category"`
	Price int `json:"price"`
	ImgURI string `json:"imgUri"`
}

// update item values
func (i *Item) update(name *string, category *string, price *int, imgURI *string) {
	i.TimeUpdated = time.Now()

	if name != nil {
		i.Name = *name
	}
	if category != nil {
		i.Category = *category
	}
	if price != nil {
		i.Price = *price
	}
	if imgURI != nil {
		i.ImgURI = *imgURI
	}
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

	// /search Route Handler.
	mux.HandleFunc("/search", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
			// HANDLE GET REQUESTS -  /search ROUTE
			case "GET":
				tmpl, err := template.ParseFS(
					tmplData, 
					"templates/layout.html",
					"templates/pages/search.html",
					"templates/components/header.html",
					"templates/components/nav.html",
					"templates/components/footer.html",
				)

				if (err != nil) {
					log.Fatal(err) }

				err = tmpl.ExecuteTemplate(w, "root_template", "habari dunia") ; if (err != nil) {
					log.Fatal(err)
				}

			// HANDLE POST REQUESTS - /search ROUTE
			case "POST":
				reqBody, err := io.ReadAll(req.Body) ; if err != nil {
					fmt.Fprintf(os.Stdout, "Unable to read request body: err", err)
					http.Error(w, "Unable to read request body: " + err.Error(), http.StatusBadRequest)
					return
				}

				// read form data
				var formData SearchFormValues
				err = json.Unmarshal(reqBody, &formData) ; if err != nil {
					http.Error(w, "Bad Request: " + err.Error(), http.StatusBadRequest)
					return
				}

				// create an item object.
				item01 := Item {
					TimeCreated: time.Now(), 
					TimeUpdated:time.Now(), 
					Name: "television", 
						Category: "electronics/televisions/digital_televisions", 
					Price: 499000, 
					ImgURI: "/static/images/product-images/television.png",
				}

				fmt.Fprintf(os.Stdout, "name: %s\n category: %s\nprice: %d\n", item01.Name, item01.Category, item01.Price)

				// -- print out to STDOUT
				fmt.Fprintf(os.Stdout, "received JSON: %v \n", formData.QueryString)
				fmt.Fprintf(w, "received on the server side: %s", formData.QueryString)
			}})

	// Static File Handler.
	mux.Handle("/static/", staticRouteHandler)
	
	srvr := &http.Server {
		Addr: ":6969",
		Handler: mux,
		ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	fmt.Printf("server is listening on PORT %s \nCTRL-C to EXIT\n", srvr.Addr)
	err = srvr.ListenAndServe() ; if (err != nil) {
		log.Fatal(err)
	}
}
