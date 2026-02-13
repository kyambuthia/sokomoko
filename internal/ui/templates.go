package ui

import (
	"html/template"
)

type Templates struct {
	Index                *template.Template
	Search               *template.Template
	Account              *template.Template
	Admin                *template.Template
	AdminLogin           *template.Template
	AdminSetup           *template.Template
	StaffSignup          *template.Template
	Login                *template.Template
	Signup               *template.Template
	PasswordResetRequest *template.Template
	PasswordResetConfirm *template.Template
}

func ParseTemplates() (*Templates, error) {
	funcMap := template.FuncMap{
		"asset": AssetURL,
	}

	baseTemplateFiles := []string{
		"templates/base/base.tmpl.html",
		"templates/base/banner.tmpl.html",
		"templates/base/header.tmpl.html",
		"templates/base/nav.tmpl.html",
		"templates/base/main.tmpl.html",
		"templates/base/footer.tmpl.html",
	}

	base, err := template.New("base").Funcs(funcMap).ParseFS(TmplData, baseTemplateFiles...)
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

	adminSetupTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/admin_setup.html")
	if err != nil {
		return nil, err
	}

	staffSignupTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/staff_signup.html")
	if err != nil {
		return nil, err
	}

	loginTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/login.html")
	if err != nil {
		return nil, err
	}

	signupTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/signup.html")
	if err != nil {
		return nil, err
	}

	passwordResetRequestTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/password_reset_request.html")
	if err != nil {
		return nil, err
	}

	passwordResetConfirmTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/password_reset_confirm.html")
	if err != nil {
		return nil, err
	}

	return &Templates{
		Index:                indexTmpl,
		Search:               searchTmpl,
		Account:              accountTmpl,
		Admin:                adminTmpl,
		AdminLogin:           adminLoginTmpl,
		AdminSetup:           adminSetupTmpl,
		StaffSignup:          staffSignupTmpl,
		Login:                loginTmpl,
		Signup:               signupTmpl,
		PasswordResetRequest: passwordResetRequestTmpl,
		PasswordResetConfirm: passwordResetConfirmTmpl,
	}, nil
}
