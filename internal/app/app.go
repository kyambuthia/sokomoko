package app

import (
	"embed"
	"html/template"
	"log"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

type App struct {
	Store     *db.Store
	Templates *ui.Templates
	StaticFS  embed.FS
}

func New(store *db.Store, templates *ui.Templates, staticFS embed.FS) *App {
	return &App{Store: store, Templates: templates, StaticFS: staticFS}
}

func (a *App) Render(w http.ResponseWriter, tmpl *template.Template, data any) {
	err := tmpl.ExecuteTemplate(w, "root_template", data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
		RenderErrorPage(w, nil, http.StatusInternalServerError, "Internal Server Error", "We could not render this page.")
		return
	}
}
