package ui

import (
	"html/template"
)

type Templates struct {
	Index      *template.Template
	Search     *template.Template
	Account    *template.Template
	Admin      *template.Template
	AdminLogin *template.Template
}

func ParseTemplates() (*Templates, error) {
	baseTemplateFiles := []string{
		"templates/base/base.tmpl.html",
		"templates/base/banner.tmpl.html",
		"templates/base/header.tmpl.html",
		"templates/base/nav.tmpl.html",
		"templates/base/main.tmpl.html",
		"templates/base/footer.tmpl.html",
	}

	base, err := template.ParseFS(TmplData, baseTemplateFiles...)
	if err != nil {
		return nil, err
	}

	indexTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/index.html")
	if err != nil {
		return nil, err
	}

	searchTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/search.html")
	if err != nil {
		return nil, err
	}

	accountTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/account.html")
	if err != nil {
		return nil, err
	}

	adminTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/admin.html")
	if err != nil {
		return nil, err
	}

	adminLoginTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/admin_login.html")
	if err != nil {
		return nil, err
	}

	return &Templates{
		Index:      indexTmpl,
		Search:     searchTmpl,
		Account:    accountTmpl,
		Admin:      adminTmpl,
		AdminLogin: adminLoginTmpl,
	}, nil
}
