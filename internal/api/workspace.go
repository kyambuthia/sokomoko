package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/auth"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

type adminOrderInput struct {
	Status         string `json:"status"`
	PartnerStatus  string `json:"partner_status"`
	DeliveryStatus string `json:"delivery_status"`
	DeliveryNotice string `json:"delivery_notice"`
}

type partnerOrderInput struct {
	PartnerStatus  string `json:"partner_status"`
	DeliveryStatus string `json:"delivery_status"`
	DeliveryNotice string `json:"delivery_notice"`
}

type partnerProductInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       string `json:"price"`
	Stock       string `json:"stock"`
	CategoryID  string `json:"category_id"`
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

func (h *Handler) admin(w http.ResponseWriter, r *http.Request) {
	tail := strings.Trim(pathTail(r.URL.Path, apiPrefix+"/admin/"), "/")
	parts := strings.Split(tail, "/")
	resource := parts[0]
	if resource == "" {
		resource = "metrics"
	}

	switch resource {
	case "metrics":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		metrics, err := h.app.Admin.Metrics()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load admin metrics")
			return
		}
		writeData(w, http.StatusOK, workspaceMetricsDTO{
			ProductCount: metrics.ProductCount, SessionCount: metrics.SessionCount,
			AdminCount: metrics.AdminCount, StaffCount: metrics.StaffCount, UserCount: metrics.UserCount,
		})
	case "products":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		products, err := h.app.Admin.Products()
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
		}{Items: items})
	case "orders":
		h.adminOrders(w, r, parts)
	case "reports":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		report, err := h.app.Admin.Reports()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load sales report")
			return
		}
		writeData(w, http.StatusOK, salesReportDTO{
			RevenueTotal: report.RevenueTotal, OrderCount: report.OrderCount,
			PendingCount: report.PendingCount, ShippedCount: report.ShippedCount, DeliveredCount: report.DeliveredCount,
		})
	case "team":
		h.adminTeam(w, r, parts)
	case "audit":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if !isAdmin(r) {
			writeError(w, http.StatusForbidden, "forbidden", "Admin role is required")
			return
		}
		logs, err := h.app.Admin.AuditLogs(100)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load audit log")
			return
		}
		writeData(w, http.StatusOK, logs)
	default:
		writeError(w, http.StatusNotFound, "resource_not_found", "Admin resource not found")
	}
}

func (h *Handler) adminOrders(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 && r.Method == http.MethodGet {
		orders, err := h.app.Admin.Orders()
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
		}{Items: items})
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin role is required to update order state")
		return
	}
	if _, ok := parsePositiveID(parts[1]); !ok {
		writeError(w, http.StatusBadRequest, "invalid_order_id", "Order id must be a positive integer")
		return
	}
	var input adminOrderInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	user := auth.GetUserFromContext(r.Context())
	err := h.app.Admin.UpdateOrder(&adminsvc.Actor{ID: user.ID, Role: user.Role}, adminsvc.UpdateOrderInput{
		OrderID: parts[1], Status: input.Status, PartnerStatus: input.PartnerStatus,
		DeliveryStatus: input.DeliveryStatus, DeliveryNotice: input.DeliveryNotice,
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"message": "Order updated"})
}

func (h *Handler) adminTeam(w http.ResponseWriter, r *http.Request, parts []string) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin role is required")
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		members, err := h.app.Admin.TeamMembers()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load team")
			return
		}
		writeData(w, http.StatusOK, members)
		return
	}
	if len(parts) != 2 || r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if _, ok := parsePositiveID(parts[1]); !ok {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "User id must be a positive integer")
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if err := h.app.Admin.DeactivateUser(&adminsvc.Actor{ID: user.ID, Role: user.Role}, parts[1]); err != nil {
		writeAdminError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"message": "User deactivated"})
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

func (h *Handler) partner(w http.ResponseWriter, r *http.Request) {
	tail := strings.Trim(pathTail(r.URL.Path, apiPrefix+"/partner/"), "/")
	parts := strings.Split(tail, "/")
	resource := parts[0]
	if resource == "" {
		resource = "dashboard"
	}

	switch resource {
	case "dashboard":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		data, err := h.app.Partner.Dashboard()
		if err != nil {
			writePartnerError(w, err)
			return
		}
		writeData(w, http.StatusOK, data)
	case "products":
		h.partnerProducts(w, r, parts)
	case "orders":
		h.partnerOrders(w, r, parts)
	case "settings":
		h.partnerSettings(w, r)
	default:
		writeError(w, http.StatusNotFound, "resource_not_found", "Partner resource not found")
	}
}

func (h *Handler) partnerProducts(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 && r.Method == http.MethodGet {
		data, err := h.app.Partner.Products()
		if err != nil {
			writePartnerError(w, err)
			return
		}
		items := make([]productDTO, 0, len(data.Products))
		for _, product := range data.Products {
			items = append(items, mapPartnerProduct(product))
		}
		writeData(w, http.StatusOK, struct {
			Items      []productDTO          `json:"items"`
			Categories []partnersvc.Category `json:"categories"`
		}{Items: items, Categories: data.Categories})
		return
	}
	if len(parts) != 1 || r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input partnerProductInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	if err := h.app.Partner.CreateProduct(partnersvc.CreateProductInput{
		Name: input.Name, Description: input.Description, Price: input.Price, Stock: input.Stock, CategoryID: input.CategoryID,
	}); err != nil {
		writePartnerError(w, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]string{"message": "Product created"})
}

func (h *Handler) partnerOrders(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 && r.Method == http.MethodGet {
		data, err := h.app.Partner.Orders(strings.TrimSpace(r.URL.Query().Get("status")))
		if err != nil {
			writePartnerError(w, err)
			return
		}
		items := make([]orderDTO, 0, len(data.Orders))
		for _, order := range data.Orders {
			items = append(items, mapPartnerOrder(order))
		}
		writeData(w, http.StatusOK, struct {
			Items   []orderDTO         `json:"items"`
			Summary partnersvc.Summary `json:"summary"`
		}{Items: items, Summary: data.Summary})
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	var input partnerOrderInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	user := auth.GetUserFromContext(r.Context())
	err := h.app.Partner.UpdateOrder(&partnersvc.Actor{ID: user.ID, Role: user.Role}, partnersvc.UpdateOrderInput{
		OrderID: parts[1], PartnerStatus: input.PartnerStatus, DeliveryStatus: input.DeliveryStatus, DeliveryNotice: input.DeliveryNotice,
	})
	if err != nil {
		writePartnerError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"message": "Fulfillment updated"})
}

func (h *Handler) partnerSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := h.app.Partner.StoreSettings()
		if err != nil {
			writePartnerError(w, err)
			return
		}
		if settings == nil {
			writeError(w, http.StatusNotFound, "store_not_configured", "Store settings are not configured")
			return
		}
		writeData(w, http.StatusOK, settings)
	case http.MethodPatch:
		var input partnersvc.StoreSettingsInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
			return
		}
		settings, err := h.app.Partner.SaveStoreSettings(input)
		if err != nil {
			writePartnerError(w, err)
			return
		}
		writeData(w, http.StatusOK, settings)
	default:
		methodNotAllowed(w)
	}
}

func writePartnerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, partnersvc.ErrForbiddenOrderUpdate):
		writeError(w, http.StatusForbidden, "forbidden", "Action is not allowed")
	case errors.Is(err, partnersvc.ErrInvalidOrderID), errors.Is(err, partnersvc.ErrMissingProductFields), errors.Is(err, partnersvc.ErrInvalidProductPrice), errors.Is(err, partnersvc.ErrInvalidProductStock), errors.Is(err, partnersvc.ErrMissingStoreFields):
		writeError(w, http.StatusBadRequest, "invalid_input", "Request data is invalid")
	case errors.Is(err, partnersvc.ErrOrderNotFound), errors.Is(err, partnersvc.ErrStoreNotConfigured):
		writeError(w, http.StatusNotFound, "resource_not_found", "Resource not found")
	case errors.Is(err, partnersvc.ErrInvalidOrderTransition):
		writeError(w, http.StatusConflict, "invalid_order_transition", "Order transition is invalid")
	default:
		writeError(w, http.StatusInternalServerError, "workspace_update_failed", "Unable to update partner workspace")
	}
}

func isAdmin(r *http.Request) bool {
	user := auth.GetUserFromContext(r.Context())
	return user != nil && user.Role == "admin"
}
