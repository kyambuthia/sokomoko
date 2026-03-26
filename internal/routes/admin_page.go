package routes

import (
	"net/http"

	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
)

func adminRoleFromContext(r *http.Request) string {
	role := "staff"
	user := requestUserFromContext(r)
	if user != nil {
		role = user.Role
	}
	return role
}

func adminActorFromContext(r *http.Request) *adminsvc.Actor {
	user := requestUserFromContext(r)
	if user == nil {
		return nil
	}
	return &adminsvc.Actor{
		ID:   user.ID,
		Role: user.Role,
	}
}

func newAdminPage(metrics adminsvc.Metrics, role, title, message string) AdminPageData {
	return AdminPageData{
		Title:        title,
		Message:      message,
		Role:         role,
		ProductCount: metrics.ProductCount,
		SessionCount: metrics.SessionCount,
		AdminCount:   metrics.AdminCount,
		StaffCount:   metrics.StaffCount,
		UserCount:    metrics.UserCount,
	}
}

func adminDashboardPage(metrics adminsvc.Metrics, role string) AdminPageData {
	return newAdminPage(metrics, role, "Admin Dashboard", "Platform operations and health overview")
}

func adminProductsPage(metrics adminsvc.Metrics, role string, products []adminsvc.Product) AdminPageData {
	page := newAdminPage(metrics, role, "Product Management", "Review catalog quality and inventory")
	page.Products = products
	return page
}

func adminOrdersPage(metrics adminsvc.Metrics, role string, orders []adminsvc.Order) AdminPageData {
	page := newAdminPage(metrics, role, "Order Management", "Review and control order lifecycle state")
	page.Orders = orders
	return page
}

func adminReportsPage(metrics adminsvc.Metrics, role string, report adminsvc.SalesReport) AdminPageData {
	page := newAdminPage(metrics, role, "Sales Reports", "Live operational metrics from orders and fulfillment")
	page.RevenueTotal = report.RevenueTotal
	page.PendingCount = report.PendingCount
	page.ShippedCount = report.ShippedCount
	page.DeliveredCount = report.DeliveredCount
	page.OrderCount = report.OrderCount
	return page
}

func adminDeliveriesPage(metrics adminsvc.Metrics, role string) AdminPageData {
	return newAdminPage(metrics, role, "Delivery Management", "Delivery pipeline placeholders are ready for integration")
}

func adminTeamPage(metrics adminsvc.Metrics, role string, teamMembers []adminsvc.TeamMember) AdminPageData {
	page := newAdminPage(metrics, role, "Team Management", "Manage admin, staff, and customer accounts")
	page.TeamMembers = teamMembers
	return page
}

func adminAuditPage(metrics adminsvc.Metrics, role string, logs []adminsvc.AuditLog) AdminPageData {
	page := newAdminPage(metrics, role, "Audit Log", "Recent privileged actions")
	page.AuditLogs = logs
	return page
}
