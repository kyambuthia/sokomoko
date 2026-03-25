package routes

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
)

type CartPageData struct {
	Title    string
	Items    []db.CartItem
	Subtotal float64
	Error    string
	Message  string
}

type CheckoutPageData struct {
	Title           string
	Items           []db.CartItem
	Subtotal        float64
	ShippingFee     float64
	TaxAmount       float64
	TotalAmount     float64
	DeliveryAddress string
	PaymentMethod   string
	PaymentMethods  []PaymentMethodOption
	Error           string
	Message         string
	CanCheckout     bool
}

type PaymentMethodOption struct {
	Value string
	Label string
}

func checkoutPaymentOptions() []PaymentMethodOption {
	return []PaymentMethodOption{
		{Value: commerceSvc.PaymentMethodCashOnDelivery, Label: "Cash on Delivery"},
		{Value: commerceSvc.PaymentMethodCardPlaceholder, Label: "Card (placeholder)"},
		{Value: commerceSvc.PaymentMethodMobilePlaceholder, Label: "Mobile Money (placeholder)"},
	}
}

func CartPage(a *app.App) http.HandlerFunc {
	svc := a.Commerce

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		user := auth.GetUserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		items, subtotal, err := svc.GetCart(user.ID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := CartPageData{
			Title:    "Cart",
			Items:    items,
			Subtotal: subtotal,
			Message:  strings.TrimSpace(r.URL.Query().Get("message")),
			Error:    strings.TrimSpace(r.URL.Query().Get("error")),
		}
		a.Render(w, a.Templates.Cart, data)
	}
}

func CartAdd(a *app.App) http.HandlerFunc {
	svc := a.Commerce

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		user := auth.GetUserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		productID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("product_id")))
		if err != nil {
			http.Redirect(w, r, "/cart?error=Invalid+product", http.StatusFound)
			return
		}
		qty := 1
		if qRaw := strings.TrimSpace(r.FormValue("quantity")); qRaw != "" {
			if parsed, parseErr := strconv.Atoi(qRaw); parseErr == nil && parsed > 0 {
				qty = parsed
			}
		}

		if err := svc.AddToCart(user.ID, productID, qty); err != nil {
			switch {
			case errors.Is(err, commerceSvc.ErrInvalidProduct):
				http.Redirect(w, r, "/cart?error=Invalid+product", http.StatusFound)
			case errors.Is(err, commerceSvc.ErrInvalidQuantity):
				http.Redirect(w, r, "/cart?error=Invalid+quantity", http.StatusFound)
			case errors.Is(err, commerceSvc.ErrProductNotFound):
				http.Redirect(w, r, "/cart?error=Product+not+found", http.StatusFound)
			case errors.Is(err, commerceSvc.ErrOutOfStock):
				http.Redirect(w, r, "/cart?error=Product+is+out+of+stock", http.StatusFound)
			case errors.Is(err, commerceSvc.ErrInsufficientStock):
				http.Redirect(w, r, "/cart?error=Requested+quantity+exceeds+available+stock", http.StatusFound)
			default:
				http.Redirect(w, r, "/cart?error=Unable+to+add+item", http.StatusFound)
			}
			return
		}

		http.Redirect(w, r, "/cart?message=Item+added+to+cart", http.StatusFound)
	}
}

func CartUpdate(a *app.App) http.HandlerFunc {
	svc := a.Commerce

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		user := auth.GetUserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		productID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("product_id")))
		if err != nil {
			http.Redirect(w, r, "/cart?error=Invalid+product", http.StatusFound)
			return
		}
		qty, err := strconv.Atoi(strings.TrimSpace(r.FormValue("quantity")))
		if err != nil {
			http.Redirect(w, r, "/cart?error=Invalid+quantity", http.StatusFound)
			return
		}

		if err := svc.UpdateCartItem(user.ID, productID, qty); err != nil {
			if errors.Is(err, commerceSvc.ErrInvalidProduct) {
				http.Redirect(w, r, "/cart?error=Invalid+product", http.StatusFound)
				return
			}
			if errors.Is(err, commerceSvc.ErrInvalidQuantity) {
				http.Redirect(w, r, "/cart?error=Invalid+quantity", http.StatusFound)
				return
			}
			if errors.Is(err, commerceSvc.ErrOutOfStock) || errors.Is(err, commerceSvc.ErrInsufficientStock) {
				http.Redirect(w, r, "/cart?error=Requested+quantity+exceeds+available+stock", http.StatusFound)
				return
			}
			http.Redirect(w, r, "/cart?error=Unable+to+update+item", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/cart?message=Cart+updated", http.StatusFound)
	}
}

func CartRemove(a *app.App) http.HandlerFunc {
	svc := a.Commerce

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		user := auth.GetUserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		productID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("product_id")))
		if err != nil {
			http.Redirect(w, r, "/cart?error=Invalid+product", http.StatusFound)
			return
		}

		if err := svc.RemoveFromCart(user.ID, productID); err != nil {
			if errors.Is(err, commerceSvc.ErrInvalidProduct) {
				http.Redirect(w, r, "/cart?error=Invalid+product", http.StatusFound)
				return
			}
			http.Redirect(w, r, "/cart?error=Unable+to+remove+item", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/cart?message=Item+removed", http.StatusFound)
	}
}

func Checkout(a *app.App) http.HandlerFunc {
	svc := a.Commerce

	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.GetUserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		items, subtotal, err := svc.GetCart(user.ID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		paymentMethod := strings.TrimSpace(r.FormValue("payment_method"))
		if paymentMethod == "" {
			paymentMethod = commerceSvc.PaymentMethodCashOnDelivery
		}
		summary := commerceSvc.CalculateCheckoutSummary(subtotal)

		data := CheckoutPageData{
			Title:          "Checkout",
			Items:          items,
			Subtotal:       summary.Subtotal,
			ShippingFee:    summary.ShippingFee,
			TaxAmount:      summary.TaxAmount,
			TotalAmount:    summary.Total,
			PaymentMethod:  paymentMethod,
			PaymentMethods: checkoutPaymentOptions(),
			CanCheckout:    len(items) > 0,
		}

		if r.Method == http.MethodGet {
			a.Render(w, a.Templates.Checkout, data)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		address := strings.TrimSpace(r.FormValue("delivery_address"))
		data.DeliveryAddress = address
		if len(items) == 0 {
			data.Error = "Cart is empty"
			a.Render(w, a.Templates.Checkout, data)
			return
		}

		orderID, finalSummary, err := svc.CheckoutWithPayment(user.ID, address, paymentMethod)
		if err != nil {
			switch {
			case errors.Is(err, commerceSvc.ErrDeliveryAddress):
				data.Error = "Delivery address is required"
			case errors.Is(err, commerceSvc.ErrCartEmpty):
				data.Error = "Cart is empty"
			case errors.Is(err, commerceSvc.ErrInvalidPaymentMethod):
				data.Error = "Choose a supported payment method"
			case errors.Is(err, commerceSvc.ErrInsufficientStock):
				data.Error = "One or more cart items exceed available stock. Review your cart quantities."
			default:
				data.Error = fmt.Sprintf("Checkout failed: %s", err.Error())
			}
			a.Render(w, a.Templates.Checkout, data)
			return
		}

		msg := fmt.Sprintf(
			"Order %d placed successfully. Total $%.2f using %s. Delivery notice will update as partner fulfills.",
			orderID,
			finalSummary.Total,
			commerceSvc.PaymentMethodLabel(paymentMethod),
		)
		http.Redirect(w, r, "/account?message="+url.QueryEscape(msg), http.StatusFound)
	}
}
