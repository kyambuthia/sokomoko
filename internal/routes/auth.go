package routes

import (
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
)

type AccountPageData struct {
	Title   string
	Message string
	Error   string
	Orders  []db.CustomerOrder
}

func Auth(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		user := auth.GetUserFromContext(req.Context())
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		orders, err := a.Account.OrdersForUser(user.ID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		a.Render(w, a.Templates.Account, AccountPageData{
			Title:   "Your Account",
			Message: strings.TrimSpace(req.URL.Query().Get("message")),
			Error:   strings.TrimSpace(req.URL.Query().Get("error")),
			Orders:  orders,
		})
	}
}
