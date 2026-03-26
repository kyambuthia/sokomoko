package routes

import (
	"errors"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
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
	Products       []adminsvc.Product
	TeamMembers    []adminsvc.TeamMember
	Orders         []adminsvc.Order
	AuditLogs      []adminsvc.AuditLog
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

func renderAdminPage(a *app.App, w http.ResponseWriter, data AdminPageData) {
	a.Render(w, a.Templates.Admin, data)
}

func loadAdminMetrics(svc app.AdminService) (adminsvc.Metrics, error) {
	return svc.Metrics()
}

// AdminDashboard serves the main admin dashboard.
func AdminDashboard(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		renderAdminPage(a, w, adminDashboardPage(metrics, adminRoleFromContext(r)))
	}
}

// AdminProducts handles product management.
func AdminProducts(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		products, err := svc.Products()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		renderAdminPage(a, w, adminProductsPage(metrics, adminRoleFromContext(r), products))
	}
}

// AdminOrders handles order management.
func AdminOrders(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		page := adminOrdersPage(metrics, adminRoleFromContext(r), nil)

		responseStatus := http.StatusOK
		if r.Method == http.MethodPost {
			err := svc.UpdateOrder(adminActorFromContext(r), adminsvc.UpdateOrderInput{
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
				page.OrderError = "Invalid order id"
				responseStatus = http.StatusBadRequest
			case errors.Is(err, adminsvc.ErrOrderNotFound):
				page.OrderError = "Order does not exist"
				responseStatus = http.StatusBadRequest
			case errors.Is(err, adminsvc.ErrInvalidOrderState):
				page.OrderError = "Order state is invalid"
				responseStatus = http.StatusBadRequest
			case err != nil:
				page.OrderError = "Unable to update order state"
				responseStatus = http.StatusBadRequest
			default:
				page.OrderMessage = "Order updated"
			}
		}

		orders, err := svc.Orders()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		page.Orders = orders
		if responseStatus != http.StatusOK {
			w.WriteHeader(responseStatus)
		}
		renderAdminPage(a, w, page)
	}
}

// AdminReports handles sales reports.
func AdminReports(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		report, err := svc.Reports()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		renderAdminPage(a, w, adminReportsPage(metrics, adminRoleFromContext(r), report))
	}
}

// AdminDeliveries handles delivery management.
func AdminDeliveries(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		renderAdminPage(a, w, adminDeliveriesPage(metrics, adminRoleFromContext(r)))
	}
}

// AdminTeam lists users and allows admin-only staff/user deactivation.
func AdminTeam(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		page := adminTeamPage(metrics, adminRoleFromContext(r), nil)

		if r.Method == http.MethodPost {
			err := svc.DeactivateUser(adminActorFromContext(r), r.FormValue("user_id"))
			switch {
			case errors.Is(err, adminsvc.ErrForbiddenUserAction):
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case errors.Is(err, adminsvc.ErrInvalidUserID):
				page.TeamError = "Invalid user ID"
			case errors.Is(err, adminsvc.ErrUserNotFound):
				page.TeamError = "User does not exist"
			case errors.Is(err, adminsvc.ErrProtectedUser):
				page.TeamError = "Admin accounts cannot be deactivated from this view"
			case err != nil:
				page.TeamError = "Unable to deactivate user"
			default:
				page.TeamMessage = "User account deactivated"
			}
		}

		teamMembers, err := svc.TeamMembers()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		page.TeamMembers = teamMembers
		renderAdminPage(a, w, page)
	}
}

func AdminAudit(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logs, err := svc.AuditLogs(100)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		renderAdminPage(a, w, adminAuditPage(metrics, adminRoleFromContext(r), logs))
	}
}
