package routes

import (
	"net/http"
	"strings"

	checkoutsvc "github.com/kyambuthia/sokomoko/internal/service/checkout"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
)

func cartPage(r *http.Request, items []commerceSvc.CartItem, subtotal float64) CartPageData {
	return CartPageData{
		Title:    "Cart",
		Items:    items,
		Subtotal: subtotal,
		Message:  strings.TrimSpace(r.URL.Query().Get("message")),
		Error:    strings.TrimSpace(r.URL.Query().Get("error")),
	}
}

func checkoutPage(state checkoutsvc.PageState, paymentMethod string, paymentMethods []PaymentMethodOption) CheckoutPageData {
	selectedPaymentMethod := strings.TrimSpace(paymentMethod)
	if selectedPaymentMethod == "" {
		selectedPaymentMethod = state.PaymentMethod
	}
	if selectedPaymentMethod == "" {
		selectedPaymentMethod = "cash_on_delivery"
	}
	return CheckoutPageData{
		Title:           "Checkout",
		Items:           state.Items,
		Subtotal:        state.Summary.Subtotal,
		ShippingFee:     state.Summary.ShippingFee,
		TaxAmount:       state.Summary.TaxAmount,
		TotalAmount:     state.Summary.Total,
		DeliveryAddress: state.DeliveryAddress,
		PaymentMethod:   selectedPaymentMethod,
		PaymentMethods:  paymentMethods,
		IdempotencyKey:  state.Token,
		CanCheckout:     state.CanCheckout,
	}
}
