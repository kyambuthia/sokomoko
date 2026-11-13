package routes

import (
	"errors"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	logisticsvc "github.com/kyambuthia/sokomoko/internal/service/logistics"
)

const adminDeliveriesPageSize = 50 // Pending deliveries shown on each admin page.

func AdminDeliveries(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodGet, http.MethodPost)
			return
		}

		requestedPage := pageParam(r)
		status := http.StatusOK
		dispatchError := ""
		if r.Method == http.MethodPost {
			err := a.Logistics.Assign(r.Context(), logisticsActorFromContext(r), r.FormValue("order_id"), r.FormValue("courier_id"))
			switch {
			case err == nil:
				redirectWithFlash(w, r, pageHref(r, requestedPage), "success", "Courier assignment saved")
				return
			case errors.Is(err, logisticsvc.ErrForbidden):
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case errors.Is(err, logisticsvc.ErrInvalidOrderID), errors.Is(err, logisticsvc.ErrInvalidCourierID):
				dispatchError = "Choose a valid order and courier."
				status = http.StatusBadRequest
			case errors.Is(err, logisticsvc.ErrCourierNotFound):
				dispatchError = "That courier account is not active. Refresh and choose an active courier."
				status = http.StatusBadRequest
			case errors.Is(err, logisticsvc.ErrOrderNotReady):
				dispatchError = "Only shipped orders awaiting delivery can be assigned."
				status = http.StatusBadRequest
			case errors.Is(err, logisticsvc.ErrDeliveryStarted):
				dispatchError = "This delivery has already been picked up and cannot be reassigned."
				status = http.StatusConflict
			default:
				http.Error(w, "Unable to assign courier", http.StatusInternalServerError)
				return
			}
		}

		metrics, err := loadAdminMetrics(r.Context(), svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		board, total, err := svc.DeliveriesPage(r.Context(), requestedPage, adminDeliveriesPageSize)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		displayPage := min(requestedPage, pageCount(adminDeliveriesPageSize, total))
		if r.Method == http.MethodGet && redirectPastLastPage(w, r, requestedPage, adminDeliveriesPageSize, total) {
			return
		}
		if r.Method == http.MethodPost && dispatchError != "" && total > 0 && displayPage != requestedPage {
			board, total, err = svc.DeliveriesPage(r.Context(), displayPage, adminDeliveriesPageSize)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
		dispatch, err := a.Logistics.DispatchBoard(r.Context(), logisticsActorFromContext(r))
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		page := adminDeliveriesPage(metrics, adminRoleFromContext(r), board)
		page.DeliveriesPager = newPager(r, displayPage, adminDeliveriesPageSize, total)
		page.Dispatch = dispatch
		page.DispatchError = dispatchError
		page.CourierHostURL = siblingURL(r, courierHostPrefix, "/login")
		if flash := popFlash(w, r); flash.Message != "" {
			if flash.Kind == "danger" {
				page.DispatchError = flash.Message
			} else {
				page.Message = flash.Message
			}
		}
		renderAdminPage(a, w, r, status, page)
	}
}
