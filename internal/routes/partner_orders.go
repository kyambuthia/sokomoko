package routes

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
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
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.GetUserFromContext(r.Context())
		if user == nil || (user.Role != "admin" && user.Role != "staff") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		data := PartnerOrdersPageData{
			Title:  "Partner Orders",
			Filter: strings.TrimSpace(r.URL.Query().Get("status")),
		}
		if data.Filter == "" {
			data.Filter = "all"
		}

		if r.Method == http.MethodPost {
			orderID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("order_id")))
			if err != nil || orderID <= 0 {
				data.Error = "Invalid order id"
			} else {
				partnerStatus := strings.TrimSpace(r.FormValue("partner_status"))
				deliveryStatus := strings.TrimSpace(r.FormValue("delivery_status"))
				note := strings.TrimSpace(r.FormValue("delivery_notice"))
				if err := a.Store.UpdateOrderFulfillment(orderID, partnerStatus, deliveryStatus, note); err != nil {
					data.Error = "Unable to update order: " + err.Error()
				} else {
					data.Message = "Order fulfillment updated"
					_ = a.Store.CreateAuditLog(user.ID, "partner.fulfillment.update", "order", orderID, "partner="+partnerStatus+",delivery="+deliveryStatus)
				}
			}
		} else if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		orders, err := a.Store.ListOrdersForFulfillment()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		filtered := make([]db.FulfillmentOrder, 0, len(orders))
		for _, order := range orders {
			include := data.Filter == "all" || order.PartnerStatus == data.Filter
			if include {
				filtered = append(filtered, order)
			}
		}
		data.Orders = filtered

		summary, err := a.Store.GetPartnerOrderSummary()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		data.NewCount = summary.NewCount
		data.InProgressCount = summary.InProgressCount
		data.DispatchedCount = summary.DispatchedCount
		data.CompletedCount = summary.CompletedCount
		data.OverdueCount = summary.OverdueCount

		a.Render(w, a.Templates.PartnerOrders, data)
	}
}
