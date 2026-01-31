package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
)

// AdminDashboard serves the main admin dashboard
func AdminDashboard(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.Render(w, a.Templates.Admin, map[string]interface{}{
			"Title":   "Admin Dashboard",
			"Message": "Welcome to Admin Dashboard",
		})
	}
}

// AdminProducts handles product management
func AdminProducts(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.Render(w, a.Templates.Admin, map[string]interface{}{
			"Title":   "Product Management",
			"Message": "Manage your products here",
		})
	}
}

// AdminOrders handles order management
func AdminOrders(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.Render(w, a.Templates.Admin, map[string]interface{}{
			"Title":   "Order Management",
			"Message": "View and manage orders",
		})
	}
}

// AdminReports handles sales reports
func AdminReports(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.Render(w, a.Templates.Admin, map[string]interface{}{
			"Title":   "Sales Reports",
			"Message": "View your sales analytics",
		})
	}
}

// AdminDeliveries handles delivery management
func AdminDeliveries(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.Render(w, a.Templates.Admin, map[string]interface{}{
			"Title":   "Delivery Management",
			"Message": "Manage product deliveries",
		})
	}
}
