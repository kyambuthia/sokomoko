package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
)

type cartItemInput struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

func (h *Handler) cart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.writeCart(w, r, http.StatusOK)
}

func (h *Handler) cartItemCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	uid, ok := userID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}
	var input cartItemInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	if err := h.app.Commerce.AddToCart(uid, input.ProductID, input.Quantity); err != nil {
		writeCommerceError(w, err)
		return
	}
	h.writeCart(w, r, http.StatusOK)
}

func (h *Handler) cartItem(w http.ResponseWriter, r *http.Request) {
	productID, err := strconv.Atoi(strings.Trim(pathTail(r.URL.Path, apiPrefix+"/cart/items/"), "/"))
	if err != nil || productID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_product", "Product id must be a positive integer")
		return
	}
	uid, ok := userID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var input struct {
			Quantity int `json:"quantity"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
			return
		}
		if err := h.app.Commerce.UpdateCartItem(uid, productID, input.Quantity); err != nil {
			writeCommerceError(w, err)
			return
		}
	case http.MethodDelete:
		if err := h.app.Commerce.RemoveFromCart(uid, productID); err != nil {
			writeCommerceError(w, err)
			return
		}
	default:
		methodNotAllowed(w)
		return
	}
	h.writeCart(w, r, http.StatusOK)
}

func (h *Handler) writeCart(w http.ResponseWriter, r *http.Request, status int) {
	uid, ok := userID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}
	items, subtotal, err := h.app.Commerce.GetCart(uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cart_unavailable", "Unable to load cart")
		return
	}
	writeData(w, status, mapCart(items, subtotal))
}

func writeCommerceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, commerceSvc.ErrInvalidProduct):
		writeError(w, http.StatusBadRequest, "invalid_product", "Product id is invalid")
	case errors.Is(err, commerceSvc.ErrInvalidQuantity):
		writeError(w, http.StatusBadRequest, "invalid_quantity", "Quantity is invalid")
	case errors.Is(err, commerceSvc.ErrProductNotFound):
		writeError(w, http.StatusNotFound, "product_not_found", "Product not found")
	case errors.Is(err, commerceSvc.ErrOutOfStock):
		writeError(w, http.StatusConflict, "out_of_stock", "Product is out of stock")
	case errors.Is(err, commerceSvc.ErrInsufficientStock):
		writeError(w, http.StatusConflict, "insufficient_stock", "Requested quantity is unavailable")
	default:
		writeError(w, http.StatusInternalServerError, "cart_update_failed", "Unable to update cart")
	}
}
