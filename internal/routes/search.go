package routes

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type SearchFormValues struct {
	QueryString string `json:"queryString"`
}

// item
type Item struct {
	TimeCreated time.Time `json:"time_created"`
	TimeUpdated time.Time `time:"time_updated"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Price       int       `json:"price"`
	ImgURI      string    `json:"imgUri"`
}

func Search(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		// HANDLE GET REQUESTS -  /search ROUTE
		case "GET":
			query := req.URL.Query().Get("q")
			if query != "" {
				// Perform search
				products, err := db.SearchProducts(strings.TrimSpace(query))
				if err != nil {
					log.Printf("Error searching products: %v", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}

				// Build template data
				data := map[string]interface{}{
					"Query":     query,
					"Products":  products,
					"NoResults": len(products) == 0,
				}

				err = tmpl.ExecuteTemplate(w, "root_template", data)
				if err != nil {
					log.Fatal(err)
				}
			} else {
				data := map[string]interface{}{
					"Query":     "",
					"Products":  []db.Product{},
					"NoResults": false,
				}
				err := tmpl.ExecuteTemplate(w, "root_template", data)
				if err != nil {
					log.Fatal(err)
				}
			}

		// HANDLE POST REQUESTS - /search ROUTE
		case "POST":
			reqBody, err := io.ReadAll(req.Body)
			if err != nil {
				fmt.Fprintf(os.Stdout, "Unable to read request body: err %v", err)
				http.Error(w, "Unable to read request body: "+err.Error(), http.StatusBadRequest)
				return
			}

			// read form data
			var formData SearchFormValues
			err = json.Unmarshal(reqBody, &formData)
			if err != nil {
				http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
				return
			}

			// create an item object.
			item01 := Item{
				TimeCreated: time.Now(),
				TimeUpdated: time.Now(),
				Name:        "television",
				Category:    "electronics/televisions/digital_televisions",
				Price:       499000,
				ImgURI:      "/static/images/product-images/television.png",
			}

			fmt.Fprintf(os.Stdout, "name: %s\n category: %s\nprice: %d\n", item01.Name, item01.Category, item01.Price)

			// -- print out to STDOUT
			fmt.Fprintf(os.Stdout, "received JSON: %v \n", formData.QueryString)
			fmt.Fprintf(w, "received on the server side: %s", formData.QueryString)
		}
	}
}
