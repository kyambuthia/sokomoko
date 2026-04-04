package routes

import (
	"net/http"
	"strings"

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

func checkoutPage(items []commerceSvc.CartItem, subtotal float64, paymentMethod string, paymentMethods []PaymentMethodOption) CheckoutPageData {
	summary := commerceSvc.CalculateCheckoutSummary(subtotal)
	return CheckoutPageData{
		Title:          "Checkout",
		Items:          items,
		Subtotal:       summary.Subtotal,
		ShippingFee:    summary.ShippingFee,
		TaxAmount:      summary.TaxAmount,
		TotalAmount:    summary.Total,
		PaymentMethod:  paymentMethod,
		PaymentMethods: paymentMethods,
		CanCheckout:    len(items) > 0,
	}
}
