package main

import (
        "log"
        "text/template"
        "net/http"
        "time"
)

type Product struct {
        Name string     `json:"name"`
        Price float64   `json:"price"`
        Category string `json:"category"`
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
        p := Product{"Azimia", 60.00, "mineral drinking water"}
        t, err := template.New("tmpl").Parse("{{.Name}} {{.Price}} {{.Category}}")
        if (err != nil) {
                log.Fatal("Error: template parsing has failed")
        }
        t.Execute(w, p)
}

func main() {
        mux := http.NewServeMux()

        mux.HandleFunc("/checkout", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.New("aboutPageTmpl").Parse(`{{define "T"}}, Hello, {{.}}!{{end}}`)
                if (err!=nil) {
                        log.Fatal(err)
                }
                tmpl.ExecuteTemplate(w, "T", "<script>alert("You've been pawned")</script>")
        })


        mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
                p := Product{"Azimia", 60.00, "mineral drinking water"}
                t, err := template.New("tmpl").Parse("{{.Name}} {{.Price}} {{.Category}}")

                if (err != nil) {
                        log.Fatal(err)
                }
                t.Execute(w, p)
        })

        srv := &http.Server{
                Handler: mux,
                Addr: "127.0.0.1:8000",
                WriteTimeout: 15 * time.Second,
                ReadTimeout: 15 * time.Second,
        }

        log.Fatal(srv.ListenAndServe())
}
