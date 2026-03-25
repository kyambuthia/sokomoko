package app

import (
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

type Store interface {
	AddToCart(userID, productID, quantity int) error
	CountActiveSessions() (int, error)
	CountProducts() (int, error)
	CountUsersByRole(role string) (int, error)
	CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error
	CreateProduct(product db.Product) (int64, error)
	DeleteUser(id int) error
	GetAllCategories() ([]db.Category, error)
	GetAllProducts() ([]db.Product, error)
	GetCartItems(userID int) ([]db.CartItem, float64, error)
	GetOrderStatusCounts() (map[string]int, error)
	GetPartnerOrderSummary() (db.PartnerOrderSummary, error)
	GetProductByID(id int) (*db.Product, error)
	GetProductBySlug(slug string) (*db.Product, error)
	GetStoreSettings() (*db.StoreSettings, error)
	GetUserByID(id int) (*db.User, error)
	ListAllOrders() ([]db.FulfillmentOrder, error)
	ListAuditLogs(limit int) ([]db.AuditLog, error)
	ListOrdersByUser(userID int) ([]db.CustomerOrder, error)
	ListOrdersForFulfillment() ([]db.FulfillmentOrder, error)
	ListUsersByRoles(roles []string) ([]db.User, error)
	PingContext(ctx context.Context) error
	PlaceOrderFromCartWithPricing(userID int, deliveryAddress string, totalAmount float64, deliveryNotice string) (int64, error)
	RemoveFromCart(userID, productID int) error
	SearchProducts(query string) ([]db.Product, error)
	SumOrderRevenue() (float64, error)
	UpdateCartQuantity(userID, productID, quantity int) error
	UpdateOrderByAdmin(orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error
	UpdateOrderFulfillment(orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error
	UpsertStoreSettings(settings db.StoreSettings) error
}

type AuthService interface {
	AdminLogin(tmpl *template.Template) http.HandlerFunc
	AdminSetup(tmpl *template.Template) http.HandlerFunc
	AuthMiddleware(next http.Handler) http.Handler
	Login(tmpl *template.Template) http.HandlerFunc
	Logout() http.HandlerFunc
	PasswordResetConfirm(tmpl *template.Template, allowedRoles []string, title string, helper string, loginPath string) http.HandlerFunc
	PasswordResetRequest(tmpl *template.Template, allowedRoles []string, title string, helper string) http.HandlerFunc
	SignUp(tmpl *template.Template) http.HandlerFunc
	StaffSignUp(tmpl *template.Template) http.HandlerFunc
}

type App struct {
	Store     Store
	Auth      AuthService
	Templates *ui.Templates
	StaticFS  embed.FS
}

func New(store Store, authService AuthService, templates *ui.Templates, staticFS embed.FS) *App {
	return &App{Store: store, Auth: authService, Templates: templates, StaticFS: staticFS}
}

func (a *App) Render(w http.ResponseWriter, tmpl *template.Template, data any) {
	err := tmpl.ExecuteTemplate(w, "root_template", data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
		RenderErrorPage(w, nil, http.StatusInternalServerError, "Internal Server Error", "We could not render this page.")
		return
	}
}
