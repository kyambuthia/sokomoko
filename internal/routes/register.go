package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
)

var (
	roleUser       = []string{"user"}
	roleAdminStaff = []string{"admin", "staff"}
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

func RegisterPublic(a *app.App, mux *http.ServeMux) {
	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.HandleFunc("/products/", ProductDetail(a))
	mux.HandleFunc("/", Root(a))
	mux.HandleFunc("/search", Search(a))
	mux.HandleFunc("/login", a.Auth.Login(a.Templates.Login))
	mux.HandleFunc("/signup", a.Auth.SignUp(a.Templates.Signup))
	mux.Handle("/cart", withAuth(a, http.HandlerFunc(CartPage(a))))
	mux.Handle("/cart/add", withAuth(a, http.HandlerFunc(CartAdd(a))))
	mux.Handle("/cart/update", withAuth(a, http.HandlerFunc(CartUpdate(a))))
	mux.Handle("/cart/remove", withAuth(a, http.HandlerFunc(CartRemove(a))))
	mux.Handle("/checkout", withAuth(a, http.HandlerFunc(Checkout(a))))
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
	mux.HandleFunc("/logout", a.Auth.Logout())
	mux.Handle("/account", withAuth(a, http.HandlerFunc(Auth(a))))
	mux.Handle("/static/", Static(a.StaticFS))
}

func RegisterAdmin(a *app.App, mux *http.ServeMux) {
	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.HandleFunc("/setup", a.Auth.AdminSetup(a.Templates.AdminSetup))
	mux.HandleFunc("/login", a.Auth.AdminLogin(a.Templates.AdminLogin))
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
	mux.Handle("/staff/signup", withRole(a, "admin", a.Auth.StaffSignUp(a.Templates.StaffSignup)))

	adminHandlers := http.NewServeMux()
	adminHandlers.HandleFunc("/", AdminDashboard(a))
	adminHandlers.HandleFunc("/products", AdminProducts(a))
	adminHandlers.HandleFunc("/orders", AdminOrders(a))
	adminHandlers.HandleFunc("/reports", AdminReports(a))
	adminHandlers.HandleFunc("/deliveries", AdminDeliveries(a))
	adminHandlers.Handle("/audit", auth.RequireRole("admin", http.HandlerFunc(AdminAudit(a))))
	adminHandlers.Handle("/team", auth.RequireRole("admin", http.HandlerFunc(AdminTeam(a))))

	protectedAdmin := withAnyRole(a, roleAdminStaff, adminHandlers)
	mux.Handle("/", protectedAdmin)
	mux.Handle("/static/", Static(a.StaticFS))
}

func RegisterPartner(a *app.App, mux *http.ServeMux) {
	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.HandleFunc("/", PartnerRoot(a))
	mux.HandleFunc("/login", a.Auth.AdminLogin(a.Templates.AdminLogin))
	mux.Handle(
		"/signup",
		a.Auth.AuthMiddleware(auth.RequireRole("admin", a.Auth.StaffSignUp(a.Templates.StaffSignup))),
	)
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
	mux.Handle("/setup", withAnyRole(a, roleAdminStaff, http.HandlerFunc(PartnerSetup(a))))
	mux.Handle("/dashboard", withAnyRole(a, roleAdminStaff, http.HandlerFunc(PartnerDashboard(a))))
	mux.Handle("/products", withAnyRole(a, roleAdminStaff, http.HandlerFunc(PartnerProducts(a))))
	mux.Handle("/products/new", withAnyRole(a, roleAdminStaff, http.HandlerFunc(PartnerProductNew(a))))
	mux.Handle("/orders", withAnyRole(a, roleAdminStaff, http.HandlerFunc(PartnerOrders(a))))
	mux.Handle("/static/", Static(a.StaticFS))
}
