package main

import (
        "io"
        "os"
        "fmt"
        "log"
        "html/template"
        "net/http"
        "time"
        "context"

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
        Name string
        Phonenumber string
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
        io.WriteString(os.Stdout, version)
        defer db.Close()

        //mux
        mux := http.NewServeMux()

        mux.HandleFunc("/names", func(w http.ResponseWriter, req *http.Request) {
                ctx := context.Background()
                var result []Persona 

                rows, err := db.QueryContext(ctx, `SELECT * FROM names;`); if (err != nil) {
                        log.Fatal(err)
                }
                defer rows.Close()

                // iterate throught the rows, append to queryResult slice.
                for rows.Next() {
                        var item Persona
                        if err := rows.Scan(&item.Name, &item.Phonenumber); err != nil {
                                log.Fatal(err)
                        }
                        result = append(result, item)
                }

                if err := rows.Err(); err != nil {
                        log.Fatal(err)
                }

                tmpl, err := template.ParseFiles("./templates/items.html"); if err != nil {
                        log.Fatal(err)
                }

                tmpl.Execute(w, result)
        })

        mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
                products := []Product{
                    {"Cocaine", 99, "Health"},
                    {"Heroin", 120, "Electronics"},
                    {"Apple Juice", 60, "Juice"},
                }
                tmpl, err := template.ParseFiles("./templates/index.html")
                if (err != nil) {
                        log.Fatal(err)
                }
                var productList []Product = products[0:3]
                tmpl.Execute(w, productList)
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
