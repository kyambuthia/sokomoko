package routes

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
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

func buildAdminMetrics(a *app.App) (AdminPageData, error) {
	productCount, err := a.Store.CountProducts()
	if err != nil {
		return AdminPageData{}, err
	}
	sessionCount, err := a.Store.CountActiveSessions()
	if err != nil {
		return AdminPageData{}, err
	}
	adminCount, err := a.Store.CountUsersByRole("admin")
	if err != nil {
		return AdminPageData{}, err
	}
	staffCount, err := a.Store.CountUsersByRole("staff")
	if err != nil {
		return AdminPageData{}, err
	}
	userCount, err := a.Store.CountUsersByRole("user")
	if err != nil {
		return AdminPageData{}, err
	}

	return AdminPageData{
		ProductCount: productCount,
		SessionCount: sessionCount,
		AdminCount:   adminCount,
		StaffCount:   staffCount,
		UserCount:    userCount,
	}, nil
}

// AdminDashboard serves the main admin dashboard.
func AdminDashboard(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(a)
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
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(a)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		products, err := a.Store.GetAllProducts()
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
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(a)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if r.Method == http.MethodPost {
			user := auth.GetUserFromContext(r.Context())
			if user == nil || user.Role != "admin" {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			orderID, parseErr := strconv.Atoi(r.FormValue("order_id"))
			if parseErr != nil || orderID <= 0 {
				metrics.OrderError = "Invalid order id"
			} else {
				status := strings.TrimSpace(r.FormValue("status"))
				partnerStatus := strings.TrimSpace(r.FormValue("partner_status"))
				deliveryStatus := strings.TrimSpace(r.FormValue("delivery_status"))
				notice := strings.TrimSpace(r.FormValue("delivery_notice"))
				if updateErr := a.Store.UpdateOrderByAdmin(orderID, status, partnerStatus, deliveryStatus, notice); updateErr != nil {
					metrics.OrderError = "Unable to update order state"
				} else {
					metrics.OrderMessage = "Order updated"
					actorID := 0
					if user != nil {
						actorID = user.ID
					}
					_ = a.Store.CreateAuditLog(actorID, "order.update", "order", orderID, "status="+status+",partner="+partnerStatus+",delivery="+deliveryStatus)
				}
			}
		}

		orders, err := a.Store.ListAllOrders()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		metrics.Title = "Order Management"
		metrics.Message = "Review and control order lifecycle state"
		metrics.Role = adminRoleFromContext(r)
		metrics.Orders = orders
		renderAdminPage(a, w, metrics)
	}
}

// AdminReports handles sales reports.
func AdminReports(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(a)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		revenueTotal, err := a.Store.SumOrderRevenue()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		statusCounts, err := a.Store.GetOrderStatusCounts()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		metrics.Title = "Sales Reports"
		metrics.Message = "Live operational metrics from orders and fulfillment"
		metrics.Role = adminRoleFromContext(r)
		metrics.RevenueTotal = revenueTotal
		metrics.PendingCount = statusCounts["pending"] + statusCounts["processing"]
		metrics.ShippedCount = statusCounts["shipped"]
		metrics.DeliveredCount = statusCounts["delivered"]
		metrics.OrderCount = metrics.PendingCount + metrics.ShippedCount + metrics.DeliveredCount + statusCounts["cancelled"]
		renderAdminPage(a, w, metrics)
	}
}

// AdminDeliveries handles delivery management.
func AdminDeliveries(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(a)
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
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(a)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		metrics.Title = "Team Management"
		metrics.Message = "Manage admin, staff, and customer accounts"
		metrics.Role = adminRoleFromContext(r)

		if r.Method == http.MethodPost {
			userIDRaw := r.FormValue("user_id")
			userID, parseErr := strconv.Atoi(userIDRaw)
			if parseErr != nil || userID <= 0 {
				metrics.TeamError = "Invalid user ID"
			} else {
				targetUser, getErr := a.Store.GetUserByID(userID)
				if getErr != nil {
					metrics.TeamError = "Unable to load user"
				} else if targetUser == nil {
					metrics.TeamError = "User does not exist"
				} else if targetUser.Role == "admin" {
					metrics.TeamError = "Admin accounts cannot be deactivated from this view"
				} else {
					if err := a.Store.DeleteUser(userID); err != nil {
						metrics.TeamError = "Unable to deactivate user"
					} else {
						metrics.TeamMessage = "User account deactivated"
						user := auth.GetUserFromContext(r.Context())
						actorID := 0
						if user != nil {
							actorID = user.ID
						}
						_ = a.Store.CreateAuditLog(actorID, "user.deactivate", "user", userID, "deactivated via admin team")
					}
				}
			}
		}

		teamMembers, err := a.Store.ListUsersByRoles([]string{"admin", "staff", "user"})
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		metrics.TeamMembers = teamMembers
		renderAdminPage(a, w, metrics)
	}
}

func AdminAudit(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := buildAdminMetrics(a)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logs, err := a.Store.ListAuditLogs(100)
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
