package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	checkoutsvc "github.com/kyambuthia/sokomoko/internal/service/checkout"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

type checkoutInput struct {
	Token           string `json:"token"`
	DeliveryAddress string `json:"delivery_address"`
	PaymentMethod   string `json:"payment_method"`
}

type placedOrderDTO struct {
	OrderID     int64              `json:"order_id"`
	Summary     checkoutSummaryDTO `json:"summary"`
	Idempotency string             `json:"idempotency_key"`
}

type checkoutSummaryDTO struct {
	Subtotal    float64 `json:"subtotal"`
	ShippingFee float64 `json:"shipping_fee"`
	TaxAmount   float64 `json:"tax_amount"`
	Total       float64 `json:"total"`
}

func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var input checkoutInput
		if r.Body != http.NoBody {
			if err := decodeJSON(r, &input); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
				return
			}
		}
		state, err := h.app.Checkout.PreparedCheckout(uid, strings.TrimSpace(input.Token))
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		writeData(w, http.StatusCreated, h.checkoutDTO(state))
	case http.MethodGet:
		token := strings.TrimSpace(r.URL.Query().Get("token"))
		if token == "" {
			writeError(w, http.StatusBadRequest, "checkout_token_required", "Checkout token is required")
			return
		}
		state, err := h.app.Checkout.PreparedCheckout(uid, token)
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		writeData(w, http.StatusOK, h.checkoutDTO(state))
	default:
		methodNotAllowed(w)
	}
}

func (h *Handler) checkoutSession(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}
	parts := strings.Split(strings.Trim(pathTail(r.URL.Path, apiPrefix+"/checkout/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "checkout_not_found", "Checkout not found")
		return
	}
	token := parts[0]

	if len(parts) == 1 {
		if r.Method != http.MethodPatch {
			if r.Method == http.MethodGet {
				state, err := h.app.Checkout.PreparedCheckout(uid, token)
				if err != nil {
					writeCheckoutError(w, err)
					return
				}
				writeData(w, http.StatusOK, h.checkoutDTO(state))
				return
			}
			methodNotAllowed(w)
			return
		}
		var input checkoutInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
			return
		}
		state, err := h.app.Checkout.SaveDraft(uid, token, input.DeliveryAddress, input.PaymentMethod)
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		writeData(w, http.StatusOK, h.checkoutDTO(state))
		return
	}

	if len(parts) == 2 && parts[1] == "place-order" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idempotencyKey != "" && idempotencyKey != token {
			writeError(w, http.StatusBadRequest, "idempotency_key_mismatch", "Idempotency-Key must match the checkout token")
			return
		}
		var input checkoutInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
			return
		}
		orderID, summary, err := h.app.Checkout.CheckoutWithPayment(uid, input.DeliveryAddress, input.PaymentMethod, token)
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		w.Header().Set("Idempotency-Key", token)
		writeData(w, http.StatusCreated, placedOrderDTO{
			OrderID:     orderID,
			Summary:     checkoutSummaryDTO{Subtotal: summary.Subtotal, ShippingFee: summary.ShippingFee, TaxAmount: summary.TaxAmount, Total: summary.Total},
			Idempotency: token,
		})
		return
	}

	writeError(w, http.StatusNotFound, "checkout_not_found", "Checkout route not found")
}

func (h *Handler) checkoutDTO(state checkoutsvc.PageState) checkoutDTO {
	methods := make([]paymentMethodDTO, 0)
	for _, option := range h.app.Payment.SupportedMethodOptions() {
		methods = append(methods, paymentMethodDTO{Value: option.Value, Label: option.Label})
	}
	return mapCheckout(state, methods)
}

func writeCheckoutError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, checkoutsvc.ErrDeliveryAddress):
		writeError(w, http.StatusBadRequest, "delivery_address_required", "Delivery address is required")
	case errors.Is(err, checkoutsvc.ErrInvalidPaymentMethod), errors.Is(err, paymentsvc.ErrInvalidMethod):
		writeError(w, http.StatusBadRequest, "invalid_payment_method", "Choose a supported payment method")
	case errors.Is(err, checkoutsvc.ErrCartEmpty):
		writeError(w, http.StatusConflict, "cart_empty", "Cart is empty or the checkout has expired")
	case errors.Is(err, checkoutsvc.ErrInsufficientStock), errors.Is(err, db.ErrInsufficientStock):
		writeError(w, http.StatusConflict, "insufficient_stock", "One or more items are unavailable")
	case errors.Is(err, db.ErrCheckoutNotFound):
		writeError(w, http.StatusNotFound, "checkout_not_found", "Checkout not found")
	default:
		writeError(w, http.StatusInternalServerError, "checkout_failed", "Unable to process checkout")
	}
}
