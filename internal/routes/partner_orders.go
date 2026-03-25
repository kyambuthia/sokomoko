package routes

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

type PartnerOrdersPageData struct {
	Title           string
	Orders          []db.FulfillmentOrder
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
	svc := partnersvc.New(a.Store)

	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.GetUserFromContext(r.Context())
		data := PartnerOrdersPageData{
			Title: "Partner Orders",
		}
		responseStatus := http.StatusOK

		if r.Method == http.MethodPost {
			err := svc.UpdateOrder(user, partnersvc.UpdateOrderInput{
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
		data.Filter = view.Filter
		data.Orders = view.Orders
		data.NewCount = view.Summary.NewCount
		data.InProgressCount = view.Summary.InProgressCount
		data.DispatchedCount = view.Summary.DispatchedCount
		data.CompletedCount = view.Summary.CompletedCount
		data.OverdueCount = view.Summary.OverdueCount

		if responseStatus != http.StatusOK {
			w.WriteHeader(responseStatus)
		}
		a.Render(w, a.Templates.PartnerOrders, data)
	}
}
