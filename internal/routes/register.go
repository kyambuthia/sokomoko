package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

var (
	rolePartnerOps = []string{domain.RoleAdmin, domain.RoleStaff, domain.RolePartner}
	roleUser       = []string{domain.RoleUser}
	roleAdminStaff = []string{domain.RoleAdmin, domain.RoleStaff}
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
	mux.HandleFunc("/favicon.ico", Favicon())
	mux.HandleFunc("/sitemap.xml", Sitemap(a))
	mux.HandleFunc("/robots.txt", RobotsTxt(a))
	mux.HandleFunc("/llms.txt", LLMsTxt(a))
	mux.Handle("/static/", Static(a.StaticFS))

	mux.Handle("/", public(Root(a)))
	mux.Handle("/products/", public(ProductDetail(a)))
	mux.Handle("/media/product-images/{id}", ProductImage(a))
	mux.Handle("/media/product-videos/{id}", ProductVideo(a))
	mux.Handle("/search", public(Search(a)))
	mux.Handle("/services", public(Services(a, false)))
	mux.Handle("/services/{slug}", public(ServiceDetail(a)))
	mux.Handle("/stores/{slug}", public(StoreProfile(a)))
	mux.Handle("/services/{slug}/bookings", withRole(a, domain.RoleUser, ServiceBookingRequest(a)))
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
	mux.Handle("/account/profile", customer(demoReadOnly(a, "/account", AccountProfileUpdate(a))))
	mux.Handle("/account/tokens", customer(demoReadOnly(a, "/account", AccountTokenCreate(a))))
	mux.Handle("/account/tokens/{id}/revoke", customer(demoReadOnly(a, "/account", AccountTokenRevoke(a))))
	mux.Handle("/account/service-bookings", withRole(a, domain.RoleUser, AccountServiceBookings(a)))
	mux.Handle("/account/orders/{id}/cancel", customer(demoReadOnly(a, "/account", AccountOrderCancel(a))))
	mux.Handle("/account/bookings/{id}/cancel", withRole(a, domain.RoleUser, demoReadOnly(a, "/account/service-bookings", AccountBookingCancel(a))))
	mux.Handle("/account/password", customer(demoReadOnly(a, "/account/password", a.Auth.PasswordChange(a.Templates.PasswordChange, "/account/password", "/account"))))
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
	mux.HandleFunc("/favicon.ico", Favicon())
	mux.Handle("/static/", Static(a.StaticFS))

	mux.HandleFunc("/storefront", redirectToSiblingWithProxyTrust("", "/", trustProxyHeaders))
	mux.HandleFunc("/catalog", redirectToSiblingWithProxyTrust(partnerHostPrefix, "/products", trustProxyHeaders))
	mux.HandleFunc("/catalog/new", redirectToSiblingWithProxyTrust(partnerHostPrefix, "/products/new", trustProxyHeaders))
	mux.HandleFunc("/courier", redirectToSiblingWithProxyTrust(courierHostPrefix, "/login", trustProxyHeaders))

	mux.HandleFunc("/setup", a.Auth.AdminSetup(a.Templates.AdminSetup))
	mux.HandleFunc("/login", a.Auth.AdminLogin(a.Templates.AdminLogin))
	mux.HandleFunc("/logout", a.Auth.Logout())
	registerPasswordReset(a, mux, roleAdminStaff, "Reset Admin or Staff Password", "Enter username or email to request a password reset for admin or staff access.", "Set New Admin or Staff Password", "Choose a new password for your admin/staff account.", "/login")
	mux.Handle("/staff/signup", withRole(a, domain.RoleAdmin, demoReadOnly(a, "/staff/signup", a.Auth.StaffSignUp(a.Templates.StaffSignup))))
	mux.Handle("/couriers/new", withRole(a, domain.RoleAdmin, demoReadOnly(a, "/couriers/new", a.Auth.CourierSignUp(a.Templates.CourierSignup))))
	mux.Handle("/password", withAnyRole(a, roleAdminStaff, demoReadOnly(a, "/password", a.Auth.PasswordChange(a.Templates.PasswordChange, "/password", "/"))))

	adminHandlers := http.NewServeMux()
	adminHandlers.HandleFunc("/", AdminDashboard(a))
	adminHandlers.HandleFunc("/products", AdminProducts(a))
	adminHandlers.Handle("/orders", demoReadOnly(a, "/orders", http.HandlerFunc(AdminOrders(a))))
	adminHandlers.HandleFunc("/reports", AdminReports(a))
	adminHandlers.Handle("/deliveries", demoReadOnly(a, "/deliveries", http.HandlerFunc(AdminDeliveriesWithProxyTrust(a, trustProxyHeaders))))
	adminHandlers.Handle("/categories", demoReadOnly(a, "/categories", http.HandlerFunc(AdminCategories(a))))
	adminHandlers.Handle("/audit", auth.RequireRole(domain.RoleAdmin, http.HandlerFunc(AdminAudit(a))))
	adminHandlers.Handle("/team", demoReadOnly(a, "/team", auth.RequireRole(domain.RoleAdmin, http.HandlerFunc(AdminTeam(a)))))
	adminHandlers.Handle("/partners", demoReadOnly(a, "/partners", auth.RequireRole(domain.RoleAdmin, http.HandlerFunc(AdminPartners(a)))))
	adminHandlers.Handle("/payments", demoReadOnly(a, "/payments", auth.RequireRole(domain.RoleAdmin, http.HandlerFunc(AdminPayments(a)))))
	adminHandlers.Handle("/message-reports", demoReadOnly(a, "/message-reports", auth.RequireRole(domain.RoleAdmin, messageReports(a))))
	mux.Handle("/", withAnyRole(a, roleAdminStaff, adminHandlers))
}

// RegisterPartner wires the partner workspace host.
func RegisterPartner(a *app.App, mux *http.ServeMux, trustProxyHeaders bool) {
	staff := func(h http.HandlerFunc) http.Handler { return withAnyRole(a, rolePartnerOps, h) }

	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.HandleFunc("/favicon.ico", Favicon())
	mux.Handle("/static/", Static(a.StaticFS))

	mux.HandleFunc("/storefront", redirectToSiblingWithProxyTrust("", "/", trustProxyHeaders))

	mux.Handle("/", staff(PartnerRoot(a)))
	mux.HandleFunc("/login", a.Auth.WorkspaceLogin(a.Templates.AdminLogin))
	mux.HandleFunc("/logout", a.Auth.Logout())
	registerPasswordReset(a, mux, rolePartnerOps, "Reset Partner Password", "Enter username or email to request a password reset for partner access.", "Set New Partner Password", "Choose a new password for your partner account.", "/login")
	mux.Handle("/setup", staff(demoReadOnly(a, "/setup", PartnerSetup(a))))
	mux.Handle("/dashboard", staff(PartnerDashboard(a)))
	mux.Handle("/products", staff(PartnerProducts(a)))
	mux.Handle("/media/product-images/{id}", ProductImage(a))
	mux.Handle("/media/product-videos/{id}", ProductVideo(a))
	mux.Handle("/products/new", staff(demoReadOnly(a, "/products/new", PartnerProductNew(a))))
	mux.Handle("/services", withRole(a, domain.RolePartner, Services(a, true)))
	mux.Handle("/service-bookings", withRole(a, domain.RolePartner, PartnerServiceBookings(a)))
	mux.Handle("/service-bookings/{id}/status", withRole(a, domain.RolePartner, demoReadOnly(a, "/service-bookings", PartnerServiceBookingAction(a))))
	mux.Handle("/services/new", withRole(a, domain.RolePartner, demoReadOnly(a, "/services/new", PartnerServiceEdit(a))))
	mux.Handle("/services/{id}/preview", withRole(a, domain.RolePartner, PartnerServicePreview(a)))
	mux.Handle("/services/{id}/edit", withRole(a, domain.RolePartner, demoReadOnly(a, "", PartnerServiceEdit(a))))
	mux.Handle("/products/{id}/edit", staff(demoReadOnly(a, "", PartnerProductEdit(a))))
	mux.Handle("/products/{id}/delete", staff(demoReadOnly(a, "/products", PartnerProductDelete(a))))
	mux.Handle("/password", staff(demoReadOnly(a, "/password", a.Auth.PasswordChange(a.Templates.PasswordChange, "/password", "/dashboard"))))
	mux.Handle("/orders", staff(demoReadOnly(a, "/orders", PartnerOrders(a))))
	registerMessaging(a, mux, "partner", []string{domain.RolePartner})
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

func demoReadOnly(a *app.App, redirectTo string, next http.Handler) http.HandlerFunc {
	if !a.DemoMode {
		return next.ServeHTTP
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || auth.GetUserFromContext(r.Context()) == nil {
			next.ServeHTTP(w, r)
			return
		}
		target := redirectTo
		if target == "" {
			target = r.URL.RequestURI()
		}
		redirectWithFlash(w, r, target, "danger", "Changes are disabled in demo mode.")
	})
}
