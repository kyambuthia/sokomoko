package routes

import (
	"errors"
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
				data.Error = "Unable to update order"
				responseStatus = http.StatusBadRequest
			case errors.Is(err, partnersvc.ErrInvalidOrderTransition):
				data.Error = "Unable to update order"
				responseStatus = http.StatusBadRequest
			case err != nil:
				data.Error = "Unable to update order"
				responseStatus = http.StatusBadRequest
			default:
				data.Message = "Order fulfillment updated"
			}
		} else if r.Method != http.MethodGet {
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

		if responseStatus != http.StatusOK {
			w.WriteHeader(responseStatus)
		}
		a.Render(w, a.Templates.PartnerOrders, page)
	}
}
