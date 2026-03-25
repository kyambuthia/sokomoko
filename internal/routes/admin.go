package routes

import (
	"errors"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
)

type AdminPageData struct {
	Title          string
	Message        string
	Role           string
	ProductCount   int
	SessionCount   int
	AdminCount     int
	StaffCount     int
	UserCount      int
	Products       []db.Product
	TeamMembers    []db.User
	Orders         []db.FulfillmentOrder
	AuditLogs      []db.AuditLog
	OrderCount     int
	RevenueTotal   float64
	PendingCount   int
	ShippedCount   int
	DeliveredCount int
	TeamError      string
	TeamMessage    string
	OrderError     string
	OrderMessage   string
}

func adminRoleFromContext(r *http.Request) string {
	role := "staff"
	user := auth.GetUserFromContext(r.Context())
	if user != nil {
		role = user.Role
	}
	return role
}

func renderAdminPage(a *app.App, w http.ResponseWriter, data AdminPageData) {
	a.Render(w, a.Templates.Admin, data)
}

func buildAdminMetrics(svc *adminsvc.Service) (AdminPageData, error) {
	metrics, err := svc.Metrics()
	if err != nil {
		return AdminPageData{}, err
	}

	return AdminPageData{
		ProductCount: metrics.ProductCount,
		SessionCount: metrics.SessionCount,
		AdminCount:   metrics.AdminCount,
		StaffCount:   metrics.StaffCount,
		UserCount:    metrics.UserCount,
	}, nil
}

// AdminDashboard serves the main admin dashboard.
func AdminDashboard(a *app.App) http.HandlerFunc {
	svc := adminsvc.New(a.Store)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		metrics.Title = "Admin Dashboard"
		metrics.Message = "Platform operations and health overview"
		metrics.Role = adminRoleFromContext(r)
		renderAdminPage(a, w, metrics)
	}
}

// AdminProducts handles product management.
func AdminProducts(a *app.App) http.HandlerFunc {
	svc := adminsvc.New(a.Store)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		products, err := svc.Products()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		metrics.Title = "Product Management"
		metrics.Message = "Review catalog quality and inventory"
		metrics.Role = adminRoleFromContext(r)
		metrics.Products = products
		renderAdminPage(a, w, metrics)
	}
}

// AdminOrders handles order management.
func AdminOrders(a *app.App) http.HandlerFunc {
	svc := adminsvc.New(a.Store)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		responseStatus := http.StatusOK
		if r.Method == http.MethodPost {
			user := auth.GetUserFromContext(r.Context())
			err := svc.UpdateOrder(user, adminsvc.UpdateOrderInput{
				OrderID:        r.FormValue("order_id"),
				Status:         r.FormValue("status"),
				PartnerStatus:  r.FormValue("partner_status"),
				DeliveryStatus: r.FormValue("delivery_status"),
				DeliveryNotice: r.FormValue("delivery_notice"),
			})
			switch {
			case errors.Is(err, adminsvc.ErrForbiddenOrderUpdate):
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case errors.Is(err, adminsvc.ErrInvalidOrderID):
				metrics.OrderError = "Invalid order id"
				responseStatus = http.StatusBadRequest
			case err != nil:
				metrics.OrderError = "Unable to update order state"
				responseStatus = http.StatusBadRequest
			default:
				metrics.OrderMessage = "Order updated"
			}
		}

		orders, err := svc.Orders()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		metrics.Title = "Order Management"
		metrics.Message = "Review and control order lifecycle state"
		metrics.Role = adminRoleFromContext(r)
		metrics.Orders = orders
		if responseStatus != http.StatusOK {
			w.WriteHeader(responseStatus)
		}
		renderAdminPage(a, w, metrics)
	}
}

// AdminReports handles sales reports.
func AdminReports(a *app.App) http.HandlerFunc {
	svc := adminsvc.New(a.Store)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		report, err := svc.Reports()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		metrics.Title = "Sales Reports"
		metrics.Message = "Live operational metrics from orders and fulfillment"
		metrics.Role = adminRoleFromContext(r)
		metrics.RevenueTotal = report.RevenueTotal
		metrics.PendingCount = report.PendingCount
		metrics.ShippedCount = report.ShippedCount
		metrics.DeliveredCount = report.DeliveredCount
		metrics.OrderCount = report.OrderCount
		renderAdminPage(a, w, metrics)
	}
}

// AdminDeliveries handles delivery management.
func AdminDeliveries(a *app.App) http.HandlerFunc {
	svc := adminsvc.New(a.Store)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		metrics.Title = "Delivery Management"
		metrics.Message = "Delivery pipeline placeholders are ready for integration"
		metrics.Role = adminRoleFromContext(r)
		renderAdminPage(a, w, metrics)
	}
}

// AdminTeam lists users and allows admin-only staff/user deactivation.
func AdminTeam(a *app.App) http.HandlerFunc {
	svc := adminsvc.New(a.Store)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		metrics.Title = "Team Management"
		metrics.Message = "Manage admin, staff, and customer accounts"
		metrics.Role = adminRoleFromContext(r)

		if r.Method == http.MethodPost {
			err := svc.DeactivateUser(auth.GetUserFromContext(r.Context()), r.FormValue("user_id"))
			switch {
			case errors.Is(err, adminsvc.ErrForbiddenUserAction):
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case errors.Is(err, adminsvc.ErrInvalidUserID):
				metrics.TeamError = "Invalid user ID"
			case errors.Is(err, adminsvc.ErrUserNotFound):
				metrics.TeamError = "User does not exist"
			case errors.Is(err, adminsvc.ErrProtectedUser):
				metrics.TeamError = "Admin accounts cannot be deactivated from this view"
			case err != nil:
				metrics.TeamError = "Unable to deactivate user"
			default:
				metrics.TeamMessage = "User account deactivated"
			}
		}

		teamMembers, err := svc.TeamMembers()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		metrics.TeamMembers = teamMembers
		renderAdminPage(a, w, metrics)
	}
}

func AdminAudit(a *app.App) http.HandlerFunc {
	svc := adminsvc.New(a.Store)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logs, err := svc.AuditLogs(100)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		metrics.Title = "Audit Log"
		metrics.Message = "Recent privileged actions"
		metrics.Role = adminRoleFromContext(r)
		metrics.AuditLogs = logs
		renderAdminPage(a, w, metrics)
	}
}
