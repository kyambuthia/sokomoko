package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
)

func RegisterPublic(a *app.App, mux *http.ServeMux) {
	mux.HandleFunc("/", Root(a))
	mux.HandleFunc("/search", Search(a))
	mux.HandleFunc("/account", Auth(a))
	mux.Handle("/static/", Static(a.StaticFS))
}

func RegisterAdmin(a *app.App, mux *http.ServeMux) {
	mux.HandleFunc("/login", auth.AdminLogin(a.Store, a.Templates.AdminLogin))

	adminHandlers := http.NewServeMux()
	adminHandlers.HandleFunc("/", AdminDashboard(a))
	adminHandlers.HandleFunc("/products", AdminProducts(a))
	adminHandlers.HandleFunc("/orders", AdminOrders(a))
	adminHandlers.HandleFunc("/reports", AdminReports(a))
	adminHandlers.HandleFunc("/deliveries", AdminDeliveries(a))

	protectedAdmin := auth.AuthMiddleware(a.Store, auth.RequireRole("admin", adminHandlers))
	mux.Handle("/", protectedAdmin)
	mux.Handle("/static/", Static(a.StaticFS))
}
