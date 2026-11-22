package api

import (
	"errors"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	messagingSvc "github.com/kyambuthia/sokomoko/internal/service/messaging"
)

func adminActor(r *http.Request) *adminsvc.Actor {
	u := auth.GetUserFromContext(r.Context())
	if u == nil {
		return nil
	}
	return &adminsvc.Actor{ID: u.ID, Role: u.Role}
}

func (h *Handler) registerAdminManagementRoutes(mux *http.ServeMux) {
	adminStaff := []string{domain.RoleAdmin, domain.RoleStaff}
	adminOnly := []string{domain.RoleAdmin}

	mux.Handle(apiPrefix+"/admin/categories", h.role(adminStaff, h.demoWriteDisabled(h.requireJSON(http.HandlerFunc(h.adminCategories)))))
	mux.Handle(apiPrefix+"/admin/categories/{id}", h.role(adminStaff, h.demoWriteDisabled(h.requireJSON(http.HandlerFunc(h.adminCategory)))))
	mux.Handle(apiPrefix+"/admin/partners", h.role(adminOnly, h.demoWriteDisabled(h.requireJSON(http.HandlerFunc(h.adminPartners)))))
	mux.Handle(apiPrefix+"/admin/partners/{id}", h.role(adminOnly, h.demoWriteDisabled(h.requireJSON(http.HandlerFunc(h.adminPartnerUpdate)))))
	mux.Handle(apiPrefix+"/admin/partners/accounts", h.role(adminOnly, h.demoWriteDisabled(h.requireJSON(http.HandlerFunc(h.adminPartnerAccountCreate)))))
	mux.Handle(apiPrefix+"/admin/team/{id}/reactivate", h.role(adminOnly, h.demoWriteDisabled(http.HandlerFunc(h.adminTeamReactivate))))
	mux.Handle(apiPrefix+"/admin/message-reports", h.role(adminOnly, http.HandlerFunc(h.adminMessageReports)))
	mux.Handle(apiPrefix+"/admin/message-reports/{id}/resolve", h.role(adminOnly, h.demoWriteDisabled(http.HandlerFunc(h.adminMessageReportResolve))))
	mux.Handle(apiPrefix+"/admin/payments", h.role(adminOnly, http.HandlerFunc(h.adminPayments)))
}

func (h *Handler) adminCategories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		categories, err := h.app.Admin.Categories(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load categories")
			return
		}
		writeData(w, http.StatusOK, categories)
	case http.MethodPost:
		var input adminsvc.CategoryInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
			return
		}
		if err := h.app.Admin.CreateCategory(r.Context(), adminActor(r), input); err != nil {
			writeAdminCategoryError(w, err)
			return
		}
		writeData(w, http.StatusCreated, map[string]string{"status": "created"})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (h *Handler) adminCategory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	switch r.Method {
	case http.MethodPatch:
		var input adminsvc.CategoryInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
			return
		}
		if err := h.app.Admin.UpdateCategory(r.Context(), adminActor(r), id, input); err != nil {
			writeAdminCategoryError(w, err)
			return
		}
		writeData(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		if err := h.app.Admin.DeleteCategory(r.Context(), adminActor(r), id); err != nil {
			writeAdminCategoryError(w, err)
			return
		}
		writeData(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		methodNotAllowed(w, http.MethodPatch, http.MethodDelete)
	}
}

func writeAdminCategoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, adminsvc.ErrForbiddenCatalogAction):
		writeError(w, http.StatusForbidden, "forbidden", "Forbidden")
	case errors.Is(err, adminsvc.ErrCategoryNotFound):
		writeError(w, http.StatusNotFound, "category_not_found", "Category not found")
	case errors.Is(err, adminsvc.ErrInvalidCategory):
		writeError(w, http.StatusBadRequest, "invalid_category", err.Error())
	case errors.Is(err, adminsvc.ErrDuplicateCategory):
		writeError(w, http.StatusConflict, "duplicate_category", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to update category")
	}
}

func (h *Handler) adminPartners(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		partners, accounts, err := h.app.Admin.Partners(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load partners")
			return
		}
		writeData(w, http.StatusOK, map[string]any{"partners": partners, "accounts": accounts})
	case http.MethodPost:
		var input adminsvc.PartnerInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
			return
		}
		if err := h.app.Admin.CreatePartner(r.Context(), adminActor(r), input); err != nil {
			writeAdminPartnerError(w, err)
			return
		}
		writeData(w, http.StatusCreated, map[string]string{"status": "created"})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (h *Handler) adminPartnerUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	var input adminsvc.PartnerInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	if err := h.app.Admin.UpdatePartner(r.Context(), adminActor(r), r.PathValue("id"), input); err != nil {
		writeAdminPartnerError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) adminPartnerAccountCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var input adminsvc.PartnerAccountInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	if err := h.app.Admin.CreatePartnerAccount(r.Context(), adminActor(r), input); err != nil {
		writeAdminPartnerError(w, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]string{"status": "created"})
}

func writeAdminPartnerError(w http.ResponseWriter, err error) {
	var accountErr *adminsvc.AccountInputError
	switch {
	case errors.Is(err, adminsvc.ErrForbiddenPartnerAction):
		writeError(w, http.StatusForbidden, "forbidden", "Forbidden")
	case errors.Is(err, adminsvc.ErrPartnerNotFound):
		writeError(w, http.StatusNotFound, "partner_not_found", "Partner not found")
	case errors.Is(err, adminsvc.ErrDuplicatePartner), errors.Is(err, adminsvc.ErrDuplicateAccount):
		writeError(w, http.StatusConflict, "duplicate_partner", err.Error())
	case errors.Is(err, adminsvc.ErrInvalidPartner):
		writeError(w, http.StatusBadRequest, "invalid_partner", err.Error())
	case errors.As(err, &accountErr):
		writeError(w, http.StatusBadRequest, "invalid_partner_account", accountErr.Reason)
	default:
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to update partner")
	}
}

func (h *Handler) adminTeamReactivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if err := h.app.Admin.ReactivateUser(r.Context(), adminActor(r), r.PathValue("id")); err != nil {
		switch {
		case errors.Is(err, adminsvc.ErrForbiddenUserAction):
			writeError(w, http.StatusForbidden, "forbidden", "Forbidden")
		case errors.Is(err, adminsvc.ErrInvalidUserID):
			writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user id")
		case errors.Is(err, adminsvc.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		default:
			writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to reactivate user")
		}
		return
	}
	writeData(w, http.StatusOK, map[string]string{"status": "reactivated"})
}

func (h *Handler) adminMessageReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	u := auth.GetUserFromContext(r.Context())
	reports, err := h.app.Messaging.Reports(r.Context(), &messagingSvc.Actor{ID: u.ID, Role: u.Role})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load message reports")
		return
	}
	writeData(w, http.StatusOK, reports)
}

func (h *Handler) adminMessageReportResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	id, ok := parsePositiveID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_report_id", "Invalid report id")
		return
	}
	u := auth.GetUserFromContext(r.Context())
	if err := h.app.Messaging.ResolveReport(r.Context(), &messagingSvc.Actor{ID: u.ID, Role: u.Role}, id); err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to resolve report")
		return
	}
	writeData(w, http.StatusOK, map[string]string{"status": "resolved"})
}

func (h *Handler) adminPayments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	pricing, err := h.app.Payment.Pricing(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load payment pricing")
		return
	}
	exceptions, err := h.app.Payment.PaymentExceptions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load payment exceptions")
		return
	}
	providers, err := h.app.Payment.Providers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace_unavailable", "Unable to load payment providers")
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"pricing":    pricing,
		"exceptions": exceptions,
		"providers":  providers,
	})
}
