package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
)

var (
	rolePartnerOps = []string{db.RoleAdmin, db.RoleStaff, db.RolePartner}
	roleUser       = []string{db.RoleUser}
	roleAdminStaff = []string{db.RoleAdmin, db.RoleStaff}
)

func withAuth(a *app.App, h http.Handler) http.Handler {
	return a.Auth.AuthMiddleware(h)
}

func withAnyRole(a *app.App, roles []string, h http.Handler) http.Handler {
	return withAuth(a, auth.RequireAnyRole(roles, h))
}

func withRole(a *app.App, role string, h http.Handler) http.Handler {
	return withAuth(a, auth.RequireRole(role, h))
}

// RegisterPublic wires the customer storefront.
func RegisterPublic(a *app.App, mux *http.ServeMux) {
	public := func(h http.HandlerFunc) http.Handler { return a.Auth.OptionalUser(h) }
	customer := func(h http.HandlerFunc) http.Handler { return withAuth(a, h) }

	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.Handle("/static/", Static(a.StaticFS))

	mux.Handle("/", public(Root(a)))
	mux.Handle("/products/", public(ProductDetail(a)))
	mux.Handle("/media/product-images/{id}", ProductImage(a))
	mux.Handle("/search", public(Search(a)))
	mux.Handle("/services", public(Services(a, false)))
	mux.Handle("/services/{slug}", public(ServiceDetail(a)))
	mux.Handle("/services/{slug}/bookings", withRole(a, db.RoleUser, ServiceBookingRequest(a)))
	mux.HandleFunc("/api/categories", CategoriesAPI(a))

	mux.HandleFunc("/login", a.Auth.Login(a.Templates.Login))
	mux.HandleFunc("/signup", a.Auth.SignUp(a.Templates.Signup))
	mux.HandleFunc("/logout", a.Auth.Logout())
	registerPasswordReset(a, mux, roleUser, "Reset Your Password", "Enter your username or email to request a password reset for your customer account.", "Set New Password", "Choose a new password for your customer account.", "/login")

	mux.Handle("/cart", customer(CartPage(a)))
	mux.Handle("/cart/add", customer(CartAdd(a)))
	mux.Handle("/cart/update", customer(CartUpdate(a)))
	mux.Handle("/cart/remove", customer(CartRemove(a)))
	mux.Handle("/checkout", customer(Checkout(a)))
	mux.Handle("/account", customer(Account(a)))
	mux.Handle("/account/service-bookings", withRole(a, db.RoleUser, AccountServiceBookings(a)))
	mux.Handle("/account/orders/{id}/cancel", customer(AccountOrderCancel(a)))
	mux.Handle("/account/bookings/{id}/cancel", withRole(a, db.RoleUser, AccountBookingCancel(a)))
	mux.Handle("/account/password", customer(a.Auth.PasswordChange(a.Templates.PasswordChange, "/account/password", "/account")))
	registerMessaging(a, mux, "storefront", roleUser)
	if a.DemoMode {
		mux.Handle("/payments/", http.NotFoundHandler())
	} else {
		mux.Handle("/payments/order/{id}", customer(PaymentPage(a)))
		mux.Handle("/payments/return", customer(PaymentReturn(a)))
		// Provider servers call these without a session; they authenticate with
		// a signature (webhooks) or a per-payment secret URL (callbacks).
		mux.HandleFunc("/payments/webhooks/{provider}", PaymentWebhook(a))
		mux.HandleFunc("/payments/callback/{provider}/{token}", PaymentCallback(a))
	}
}

// RegisterAdmin wires the platform administration host.
func RegisterAdmin(a *app.App, mux *http.ServeMux, trustProxyHeaders bool) {
	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.Handle("/static/", Static(a.StaticFS))

	mux.HandleFunc("/storefront", redirectToSiblingWithProxyTrust("", "/", trustProxyHeaders))
	mux.HandleFunc("/catalog", redirectToSiblingWithProxyTrust(partnerHostPrefix, "/products", trustProxyHeaders))
	mux.HandleFunc("/catalog/new", redirectToSiblingWithProxyTrust(partnerHostPrefix, "/products/new", trustProxyHeaders))
	mux.HandleFunc("/courier", redirectToSiblingWithProxyTrust(courierHostPrefix, "/login", trustProxyHeaders))

	mux.HandleFunc("/setup", a.Auth.AdminSetup(a.Templates.AdminSetup))
	mux.HandleFunc("/login", a.Auth.AdminLogin(a.Templates.AdminLogin))
	mux.HandleFunc("/logout", a.Auth.Logout())
	registerPasswordReset(a, mux, roleAdminStaff, "Reset Admin or Staff Password", "Enter username or email to request a password reset for admin or staff access.", "Set New Admin or Staff Password", "Choose a new password for your admin/staff account.", "/login")
	mux.Handle("/staff/signup", withRole(a, db.RoleAdmin, a.Auth.StaffSignUp(a.Templates.StaffSignup)))
	mux.Handle("/couriers/new", withRole(a, db.RoleAdmin, a.Auth.CourierSignUp(a.Templates.CourierSignup)))
	mux.Handle("/password", withAnyRole(a, roleAdminStaff, a.Auth.PasswordChange(a.Templates.PasswordChange, "/password", "/")))

	adminHandlers := http.NewServeMux()
	adminHandlers.HandleFunc("/", AdminDashboard(a))
	adminHandlers.HandleFunc("/products", AdminProducts(a))
	adminHandlers.HandleFunc("/orders", AdminOrders(a))
	adminHandlers.HandleFunc("/reports", AdminReports(a))
	adminHandlers.HandleFunc("/deliveries", AdminDeliveries(a))
	adminHandlers.HandleFunc("/categories", AdminCategories(a))
	adminHandlers.Handle("/audit", auth.RequireRole(db.RoleAdmin, http.HandlerFunc(AdminAudit(a))))
	adminHandlers.Handle("/team", auth.RequireRole(db.RoleAdmin, http.HandlerFunc(AdminTeam(a))))
	adminHandlers.Handle("/partners", auth.RequireRole(db.RoleAdmin, http.HandlerFunc(AdminPartners(a))))
	adminHandlers.Handle("/payments", auth.RequireRole(db.RoleAdmin, http.HandlerFunc(AdminPayments(a))))
	adminHandlers.Handle("/message-reports", auth.RequireRole(db.RoleAdmin, messageReports(a)))
	mux.Handle("/", withAnyRole(a, roleAdminStaff, adminHandlers))
}

// RegisterPartner wires the partner workspace host.
func RegisterPartner(a *app.App, mux *http.ServeMux, trustProxyHeaders bool) {
	staff := func(h http.HandlerFunc) http.Handler { return withAnyRole(a, rolePartnerOps, h) }

	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.Handle("/static/", Static(a.StaticFS))

	mux.HandleFunc("/storefront", redirectToSiblingWithProxyTrust("", "/", trustProxyHeaders))

	mux.Handle("/", staff(PartnerRoot(a)))
	mux.HandleFunc("/login", a.Auth.WorkspaceLogin(a.Templates.AdminLogin))
	mux.HandleFunc("/logout", a.Auth.Logout())
	registerPasswordReset(a, mux, rolePartnerOps, "Reset Partner Password", "Enter username or email to request a password reset for partner access.", "Set New Partner Password", "Choose a new password for your partner account.", "/login")
	mux.Handle("/setup", staff(PartnerSetup(a)))
	mux.Handle("/dashboard", staff(PartnerDashboard(a)))
	mux.Handle("/products", staff(PartnerProducts(a)))
	mux.Handle("/media/product-images/{id}", ProductImage(a))
	mux.Handle("/products/new", staff(PartnerProductNew(a)))
	mux.Handle("/services", withRole(a, db.RolePartner, Services(a, true)))
	mux.Handle("/service-bookings", withRole(a, db.RolePartner, PartnerServiceBookings(a)))
	mux.Handle("/service-bookings/{id}/status", withRole(a, db.RolePartner, PartnerServiceBookingAction(a)))
	mux.Handle("/services/new", withRole(a, db.RolePartner, PartnerServiceEdit(a)))
	mux.Handle("/services/{id}/preview", withRole(a, db.RolePartner, PartnerServicePreview(a)))
	mux.Handle("/services/{id}/edit", withRole(a, db.RolePartner, PartnerServiceEdit(a)))
	mux.Handle("/products/{id}/edit", staff(PartnerProductEdit(a)))
	mux.Handle("/products/{id}/delete", staff(PartnerProductDelete(a)))
	mux.Handle("/password", staff(a.Auth.PasswordChange(a.Templates.PasswordChange, "/password", "/dashboard")))
	mux.Handle("/orders", staff(PartnerOrders(a)))
	registerMessaging(a, mux, "partner", []string{db.RolePartner})
}

func registerPasswordReset(a *app.App, mux *http.ServeMux, roles []string, requestTitle, requestHelper, confirmTitle, confirmHelper, loginPath string) {
	if a.PasswordResetDisabled {
		mux.Handle("/password-reset/", http.NotFoundHandler())
		return
	}
	mux.HandleFunc("/password-reset/request", a.Auth.PasswordResetRequest(
		a.Templates.PasswordResetRequest, roles, requestTitle, requestHelper,
	))
	mux.HandleFunc("/password-reset/confirm", a.Auth.PasswordResetConfirm(
		a.Templates.PasswordResetConfirm, roles, confirmTitle, confirmHelper, loginPath,
	))
}
