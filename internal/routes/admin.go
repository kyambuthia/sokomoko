package routes

import (
	"html/template"
	"log"
	"net/http"
)

// AdminDashboard serves the main admin dashboard
func AdminDashboard(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.ExecuteTemplate(w, "root_template", map[string]interface{}{
			"Title":   "Admin Dashboard",
			"Message": "Welcome to Admin Dashboard",
		})
		if err != nil {
			log.Printf("Error executing admin template: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

// AdminProducts handles product management
func AdminProducts(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.ExecuteTemplate(w, "root_template", map[string]interface{}{
			"Title":   "Product Management",
			"Message": "Manage your products here",
		})
		if err != nil {
			log.Printf("Error executing admin products template: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

// AdminOrders handles order management
func AdminOrders(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.ExecuteTemplate(w, "root_template", map[string]interface{}{
			"Title":   "Order Management",
			"Message": "View and manage orders",
		})
		if err != nil {
			log.Printf("Error executing admin orders template: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

// AdminReports handles sales reports
func AdminReports(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.ExecuteTemplate(w, "root_template", map[string]interface{}{
			"Title":   "Sales Reports",
			"Message": "View your sales analytics",
		})
		if err != nil {
			log.Printf("Error executing admin reports template: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

// AdminDeliveries handles delivery management
func AdminDeliveries(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.ExecuteTemplate(w, "root_template", map[string]interface{}{
			"Title":   "Delivery Management",
			"Message": "Manage product deliveries",
		})
		if err != nil {
			log.Printf("Error executing admin deliveries template: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}
