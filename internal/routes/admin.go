package routes

import (
	"context"
	"errors"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/money"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	logisticsvc "github.com/kyambuthia/sokomoko/internal/service/logistics"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

const (
	adminProductsPageSize = 50 // Number of products shown on each admin catalog page.
	adminOrdersPageSize   = 50 // Number of orders shown on each admin order page.
)

type AdminPageData struct {
	Title           string
	CSRFToken       string
	Message         string
	Role            string
	ProductCount    int
	SessionCount    int
	AdminCount      int
	StaffCount      int
	UserCount       int
	Products        []adminsvc.Product
	ProductsPager   Pager
	TeamMembers     []adminsvc.TeamMember
	TeamQuery       string
	TeamPager       Pager
	Deactivated     []adminsvc.TeamMember
	Orders          []adminsvc.Order
	OrdersPager     Pager
	Deliveries      adminsvc.DeliveryBoard
	DeliveriesPager Pager
	Dispatch        logisticsvc.DispatchBoard
	DispatchError   string
	CourierHostURL  string
	AuditLogs       []adminsvc.AuditLog
	OrderCount      int
	CancelledCount  int
	RevenueTotal    money.Cents
	PendingCount    int
	ShippedCount    int
	DeliveredCount  int
	TeamError       string
	TeamMessage     string
	OrderError      string
	OrderMessage    string
	FormError       string
	FormMessage     string
	Categories      []adminsvc.Category
	CategoryForm    adminsvc.CategoryInput
	EditCategoryID  int // the category form edits this category
	Partners        []adminsvc.Partner
	PartnerAccounts []adminsvc.PartnerAccount
	PartnerForm     adminsvc.PartnerInput
	EditPartnerID   int // the partner form edits this partner
	AccountForm     adminsvc.PartnerAccountInput

	Pricing           paymentsvc.Pricing
	Providers         []paymentsvc.ProviderView
	PaymentExceptions []paymentsvc.PaymentException
	Currencies        []money.Currency
	SecretsConfigured bool
	PublicBase        string
	FormProvider      string // provider whose form produced FormError
	OnlinePayments    bool   // an online payment method is enabled
	PaymentAttention  int    // payment exceptions waiting on an admin
}

func renderAdminPage(a *app.App, w http.ResponseWriter, r *http.Request, status int, data AdminPageData) {
	data.CSRFToken = a.Auth.CSRFToken(r)
	a.RenderStatus(w, a.Templates.Admin, status, data)
}

func loadAdminMetrics(ctx context.Context, svc app.AdminService) (adminsvc.Metrics, error) {
	return svc.Metrics(ctx)
}

// AdminDashboard serves the main admin dashboard.
func AdminDashboard(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(r.Context(), svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		page := adminDashboardPage(metrics, adminRoleFromContext(r))
		for _, option := range a.Payment.MethodOptions(r.Context()) {
			page.OnlinePayments = page.OnlinePayments || option.Online
		}
		if page.Role == "admin" {
			if page.PaymentAttention, err = a.Payment.ExceptionsNeedingAttention(r.Context()); err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
		renderAdminPage(a, w, r, http.StatusOK, page)
	}
}

// AdminProducts handles product management.
func AdminProducts(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(r.Context(), svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		requestedPage := pageParam(r)
		products, total, err := svc.ProductsPage(r.Context(), requestedPage, adminProductsPageSize)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if total == 0 && requestedPage > 1 {
			http.Redirect(w, r, pageHref(r, 1), http.StatusSeeOther)
			return
		}
		if redirectPastLastPage(w, r, requestedPage, adminProductsPageSize, total) {
			return
		}

		page := adminProductsPage(metrics, adminRoleFromContext(r), products)
		page.ProductsPager = newPager(r, requestedPage, adminProductsPageSize, total)
		renderAdminPage(a, w, r, http.StatusOK, page)
	}
}

// AdminOrders handles order management.
func AdminOrders(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(r.Context(), svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		page := adminOrdersPage(metrics, adminRoleFromContext(r), nil)
		requestedPage := pageParam(r)

		responseStatus := http.StatusOK
		if r.Method == http.MethodPost {
			err := svc.UpdateOrder(r.Context(), adminActorFromContext(r), adminsvc.UpdateOrderInput{
				OrderID:        r.FormValue("order_id"),
				Status:         r.FormValue("status"),
				PartnerStatus:  r.FormValue("partner_status"),
				DeliveryStatus: r.FormValue("delivery_status"),
				DeliveryNotice: r.FormValue("delivery_notice"),
			})
			switch {
			case errors.Is(err, adminsvc.ErrForbiddenOrderUpdate):
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case errors.Is(err, adminsvc.ErrInvalidOrderID):
				page.OrderError = "Invalid order id"
				responseStatus = http.StatusBadRequest
			case errors.Is(err, adminsvc.ErrOrderNotFound):
				page.OrderError = "Order does not exist"
				responseStatus = http.StatusBadRequest
			case errors.Is(err, adminsvc.ErrInvalidOrderState):
				page.OrderError = "Order state is invalid"
				responseStatus = http.StatusBadRequest
			case err != nil:
				page.OrderError = "Unable to update order state"
				responseStatus = http.StatusBadRequest
			default:
				redirectWithFlash(w, r, pageHref(r, requestedPage), "success", "Order updated")
				return
			}
		} else if flash := popFlash(w, r); flash.Message != "" {
			page.OrderMessage = flash.Message
		}

		orders, total, err := svc.OrdersPage(r.Context(), requestedPage, adminOrdersPageSize)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		displayPage := min(requestedPage, pageCount(adminOrdersPageSize, total))
		if r.Method == http.MethodGet && displayPage != requestedPage {
			http.Redirect(w, r, pageHref(r, displayPage), http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost && responseStatus != http.StatusOK && total > 0 && displayPage != requestedPage {
			orders, total, err = svc.OrdersPage(r.Context(), displayPage, adminOrdersPageSize)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
		page.Orders = orders
		page.OrdersPager = newPager(r, displayPage, adminOrdersPageSize, total)
		renderAdminPage(a, w, r, responseStatus, page)
	}
}

// AdminReports handles sales reports.
func AdminReports(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(r.Context(), svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		report, err := svc.Reports(r.Context())
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		renderAdminPage(a, w, r, http.StatusOK, adminReportsPage(metrics, adminRoleFromContext(r), report))
	}
}
