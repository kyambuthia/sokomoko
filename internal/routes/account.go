package routes

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
)

type AccountPageData struct {
	Title     string
	Flash     Flash
	Orders    []accountsvc.Order
	Viewer    Viewer
	CSRFToken string
}

// Account serves the customer's order history.
func Account(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		userID, _ := requestUserID(r)
		orders, err := a.Account.OrdersForUser(userID)
		if err != nil {
			log.Printf("account orders: %v", err)
			serverError(w, r)
			return
		}
		a.Render(w, a.Templates.Account, AccountPageData{
			Title:     "Your Account",
			Flash:     popFlash(w, r),
			Orders:    orders,
			Viewer:    viewerFromRequest(r),
			CSRFToken: a.Auth.CSRFToken(r),
		})
	}
}

type orderStatusJSON struct {
	ID             int    `json:"id"`
	Status         string `json:"status"`
	PartnerStatus  string `json:"partnerStatus"`
	DeliveryStatus string `json:"deliveryStatus"`
	DeliveryNotice string `json:"deliveryNotice"`
	Total          string `json:"total"`
	UpdatedAt      string `json:"updatedAt"`
}

// OrderStatusAPI serves GET /api/orders/status?id= for the live order tracker.
// Customers can only read their own orders.
func OrderStatusAPI(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		orderID, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
		if err != nil || orderID <= 0 {
			app.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid order id"})
			return
		}
		userID, _ := requestUserID(r)
		order, err := a.Account.OrderForUser(userID, orderID)
		if err != nil {
			log.Printf("order status api: %v", err)
			app.JSON(w, http.StatusInternalServerError, map[string]string{"error": "status unavailable"})
			return
		}
		if order == nil {
			app.JSON(w, http.StatusNotFound, map[string]string{"error": "order not found"})
			return
		}
		app.JSON(w, http.StatusOK, orderStatusJSON{
			ID:             order.ID,
			Status:         order.Status,
			PartnerStatus:  order.PartnerStatus,
			DeliveryStatus: order.DeliveryStatus,
			DeliveryNotice: order.DeliveryNotice,
			Total:          order.TotalAmount.String(),
			UpdatedAt:      order.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
}
