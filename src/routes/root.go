package routes

import (
	"html/template"
	"log"
	"net/http"
)

func Root(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		err := tmpl.ExecuteTemplate(w, "root_template", "root route")
		if err != nil {
			log.Fatal(err)
		}
	}
}