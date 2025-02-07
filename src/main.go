package main

import (
        "log"
        "net/http"
        "html/template"
)

func main() {
        mux := http.NewServeMux()

        fs := http.FileServer(http.Dir("./testy/"))
        mux.Handle("/static", http.StripPrefix("/static/", fs))

        mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./templates/layout.html",
                        "./templates/components/nav.html",
                        "./templates/components/footer.html",
                        "./templates/index.html",
                )
                if err != nil {
                        log.Fatal(err)
                }
                tmpl.ExecuteTemplate(w, "root_template", "Karibu, Sokomoko")
        })
 

        mux.HandleFunc("/account", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.New("tmpl").Parse(`{{define "about_template"}}about Page - {{.}}{{end}}`)
                if err != nil {
                        log.Fatal(err)
                }
                tmpl.ExecuteTemplate(w, "about_template", "Hello, world")
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
                tmpl.ExecuteTemplate(w, "root_template", "contact page")
        })

        http.ListenAndServe(":6969", mux)
}
