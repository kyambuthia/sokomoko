package app

import (
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/db"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

type ReadinessChecker interface {
	PingContext(ctx context.Context) error
}

type CatalogService interface {
	AllProducts() ([]db.Product, error)
	ProductBySlug(slug string) (*db.Product, error)
	Search(query string) ([]db.Product, error)
}

type AccountService interface {
	OrdersForUser(userID int) ([]db.CustomerOrder, error)
}

type CommerceService interface {
	AddToCart(userID, productID, quantity int) error
	CheckoutWithPayment(userID int, deliveryAddress, paymentMethod string) (int64, commerceSvc.CheckoutSummary, error)
	GetCart(userID int) ([]db.CartItem, float64, error)
	RemoveFromCart(userID, productID int) error
	UpdateCartItem(userID, productID, quantity int) error
}

type AdminService interface {
	AuditLogs(limit int) ([]db.AuditLog, error)
	DeactivateUser(actor *db.User, userIDRaw string) error
	Metrics() (adminsvc.Metrics, error)
	Orders() ([]db.FulfillmentOrder, error)
	Products() ([]db.Product, error)
	Reports() (adminsvc.SalesReport, error)
	TeamMembers() ([]db.User, error)
	UpdateOrder(actor *db.User, input adminsvc.UpdateOrderInput) error
}

type PartnerService interface {
	CreateProduct(input partnersvc.CreateProductInput) error
	Dashboard() (partnersvc.DashboardData, error)
	Orders(filter string) (partnersvc.OrdersData, error)
	Products() (partnersvc.ProductsData, error)
	SaveStoreSettings(input partnersvc.StoreSettingsInput) (db.StoreSettings, error)
	StoreSettings() (*db.StoreSettings, error)
	UpdateOrder(actor *db.User, input partnersvc.UpdateOrderInput) error
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

type Dependencies struct {
	Readiness ReadinessChecker
	Catalog   CatalogService
	Account   AccountService
	Commerce  CommerceService
	Admin     AdminService
	Partner   PartnerService
	Auth      AuthService
	Templates *ui.Templates
	StaticFS  embed.FS
}

type App struct {
	Readiness ReadinessChecker
	Catalog   CatalogService
	Account   AccountService
	Commerce  CommerceService
	Admin     AdminService
	Partner   PartnerService
	Auth      AuthService
	Templates *ui.Templates
	StaticFS  embed.FS
}

func New(deps Dependencies) *App {
	return &App{
		Readiness: deps.Readiness,
		Catalog:   deps.Catalog,
		Account:   deps.Account,
		Commerce:  deps.Commerce,
		Admin:     deps.Admin,
		Partner:   deps.Partner,
		Auth:      deps.Auth,
		Templates: deps.Templates,
		StaticFS:  deps.StaticFS,
	}
}

func (a *App) Render(w http.ResponseWriter, tmpl *template.Template, data any) {
	err := tmpl.ExecuteTemplate(w, "root_template", data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
		RenderErrorPage(w, nil, http.StatusInternalServerError, "Internal Server Error", "We could not render this page.")
		return
	}
}
