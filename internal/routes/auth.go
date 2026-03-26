package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
)

type AccountPageData struct {
	Title   string
	Message string
	Error   string
	Orders  []accountsvc.Order
}

func Auth(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID, ok := requestUserID(req)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		orders, err := a.Account.OrdersForUser(userID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		a.Render(w, a.Templates.Account, accountPage(req, orders))
	}
}
