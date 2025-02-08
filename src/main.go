package main

import (
        "io"
        "os"
        "log"
        "net/http"
        "html/template"
)

func main() {
        mux := http.NewServeMux()

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

        // serve static assets (CSS/JS/IMGS)
        // SECURITY HOLE - fileserver
        fs := http.FileServer(http.Dir("./static/"))
        mux.Handle("/static/", http.StripPrefix("/static/", fs))

        // route - /search
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
                tmpl.ExecuteTemplate(w, "root_template", "search")
        })

        // route -> /account
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
                tmpl.ExecuteTemplate(w, "root_template", "cart")
        })

        // route -> /account
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
                tmpl.ExecuteTemplate(w, "root_template", "account")
        })

        // route -> /delivery
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
                tmpl.ExecuteTemplate(w, "root_template", "contact page")
        })

        // route -> /legal
        mux.HandleFunc("/legal", func(w http.ResponseWriter, req *http.Request) {
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

        io.WriteString(os.Stdout, "Server is now running at address: http://127.0.0.1:6969 \nCTRL-C to stop the server.\n")
        http.ListenAndServe(":6969", mux)
}
