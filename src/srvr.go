package main

import (
        "log"
        "html/template"
        "net/http"
        "time"
)

type Product struct {
        Name string     `json:"name"`
        Price float64   `json:"price"`
        Category string `json:"category"`
}

func main() {
        mux := http.NewServeMux()

        mux.HandleFunc("/checkout", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.New("aboutPageTmpl").Parse(`{{define "T"}}, Hello, {{.}}! <a href="./checkout"></a>{{end}}`)
                if (err!=nil) {
                        log.Fatal(err)
                }
                tmpl.ExecuteTemplate(w, "T", "<script>alert('Youve been pawned')</script>")
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

        log.Fatal(srv.ListenAndServe())
}
