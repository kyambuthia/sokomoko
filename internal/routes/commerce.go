package routes

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	checkoutsvc "github.com/kyambuthia/sokomoko/internal/service/checkout"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

type CartPageData struct {
	Title     string
	Items     []commerceSvc.CartItem
	Subtotal  float64
	Error     string
	Message   string
	CSRFToken string
}

type CheckoutPageData struct {
	Title           string
	Items           []checkoutsvc.Item
	Subtotal        float64
	ShippingFee     float64
	TaxAmount       float64
	TotalAmount     float64
	DeliveryAddress string
	PaymentMethod   string
	PaymentMethods  []PaymentMethodOption
	IdempotencyKey  string
	FormAction      string
	Error           string
	Message         string
	CanCheckout     bool
	CSRFToken       string
}

type PaymentMethodOption struct {
	Value string
	Label string
}

func checkoutPaymentOptions(paymentOptions []paymentsvc.MethodOption) []PaymentMethodOption {
	options := make([]PaymentMethodOption, 0, len(paymentOptions))
	for _, option := range paymentOptions {
		options = append(options, PaymentMethodOption{
			Value: option.Value,
			Label: option.Label,
		})
	}
	return options
}

func CartPage(a *app.App) http.HandlerFunc {
	svc := a.Commerce

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID, ok := requestUserID(r)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		items, subtotal, err := svc.GetCart(userID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		a.Render(w, a.Templates.Cart, cartPage(r, items, subtotal, a.Auth.CSRFToken(r)))
	}
}

func CartAdd(a *app.App) http.HandlerFunc {
	svc := a.Commerce

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID, ok := requestUserID(r)
		if !ok {
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

		if err := svc.AddToCart(userID, productID, qty); err != nil {
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

		userID, ok := requestUserID(r)
		if !ok {
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

		if err := svc.UpdateCartItem(userID, productID, qty); err != nil {
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

		userID, ok := requestUserID(r)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		productID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("product_id")))
		if err != nil {
			http.Redirect(w, r, "/cart?error=Invalid+product", http.StatusFound)
			return
		}

		if err := svc.RemoveFromCart(userID, productID); err != nil {
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
	checkoutSvc := a.Checkout

	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requestUserID(r)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		csrfToken := a.Auth.CSRFToken(r)
		paymentMethod := strings.TrimSpace(r.FormValue("payment_method"))
		if r.Method == http.MethodGet {
			resumeToken := strings.TrimSpace(r.URL.Query().Get("checkout"))
			state, err := checkoutSvc.PreparedCheckout(userID, resumeToken)
			data := checkoutPage(state, paymentMethod, checkoutPaymentOptions(a.Payment.SupportedMethodOptions()), csrfToken)
			if err != nil {
				switch {
				case errors.Is(err, checkoutsvc.ErrCartEmpty):
					data.Error = "Cart is empty"
				case errors.Is(err, checkoutsvc.ErrInsufficientStock):
					data.Error = "One or more cart items exceed available stock. Review your cart quantities."
				default:
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
			}
			a.Render(w, a.Templates.Checkout, data)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		address := strings.TrimSpace(r.FormValue("delivery_address"))
		idempotencyKey := strings.TrimSpace(r.FormValue("idempotency_key"))
		if idempotencyKey == "" {
			idempotencyKey = strings.TrimSpace(r.URL.Query().Get("checkout"))
		}
		if idempotencyKey == "" {
			state, prepErr := checkoutSvc.PreparedCheckout(userID, "")
			if prepErr != nil {
				data := checkoutPage(state, paymentMethod, checkoutPaymentOptions(a.Payment.SupportedMethodOptions()), csrfToken)
				if errors.Is(prepErr, checkoutsvc.ErrCartEmpty) {
					data.Error = "Cart is empty"
					a.Render(w, a.Templates.Checkout, data)
					return
				}
				if errors.Is(prepErr, checkoutsvc.ErrInsufficientStock) {
					data.Error = "One or more cart items exceed available stock. Review your cart quantities."
					a.Render(w, a.Templates.Checkout, data)
					return
				}
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			idempotencyKey = state.Token
		}

		orderID, finalSummary, err := checkoutSvc.CheckoutWithPayment(userID, address, paymentMethod, idempotencyKey)
		if err != nil {
			state, stateErr := checkoutSvc.SaveDraft(userID, idempotencyKey, address, paymentMethod)
			if stateErr != nil && !errors.Is(stateErr, checkoutsvc.ErrCartEmpty) && !errors.Is(stateErr, checkoutsvc.ErrInsufficientStock) {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			data := checkoutPage(state, paymentMethod, checkoutPaymentOptions(a.Payment.SupportedMethodOptions()), csrfToken)
			if strings.TrimSpace(state.Token) != "" {
				idempotencyKey = state.Token
			}
			data.IdempotencyKey = idempotencyKey
			switch {
			case errors.Is(err, checkoutsvc.ErrDeliveryAddress):
				data.Error = "Delivery address is required"
			case errors.Is(err, checkoutsvc.ErrCartEmpty):
				data.Error = "Cart is empty"
			case errors.Is(err, checkoutsvc.ErrInvalidPaymentMethod):
				data.Error = "Choose a supported payment method"
			case errors.Is(err, checkoutsvc.ErrInsufficientStock):
				data.Error = "One or more cart items exceed available stock. Review your cart quantities."
			default:
				data.Error = fmt.Sprintf("Checkout failed: %s", err.Error())
			}
			a.Render(w, a.Templates.Checkout, data)
			return
		}

		successMethod := strings.TrimSpace(paymentMethod)
		if successMethod == "" {
			successMethod = paymentsvc.MethodCashOnDelivery
		}

		msg := fmt.Sprintf(
			"Order %d placed successfully. Total $%.2f using %s. Delivery notice will update as partner fulfills.",
			orderID,
			finalSummary.Total,
			a.Payment.MethodLabel(successMethod),
		)
		http.Redirect(w, r, "/account?message="+url.QueryEscape(msg), http.StatusFound)
	}
}
