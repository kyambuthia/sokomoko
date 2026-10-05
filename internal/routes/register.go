package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
)

var (
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
	mux.Handle("/search", public(Search(a)))
	mux.HandleFunc("/api/search", SearchAPI(a))
	mux.HandleFunc("/api/categories", CategoriesAPI(a))

	mux.HandleFunc("/login", a.Auth.Login(a.Templates.Login))
	mux.HandleFunc("/signup", a.Auth.SignUp(a.Templates.Signup))
	mux.HandleFunc("/logout", a.Auth.Logout())
	mux.HandleFunc(
		"/password-reset/request",
		a.Auth.PasswordResetRequest(
			a.Templates.PasswordResetRequest,
			roleUser,
			"Reset Your Password",
			"Enter your username or email to request a password reset for your customer account.",
		),
	)
	mux.HandleFunc(
		"/password-reset/confirm",
		a.Auth.PasswordResetConfirm(
			a.Templates.PasswordResetConfirm,
			roleUser,
			"Set New Password",
			"Choose a new password for your customer account.",
			"/login",
		),
	)

	mux.Handle("/cart", customer(CartPage(a)))
	mux.Handle("/cart/add", customer(CartAdd(a)))
	mux.Handle("/cart/update", customer(CartUpdate(a)))
	mux.Handle("/cart/remove", customer(CartRemove(a)))
	mux.Handle("/checkout", customer(Checkout(a)))
	mux.Handle("/account", customer(Account(a)))
	mux.Handle("/api/cart", customer(CartSummaryAPI(a)))
	mux.Handle("/api/orders/status", customer(OrderStatusAPI(a)))
}

// RegisterAdmin wires the platform administration host.
func RegisterAdmin(a *app.App, mux *http.ServeMux) {
	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.Handle("/static/", Static(a.StaticFS))

	mux.HandleFunc("/setup", a.Auth.AdminSetup(a.Templates.AdminSetup))
	mux.HandleFunc("/login", a.Auth.AdminLogin(a.Templates.AdminLogin))
	mux.HandleFunc("/logout", a.Auth.Logout())
	mux.HandleFunc(
		"/password-reset/request",
		a.Auth.PasswordResetRequest(
			a.Templates.PasswordResetRequest,
			roleAdminStaff,
			"Reset Admin or Staff Password",
			"Enter username or email to request a password reset for admin or staff access.",
		),
	)
	mux.HandleFunc(
		"/password-reset/confirm",
		a.Auth.PasswordResetConfirm(
			a.Templates.PasswordResetConfirm,
			roleAdminStaff,
			"Set New Admin or Staff Password",
			"Choose a new password for your admin/staff account.",
			"/login",
		),
	)
	mux.Handle("/staff/signup", withRole(a, db.RoleAdmin, a.Auth.StaffSignUp(a.Templates.StaffSignup)))

	adminHandlers := http.NewServeMux()
	adminHandlers.HandleFunc("/", AdminDashboard(a))
	adminHandlers.HandleFunc("/products", AdminProducts(a))
	adminHandlers.HandleFunc("/orders", AdminOrders(a))
	adminHandlers.HandleFunc("/reports", AdminReports(a))
	adminHandlers.HandleFunc("/deliveries", AdminDeliveries(a))
	adminHandlers.Handle("/audit", auth.RequireRole(db.RoleAdmin, http.HandlerFunc(AdminAudit(a))))
	adminHandlers.Handle("/team", auth.RequireRole(db.RoleAdmin, http.HandlerFunc(AdminTeam(a))))
	mux.Handle("/", withAnyRole(a, roleAdminStaff, adminHandlers))
}

// RegisterPartner wires the partner workspace host.
func RegisterPartner(a *app.App, mux *http.ServeMux) {
	staff := func(h http.HandlerFunc) http.Handler { return withAnyRole(a, roleAdminStaff, h) }

	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.Handle("/static/", Static(a.StaticFS))

	mux.Handle("/", staff(PartnerRoot(a)))
	mux.HandleFunc("/login", a.Auth.WorkspaceLogin(a.Templates.AdminLogin))
	mux.HandleFunc("/logout", a.Auth.Logout())
	mux.Handle("/signup", withRole(a, db.RoleAdmin, a.Auth.StaffSignUp(a.Templates.StaffSignup)))
	mux.HandleFunc(
		"/password-reset/request",
		a.Auth.PasswordResetRequest(
			a.Templates.PasswordResetRequest,
			roleAdminStaff,
			"Reset Partner Password",
			"Enter username or email to request a password reset for partner access.",
		),
	)
	mux.HandleFunc(
		"/password-reset/confirm",
		a.Auth.PasswordResetConfirm(
			a.Templates.PasswordResetConfirm,
			roleAdminStaff,
			"Set New Partner Password",
			"Choose a new password for your partner account.",
			"/login",
		),
	)
	mux.Handle("/setup", staff(PartnerSetup(a)))
	mux.Handle("/dashboard", staff(PartnerDashboard(a)))
	mux.Handle("/products", staff(PartnerProducts(a)))
	mux.Handle("/products/new", staff(PartnerProductNew(a)))
	mux.Handle("/orders", staff(PartnerOrders(a)))
}
