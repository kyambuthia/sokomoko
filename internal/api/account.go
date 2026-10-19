package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
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
	page, limit := pageParams(r)
	orders, total, err := h.app.Account.OrdersForUserPage(r.Context(), uid, page, limit)
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
		Page  int        `json:"page"`
		Limit int        `json:"limit"`
		Total int        `json:"total"`
	}{Items: items, Page: page, Limit: limit, Total: total})
}

func (h *Handler) accountOrder(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}
	tail := strings.Trim(pathTail(r.URL.Path, apiPrefix+"/account/orders/"), "/")
	if idPart, ok := strings.CutSuffix(tail, "/cancel"); ok {
		h.cancelAccountOrder(w, r, uid, idPart)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if idPart, ok := strings.CutSuffix(tail, "/payment"); ok {
		h.accountOrderPayment(w, r, uid, idPart)
		return
	}
	id, err := strconv.Atoi(tail)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_order_id", "Order id must be a positive integer")
		return
	}
	order, err := h.app.Account.OrderForUser(r.Context(), uid, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "orders_unavailable", "Unable to load order")
		return
	}
	if order != nil {
		writeData(w, http.StatusOK, mapAccountOrder(*order))
		return
	}
	writeError(w, http.StatusNotFound, "order_not_found", "Order not found")
}

// cancelAccountOrder cancels one of the customer's orders while it is still
// new and unpaid. It answers with the updated order.
func (h *Handler) cancelAccountOrder(w http.ResponseWriter, r *http.Request, uid int, idPart string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	id, err := strconv.Atoi(idPart)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_order_id", "Order id must be a positive integer")
		return
	}
	switch err := h.app.Account.CancelOrder(r.Context(), uid, id); {
	case errors.Is(err, accountsvc.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, "order_not_found", "Order not found")
		return
	case errors.Is(err, accountsvc.ErrOrderNotCancellable):
		writeError(w, http.StatusConflict, "order_not_cancellable", "The order is already being prepared or paid")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "cancel_failed", "Unable to cancel the order")
		return
	}
	order, err := h.app.Account.OrderForUser(r.Context(), uid, id)
	if err != nil || order == nil {
		writeError(w, http.StatusInternalServerError, "orders_unavailable", "Unable to load order")
		return
	}
	writeData(w, http.StatusOK, mapAccountOrder(*order))
}

type paymentStatusDTO struct {
	OrderID      int     `json:"order_id"`
	Method       string  `json:"method"`
	Status       string  `json:"status"` // pending, paid, failed, cancelled, offline
	Message      string  `json:"message,omitempty"`
	Amount       float64 `json:"amount"`
	AmountMinor  int64   `json:"amount_minor"`
	ChargedMinor int64   `json:"charged_minor"`
	Currency     string  `json:"currency"`
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
		Amount: view.Amount.Float64(), AmountMinor: int64(view.Amount), ChargedMinor: int64(view.Charged), Currency: view.Currency,
	})
}
