package routes

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
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
	DeliveryAddress string
	Error           string
	Message         string
	CanCheckout     bool
}

func CartPage(a *app.App) http.HandlerFunc {
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

		items, subtotal, err := a.Store.GetCartItems(user.ID)
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
		if err != nil || productID <= 0 {
			http.Redirect(w, r, "/cart?error=Invalid+product", http.StatusFound)
			return
		}
		qty := 1
		if qRaw := strings.TrimSpace(r.FormValue("quantity")); qRaw != "" {
			if parsed, parseErr := strconv.Atoi(qRaw); parseErr == nil && parsed > 0 {
				qty = parsed
			}
		}

		product, err := a.Store.GetProductByID(productID)
		if err != nil || product == nil {
			http.Redirect(w, r, "/cart?error=Product+not+found", http.StatusFound)
			return
		}
		if product.StockQuantity <= 0 {
			http.Redirect(w, r, "/cart?error=Product+is+out+of+stock", http.StatusFound)
			return
		}

		if err := a.Store.AddToCart(user.ID, productID, qty); err != nil {
			http.Redirect(w, r, "/cart?error=Unable+to+add+item", http.StatusFound)
			return
		}

		http.Redirect(w, r, "/cart?message=Item+added+to+cart", http.StatusFound)
	}
}

func CartUpdate(a *app.App) http.HandlerFunc {
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
		if err != nil || productID <= 0 {
			http.Redirect(w, r, "/cart?error=Invalid+product", http.StatusFound)
			return
		}
		qty, err := strconv.Atoi(strings.TrimSpace(r.FormValue("quantity")))
		if err != nil {
			http.Redirect(w, r, "/cart?error=Invalid+quantity", http.StatusFound)
			return
		}

		if err := a.Store.UpdateCartQuantity(user.ID, productID, qty); err != nil {
			http.Redirect(w, r, "/cart?error=Unable+to+update+item", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/cart?message=Cart+updated", http.StatusFound)
	}
}

func CartRemove(a *app.App) http.HandlerFunc {
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
		if err != nil || productID <= 0 {
			http.Redirect(w, r, "/cart?error=Invalid+product", http.StatusFound)
			return
		}

		if err := a.Store.RemoveFromCart(user.ID, productID); err != nil {
			http.Redirect(w, r, "/cart?error=Unable+to+remove+item", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/cart?message=Item+removed", http.StatusFound)
	}
}

func Checkout(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.GetUserFromContext(r.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		items, subtotal, err := a.Store.GetCartItems(user.ID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := CheckoutPageData{
			Title:       "Checkout",
			Items:       items,
			Subtotal:    subtotal,
			CanCheckout: len(items) > 0,
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
		if address == "" {
			data.Error = "Delivery address is required"
			a.Render(w, a.Templates.Checkout, data)
			return
		}
		if len(items) == 0 {
			data.Error = "Cart is empty"
			a.Render(w, a.Templates.Checkout, data)
			return
		}

		orderID, err := a.Store.PlaceOrderFromCart(user.ID, address)
		if err != nil {
			data.Error = fmt.Sprintf("Checkout failed: %s", err.Error())
			a.Render(w, a.Templates.Checkout, data)
			return
		}

		msg := fmt.Sprintf("Order %d placed successfully. Delivery notice will update as partner fulfills.", orderID)
		http.Redirect(w, r, "/account?message="+url.QueryEscape(msg), http.StatusFound)
	}
}
