package routes

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/money"
	checkoutsvc "github.com/kyambuthia/sokomoko/internal/service/checkout"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

type CartPageData struct {
	Title     string
	Items     []commerceSvc.CartItem
	Subtotal  money.Cents
	ItemCount int
	Flash     Flash
	CSRFToken string
	MaxQty    int
}

type CheckoutPageData struct {
	Title           string
	Items           []checkoutsvc.Item
	Subtotal        money.Cents
	ShippingFee     money.Cents
	TaxAmount       money.Cents
	TotalAmount     money.Cents
	DeliveryAddress string
	PaymentMethod   string
	PaymentMethods  []PaymentMethodOption
	IdempotencyKey  string
	FormAction      string
	ExpiresAt       string
	Error           string
	CanCheckout     bool
	CSRFToken       string
}

type PaymentMethodOption struct {
	Value string
	Label string
}

// cartJSON is returned by cart endpoints to JSON clients.
type cartJSON struct {
	OK        bool          `json:"ok"`
	Message   string        `json:"message,omitempty"`
	Error     string        `json:"error,omitempty"`
	ItemCount int           `json:"itemCount"`
	Subtotal  string        `json:"subtotal"`
	Lines     []cartLineDTO `json:"lines"`
}

type cartLineDTO struct {
	ProductID int    `json:"productId"`
	Quantity  int    `json:"quantity"`
	LineTotal string `json:"lineTotal"`
	Available int    `json:"available"`
}

func newCartJSON(cart commerceSvc.Cart) cartJSON {
	out := cartJSON{OK: true, ItemCount: cart.ItemCount, Subtotal: cart.Subtotal.String(), Lines: []cartLineDTO{}}
	for _, item := range cart.Items {
		out.Lines = append(out.Lines, cartLineDTO{
			ProductID: item.ProductID, Quantity: item.Quantity, LineTotal: item.LineTotal.String(), Available: item.StockQuantity,
		})
	}
	return out
}

func CartPage(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		userID, _ := requestUserID(r)
		cart, err := a.Commerce.Cart(userID)
		if err != nil {
			log.Printf("cart page: %v", err)
			serverError(w, r)
			return
		}
		a.Render(w, a.Templates.Cart, CartPageData{
			Title:     "Cart",
			Items:     cart.Items,
			Subtotal:  cart.Subtotal,
			ItemCount: cart.ItemCount,
			Flash:     popFlash(w, r),
			CSRFToken: a.Auth.CSRFToken(r),
			MaxQty:    commerceSvc.MaxLineQuantity,
		})
	}
}

// CartSummaryAPI serves GET /api/cart for the header cart badge.
func CartSummaryAPI(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		userID, _ := requestUserID(r)
		cart, err := a.Commerce.Cart(userID)
		if err != nil {
			log.Printf("cart api: %v", err)
			app.JSON(w, http.StatusInternalServerError, cartJSON{Error: "cart unavailable"})
			return
		}
		app.JSON(w, http.StatusOK, newCartJSON(cart))
	}
}

type cartMutation func(userID, productID, quantity int) error

// cartAction handles the add/update/remove form posts. Browsers get a redirect
// with a flash message; JSON clients get the updated cart.
func cartAction(a *app.App, mutate cartMutation, successMessage string, quantityRequired bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		userID, _ := requestUserID(r)

		productID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("product_id")))
		if err != nil || productID <= 0 {
			respondCartError(a, w, r, userID, http.StatusBadRequest, "Invalid product")
			return
		}
		quantity := 1
		if raw := strings.TrimSpace(r.FormValue("quantity")); raw != "" {
			if quantity, err = strconv.Atoi(raw); err != nil {
				respondCartError(a, w, r, userID, http.StatusBadRequest, "Invalid quantity")
				return
			}
		} else if quantityRequired {
			respondCartError(a, w, r, userID, http.StatusBadRequest, "Invalid quantity")
			return
		}

		if err := mutate(userID, productID, quantity); err != nil {
			status, message := cartErrorMessage(err)
			if status == http.StatusInternalServerError {
				log.Printf("cart mutation: %v", err)
			}
			respondCartError(a, w, r, userID, status, message)
			return
		}

		if auth.WantsJSON(r) {
			cart, err := a.Commerce.Cart(userID)
			if err != nil {
				app.JSON(w, http.StatusInternalServerError, cartJSON{Error: "cart unavailable"})
				return
			}
			body := newCartJSON(cart)
			body.Message = successMessage
			app.JSON(w, http.StatusOK, body)
			return
		}
		redirectWithFlash(w, r, "/cart", "success", successMessage)
	}
}

func respondCartError(a *app.App, w http.ResponseWriter, r *http.Request, userID, status int, message string) {
	if auth.WantsJSON(r) {
		body := cartJSON{Error: message, Lines: []cartLineDTO{}}
		if cart, err := a.Commerce.Cart(userID); err == nil {
			body = newCartJSON(cart)
			body.OK = false
			body.Error = message
		}
		app.JSON(w, status, body)
		return
	}
	redirectWithFlash(w, r, "/cart", "danger", message)
}

func cartErrorMessage(err error) (int, string) {
	switch {
	case errors.Is(err, commerceSvc.ErrInvalidProduct):
		return http.StatusBadRequest, "Invalid product"
	case errors.Is(err, commerceSvc.ErrInvalidQuantity):
		return http.StatusBadRequest, fmt.Sprintf("Quantity must be between 1 and %d", commerceSvc.MaxLineQuantity)
	case errors.Is(err, commerceSvc.ErrProductNotFound):
		return http.StatusNotFound, "Product not found"
	case errors.Is(err, commerceSvc.ErrOutOfStock):
		return http.StatusConflict, "Product is out of stock"
	case errors.Is(err, commerceSvc.ErrInsufficientStock):
		return http.StatusConflict, "Requested quantity exceeds available stock"
	default:
		return http.StatusInternalServerError, "Unable to update your cart right now"
	}
}

func CartAdd(a *app.App) http.HandlerFunc {
	return cartAction(a, a.Commerce.AddToCart, "Item added to cart", false)
}

func CartUpdate(a *app.App) http.HandlerFunc {
	return cartAction(a, a.Commerce.UpdateCartItem, "Cart updated", true)
}

func CartRemove(a *app.App) http.HandlerFunc {
	return cartAction(a, func(userID, productID, _ int) error {
		return a.Commerce.RemoveFromCart(userID, productID)
	}, "Item removed", false)
}

func checkoutPaymentOptions(paymentOptions []paymentsvc.MethodOption) []PaymentMethodOption {
	options := make([]PaymentMethodOption, 0, len(paymentOptions))
	for _, option := range paymentOptions {
		options = append(options, PaymentMethodOption{Value: option.Value, Label: option.Label})
	}
	return options
}

func checkoutPage(a *app.App, r *http.Request, state checkoutsvc.PageState, paymentMethod string) CheckoutPageData {
	selected := strings.TrimSpace(paymentMethod)
	if selected == "" {
		selected = state.PaymentMethod
	}
	if selected == "" {
		selected = paymentsvc.MethodCashOnDelivery
	}
	expires := ""
	if !state.ExpiresAt.IsZero() {
		expires = state.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return CheckoutPageData{
		Title:           "Checkout",
		Items:           state.Items,
		Subtotal:        state.Summary.Subtotal,
		ShippingFee:     state.Summary.ShippingFee,
		TaxAmount:       state.Summary.TaxAmount,
		TotalAmount:     state.Summary.Total,
		DeliveryAddress: state.DeliveryAddress,
		PaymentMethod:   selected,
		PaymentMethods:  checkoutPaymentOptions(a.Payment.SupportedMethodOptions()),
		IdempotencyKey:  state.Token,
		FormAction:      checkoutFormAction(state.Token),
		ExpiresAt:       expires,
		CanCheckout:     state.CanCheckout,
		CSRFToken:       a.Auth.CSRFToken(r),
	}
}

func checkoutFormAction(token string) string {
	key := strings.TrimSpace(token)
	if key == "" {
		return "/checkout"
	}
	return "/checkout?checkout=" + urlQueryEscape(key)
}

func checkoutErrorMessage(err error) (string, bool) {
	switch {
	case errors.Is(err, checkoutsvc.ErrDeliveryAddress):
		return "Delivery address is required", true
	case errors.Is(err, checkoutsvc.ErrCartEmpty):
		return "Your cart is empty", true
	case errors.Is(err, checkoutsvc.ErrInvalidPaymentMethod):
		return "Choose a supported payment method", true
	case errors.Is(err, checkoutsvc.ErrInsufficientStock):
		return "One or more cart items exceed available stock. Review your cart quantities.", true
	default:
		return "", false
	}
}

func Checkout(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := requestUserID(r)
		paymentMethod := strings.TrimSpace(r.FormValue("payment_method"))

		switch r.Method {
		case http.MethodGet, http.MethodHead:
			state, err := a.Checkout.PreparedCheckout(userID, r.URL.Query().Get("checkout"))
			data := checkoutPage(a, r, state, paymentMethod)
			if err != nil {
				message, known := checkoutErrorMessage(err)
				if !known {
					log.Printf("checkout prepare: %v", err)
					serverError(w, r)
					return
				}
				data.Error = message
				data.CanCheckout = false
			}
			a.Render(w, a.Templates.Checkout, data)
			return
		case http.MethodPost:
		default:
			methodNotAllowed(w, http.MethodGet, http.MethodHead, http.MethodPost)
			return
		}

		address := strings.TrimSpace(r.FormValue("delivery_address"))
		key := strings.TrimSpace(r.FormValue("idempotency_key"))
		if key == "" {
			key = strings.TrimSpace(r.URL.Query().Get("checkout"))
		}

		orderID, summary, err := a.Checkout.CheckoutWithPayment(userID, address, paymentMethod, key)
		if err != nil {
			message, known := checkoutErrorMessage(err)
			if !known {
				log.Printf("checkout place order: %v", err)
				message = "We could not place your order. Please try again."
			}
			state, stateErr := a.Checkout.SaveDraft(userID, key, address, paymentMethod)
			if stateErr != nil {
				if _, known := checkoutErrorMessage(stateErr); !known {
					log.Printf("checkout save draft: %v", stateErr)
					serverError(w, r)
					return
				}
			}
			data := checkoutPage(a, r, state, paymentMethod)
			if strings.TrimSpace(state.Token) == "" {
				data.IdempotencyKey = key
				data.FormAction = checkoutFormAction(key)
			}
			if stateErr != nil {
				data.CanCheckout = false
			}
			data.Error = message
			a.RenderStatus(w, a.Templates.Checkout, http.StatusUnprocessableEntity, data)
			return
		}

		method := paymentMethod
		if method == "" {
			method = paymentsvc.MethodCashOnDelivery
		}
		redirectWithFlash(w, r, fmt.Sprintf("/account#order-%d", orderID), "success", fmt.Sprintf(
			"Order #%d placed. Total $%s using %s. We'll update the delivery notice as your order is fulfilled.",
			orderID, summary.Total, a.Payment.MethodLabel(method),
		))
	}
}
