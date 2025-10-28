package routes

import (
	"log"
	"net/http"
	"html/template"
)

func Auth(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		err := tmpl.ExecuteTemplate(w, "root_template", "authentication route"); if err != nil {
			log.Fatal(err)
		}
	}
}