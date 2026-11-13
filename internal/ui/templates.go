package ui

import (
	"html/template"
)

type Templates struct {
	Services             *template.Template
	ServiceDetail        *template.Template
	PartnerServices      *template.Template
	PartnerServiceEdit   *template.Template
	PartnerBookings      *template.Template
	Index                *template.Template
	Search               *template.Template
	Store                *template.Template
	Product              *template.Template
	Cart                 *template.Template
	Checkout             *template.Template
	Payment              *template.Template
	Account              *template.Template
	AccountBookings      *template.Template
	Messages             *template.Template
	MessageReports       *template.Template
	Admin                *template.Template
	AdminLogin           *template.Template
	AdminSetup           *template.Template
	StaffSignup          *template.Template
	PartnerSetup         *template.Template
	PartnerDashboard     *template.Template
	PartnerProducts      *template.Template
	PartnerProductNew    *template.Template
	PartnerOrders        *template.Template
	CourierSignup        *template.Template
	CourierDashboard     *template.Template
	Login                *template.Template
	Signup               *template.Template
	PasswordResetRequest *template.Template
	PasswordResetConfirm *template.Template
	PasswordChange       *template.Template
}

func ParseTemplates() (*Templates, error) {
	return ParseTemplatesWithDemo(false)
}

// ParseTemplatesWithDemo parses the UI templates with a configurable demo-site banner.
func ParseTemplatesWithDemo(demoMode bool) (*Templates, error) {
	funcMap := templateFuncs(demoMode)

	baseTemplateFiles := []string{
		"templates/base/base.tmpl.html",
		"templates/base/header.tmpl.html",
		"templates/base/nav.tmpl.html",
		"templates/base/footer.tmpl.html",
		"templates/base/partials.tmpl.html",
	}

	base, err := template.New("base").Funcs(funcMap).ParseFS(TmplData, baseTemplateFiles...)
	if err != nil {
		return nil, err
	}

	var tmpls Templates
	pages := []struct {
		target **template.Template
		path   string
	}{
		{&tmpls.Services, "templates/pages/services.html"},
		{&tmpls.ServiceDetail, "templates/pages/service_detail.html"},
		{&tmpls.PartnerServices, "templates/pages/partner_services.html"},
		{&tmpls.PartnerServiceEdit, "templates/pages/partner_service_edit.html"},
		{&tmpls.PartnerBookings, "templates/pages/partner_bookings.html"},
		{&tmpls.Index, "templates/pages/index.html"},
		{&tmpls.Search, "templates/pages/search.html"},
		{&tmpls.Store, "templates/pages/store.html"},
		{&tmpls.Product, "templates/pages/product.html"},
		{&tmpls.Cart, "templates/pages/cart.html"},
		{&tmpls.Checkout, "templates/pages/checkout.html"},
		{&tmpls.Payment, "templates/pages/payment.html"},
		{&tmpls.Account, "templates/pages/account.html"},
		{&tmpls.AccountBookings, "templates/pages/account_service_bookings.html"},
		{&tmpls.Messages, "templates/pages/messages.html"},
		{&tmpls.MessageReports, "templates/pages/message_reports.html"},
		{&tmpls.Admin, "templates/pages/admin.html"},
		{&tmpls.AdminLogin, "templates/pages/admin_login.html"},
		{&tmpls.AdminSetup, "templates/pages/admin_setup.html"},
		{&tmpls.StaffSignup, "templates/pages/staff_signup.html"},
		{&tmpls.PartnerSetup, "templates/pages/partner_setup.html"},
		{&tmpls.PartnerDashboard, "templates/pages/partner_dashboard.html"},
		{&tmpls.PartnerProducts, "templates/pages/partner_products.html"},
		{&tmpls.PartnerProductNew, "templates/pages/partner_product_new.html"},
		{&tmpls.PartnerOrders, "templates/pages/partner_orders.html"},
		{&tmpls.CourierSignup, "templates/pages/courier_signup.html"},
		{&tmpls.CourierDashboard, "templates/pages/courier_dashboard.html"},
		{&tmpls.Login, "templates/pages/login.html"},
		{&tmpls.Signup, "templates/pages/signup.html"},
		{&tmpls.PasswordResetRequest, "templates/pages/password_reset_request.html"},
		{&tmpls.PasswordResetConfirm, "templates/pages/password_reset_confirm.html"},
		{&tmpls.PasswordChange, "templates/pages/password_change.html"},
	}

	for _, page := range pages {
		cloned, err := base.Clone()
		if err != nil {
			return nil, err
		}
		parsed, err := cloned.ParseFS(TmplData, page.path)
		if err != nil {
			return nil, err
		}
		*page.target = parsed
	}

	return &tmpls, nil
}
