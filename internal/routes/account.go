package routes

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

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

// AccountOrderCancel cancels one of the customer's orders before the vendor
// accepts it and before it is paid.
func AccountOrderCancel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		userID, _ := requestUserID(r)
		orderID, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || orderID <= 0 {
			http.NotFound(w, r)
			return
		}
		err = a.Account.CancelOrder(r.Context(), userID, orderID)
		switch {
		case err == nil:
			redirectWithFlash(w, r, "/account", "success", fmt.Sprintf("Order #%d cancelled", orderID))
		case errors.Is(err, accountsvc.ErrOrderNotFound):
			http.NotFound(w, r)
		case errors.Is(err, accountsvc.ErrOrderNotCancellable):
			redirectWithFlash(w, r, "/account", "danger", fmt.Sprintf("Order #%d is already being prepared or paid. Contact the store to cancel it.", orderID))
		default:
			log.Printf("cancel order: %v", err)
			serverError(w, r)
		}
	}
}
