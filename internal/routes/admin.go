package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
)

func renderAdminPage(a *app.App, w http.ResponseWriter, r *http.Request, title, message string) {
	role := "staff"
	user := auth.GetUserFromContext(r.Context())
	if user != nil {
		role = user.Role
	}
	a.Render(w, a.Templates.Admin, map[string]interface{}{
		"Title":   title,
		"Message": message,
		"Role":    role,
	})
}

// AdminDashboard serves the main admin dashboard
func AdminDashboard(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderAdminPage(a, w, r, "Admin Dashboard", "Welcome to Admin Dashboard")
	}
}

// AdminProducts handles product management
func AdminProducts(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderAdminPage(a, w, r, "Product Management", "Manage your products here")
	}
}

// AdminOrders handles order management
func AdminOrders(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderAdminPage(a, w, r, "Order Management", "View and manage orders")
	}
}

// AdminReports handles sales reports
func AdminReports(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderAdminPage(a, w, r, "Sales Reports", "View your sales analytics")
	}
}

// AdminDeliveries handles delivery management
func AdminDeliveries(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderAdminPage(a, w, r, "Delivery Management", "Manage product deliveries")
	}
}
