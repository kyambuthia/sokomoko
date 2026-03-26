package routes

import (
	"net/http"
	"strings"

	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
)

func accountPage(r *http.Request, orders []accountsvc.Order) AccountPageData {
	return AccountPageData{
		Title:   "Your Account",
		Message: strings.TrimSpace(r.URL.Query().Get("message")),
		Error:   strings.TrimSpace(r.URL.Query().Get("error")),
		Orders:  orders,
	}
}
