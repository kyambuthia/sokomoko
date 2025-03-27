package main

import (
        "fmt"
        "log"
        "html/template"
        "net/http"
        "time"

        "database/sql"
        _ "github.com/ncruces/go-sqlite3/driver"
        _ "github.com/ncruces/go-sqlite3/embed"
)

type Product struct {
        Name string     `json:"name"`
        Price float64   `json:"price"`
        Category string `json:"category"`
}

type Persona struct {
	Name string		`json: "name"`
	Phonenumber string	`json: "phonenumber"`
}

type Item struct {
        Title string    `json: "title"`
        Type string     `json: "type"`
        Category string `json: "category"`
        Price float64   `json: "Price"`
        Quantity int    `json: "quantity"`
        Note string     `json: "note"`
}

type Blog struct {
        Title string `json:"title"`
        Author string `json:"author"`
        Created time.Time `json: "created"`
        content string `json: "content"`
}

var db *sql.DB
var version string

func main() {
        db, err := sql.Open("sqlite3", "../db/t.db"); if (err != nil) {
                log.Fatal(err)
        }

        db.QueryRow(`SELECT sqlite_version()`).Scan(&version)

        fmt.Printf("SQLLITE DB VERSION %s - Up and running", version)
        defer db.Close()

        //mux
        mux := http.NewServeMux()

	// serving static files (CSS, JS)
        staticFS := http.FileServer(http.Dir("./static"))
        mux.Handle("/static/", http.StripPrefix("/static/", staticFS))

        mux.HandleFunc("/search", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./templates/layout.html",
                        "./templates/components/nav.html", 
			"./templates/components/footer.html",
                        "./templates/pages/search.html",
                )
                      
                if err != nil {
                        log.Fatal(err)
                }

                tmpl.ExecuteTemplate(w, "root_template", "this is the search page")
        })

        mux.HandleFunc("/account", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./templates/layout.html",
                        "./templates/components/nav.html", 
			"./templates/components/footer.html",
                        "./templates/pages/account.html",
                )
                      
                if err != nil {
                        log.Fatal(err)
                }

                tmpl.ExecuteTemplate(w, "root_template", "this is the account page")
        })
         mux.HandleFunc("/cart", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./templates/layout.html",
                        "./templates/components/nav.html", 
			"./templates/components/footer.html",
                        "./templates/pages/cart.html",
                )
                      
                if err != nil {
                        log.Fatal(err)
                }

                tmpl.ExecuteTemplate(w, "root_template", "this is the cart page")
        })
        mux.HandleFunc("/checkout", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./templates/layout.html",
                        "./templates/components/nav.html", 
			"./templates/components/footer.html",
                        "./templates/pages/checkout.html",
                )
                      
                if err != nil {
                        log.Fatal(err)
                }

                tmpl.ExecuteTemplate(w, "root_template", "this is the checkout page")
        })

        mux.HandleFunc("/delivery", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./templates/layout.html",
                        "./templates/components/nav.html", 
			"./templates/components/footer.html",
                        "./templates/pages/delivery.html",
                )
                      
                if err != nil {
                        log.Fatal(err)
                }

                tmpl.ExecuteTemplate(w, "root_template", "this is the checkout page")
        })
 
        mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./templates/layout.html",
                        "./templates/components/nav.html",
                        "./templates/components/footer.html",
                        "./templates/index.html",
                )
                if (err != nil) {
                        log.Fatal(err)
                }

		showcase_items_query := `
		SELECT * FROM names;
		`

		rows, err := db.Query(showcase_items_query)
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()
		
		var persona_array[] Persona
		for rows.Next() {
			var persona Persona
			if err := rows.Scan(&persona.Name, &persona.Phonenumber); err != nil {
				log.Fatal(err)
			}

			persona_array = append(persona_array, Persona{persona.Name, persona.Phonenumber})
		}

                tmpl.ExecuteTemplate(w, "root_template", persona_array)
        })
         
        srv := &http.Server{
                Handler: mux,
                Addr: "127.0.0.1:8000",
                WriteTimeout: 15 * time.Second,
                ReadTimeout: 15 * time.Second,
        }

        fmt.Printf("\n---\nServer is running locally at ADDR: http://%s \nCTRL-C to stop the server\n", srv.Addr)

        log.Fatal(srv.ListenAndServe())
}
