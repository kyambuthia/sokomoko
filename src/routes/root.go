package routes

func LoadTemplates() *template.Template {
	tmpl, err := template.ParseFS(
		templateData,
		"templates/layout.html".
		"templates/index.html",
	)
	if (err != nil) {
		log.Fatal(err)
	}
}


