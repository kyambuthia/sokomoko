package routes

import (
	"log"
	"net/http"

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
