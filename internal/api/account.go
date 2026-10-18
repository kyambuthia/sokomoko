package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
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
	tail := strings.Trim(pathTail(r.URL.Path, apiPrefix+"/account/orders/"), "/")
	if idPart, ok := strings.CutSuffix(tail, "/payment"); ok {
		h.accountOrderPayment(w, r, uid, idPart)
		return
	}
	id, err := strconv.Atoi(tail)
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

type paymentStatusDTO struct {
	OrderID  int     `json:"order_id"`
	Method   string  `json:"method"`
	Status   string  `json:"status"` // pending, paid, failed, cancelled, offline
	Message  string  `json:"message,omitempty"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// accountOrderPayment reports the payment state of one of the customer's
// orders. Polling it also confirms pending payments with the provider.
func (h *Handler) accountOrderPayment(w http.ResponseWriter, r *http.Request, uid int, idPart string) {
	id, err := strconv.Atoi(idPart)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_order_id", "Order id must be a positive integer")
		return
	}
	view, err := h.app.Payment.Status(r.Context(), uid, id)
	if errors.Is(err, db.ErrPaymentNotFound) {
		writeError(w, http.StatusNotFound, "order_not_found", "Order not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payment_unavailable", "Unable to load payment")
		return
	}
	writeData(w, http.StatusOK, paymentStatusDTO{
		OrderID: view.OrderID, Method: view.Method, Status: view.Status, Message: view.Message,
		Amount: view.Amount.Float64(), Currency: view.Currency,
	})
}
