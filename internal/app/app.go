package app

import (
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"

	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
	checkoutsvc "github.com/kyambuthia/sokomoko/internal/service/checkout"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

type ReadinessChecker interface {
	PingContext(ctx context.Context) error
}

type CatalogService interface {
	AllProducts() ([]catalogsvc.Product, error)
	ProductBySlug(slug string) (*catalogsvc.Product, error)
	Search(query string) ([]catalogsvc.Product, error)
}

type AccountService interface {
	OrdersForUser(userID int) ([]accountsvc.Order, error)
}

type CommerceService interface {
	AddToCart(userID, productID, quantity int) error
	GetCart(userID int) ([]commerceSvc.CartItem, float64, error)
	RemoveFromCart(userID, productID int) error
	UpdateCartItem(userID, productID, quantity int) error
}

type CheckoutService interface {
	CheckoutWithPayment(userID int, deliveryAddress, paymentMethod, idempotencyKey string) (int64, checkoutsvc.Summary, error)
	Prepare(userID int, reservationKey string) error
}

type PaymentService interface {
	GenerateIdempotencyKey() (string, error)
	MethodLabel(method string) string
	SupportedMethodOptions() []paymentsvc.MethodOption
}

type AdminService interface {
	AuditLogs(limit int) ([]adminsvc.AuditLog, error)
	DeactivateUser(actor *adminsvc.Actor, userIDRaw string) error
	Metrics() (adminsvc.Metrics, error)
	Orders() ([]adminsvc.Order, error)
	Products() ([]adminsvc.Product, error)
	Reports() (adminsvc.SalesReport, error)
	TeamMembers() ([]adminsvc.TeamMember, error)
	UpdateOrder(actor *adminsvc.Actor, input adminsvc.UpdateOrderInput) error
}

type PartnerService interface {
	CreateProduct(input partnersvc.CreateProductInput) error
	Dashboard() (partnersvc.DashboardData, error)
	Orders(filter string) (partnersvc.OrdersData, error)
	Products() (partnersvc.ProductsData, error)
	SaveStoreSettings(input partnersvc.StoreSettingsInput) (partnersvc.StoreSettings, error)
	StoreSettings() (*partnersvc.StoreSettings, error)
	UpdateOrder(actor *partnersvc.Actor, input partnersvc.UpdateOrderInput) error
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
	Checkout  CheckoutService
	Payment   PaymentService
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
	Checkout  CheckoutService
	Payment   PaymentService
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
		Checkout:  deps.Checkout,
		Payment:   deps.Payment,
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
