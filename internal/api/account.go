package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/auth"
)

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}
	writeData(w, http.StatusOK, struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}{ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role})
}

func (h *Handler) accountOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	uid, ok := userID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}
	orders, err := h.app.Account.OrdersForUser(uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "orders_unavailable", "Unable to load orders")
		return
	}
	items := make([]orderDTO, 0, len(orders))
	for _, order := range orders {
		items = append(items, mapAccountOrder(order))
	}
	writeData(w, http.StatusOK, struct {
		Items []orderDTO `json:"items"`
	}{Items: items})
}

func (h *Handler) accountOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	uid, ok := userID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}
	id, err := strconv.Atoi(strings.Trim(pathTail(r.URL.Path, apiPrefix+"/account/orders/"), "/"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_order_id", "Order id must be a positive integer")
		return
	}
	orders, err := h.app.Account.OrdersForUser(uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "orders_unavailable", "Unable to load order")
		return
	}
	for _, order := range orders {
		if order.ID == id {
			writeData(w, http.StatusOK, mapAccountOrder(order))
			return
		}
	}
	writeError(w, http.StatusNotFound, "order_not_found", "Order not found")
}
