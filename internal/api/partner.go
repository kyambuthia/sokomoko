package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

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

// partnerProductPatch changes only the fields present in the request.
type partnerProductPatch struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Price       *string `json:"price"`
	Stock       *string `json:"stock"`
	CategoryID  *string `json:"category_id"`
}

type partnerSettingsInput struct {
	StoreName    string `json:"store_name"`
	StoreSlug    string `json:"store_slug"`
	Description  string `json:"description"`
	ContactEmail string `json:"contact_email"`
}

type partnerSettingsDTO struct {
	StoreName    string `json:"store_name"`
	StoreSlug    string `json:"store_slug"`
	Description  string `json:"description"`
	ContactEmail string `json:"contact_email"`
}

type categoryDTO struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type partnerSummaryDTO struct {
	NewCount        int `json:"new_count"`
	InProgressCount int `json:"in_progress_count"`
	DispatchedCount int `json:"dispatched_count"`
	CompletedCount  int `json:"completed_count"`
	OverdueCount    int `json:"overdue_count"`
}

type partnerDashboardDTO struct {
	StoreName    string            `json:"store_name"`
	StoreSlug    string            `json:"store_slug"`
	Description  string            `json:"description"`
	ContactEmail string            `json:"contact_email"`
	ProductCount int               `json:"product_count"`
	OrderSummary partnerSummaryDTO `json:"order_summary"`
}

func (h *Handler) registerPartnerRoutes(mux *http.ServeMux) {
	partnerOps := []string{domain.RoleAdmin, domain.RoleStaff, domain.RolePartner}

	mux.Handle(apiPrefix+"/partner/{$}", h.role(partnerOps, http.HandlerFunc(h.partnerDashboard)))
	mux.Handle(apiPrefix+"/partner/dashboard", h.role(partnerOps, http.HandlerFunc(h.partnerDashboard)))
	mux.Handle(apiPrefix+"/partner/products", h.role(partnerOps, http.HandlerFunc(h.partnerProducts)))
	mux.Handle(apiPrefix+"/partner/products/{id}", h.role(partnerOps, http.HandlerFunc(h.partnerProductPatch)))
	mux.Handle(apiPrefix+"/partner/orders", h.role(partnerOps, http.HandlerFunc(h.partnerOrdersList)))
	mux.Handle(apiPrefix+"/partner/orders/{id}", h.role(partnerOps, http.HandlerFunc(h.partnerOrderUpdate)))
	mux.Handle(apiPrefix+"/partner/settings", h.role(partnerOps, http.HandlerFunc(h.partnerSettings)))
}

func (h *Handler) partnerDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	data, err := h.app.Partner.DashboardForActor(r.Context(), partnerActor(r))
	if err != nil {
		writePartnerError(w, err)
		return
	}
	writeData(w, http.StatusOK, partnerDashboardDTO{
		StoreName: data.Settings.StoreName, StoreSlug: data.Settings.StoreSlug,
		Description: data.Settings.Description, ContactEmail: data.Settings.ContactEmail,
		ProductCount: data.ProductCount, OrderSummary: mapPartnerSummary(data.OrderSummary),
	})
}

func (h *Handler) partnerProducts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		page, limit := pageParams(r)
		data, total, err := h.app.Partner.ProductsPageForActor(r.Context(), partnerActor(r), page, limit)
		if err != nil {
			writePartnerError(w, err)
			return
		}
		items := make([]productDTO, 0, len(data.Products))
		for _, product := range data.Products {
			items = append(items, mapPartnerProduct(product))
		}
		writeData(w, http.StatusOK, struct {
			Items      []productDTO  `json:"items"`
			Categories []categoryDTO `json:"categories"`
			Page       int           `json:"page"`
			Limit      int           `json:"limit"`
			Total      int           `json:"total"`
		}{Items: items, Categories: mapCategories(data.Categories), Page: page, Limit: limit, Total: total})
	case http.MethodPost:
		var input partnerProductInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
			return
		}
		if err := h.app.Partner.CreateProductForActor(r.Context(), partnerActor(r), partnersvc.CreateProductInput{
			Name: input.Name, Description: input.Description, Price: input.Price, Stock: input.Stock, CategoryID: input.CategoryID,
		}); err != nil {
			writePartnerError(w, err)
			return
		}
		writeData(w, http.StatusCreated, map[string]string{"message": "Product created"})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (h *Handler) partnerProductPatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	var patch partnerProductPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	id := r.PathValue("id")
	actor := partnerActor(r)
	_, input, err := h.app.Partner.ProductForEdit(r.Context(), actor, id)
	if err != nil {
		writePartnerError(w, err)
		return
	}
	for field, value := range map[*string]*string{
		&input.Name: patch.Name, &input.Description: patch.Description, &input.Price: patch.Price,
		&input.Stock: patch.Stock, &input.CategoryID: patch.CategoryID,
	} {
		if value != nil {
			*field = *value
		}
	}
	if err := h.app.Partner.UpdateProductForActor(r.Context(), actor, id, input); err != nil {
		writePartnerError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"message": "Product updated"})
}

func (h *Handler) partnerOrdersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	page, limit := pageParams(r)
	data, total, err := h.app.Partner.OrdersPageForActor(r.Context(), partnerActor(r), strings.TrimSpace(r.URL.Query().Get("status")), page, limit)
	if err != nil {
		writePartnerError(w, err)
		return
	}
	items := make([]orderDTO, 0, len(data.Orders))
	for _, order := range data.Orders {
		items = append(items, mapPartnerOrder(order))
	}
	writeData(w, http.StatusOK, struct {
		Items   []orderDTO        `json:"items"`
		Summary partnerSummaryDTO `json:"summary"`
		Page    int               `json:"page"`
		Limit   int               `json:"limit"`
		Total   int               `json:"total"`
	}{Items: items, Summary: mapPartnerSummary(data.Summary), Page: page, Limit: limit, Total: total})
}

func (h *Handler) partnerOrderUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	var input partnerOrderInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	err := h.app.Partner.UpdateOrder(r.Context(), partnerActor(r), partnersvc.UpdateOrderInput{
		OrderID: r.PathValue("id"), PartnerStatus: input.PartnerStatus,
		DeliveryStatus: input.DeliveryStatus, DeliveryNotice: input.DeliveryNotice,
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
		settings, err := h.app.Partner.StoreSettings(r.Context())
		if err != nil {
			writePartnerError(w, err)
			return
		}
		if settings == nil {
			writeError(w, http.StatusNotFound, "store_not_configured", "Store settings are not configured")
			return
		}
		writeData(w, http.StatusOK, mapPartnerSettings(*settings))
	case http.MethodPatch:
		var input partnerSettingsInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
			return
		}
		settings, err := h.app.Partner.SaveStoreSettings(r.Context(), partnerActor(r), partnersvc.StoreSettingsInput{
			StoreName: input.StoreName, StoreSlug: input.StoreSlug,
			Description: input.Description, ContactEmail: input.ContactEmail,
		})
		if err != nil {
			writePartnerError(w, err)
			return
		}
		writeData(w, http.StatusOK, mapPartnerSettings(settings))
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPatch)
	}
}

func mapPartnerSummary(summary partnersvc.Summary) partnerSummaryDTO {
	return partnerSummaryDTO{
		NewCount: summary.NewCount, InProgressCount: summary.InProgressCount,
		DispatchedCount: summary.DispatchedCount, CompletedCount: summary.CompletedCount,
		OverdueCount: summary.OverdueCount,
	}
}

func mapPartnerSettings(settings partnersvc.StoreSettings) partnerSettingsDTO {
	return partnerSettingsDTO{
		StoreName: settings.StoreName, StoreSlug: settings.StoreSlug,
		Description: settings.Description, ContactEmail: settings.ContactEmail,
	}
}

func mapCategories(categories []partnersvc.Category) []categoryDTO {
	items := make([]categoryDTO, 0, len(categories))
	for _, category := range categories {
		items = append(items, categoryDTO{ID: category.ID, Name: category.Name})
	}
	return items
}

func writePartnerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, partnersvc.ErrForbiddenWorkspace), errors.Is(err, partnersvc.ErrForbiddenOrderUpdate), errors.Is(err, partnersvc.ErrForbiddenSettingsUpdate):
		writeError(w, http.StatusForbidden, "forbidden", "Action is not allowed")
	case errors.Is(err, partnersvc.ErrInvalidOrderID), errors.Is(err, partnersvc.ErrMissingProductFields), errors.Is(err, partnersvc.ErrInvalidProductPrice), errors.Is(err, partnersvc.ErrInvalidProductStock), errors.Is(err, partnersvc.ErrMissingStoreFields),
		errors.Is(err, partnersvc.ErrInvalidStoreSlug), errors.Is(err, partnersvc.ErrInvalidContactEmail):
		writeError(w, http.StatusBadRequest, "invalid_input", "Request data is invalid")
	case errors.Is(err, partnersvc.ErrOrderNotFound), errors.Is(err, partnersvc.ErrStoreNotConfigured), errors.Is(err, partnersvc.ErrProductNotFound):
		writeError(w, http.StatusNotFound, "resource_not_found", "Resource not found")
	case errors.Is(err, partnersvc.ErrInvalidOrderTransition):
		writeError(w, http.StatusConflict, "invalid_order_transition", "Order transition is invalid")
	default:
		writeError(w, http.StatusInternalServerError, "workspace_update_failed", "Unable to update partner workspace")
	}
}

func partnerActor(r *http.Request) *partnersvc.Actor {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		return nil
	}
	return &partnersvc.Actor{ID: user.ID, Role: user.Role}
}
