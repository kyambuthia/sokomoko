package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
)

func RegisterPublic(a *app.App, mux *http.ServeMux) {
	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.HandleFunc("/", Root(a))
	mux.HandleFunc("/search", Search(a))
	mux.HandleFunc("/login", auth.Login(a.Store, a.Templates.Login))
	mux.HandleFunc("/signup", auth.SignUp(a.Store, a.Templates.Signup))
	mux.Handle("/cart", auth.AuthMiddleware(a.Store, http.HandlerFunc(CartPage(a))))
	mux.Handle("/cart/add", auth.AuthMiddleware(a.Store, http.HandlerFunc(CartAdd(a))))
	mux.Handle("/cart/update", auth.AuthMiddleware(a.Store, http.HandlerFunc(CartUpdate(a))))
	mux.Handle("/cart/remove", auth.AuthMiddleware(a.Store, http.HandlerFunc(CartRemove(a))))
	mux.Handle("/checkout", auth.AuthMiddleware(a.Store, http.HandlerFunc(Checkout(a))))
	mux.HandleFunc(
		"/password-reset/request",
		auth.PasswordResetRequest(
			a.Store,
			a.Templates.PasswordResetRequest,
			[]string{"user"},
			"Reset Your Password",
			"Enter your username or email to request a password reset for your customer account.",
		),
	)
	mux.HandleFunc(
		"/password-reset/confirm",
		auth.PasswordResetConfirm(
			a.Store,
			a.Templates.PasswordResetConfirm,
			[]string{"user"},
			"Set New Password",
			"Choose a new password for your customer account.",
			"/login",
		),
	)
	mux.HandleFunc("/logout", auth.Logout(a.Store))
	mux.Handle("/account", auth.AuthMiddleware(a.Store, http.HandlerFunc(Auth(a))))
	mux.Handle("/static/", Static(a.StaticFS))
}

func RegisterAdmin(a *app.App, mux *http.ServeMux) {
	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.HandleFunc("/setup", auth.AdminSetup(a.Store, a.Templates.AdminSetup))
	mux.HandleFunc("/login", auth.AdminLogin(a.Store, a.Templates.AdminLogin))
	mux.HandleFunc(
		"/password-reset/request",
		auth.PasswordResetRequest(
			a.Store,
			a.Templates.PasswordResetRequest,
			[]string{"admin", "staff"},
			"Reset Admin or Staff Password",
			"Enter username or email to request a password reset for admin or staff access.",
		),
	)
	mux.HandleFunc(
		"/password-reset/confirm",
		auth.PasswordResetConfirm(
			a.Store,
			a.Templates.PasswordResetConfirm,
			[]string{"admin", "staff"},
			"Set New Admin or Staff Password",
			"Choose a new password for your admin/staff account.",
			"/login",
		),
	)
	mux.Handle(
		"/staff/signup",
		auth.AuthMiddleware(a.Store, auth.RequireRole("admin", auth.StaffSignUp(a.Store, a.Templates.StaffSignup))),
	)

	adminHandlers := http.NewServeMux()
	adminHandlers.HandleFunc("/", AdminDashboard(a))
	adminHandlers.HandleFunc("/products", AdminProducts(a))
	adminHandlers.HandleFunc("/orders", AdminOrders(a))
	adminHandlers.HandleFunc("/reports", AdminReports(a))
	adminHandlers.HandleFunc("/deliveries", AdminDeliveries(a))
	adminHandlers.Handle("/team", auth.RequireRole("admin", http.HandlerFunc(AdminTeam(a))))

	protectedAdmin := auth.AuthMiddleware(a.Store, auth.RequireAnyRole([]string{"admin", "staff"}, adminHandlers))
	mux.Handle("/", protectedAdmin)
	mux.Handle("/static/", Static(a.StaticFS))
}

func RegisterPartner(a *app.App, mux *http.ServeMux) {
	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.HandleFunc("/", PartnerRoot(a))
	mux.HandleFunc("/login", auth.AdminLogin(a.Store, a.Templates.AdminLogin))
	mux.Handle(
		"/signup",
		auth.AuthMiddleware(a.Store, auth.RequireRole("admin", auth.StaffSignUp(a.Store, a.Templates.StaffSignup))),
	)
	mux.HandleFunc(
		"/password-reset/request",
		auth.PasswordResetRequest(
			a.Store,
			a.Templates.PasswordResetRequest,
			[]string{"admin", "staff"},
			"Reset Partner Password",
			"Enter username or email to request a password reset for partner access.",
		),
	)
	mux.HandleFunc(
		"/password-reset/confirm",
		auth.PasswordResetConfirm(
			a.Store,
			a.Templates.PasswordResetConfirm,
			[]string{"admin", "staff"},
			"Set New Partner Password",
			"Choose a new password for your partner account.",
			"/login",
		),
	)
	mux.Handle(
		"/setup",
		auth.AuthMiddleware(a.Store, auth.RequireAnyRole([]string{"admin", "staff"}, http.HandlerFunc(PartnerSetup(a)))),
	)
	mux.Handle(
		"/dashboard",
		auth.AuthMiddleware(a.Store, auth.RequireAnyRole([]string{"admin", "staff"}, http.HandlerFunc(PartnerDashboard(a)))),
	)
	mux.Handle(
		"/products",
		auth.AuthMiddleware(a.Store, auth.RequireAnyRole([]string{"admin", "staff"}, http.HandlerFunc(PartnerProducts(a)))),
	)
	mux.Handle(
		"/products/new",
		auth.AuthMiddleware(a.Store, auth.RequireAnyRole([]string{"admin", "staff"}, http.HandlerFunc(PartnerProductNew(a)))),
	)
	mux.Handle(
		"/orders",
		auth.AuthMiddleware(a.Store, auth.RequireAnyRole([]string{"admin", "staff"}, http.HandlerFunc(PartnerOrders(a)))),
	)
	mux.Handle("/static/", Static(a.StaticFS))
}
