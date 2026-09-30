package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
	checkoutsvc "github.com/kyambuthia/sokomoko/internal/service/checkout"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

const (
	apiPrefix       = "/api/v1"
	maxPageLimit    = 50
	defaultPageSize = 20
)

type Handler struct {
	app *app.App
}

func Register(a *app.App, mux *http.ServeMux) {
	h := &Handler{app: a}
	mux.HandleFunc(apiPrefix, h.notFound)
	mux.HandleFunc(apiPrefix+"/", h.notFound)

	// Public catalog reads.
	mux.HandleFunc(apiPrefix+"/catalog/products", h.catalogProducts)
	mux.HandleFunc(apiPrefix+"/catalog/products/", h.catalogProduct)
	mux.HandleFunc(apiPrefix+"/catalog/search", h.catalogSearch)

	// Customer session and commerce APIs.
	mux.Handle(apiPrefix+"/me", h.authenticated(http.HandlerFunc(h.me)))
	mux.Handle(apiPrefix+"/cart", h.authenticated(http.HandlerFunc(h.cart)))
	mux.Handle(apiPrefix+"/cart/items", h.authenticated(http.HandlerFunc(h.cartItemCreate)))
	mux.Handle(apiPrefix+"/cart/items/", h.authenticated(http.HandlerFunc(h.cartItem)))
	mux.Handle(apiPrefix+"/checkout", h.authenticated(http.HandlerFunc(h.checkout)))
	mux.Handle(apiPrefix+"/checkout/", h.authenticated(http.HandlerFunc(h.checkoutSession)))
	mux.Handle(apiPrefix+"/account/orders", h.authenticated(http.HandlerFunc(h.accountOrders)))
	mux.Handle(apiPrefix+"/account/orders/", h.authenticated(http.HandlerFunc(h.accountOrder)))

	// Workspace APIs. Role checks are applied per resource so one mux can be
	// used on storefront, admin, and partner hosts without duplicating routes.
	mux.Handle(apiPrefix+"/admin/", h.role([]string{"admin", "staff"}, http.HandlerFunc(h.admin)))
	mux.Handle(apiPrefix+"/partner/", h.role([]string{"admin", "staff", "partner"}, http.HandlerFunc(h.partner)))
}

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "API route not found")
}

func (h *Handler) authenticated(next http.Handler) http.Handler {
	return h.app.Auth.APIAuthMiddleware(next)
}

func (h *Handler) role(roles []string, next http.Handler) http.Handler {
	return h.authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := auth.GetUserFromContext(r.Context())
		if user == nil {
			writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
			return
		}
		for _, role := range roles {
			if user.Role == role {
				next.ServeHTTP(w, r)
				return
			}
		}
		writeError(w, http.StatusForbidden, "forbidden", "You do not have access to this resource")
	}))
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func parsePositiveID(raw string) (int, bool) {
	id, err := strconv.Atoi(strings.TrimSpace(raw))
	return id, err == nil && id > 0
}

func pathTail(path, prefix string) string {
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}

func userID(r *http.Request) (int, bool) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil || user.ID <= 0 {
		return 0, false
	}
	return user.ID, true
}

type productDTO struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	Slug          string  `json:"slug"`
	Description   string  `json:"description"`
	Price         float64 `json:"price"`
	StockQuantity int     `json:"stock_quantity"`
	Category      string  `json:"category,omitempty"`
	PrimaryImage  string  `json:"primary_image"`
}

func mapProduct(product catalogsvc.Product) productDTO {
	return productDTO{
		ID: product.ID, Name: product.Name, Slug: product.Slug,
		Description: product.Description, Price: product.Price,
		StockQuantity: product.StockQuantity, Category: product.Category,
		PrimaryImage: product.PrimaryImage,
	}
}

type cartItemDTO struct {
	ProductID     int     `json:"product_id"`
	ProductName   string  `json:"product_name"`
	ProductSlug   string  `json:"product_slug,omitempty"`
	ProductImage  string  `json:"product_image,omitempty"`
	UnitPrice     float64 `json:"unit_price"`
	Quantity      int     `json:"quantity"`
	StockQuantity int     `json:"stock_quantity"`
	LineTotal     float64 `json:"line_total"`
}

type cartDTO struct {
	Items    []cartItemDTO `json:"items"`
	Subtotal float64       `json:"subtotal"`
}

func mapCart(items []commerceSvc.CartItem, subtotal float64) cartDTO {
	mapped := make([]cartItemDTO, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, cartItemDTO{
			ProductID: item.ProductID, ProductName: item.ProductName,
			UnitPrice: item.UnitPrice, Quantity: item.Quantity,
			StockQuantity: item.StockQuantity, LineTotal: item.LineTotal,
		})
	}
	return cartDTO{Items: mapped, Subtotal: subtotal}
}

type checkoutItemDTO struct {
	ProductID   int     `json:"product_id"`
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	LineTotal   float64 `json:"line_total"`
}

type checkoutDTO struct {
	Token           string             `json:"token"`
	Items           []checkoutItemDTO  `json:"items"`
	Subtotal        float64            `json:"subtotal"`
	ShippingFee     float64            `json:"shipping_fee"`
	TaxAmount       float64            `json:"tax_amount"`
	TotalAmount     float64            `json:"total_amount"`
	PaymentMethod   string             `json:"payment_method"`
	DeliveryAddress string             `json:"delivery_address,omitempty"`
	CanCheckout     bool               `json:"can_checkout"`
	PaymentMethods  []paymentMethodDTO `json:"payment_methods,omitempty"`
}

type paymentMethodDTO struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func mapCheckout(state checkoutsvc.PageState, methods []paymentMethodDTO) checkoutDTO {
	items := make([]checkoutItemDTO, 0, len(state.Items))
	for _, item := range state.Items {
		items = append(items, checkoutItemDTO{
			ProductID: item.ProductID, ProductName: item.ProductName,
			Quantity: item.Quantity, UnitPrice: item.UnitPrice, LineTotal: item.LineTotal,
		})
	}
	return checkoutDTO{
		Token: state.Token, Items: items, Subtotal: state.Summary.Subtotal,
		ShippingFee: state.Summary.ShippingFee, TaxAmount: state.Summary.TaxAmount,
		TotalAmount: state.Summary.Total, PaymentMethod: state.PaymentMethod,
		DeliveryAddress: state.DeliveryAddress, CanCheckout: state.CanCheckout,
		PaymentMethods: methods,
	}
}

type orderItemDTO struct {
	ProductID   int     `json:"product_id"`
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	LineTotal   float64 `json:"line_total"`
}

type orderDTO struct {
	ID              int            `json:"id"`
	Status          string         `json:"status"`
	PartnerStatus   string         `json:"partner_status"`
	DeliveryStatus  string         `json:"delivery_status"`
	DeliveryNotice  string         `json:"delivery_notice,omitempty"`
	DeliveryAddress string         `json:"delivery_address,omitempty"`
	TotalAmount     float64        `json:"total_amount"`
	CreatedAt       time.Time      `json:"created_at"`
	Items           []orderItemDTO `json:"items,omitempty"`
}

func mapAccountOrder(order accountsvc.Order) orderDTO {
	items := make([]orderItemDTO, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, orderItemDTO{
			ProductID: item.ProductID, ProductName: item.ProductName,
			Quantity: item.Quantity, UnitPrice: item.UnitPrice, LineTotal: item.LineTotal,
		})
	}
	return orderDTO{
		ID: order.ID, Status: order.Status, PartnerStatus: order.PartnerStatus,
		DeliveryStatus: order.DeliveryStatus, DeliveryNotice: order.DeliveryNotice,
		DeliveryAddress: order.DeliveryAddress, TotalAmount: order.TotalAmount,
		CreatedAt: order.CreatedAt, Items: items,
	}
}

func mapAdminOrder(order adminsvc.Order) orderDTO {
	items := make([]orderItemDTO, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, orderItemDTO{ProductName: item.ProductName, Quantity: item.Quantity, LineTotal: item.LineTotal})
	}
	return orderDTO{
		ID: order.ID, Status: order.Status, PartnerStatus: order.PartnerStatus,
		DeliveryStatus: order.DeliveryStatus, DeliveryNotice: order.DeliveryNotice,
		DeliveryAddress: order.DeliveryAddress, TotalAmount: order.TotalAmount, Items: items,
	}
}

func mapPartnerOrder(order partnersvc.Order) orderDTO {
	items := make([]orderItemDTO, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, orderItemDTO{ProductID: item.ProductID, ProductName: item.ProductName, Quantity: item.Quantity, LineTotal: item.LineTotal})
	}
	return orderDTO{
		ID: order.ID, Status: order.Status, PartnerStatus: order.PartnerStatus,
		DeliveryStatus: order.DeliveryStatus, DeliveryNotice: order.DeliveryNotice,
		DeliveryAddress: order.DeliveryAddress, TotalAmount: order.TotalAmount, Items: items,
	}
}

func mapPartnerProduct(product partnersvc.Product) productDTO {
	return productDTO{ID: product.ID, Name: product.Name, Category: product.Category, Price: product.Price, StockQuantity: product.StockQuantity}
}

func mapAdminProduct(product adminsvc.Product) productDTO {
	return productDTO{Name: product.Name, Category: product.Category, Price: product.Price, StockQuantity: product.StockQuantity}
}

func isNotFound(err error) bool {
	return errors.Is(err, db.ErrOrderNotFound) || errors.Is(err, db.ErrCheckoutNotFound)
}
