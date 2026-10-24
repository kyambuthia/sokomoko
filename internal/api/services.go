package api

import (
	"errors"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	directorysvc "github.com/kyambuthia/sokomoko/internal/service/directory"
)

func serviceActor(r *http.Request) *directorysvc.Actor {
	u := auth.GetUserFromContext(r.Context())
	if u == nil {
		return nil
	}
	return &directorysvc.Actor{ID: u.ID, Role: u.Role}
}
func (h *Handler) serviceList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	page, limit := pageParams(r)
	items, total, err := h.app.Directory.Browse(r.Context(), db.ServiceFilter{Query: r.URL.Query().Get("q"), Category: r.URL.Query().Get("category"), Area: r.URL.Query().Get("area"), Limit: limit}, page)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeServicePage(w, items, total, page, limit)
}
func (h *Handler) serviceDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	v, err := h.app.Directory.Public(r.Context(), r.PathValue("slug"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, v)
}
func (h *Handler) ownServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		page, limit := pageParams(r)
		items, total, err := h.app.Directory.Own(r.Context(), serviceActor(r), page, limit)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeServicePage(w, items, total, page, limit)
	case http.MethodPost:
		var in directorysvc.Input
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must contain supported service fields")
			return
		}
		v, err := h.app.Directory.Save(r.Context(), serviceActor(r), 0, in)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, v)
	default:
		methodNotAllowed(w)
	}
}
func (h *Handler) ownService(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositiveID(r.PathValue("id"))
	if !ok {
		writeServiceError(w, db.ErrServiceNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, err := h.app.Directory.Edit(r.Context(), serviceActor(r), id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, v)
	case http.MethodPatch:
		var in directorysvc.Input
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must contain supported service fields")
			return
		}
		v, err := h.app.Directory.Save(r.Context(), serviceActor(r), id, in)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, v)
	default:
		methodNotAllowed(w)
	}
}
func writeServicePage(w http.ResponseWriter, items []db.ServiceListing, total, page, limit int) {
	writeData(w, http.StatusOK, struct {
		Items []db.ServiceListing `json:"items"`
		Total int                 `json:"total"`
		Page  int                 `json:"page"`
		Limit int                 `json:"limit"`
	}{items, total, page, limit})
}
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, directorysvc.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "A seller account is required")
	case errors.Is(err, directorysvc.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_service", "Check service fields, pricing, duration, category, public area and version")
	case errors.Is(err, db.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "service_not_found", "Service listing not found")
	case errors.Is(err, db.ErrServiceConflict):
		writeError(w, http.StatusConflict, "service_changed", "This listing changed. Reload and review the latest version before saving")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to process service listing")
	}
}
