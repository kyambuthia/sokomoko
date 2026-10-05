package routes

import (
	"errors"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/money"
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
	CancelledCount int
	RevenueTotal   money.Cents
	PendingCount   int
	ShippedCount   int
	DeliveredCount int
	TeamError      string
	TeamMessage    string
	OrderError     string
	OrderMessage   string
	CSRFToken      string
}

func renderAdminPage(a *app.App, w http.ResponseWriter, r *http.Request, status int, data AdminPageData) {
	data.CSRFToken = a.Auth.CSRFToken(r)
	a.RenderStatus(w, a.Templates.Admin, status, data)
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
		renderAdminPage(a, w, r, http.StatusOK, adminDashboardPage(metrics, adminRoleFromContext(r)))
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

		renderAdminPage(a, w, r, http.StatusOK, adminProductsPage(metrics, adminRoleFromContext(r), products))
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
				redirectWithFlash(w, r, "/orders", "success", "Order updated")
				return
			}
		} else if flash := popFlash(w, r); flash.Message != "" {
			page.OrderMessage = flash.Message
		}

		orders, err := svc.Orders()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		page.Orders = orders
		renderAdminPage(a, w, r, responseStatus, page)
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

		renderAdminPage(a, w, r, http.StatusOK, adminReportsPage(metrics, adminRoleFromContext(r), report))
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
		renderAdminPage(a, w, r, http.StatusOK, adminDeliveriesPage(metrics, adminRoleFromContext(r)))
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
				redirectWithFlash(w, r, "/team", "success", "User account deactivated")
				return
			}
		} else if flash := popFlash(w, r); flash.Message != "" {
			page.TeamMessage = flash.Message
		}

		teamMembers, err := svc.TeamMembers()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		page.TeamMembers = teamMembers
		renderAdminPage(a, w, r, http.StatusOK, page)
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

		renderAdminPage(a, w, r, http.StatusOK, adminAuditPage(metrics, adminRoleFromContext(r), logs))
	}
}
