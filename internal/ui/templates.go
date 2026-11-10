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

	indexTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/index.html")
	if err != nil {
		return nil, err
	}

	searchTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/search.html")
	if err != nil {
		return nil, err
	}

	storeTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/store.html")
	if err != nil {
		return nil, err
	}

	productTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/product.html")
	if err != nil {
		return nil, err
	}

	cartTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/cart.html")
	if err != nil {
		return nil, err
	}

	checkoutTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/checkout.html")
	if err != nil {
		return nil, err
	}
	paymentTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/payment.html")
	if err != nil {
		return nil, err
	}

	accountTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/account.html")
	if err != nil {
		return nil, err
	}
	accountBookingsTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/account_service_bookings.html")
	if err != nil {
		return nil, err
	}
	messagesTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/messages.html")
	if err != nil {
		return nil, err
	}
	messageReportsTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/message_reports.html")
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

	partnerSetupTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/partner_setup.html")
	if err != nil {
		return nil, err
	}

	partnerDashboardTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/partner_dashboard.html")
	if err != nil {
		return nil, err
	}

	partnerProductsTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/partner_products.html")
	if err != nil {
		return nil, err
	}

	partnerProductNewTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/partner_product_new.html")
	if err != nil {
		return nil, err
	}

	partnerOrdersTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/partner_orders.html")
	if err != nil {
		return nil, err
	}

	courierSignupTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/courier_signup.html")
	if err != nil {
		return nil, err
	}

	courierDashboardTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/courier_dashboard.html")
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

	passwordChangeTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/password_change.html")
	if err != nil {
		return nil, err
	}

	servicesTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/services.html")
	if err != nil {
		return nil, err
	}
	serviceDetailTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/service_detail.html")
	if err != nil {
		return nil, err
	}
	partnerServicesTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/partner_services.html")
	if err != nil {
		return nil, err
	}
	partnerServiceEditTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/partner_service_edit.html")
	if err != nil {
		return nil, err
	}
	partnerBookingsTmpl, err := template.Must(base.Clone()).ParseFS(TmplData, "templates/pages/partner_bookings.html")
	if err != nil {
		return nil, err
	}
	return &Templates{
		Services:             servicesTmpl,
		ServiceDetail:        serviceDetailTmpl,
		PartnerServices:      partnerServicesTmpl,
		PartnerServiceEdit:   partnerServiceEditTmpl,
		PartnerBookings:      partnerBookingsTmpl,
		Index:                indexTmpl,
		Search:               searchTmpl,
		Store:                storeTmpl,
		Product:              productTmpl,
		Cart:                 cartTmpl,
		Checkout:             checkoutTmpl,
		Payment:              paymentTmpl,
		Account:              accountTmpl,
		AccountBookings:      accountBookingsTmpl,
		Messages:             messagesTmpl,
		MessageReports:       messageReportsTmpl,
		Admin:                adminTmpl,
		AdminLogin:           adminLoginTmpl,
		AdminSetup:           adminSetupTmpl,
		StaffSignup:          staffSignupTmpl,
		PartnerSetup:         partnerSetupTmpl,
		PartnerDashboard:     partnerDashboardTmpl,
		PartnerProducts:      partnerProductsTmpl,
		PartnerProductNew:    partnerProductNewTmpl,
		PartnerOrders:        partnerOrdersTmpl,
		CourierSignup:        courierSignupTmpl,
		CourierDashboard:     courierDashboardTmpl,
		Login:                loginTmpl,
		Signup:               signupTmpl,
		PasswordResetRequest: passwordResetRequestTmpl,
		PasswordResetConfirm: passwordResetConfirmTmpl,
		PasswordChange:       passwordChangeTmpl,
	}, nil
}
