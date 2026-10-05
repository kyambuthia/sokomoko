package routes

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

type PartnerOrdersPageData struct {
	Title           string
	Orders          []partnersvc.Order
	Message         string
	Error           string
	Filter          string
	NewCount        int
	InProgressCount int
	DispatchedCount int
	CompletedCount  int
	OverdueCount    int
	CSRFToken       string
}

func PartnerOrders(a *app.App) http.HandlerFunc {
	svc := a.Partner

	return func(w http.ResponseWriter, r *http.Request) {
		data := PartnerOrdersPageData{Title: "Partner Orders"}
		responseStatus := http.StatusOK

		if r.Method == http.MethodPost {
			err := svc.UpdateOrder(partnerActorFromContext(r), partnersvc.UpdateOrderInput{
				OrderID:        r.FormValue("order_id"),
				PartnerStatus:  r.FormValue("partner_status"),
				DeliveryStatus: r.FormValue("delivery_status"),
				DeliveryNotice: r.FormValue("delivery_notice"),
			})
			switch {
			case errors.Is(err, partnersvc.ErrForbiddenOrderUpdate):
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case errors.Is(err, partnersvc.ErrInvalidOrderID):
				data.Error = "Invalid order id"
				responseStatus = http.StatusBadRequest
			case errors.Is(err, partnersvc.ErrOrderNotFound):
				data.Error = "Order does not exist"
				responseStatus = http.StatusNotFound
			case errors.Is(err, partnersvc.ErrInvalidOrderTransition):
				data.Error = "That status change is not allowed from the order's current state"
				responseStatus = http.StatusBadRequest
			case err != nil:
				log.Printf("partner order update: %v", err)
				data.Error = "Unable to update order"
				responseStatus = http.StatusInternalServerError
			default:
				target := "/orders"
				if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
					target += "?status=" + urlQueryEscape(status)
				}
				redirectWithFlash(w, r, target, "success", "Order fulfillment updated")
				return
			}
		} else if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if flash := popFlash(w, r); flash.Message != "" {
				data.Message = flash.Message
			}
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		view, err := svc.Orders(strings.TrimSpace(r.URL.Query().Get("status")))
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		page := partnerOrdersPage(view)
		page.Message = data.Message
		page.Error = data.Error
		page.CSRFToken = a.Auth.CSRFToken(r)
		a.RenderStatus(w, a.Templates.PartnerOrders, responseStatus, page)
	}
}
