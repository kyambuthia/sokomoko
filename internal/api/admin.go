package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
)

type adminOrderInput struct {
	Status         string `json:"status"`
	PartnerStatus  string `json:"partner_status"`
	DeliveryStatus string `json:"delivery_status"`
	DeliveryNotice string `json:"delivery_notice"`
}

type teamMemberDTO struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type auditLogDTO struct {
	CreatedAt   time.Time `json:"created_at"`
	ActorUserID *int      `json:"actor_user_id,omitempty"`
	Action      string    `json:"action"`
	TargetType  string    `json:"target_type"`
	TargetID    *int      `json:"target_id,omitempty"`
	Details     string    `json:"details"`
}

type workspaceMetricsDTO struct {
	ProductCount int `json:"product_count"`
	SessionCount int `json:"session_count"`
	AdminCount   int `json:"admin_count"`
	StaffCount   int `json:"staff_count"`
	UserCount    int `json:"user_count"`
}

type salesReportDTO struct {
	RevenueTotal   float64 `json:"revenue_total"`
	OrderCount     int     `json:"order_count"`
	PendingCount   int     `json:"pending_count"`
	ShippedCount   int     `json:"shipped_count"`
	DeliveredCount int     `json:"delivered_count"`
}

func (h *Handler) registerAdminRoutes(mux *http.ServeMux) {
	adminStaff := []string{domain.RoleAdmin, domain.RoleStaff}
	adminOnly := []string{domain.RoleAdmin}

	mux.Handle(apiPrefix+"/admin/{$}", h.role(adminStaff, http.HandlerFunc(h.adminMetrics)))
	mux.Handle(apiPrefix+"/admin/metrics", h.role(adminStaff, http.HandlerFunc(h.adminMetrics)))
	mux.Handle(apiPrefix+"/admin/products", h.role(adminStaff, http.HandlerFunc(h.adminProducts)))
	mux.Handle(apiPrefix+"/admin/orders", h.role(adminStaff, http.HandlerFunc(h.adminOrdersList)))
	mux.Handle(apiPrefix+"/admin/orders/{id}", h.role(adminStaff, http.HandlerFunc(h.adminOrderUpdate)))
	mux.Handle(apiPrefix+"/admin/reports", h.role(adminStaff, http.HandlerFunc(h.adminReports)))
	mux.Handle(apiPrefix+"/admin/team", h.role(adminOnly, http.HandlerFunc(h.adminTeamList)))
	mux.Handle(apiPrefix+"/admin/team/{id}", h.role(adminOnly, http.HandlerFunc(h.adminTeamDeactivate)))
	mux.Handle(apiPrefix+"/admin/audit", h.role(adminOnly, http.HandlerFunc(h.adminAudit)))
}

func (h *Handler) adminMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	metrics, err := h.app.Admin.Metrics(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load admin metrics")
		return
	}
	writeData(w, http.StatusOK, workspaceMetricsDTO{
		ProductCount: metrics.ProductCount, SessionCount: metrics.SessionCount,
		AdminCount: metrics.AdminCount, StaffCount: metrics.StaffCount, UserCount: metrics.UserCount,
	})
}

func (h *Handler) adminProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	page, limit := pageParams(r)
	products, total, err := h.app.Admin.ProductsPage(r.Context(), page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load products")
		return
	}
	items := make([]productDTO, 0, len(products))
	for _, product := range products {
		items = append(items, mapAdminProduct(product))
	}
	writeData(w, http.StatusOK, struct {
		Items []productDTO `json:"items"`
		Page  int          `json:"page"`
		Limit int          `json:"limit"`
		Total int          `json:"total"`
	}{Items: items, Page: page, Limit: limit, Total: total})
}

func (h *Handler) adminOrdersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	page, limit := pageParams(r)
	orders, total, err := h.app.Admin.OrdersPage(r.Context(), page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load orders")
		return
	}
	items := make([]orderDTO, 0, len(orders))
	for _, order := range orders {
		items = append(items, mapAdminOrder(order))
	}
	writeData(w, http.StatusOK, struct {
		Items []orderDTO `json:"items"`
		Page  int        `json:"page"`
		Limit int        `json:"limit"`
		Total int        `json:"total"`
	}{Items: items, Page: page, Limit: limit, Total: total})
}

func (h *Handler) adminOrderUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin role is required to update order state")
		return
	}
	orderID := r.PathValue("id")
	if _, ok := parsePositiveID(orderID); !ok {
		writeError(w, http.StatusBadRequest, "invalid_order_id", "Order id must be a positive integer")
		return
	}
	var input adminOrderInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	user := auth.GetUserFromContext(r.Context())
	err := h.app.Admin.UpdateOrder(r.Context(), &adminsvc.Actor{ID: user.ID, Role: user.Role}, adminsvc.UpdateOrderInput{
		OrderID: orderID, Status: input.Status, PartnerStatus: input.PartnerStatus,
		DeliveryStatus: input.DeliveryStatus, DeliveryNotice: input.DeliveryNotice,
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"message": "Order updated"})
}

func (h *Handler) adminReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	report, err := h.app.Admin.Reports(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load sales report")
		return
	}
	writeData(w, http.StatusOK, salesReportDTO{
		RevenueTotal: report.RevenueTotal.Float64(), OrderCount: report.OrderCount,
		PendingCount: report.PendingCount, ShippedCount: report.ShippedCount, DeliveredCount: report.DeliveredCount,
	})
}

func (h *Handler) adminTeamList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	page, limit := pageParams(r)
	members, total, err := h.app.Admin.TeamMembers(r.Context(), r.URL.Query().Get("q"), page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load team")
		return
	}
	items := make([]teamMemberDTO, 0, len(members))
	for _, member := range members {
		items = append(items, mapTeamMember(member))
	}
	writeData(w, http.StatusOK, struct {
		Items []teamMemberDTO `json:"items"`
		Page  int             `json:"page"`
		Limit int             `json:"limit"`
		Total int             `json:"total"`
	}{Items: items, Page: page, Limit: limit, Total: total})
}

func (h *Handler) adminTeamDeactivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w, http.MethodDelete)
		return
	}
	userID := r.PathValue("id")
	if _, ok := parsePositiveID(userID); !ok {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "User id must be a positive integer")
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if err := h.app.Admin.DeactivateUser(r.Context(), &adminsvc.Actor{ID: user.ID, Role: user.Role}, userID); err != nil {
		writeAdminError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"message": "User deactivated"})
}

func (h *Handler) adminAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	logs, err := h.app.Admin.AuditLogs(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load audit log")
		return
	}
	items := make([]auditLogDTO, 0, len(logs))
	for _, log := range logs {
		items = append(items, mapAuditLog(log))
	}
	writeData(w, http.StatusOK, items)
}

func mapTeamMember(member adminsvc.TeamMember) teamMemberDTO {
	return teamMemberDTO{ID: member.ID, Username: member.Username, Email: member.Email, Role: member.Role}
}

func mapAuditLog(log adminsvc.AuditLog) auditLogDTO {
	result := auditLogDTO{
		CreatedAt: log.CreatedAt, Action: log.Action, TargetType: log.TargetType, Details: log.Details,
	}
	if log.ActorUserID.Valid {
		actorID := int(log.ActorUserID.Int64)
		result.ActorUserID = &actorID
	}
	if log.TargetID.Valid {
		targetID := int(log.TargetID.Int64)
		result.TargetID = &targetID
	}
	return result
}

func writeAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, adminsvc.ErrForbiddenOrderUpdate), errors.Is(err, adminsvc.ErrForbiddenUserAction), errors.Is(err, adminsvc.ErrProtectedUser):
		writeError(w, http.StatusForbidden, "forbidden", "Action is not allowed")
	case errors.Is(err, adminsvc.ErrInvalidOrderID), errors.Is(err, adminsvc.ErrInvalidUserID):
		writeError(w, http.StatusBadRequest, "invalid_identifier", "Identifier is invalid")
	case errors.Is(err, adminsvc.ErrOrderNotFound), errors.Is(err, adminsvc.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "resource_not_found", "Resource not found")
	case errors.Is(err, adminsvc.ErrInvalidOrderState):
		writeError(w, http.StatusConflict, "invalid_order_state", "Order state transition is invalid")
	default:
		writeError(w, http.StatusInternalServerError, "workspace_update_failed", "Unable to update workspace")
	}
}

func isAdmin(r *http.Request) bool {
	user := auth.GetUserFromContext(r.Context())
	return user != nil && user.Role == domain.RoleAdmin
}
